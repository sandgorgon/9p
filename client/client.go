// Package client implements a 9P2000 client: dial or wrap a
// connection, attach to a server's exported tree, and walk, open,
// read, write, and stat files on it.
package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	p9 "p9"
)

// Client is a connection to a 9P2000 server, already past version
// negotiation.
type Client struct {
	rwc     io.ReadWriteCloser
	msize   atomic.Uint32
	mux     *callMux
	writeMu sync.Mutex
	nextFid atomic.Uint32

	rootMu sync.RWMutex
	root   *Fid
}

// Option configures a Client in NewClient or Dial.
type Option func(*options)

type options struct {
	msize uint32
}

// WithMsize sets the maximum message size the client is willing to
// negotiate. The session's effective msize is the smaller of this
// and whatever the server proposes.
func WithMsize(n uint32) Option {
	return func(o *options) { o.msize = n }
}

// Dial connects to addr (see net.Dial for the network/addr forms)
// and performs the 9P2000 version handshake over the new connection.
func Dial(network, addr string, opts ...Option) (*Client, error) {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	c, err := NewClient(conn, opts...)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

// NewClient wraps an already-connected transport and performs the
// 9P2000 version handshake. The returned Client is always
// post-negotiation.
func NewClient(rwc io.ReadWriteCloser, opts ...Option) (*Client, error) {
	o := options{msize: p9.DefaultMsize}
	for _, opt := range opts {
		opt(&o)
	}

	c := &Client{rwc: rwc, mux: newCallMux()}
	c.msize.Store(o.msize)
	go c.readLoop()

	ch, err := c.mux.registerTag(p9.NoTag)
	if err != nil {
		c.Close()
		return nil, err
	}
	reply, err := c.call(context.Background(), p9.NoTag, ch, &p9.TversionFcall{Msize: o.msize, Version: p9.Version})
	if err != nil {
		c.Close()
		return nil, err
	}
	rv, ok := reply.(*p9.RversionFcall)
	if !ok {
		c.Close()
		return nil, fmt.Errorf("client: unexpected reply to Tversion: %v", reply.MsgType())
	}
	if rv.Version != p9.Version {
		c.Close()
		return nil, fmt.Errorf("client: server does not support %s (replied %q)", p9.Version, rv.Version)
	}
	c.msize.Store(min(o.msize, rv.Msize))
	return c, nil
}

// Close closes the underlying connection and fails any outstanding
// calls.
func (c *Client) Close() error {
	err := c.rwc.Close()
	c.mux.closeAll(io.ErrClosedPipe)
	return err
}

func (c *Client) allocFid() p9.Fid {
	return p9.Fid(c.nextFid.Add(1))
}

func (c *Client) writeMessage(raw []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return p9.WriteMessage(c.rwc, raw)
}

func (c *Client) readLoop() {
	for {
		raw, err := p9.ReadMessage(c.rwc, c.msize.Load())
		if err != nil {
			c.mux.closeAll(err)
			return
		}
		tag, msg, err := p9.Unmarshal(raw)
		if err != nil {
			c.mux.closeAll(err)
			return
		}
		c.mux.deliver(tag, msg)
	}
}

// call sends req under tag on ch (as previously registered) and
// waits for the matching reply, honoring ctx cancellation by
// flushing the request per the Tflush protocol.
func (c *Client) call(ctx context.Context, tag p9.Tag, ch chan p9.Message, req p9.Message) (p9.Message, error) {
	raw := p9.Marshal(tag, req)
	if err := c.writeMessage(raw); err != nil {
		c.mux.forget(tag)
		return nil, err
	}
	select {
	case msg, ok := <-ch:
		if !ok {
			return nil, c.mux.err()
		}
		if rerr, isErr := msg.(*p9.RerrorFcall); isErr {
			return nil, rerr
		}
		return msg, nil
	case <-ctx.Done():
		c.flush(tag)
		c.mux.forget(tag)
		return nil, ctx.Err()
	}
}

// rpc allocates a fresh tag and issues req, blocking until the
// matching reply arrives.
func (c *Client) rpc(ctx context.Context, req p9.Message) (p9.Message, error) {
	tag, ch, err := c.mux.register()
	if err != nil {
		return nil, err
	}
	return c.call(ctx, tag, ch, req)
}

// flush best-effort cancels the request under oldtag. Any reply that
// still arrives for oldtag after this is dropped by callMux.deliver,
// since forget removes it from the pending table first.
func (c *Client) flush(oldtag p9.Tag) {
	tag, ch, err := c.mux.register()
	if err != nil {
		return
	}
	raw := p9.Marshal(tag, &p9.TflushFcall{Oldtag: oldtag})
	if err := c.writeMessage(raw); err != nil {
		c.mux.forget(tag)
		return
	}
	<-ch
}

func mismatchErr(want p9.FcallType, got p9.Message) error {
	return fmt.Errorf("client: expected %v reply, got %v", want, got.MsgType())
}
