package dirfs_test

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	p9 "p9"
	"p9/client"
	"p9/examples/dirfs"
	"p9/server"
)

func newTestClient(t *testing.T, root string) *client.Client {
	t.Helper()
	fs, err := dirfs.New(root)
	if err != nil {
		t.Fatalf("dirfs.New: %v", err)
	}
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

func TestReadExistingFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "greeting.txt"), []byte("hello, plan 9\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0755); err != nil {
		t.Fatalf("Mkdir: %v", err)
	}

	c := newTestClient(t, dir)
	if _, err := c.Attach("glenda", ""); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	f, err := c.Open("/greeting.txt", p9.OREAD)
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
}

func TestReadDirAndCreate(t *testing.T) {
	dir := t.TempDir()
	c := newTestClient(t, dir)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	child, err := root.Walk()
	if err != nil {
		t.Fatalf("Walk (clone): %v", err)
	}
	if _, _, err := child.Create("new.txt", 0644, p9.OWRITE); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := child.Clunk(); err != nil {
		t.Fatalf("Clunk: %v", err)
	}

	f, err := c.Open("/new.txt", p9.OWRITE)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := f.Write([]byte("created via 9P\n")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	f.Close()

	body, err := os.ReadFile(filepath.Join(dir, "new.txt"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(body) != "created via 9P\n" {
		t.Errorf("on-disk content = %q", body)
	}

	rd, err := c.Open("/", p9.OREAD)
	if err != nil {
		t.Fatalf("Open /: %v", err)
	}
	defer rd.Close()
	entries, err := rd.ReadDir()
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "new.txt" {
		t.Errorf("ReadDir = %+v", entries)
	}
}

// Walking ".." at the exported root clamps to the root itself
// (chroot-style), rather than escaping it or erroring.
func TestWalkCannotEscapeRoot(t *testing.T) {
	dir := t.TempDir()
	c := newTestClient(t, dir)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	rootQid, err := root.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}

	above, err := root.Walk("..")
	if err != nil {
		t.Fatalf("Walk('..'): %v", err)
	}
	aboveQid, err := above.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if aboveQid.Qid != rootQid.Qid {
		t.Errorf("Walk('..') at root landed on a different file: %+v, want root %+v", aboveQid.Qid, rootQid.Qid)
	}
}
