package client_test

import (
	"io"
	"net"
	"testing"

	p9 "github.com/sandgorgon/9p"
	"github.com/sandgorgon/9p/client"
	"github.com/sandgorgon/9p/examples/memfs"
	"github.com/sandgorgon/9p/server"
)

func newCreateTestClient(t *testing.T) *client.Client {
	t.Helper()
	fs := memfs.New()
	srv := &server.Server{FS: fs}

	clientConn, serverConn := net.Pipe()
	go func() {
		defer serverConn.Close()
		srv.ServeConn(serverConn)
	}()

	c, err := client.NewClient(clientConn)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestClientCreateAtRoot(t *testing.T) {
	c := newCreateTestClient(t)
	if _, err := c.Attach("glenda", ""); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	f, err := c.Create("newfile", 0644, p9.ORDWR)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	content := []byte("hello from Create\n")
	if _, err := f.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	rf, err := c.Open("/newfile", p9.OREAD)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rf.Close()
	got, err := io.ReadAll(rf)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

// TestClientCreateUnderSubdirectory exercises Create's multi-element
// path case (a non-empty dir to walk before creating), using Create
// itself to make the subdirectory first — unlike Fid.Create, which
// repositions its receiver onto the new entry, Client.Create always
// walks a fresh fid per call and never disturbs the client's stored
// root, so it's safe to call repeatedly like this.
func TestClientCreateUnderSubdirectory(t *testing.T) {
	c := newCreateTestClient(t)
	if _, err := c.Attach("glenda", ""); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	subDir, err := c.Create("sub", p9.DMDIR|0755, p9.OREAD)
	if err != nil {
		t.Fatalf("Create sub: %v", err)
	}
	if err := subDir.Close(); err != nil {
		t.Fatalf("Close sub: %v", err)
	}

	f, err := c.Create("sub/nested", 0644, p9.OWRITE)
	if err != nil {
		t.Fatalf("Create sub/nested: %v", err)
	}
	content := []byte("nested content\n")
	if _, err := f.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	rf, err := c.Open("/sub/nested", p9.OREAD)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rf.Close()
	got, err := io.ReadAll(rf)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestClientCreateEmptyPath(t *testing.T) {
	c := newCreateTestClient(t)
	if _, err := c.Attach("glenda", ""); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if _, err := c.Create("", 0644, p9.OWRITE); err == nil {
		t.Error("Create with an empty path succeeded, want error")
	}
}

func TestClientCreateNotAttached(t *testing.T) {
	fs := memfs.New()
	srv := &server.Server{FS: fs}
	clientConn, serverConn := net.Pipe()
	go func() {
		defer serverConn.Close()
		srv.ServeConn(serverConn)
	}()
	c, err := client.NewClient(clientConn)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	if _, err := c.Create("newfile", 0644, p9.OWRITE); err == nil {
		t.Error("Create before Attach succeeded, want error")
	}
}
