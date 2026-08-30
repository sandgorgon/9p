package main

import (
	"net"
	"os"
	"testing"

	"github.com/sandgorgon/9p/client"
	"github.com/sandgorgon/9p/examples/memfs"
	"github.com/sandgorgon/9p/server"
)

func newTestClient(t *testing.T) (*client.Client, *client.Fid) {
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
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	return c, root
}

func writeTemp(t *testing.T, contents string) string {
	t.Helper()
	f, err := os.CreateTemp("", "9pc-put-test")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(f.Name()) })
	if _, err := f.WriteString(contents); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func readRemote(t *testing.T, c *client.Client, remote string) string {
	t.Helper()
	f, err := c.Open(remote, 0)
	if err != nil {
		t.Fatalf("open %s: %v", remote, err)
	}
	defer f.Close()
	buf := make([]byte, 128)
	n, _ := f.Read(buf)
	return string(buf[:n])
}

func TestRunPutNewFile(t *testing.T) {
	c, root := newTestClient(t)
	local := writeTemp(t, "hello")

	runPut(c, root, local, "/greeting")

	if got := readRemote(t, c, "/greeting"); got != "hello" {
		t.Fatalf("remote content = %q, want %q", got, "hello")
	}
}

// TestRunPutOverwritesExistingFile guards against the bug in #6: put
// always went through Walk-parent + Create, which fails with a
// protocol error when the remote file already exists.
func TestRunPutOverwritesExistingFile(t *testing.T) {
	c, root := newTestClient(t)
	local := writeTemp(t, "first")
	runPut(c, root, local, "/greeting")

	local2 := writeTemp(t, "second, overwritten")
	runPut(c, root, local2, "/greeting")

	if got := readRemote(t, c, "/greeting"); got != "second, overwritten" {
		t.Fatalf("remote content = %q, want %q", got, "second, overwritten")
	}
}
