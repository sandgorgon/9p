package server

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

	p9 "p9"
)

// openFile is the server-side state for one client fid. Its file
// field is guarded separately from the conn-level fid table because
// Tcreate repositions a fid onto a newly created File in place, and
// that mutation can race with concurrent requests against the same
// fid handled by other goroutines.
type openFile struct {
	mu   sync.Mutex
	file File
}

func (o *openFile) get() File {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.file
}

func (o *openFile) set(f File) {
	o.mu.Lock()
	o.file = f
	o.mu.Unlock()
}

type conn struct {
	srv   *Server
	rwc   io.ReadWriteCloser
	msize atomic.Uint32

	writeMu sync.Mutex

	mu       sync.Mutex
	fids     map[p9.Fid]*openFile
	inflight map[p9.Tag]context.CancelFunc
}

func (s *Server) newConn(rwc io.ReadWriteCloser) *conn {
	c := &conn{
		srv:      s,
		rwc:      rwc,
		fids:     make(map[p9.Fid]*openFile),
		inflight: make(map[p9.Tag]context.CancelFunc),
	}
	c.msize.Store(s.maxMsize())
	return c
}

func (c *conn) getFid(fid p9.Fid) (*openFile, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	of, ok := c.fids[fid]
	return of, ok
}

func (c *conn) putFid(fid p9.Fid, of *openFile) {
	c.mu.Lock()
	c.fids[fid] = of
	c.mu.Unlock()
}

func (c *conn) delFid(fid p9.Fid) (*openFile, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	of, ok := c.fids[fid]
	if ok {
		delete(c.fids, fid)
	}
	return of, ok
}

func (c *conn) writeReply(tag p9.Tag, reply p9.Message) error {
	raw := p9.Marshal(tag, reply)
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return p9.WriteMessage(c.rwc, raw)
}

// serve runs the connection: an explicit Tversion handshake (which
// must be the first message, per spec), then a loop that dispatches
// every subsequent request to its own goroutine so that a slow
// request cannot block others and so Tflush has something to cancel.
func (c *conn) serve() error {
	raw, err := p9.ReadMessage(c.rwc, c.msize.Load())
	if err != nil {
		return err
	}
	tag, msg, err := p9.Unmarshal(raw)
	if err != nil {
		return err
	}
	tv, ok := msg.(*p9.TversionFcall)
	if !ok {
		return c.writeReply(tag, &p9.RerrorFcall{Ename: "expected Tversion"})
	}
	if err := c.writeReply(tag, c.tVersion(tv)); err != nil {
		return err
	}

	for {
		raw, err := p9.ReadMessage(c.rwc, c.msize.Load())
		if err != nil {
			return err
		}
		tag, msg, err := p9.Unmarshal(raw)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithCancel(context.Background())
		c.mu.Lock()
		c.inflight[tag] = cancel
		c.mu.Unlock()
		go c.handle(ctx, tag, msg, cancel)
	}
}

func (c *conn) handle(ctx context.Context, tag p9.Tag, msg p9.Message, cancel context.CancelFunc) {
	defer func() {
		cancel()
		c.mu.Lock()
		delete(c.inflight, tag)
		c.mu.Unlock()
	}()
	reply := c.dispatch(ctx, msg)
	// A write failure here means the connection is dead; the read
	// loop will observe that on its next ReadMessage and shut down.
	_ = c.writeReply(tag, reply)
}

// tVersion resets the connection's fid table, per spec, and
// negotiates msize and the protocol version.
func (c *conn) tVersion(m *p9.TversionFcall) p9.Message {
	c.mu.Lock()
	c.fids = make(map[p9.Fid]*openFile)
	c.mu.Unlock()

	msize := min(m.Msize, c.srv.maxMsize())
	c.msize.Store(msize)
	if m.Version != p9.Version {
		return &p9.RversionFcall{Msize: msize, Version: p9.VersionUnknown}
	}
	return &p9.RversionFcall{Msize: msize, Version: p9.Version}
}
