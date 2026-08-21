package client

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	p9 "github.com/sandgorgon/9p"
)

// fakeServer is a minimal hand-rolled 9P2000 server used only to
// exercise the client package end to end, independent of this
// module's own server package.
type fakeServer struct {
	rwc   io.ReadWriteCloser
	msize uint32
	fids  map[p9.Fid]string
	files map[string][]byte
}

func newFakeServer(rwc io.ReadWriteCloser) *fakeServer {
	return &fakeServer{
		rwc:   rwc,
		msize: p9.DefaultMsize,
		fids:  map[p9.Fid]string{},
		files: map[string][]byte{
			"/hello": []byte("hello, plan 9\n"),
		},
	}
}

func qidFor(path string) p9.Qid {
	if path == "/" {
		return p9.Qid{Type: p9.QTDIR, Path: 1}
	}
	return p9.Qid{Type: p9.QTFILE, Path: uint64(len(path)) + 100}
}

func (s *fakeServer) run() {
	for {
		raw, err := p9.ReadMessage(s.rwc, s.msize)
		if err != nil {
			return
		}
		tag, msg, err := p9.Unmarshal(raw)
		if err != nil {
			return
		}
		reply := s.handle(msg)
		if err := p9.WriteMessage(s.rwc, p9.Marshal(tag, reply)); err != nil {
			return
		}
	}
}

func (s *fakeServer) handle(msg p9.Message) p9.Message {
	switch m := msg.(type) {
	case *p9.TversionFcall:
		s.msize = m.Msize
		return &p9.RversionFcall{Msize: m.Msize, Version: p9.Version}

	case *p9.TattachFcall:
		s.fids[m.Fid] = "/"
		return &p9.RattachFcall{Qid: qidFor("/")}

	case *p9.TwalkFcall:
		base, ok := s.fids[m.Fid]
		if !ok {
			return &p9.RerrorFcall{Ename: "unknown fid"}
		}
		if len(m.Wname) == 0 {
			s.fids[m.Newfid] = base
			return &p9.RwalkFcall{Wqid: []p9.Qid{}}
		}
		path := base
		qids := []p9.Qid{}
		for _, name := range m.Wname {
			var next string
			switch {
			case path == "/" && name == "hello":
				next = "/hello"
			case name == "..":
				next = "/"
			default:
				next = "" // not found
			}
			if next == "" {
				break
			}
			path = next
			qids = append(qids, qidFor(path))
		}
		if len(qids) > 0 || len(m.Wname) == 0 {
			s.fids[m.Newfid] = path
		}
		return &p9.RwalkFcall{Wqid: qids}

	case *p9.TopenFcall:
		path, ok := s.fids[m.Fid]
		if !ok {
			return &p9.RerrorFcall{Ename: "unknown fid"}
		}
		if m.Mode&p9.OTRUNC != 0 {
			s.files[path] = []byte{}
		}
		return &p9.RopenFcall{Qid: qidFor(path), Iounit: 0}

	case *p9.TreadFcall:
		path, ok := s.fids[m.Fid]
		if !ok {
			return &p9.RerrorFcall{Ename: "unknown fid"}
		}
		data := s.files[path]
		off := int(m.Offset)
		if off >= len(data) {
			return &p9.RreadFcall{Data: []byte{}}
		}
		end := min(off+int(m.Count), len(data))
		return &p9.RreadFcall{Data: data[off:end]}

	case *p9.TwriteFcall:
		path, ok := s.fids[m.Fid]
		if !ok {
			return &p9.RerrorFcall{Ename: "unknown fid"}
		}
		data := s.files[path]
		end := int(m.Offset) + len(m.Data)
		if end > len(data) {
			grown := make([]byte, end)
			copy(grown, data)
			data = grown
		}
		copy(data[m.Offset:], m.Data)
		s.files[path] = data
		return &p9.RwriteFcall{Count: uint32(len(m.Data))}

	case *p9.TclunkFcall:
		delete(s.fids, m.Fid)
		return &p9.RclunkFcall{}

	case *p9.TstatFcall:
		path, ok := s.fids[m.Fid]
		if !ok {
			return &p9.RerrorFcall{Ename: "unknown fid"}
		}
		return &p9.RstatFcall{Stat: p9.Stat{
			Qid:    qidFor(path),
			Length: uint64(len(s.files[path])),
			Name:   path,
		}}

	default:
		return &p9.RerrorFcall{Ename: "unsupported message"}
	}
}

func newFakeClient(t *testing.T) *Client {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	go newFakeServer(serverConn).run()

	c, err := NewClient(clientConn)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestVersionHandshake(t *testing.T) {
	newFakeClient(t) // NewClient performing the handshake without error is the assertion
}

func TestAttachWalkOpenReadWriteClunk(t *testing.T) {
	c := newFakeClient(t)

	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	hello, err := root.Walk("hello")
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if _, _, err := hello.Open(p9.ORDWR); err != nil {
		t.Fatalf("Open: %v", err)
	}

	buf := make([]byte, 64)
	n, err := c.rpcRead(t, hello, buf)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got, want := string(buf[:n]), "hello, plan 9\n"; got != want {
		t.Errorf("read content = %q, want %q", got, want)
	}

	if err := c.rpcWrite(t, hello); err != nil {
		t.Fatalf("write: %v", err)
	}

	if err := hello.Clunk(); err != nil {
		t.Fatalf("Clunk: %v", err)
	}
}

// rpcRead/rpcWrite exercise Tread/Twrite directly through the Fid's
// underlying rpc plumbing without going through File, since File is
// tested separately in file_test.go.
func (c *Client) rpcRead(t *testing.T, f *Fid, buf []byte) (int, error) {
	t.Helper()
	reply, err := c.rpc(context.Background(), &p9.TreadFcall{Fid: f.fid, Offset: 0, Count: uint32(len(buf))})
	if err != nil {
		return 0, err
	}
	rr := reply.(*p9.RreadFcall)
	return copy(buf, rr.Data), nil
}

func (c *Client) rpcWrite(t *testing.T, f *Fid) error {
	t.Helper()
	_, err := c.rpc(context.Background(), &p9.TwriteFcall{Fid: f.fid, Offset: 0, Data: []byte("overwritten\n")})
	return err
}

func TestWalkNotFound(t *testing.T) {
	c := newFakeClient(t)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if _, err := root.Walk("does-not-exist"); err == nil {
		t.Error("Walk to nonexistent file succeeded, want error")
	}
}

func TestRerrorPropagation(t *testing.T) {
	c := newFakeClient(t)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if err := root.Clunk(); err != nil {
		t.Fatalf("Clunk: %v", err)
	}
	// root's fid is now unknown to the server; any further use of it
	// should come back as an Rerror, surfaced as a *p9.RerrorFcall.
	_, _, err = root.Open(p9.OREAD)
	if err == nil {
		t.Fatal("Open on clunked fid succeeded, want error")
	}
	var rerr *p9.RerrorFcall
	if !errors.As(err, &rerr) {
		t.Errorf("error = %v (%T), want *p9.RerrorFcall", err, err)
	}
}

func TestFile(t *testing.T) {
	c := newFakeClient(t)
	if _, err := c.Attach("glenda", ""); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	f, err := c.Open("/hello", p9.ORDWR)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != "hello, plan 9\n" {
		t.Errorf("content = %q", got)
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	if _, err := f.Write([]byte("bye\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	got, err = io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	// Write does not truncate: it only overwrote the first 4 bytes
	// of the original 14-byte file.
	want := "bye\no, plan 9\n"
	if string(got) != want {
		t.Errorf("content after write = %q, want %q", got, want)
	}
}
