package dirfs_test

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"

	p9 "github.com/sandgorgon/9p"
	"github.com/sandgorgon/9p/client"
	"github.com/sandgorgon/9p/examples/dirfs"
	"github.com/sandgorgon/9p/server"
)

func newTestClient(t *testing.T, root string, opts ...client.Option) *client.Client {
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

	c, err := client.NewClient(clientConn, opts...)
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

// A symlink planted at an intermediate path component must not let
// Walk, Create, or WStat's rename destination escape root, even
// though the joined path string still looks like it's inside root.
func TestWalkCannotEscapeThroughSymlink(t *testing.T) {
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("outside root\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	dir := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	c := newTestClient(t, dir)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	if _, err := root.Walk("escape", "secret.txt"); err == nil {
		t.Fatal("Walk through symlinked intermediate component succeeded, want error")
	}

	if _, err := c.Open("/escape/secret.txt", p9.OREAD); err == nil {
		t.Fatal("Open through symlinked intermediate component succeeded, want error")
	}

	if _, err := c.Create("/escape/new.txt", 0644, p9.OWRITE); err == nil {
		t.Fatal("Create through symlinked intermediate component succeeded, want error")
	}
	if _, err := os.Lstat(filepath.Join(outside, "new.txt")); err == nil {
		t.Fatal("Create through symlinked intermediate component wrote outside root")
	}
}

// A client that negotiates 9P2000.u can create a symlink and read
// its target back via Stat's Extension field and Qid.IsSymlink.
func TestSymlinkCreateAndStat(t *testing.T) {
	dir := t.TempDir()
	c := newTestClient(t, dir, client.WithUnixExtensions())
	if _, err := c.Attach("glenda", ""); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	if _, err := c.Symlink("/link", "target/path"); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	// The symlink exists as a real symlink on disk.
	target, err := os.Readlink(filepath.Join(dir, "link"))
	if err != nil {
		t.Fatalf("os.Readlink: %v", err)
	}
	if target != "target/path" {
		t.Errorf("on-disk symlink target = %q, want %q", target, "target/path")
	}

	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	link, err := root.Walk("link")
	if err != nil {
		t.Fatalf("Walk: %v", err)
	}
	st, err := link.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !st.Qid.IsSymlink() {
		t.Errorf("Qid = %+v, want IsSymlink() true", st.Qid)
	}
	if st.Extension != "target/path" {
		t.Errorf("Stat.Extension = %q, want %q", st.Extension, "target/path")
	}

	// A well-behaved client never Opens a symlink directly.
	if _, err := c.Open("/link", p9.OREAD); err == nil {
		t.Error("Open on a symlink succeeded, want error")
	}
}

// A directory listing over a 9P2000.u connection reports a symlink
// child's target; over a plain connection the Extension field is
// silently absent (dropped by the wire encoder), not an error.
func TestSymlinkInDirectoryListing(t *testing.T) {
	dir := t.TempDir()
	c := newTestClient(t, dir, client.WithUnixExtensions())
	if _, err := c.Attach("glenda", ""); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if _, err := c.Symlink("/link", "somewhere"); err != nil {
		t.Fatalf("Symlink: %v", err)
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
	if len(entries) != 1 || entries[0].Name != "link" {
		t.Fatalf("ReadDir = %+v", entries)
	}
	if !entries[0].Qid.IsSymlink() {
		t.Errorf("listed entry Qid = %+v, want IsSymlink() true", entries[0].Qid)
	}
	if entries[0].Extension != "somewhere" {
		t.Errorf("listed entry Extension = %q, want %q", entries[0].Extension, "somewhere")
	}
}

// Client.Symlink against a connection that never negotiated
// 9P2000.u fails client-side with a clear error, without ever
// sending a Tcreate.
func TestSymlinkRequiresUnixExtensions(t *testing.T) {
	dir := t.TempDir()
	c := newTestClient(t, dir)
	if _, err := c.Attach("glenda", ""); err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if _, err := c.Symlink("/link", "target"); err == nil {
		t.Error("Symlink on a plain 9P2000 connection succeeded, want error")
	}
	if _, err := os.Lstat(filepath.Join(dir, "link")); err == nil {
		t.Error("Symlink on a plain 9P2000 connection created a file on disk")
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
