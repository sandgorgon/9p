package client_test

import (
	"net"
	"testing"

	p9 "github.com/sandgorgon/9p"
	"github.com/sandgorgon/9p/client"
	"github.com/sandgorgon/9p/examples/memfs"
	"github.com/sandgorgon/9p/server"
)

func newUnixTestClient(t *testing.T) *client.Client {
	t.Helper()
	fs := memfs.New()
	srv := &server.Server{FS: fs}

	clientConn, serverConn := net.Pipe()
	go func() {
		defer serverConn.Close()
		srv.ServeConn(serverConn)
	}()

	c, err := client.NewClient(clientConn, client.WithUnixExtensions())
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestClientSymlink(t *testing.T) {
	c := newUnixTestClient(t)
	if _, err := c.Attach("glenda", ""); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	qid, err := c.Symlink("link", "some/target")
	if err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if !qid.IsSymlink() {
		t.Errorf("Symlink returned Qid = %+v, want IsSymlink() true", qid)
	}

	f, err := c.Open("/link", p9.OREAD)
	if err == nil {
		f.Close()
		t.Error("Open on a symlink succeeded, want error")
	}
}

func TestClientSymlinkStatRoundTrip(t *testing.T) {
	c := newUnixTestClient(t)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if _, err := c.Symlink("link", "some/target"); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	link, err := root.Walk("link")
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	st, err := link.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if st.Extension != "some/target" {
		t.Errorf("Stat.Extension = %q, want %q", st.Extension, "some/target")
	}
}

// Fid.Symlink and Client.Symlink both fail client-side, without
// sending a Tcreate, against a connection that never negotiated
// 9P2000.u.
func TestSymlinkRequiresUnixExtensions(t *testing.T) {
	c := newCreateTestClient(t)
	if _, err := c.Attach("glenda", ""); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if _, err := c.Symlink("link", "target"); err == nil {
		t.Error("Symlink on a plain 9P2000 connection succeeded, want error")
	}
	if _, err := c.Open("/link", p9.OREAD); err == nil {
		t.Error("plain connection's Symlink call created a file on the server")
	}
}
