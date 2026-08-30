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

func newOpenFileTestClient(t *testing.T) *client.Client {
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

// TestFidCreateFileThenOpenFile exercises the Walk-once-then-do-I/O
// pattern the issue is about: no path is ever re-walked from the
// root to obtain a *File.
func TestFidCreateFileThenOpenFile(t *testing.T) {
	c := newOpenFileTestClient(t)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	fid, err := root.Walk()
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	content := []byte("hello from CreateFile\n")
	wf, err := fid.CreateFile("newfile", 0644, p9.ORDWR)
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
	if _, err := wf.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := wf.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	fid2, err := root.Walk("newfile")
	if err != nil {
		t.Fatalf("Walk newfile: %v", err)
	}
	rf, err := fid2.OpenFile(p9.OREAD)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
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

// TestFidCreateFileDuplicateName exercises CreateFile's error path:
// creating a name that already exists must return an error rather
// than a usable *File, and must leave f usable for a retry (Create's
// own contract: f is only repositioned on success).
func TestFidCreateFileDuplicateName(t *testing.T) {
	c := newOpenFileTestClient(t)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	fid, err := root.Walk()
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	first, err := fid.CreateFile("dup", 0644, p9.OWRITE)
	if err != nil {
		t.Fatalf("CreateFile: %v", err)
	}
	if err := first.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	fid2, err := root.Walk()
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	if _, err := fid2.CreateFile("dup", 0644, p9.OWRITE); err == nil {
		t.Fatalf("CreateFile with duplicate name succeeded, want error")
	}
}
