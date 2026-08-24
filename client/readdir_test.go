package client_test

import (
	"fmt"
	"net"
	"testing"

	p9 "github.com/sandgorgon/9p"
	"github.com/sandgorgon/9p/client"
	"github.com/sandgorgon/9p/examples/memfs"
	"github.com/sandgorgon/9p/server"
)

// TestReadDirAcrossMultipleChunks guards against a real bug: a
// directory listing that doesn't fit in one negotiated-msize buffer
// used to come back silently truncated, with no error at all.
// server.MarshalDir (the server side of every directory Read in this
// repo) only ever returns whole Stat entries, so on any nontrivial
// listing it returns fewer bytes than the buffer offered — normal
// mid-listing behavior, not end-of-file. ReadDirContext used to
// reinterpret that short read as EOF and stop early. Forcing a small
// msize here (so several entries don't fit in one read) reproduces
// that on the very first listing that needs more than one chunk.
func TestReadDirAcrossMultipleChunks(t *testing.T) {
	fs := memfs.New()
	srv := &server.Server{FS: fs}

	clientConn, serverConn := net.Pipe()
	go func() {
		defer serverConn.Close()
		srv.ServeConn(serverConn)
	}()

	c, err := client.NewClient(clientConn, client.WithMsize(128))
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	defer c.Close()

	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	const want = 50
	names := make(map[string]bool, want)
	for i := range want {
		name := fmt.Sprintf("file-%02d", i)
		names[name] = true

		child, err := root.Walk()
		if err != nil {
			t.Fatalf("Walk (clone): %v", err)
		}
		if _, _, err := child.Create(name, 0644, p9.OWRITE); err != nil {
			t.Fatalf("Create %s: %v", name, err)
		}
		if err := child.Clunk(); err != nil {
			t.Fatalf("Clunk: %v", err)
		}
	}

	rd, err := c.Open("/", p9.OREAD)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer rd.Close()

	entries, err := rd.ReadDir()
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != want {
		t.Fatalf("ReadDir returned %d entries, want %d", len(entries), want)
	}
	for _, e := range entries {
		if !names[e.Name] {
			t.Errorf("unexpected entry %q", e.Name)
		}
		delete(names, e.Name)
	}
	if len(names) > 0 {
		t.Errorf("missing entries: %v", names)
	}
}
