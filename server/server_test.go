package server_test

import (
	"errors"
	"io"
	"net"
	"testing"

	p9 "p9"
	"p9/client"
	"p9/examples/memfs"
	"p9/server"
)

func newTestClient(t *testing.T) *client.Client {
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

func TestAttach(t *testing.T) {
	c := newTestClient(t)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	st, err := root.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !st.Qid.IsDir() {
		t.Error("root Qid.IsDir() = false, want true")
	}
}

func TestCreateWriteReadStat(t *testing.T) {
	c := newTestClient(t)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	// Create repositions its receiver's own fid onto the new file,
	// so clone root's fid first — clunking the clone afterward must
	// not disturb the client's stored root fid (used by c.Open).
	dir, err := root.Walk()
	if err != nil {
		t.Fatalf("Walk (clone): %v", err)
	}
	if _, _, err := dir.Create("hello", 0644, p9.OWRITE); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := dir.Clunk(); err != nil {
		t.Fatalf("Clunk: %v", err)
	}

	f, err := c.Open("/hello", p9.ORDWR)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	content := []byte("hello, plan 9\n")
	if _, err := f.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	got, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != string(content) {
		t.Errorf("content = %q, want %q", got, content)
	}

	st, err := f.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if st.Name != "hello" || st.Length != uint64(len(content)) {
		t.Errorf("Stat = %+v", st)
	}
}

func TestReadDir(t *testing.T) {
	c := newTestClient(t)
	dir, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	for _, name := range []string{"a", "b", "c"} {
		child, err := dir.Walk() // clone dir's fid so it stays usable for the next Create
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

	root, err := c.Open("/", p9.OREAD)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer root.Close()

	entries, err := root.ReadDir()
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("ReadDir returned %d entries, want 3: %+v", len(entries), entries)
	}
	names := map[string]bool{}
	for _, e := range entries {
		names[e.Name] = true
	}
	for _, want := range []string{"a", "b", "c"} {
		if !names[want] {
			t.Errorf("ReadDir missing entry %q", want)
		}
	}
}

func TestRemove(t *testing.T) {
	c := newTestClient(t)
	dir, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if _, _, err := dir.Create("gone", 0644, p9.OWRITE); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := dir.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if _, err := root.Walk("gone"); err == nil {
		t.Error("Walk to removed file succeeded, want error")
	}
}

func TestUnknownFid(t *testing.T) {
	c := newTestClient(t)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if err := root.Clunk(); err != nil {
		t.Fatalf("Clunk: %v", err)
	}
	_, err = root.Stat()
	if err == nil {
		t.Fatal("Stat on clunked fid succeeded, want error")
	}
	var rerr *p9.RerrorFcall
	if !errors.As(err, &rerr) {
		t.Errorf("error = %v (%T), want *p9.RerrorFcall", err, err)
	}
}
