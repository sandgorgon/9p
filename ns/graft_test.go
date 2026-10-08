package ns

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"testing"

	p9 "github.com/sandgorgon/9p"
	"github.com/sandgorgon/9p/client"
	"github.com/sandgorgon/9p/examples/dirfs"
	"github.com/sandgorgon/9p/server"
)

// pipeClient serves fs over an in-memory connection and returns a
// client attached to it.
func pipeClient(t *testing.T, fs server.FileSystem) *client.Fid {
	t.Helper()
	srv := &server.Server{FS: fs}
	cc, sc := net.Pipe()
	go func() {
		defer sc.Close()
		srv.ServeConn(sc)
	}()
	c, err := client.NewClient(cc)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	return root
}

func writeTree(t *testing.T, files map[string]string) server.FileSystem {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fs, err := dirfs.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	return fs
}

// An app serves its own namespace and grafts a peer into it: a client
// of the app sees the app's own files and the peer's under one tree.
func TestServeNamespaceWithGraftedPeer(t *testing.T) {
	own := writeTree(t, map[string]string{"status": "ok\n"})
	peer := writeTree(t, map[string]string{"hello.txt": "from peer\n", "sub/deep.txt": "deep\n"})

	n := New(WithUser("app"))
	if err := n.BindFS(own, "", "/", Replace); err != nil {
		t.Fatal(err)
	}
	if err := n.BindFS(FromFid(pipeClient(t, peer)), "", "/mnt/peer", Replace); err != nil {
		t.Fatal(err)
	}

	// Serve the namespace and reach it as an ordinary 9P client would.
	root := pipeClient(t, n)

	read := func(parts ...string) string {
		t.Helper()
		fid, err := root.Walk(parts...)
		if err != nil {
			t.Fatalf("walk %v: %v", parts, err)
		}
		f, err := fid.OpenFile(p9.OREAD)
		if err != nil {
			t.Fatalf("open %v: %v", parts, err)
		}
		defer f.Close()
		b, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("read %v: %v", parts, err)
		}
		return string(b)
	}
	if got := read("status"); got != "ok\n" {
		t.Errorf("own file = %q", got)
	}
	if got := read("mnt", "peer", "hello.txt"); got != "from peer\n" {
		t.Errorf("grafted file = %q", got)
	}
	if got := read("mnt", "peer", "sub", "deep.txt"); got != "deep\n" {
		t.Errorf("grafted nested file = %q", got)
	}

	// The graft shows up when listing.
	dir, err := root.Walk("mnt", "peer")
	if err != nil {
		t.Fatal(err)
	}
	df, err := dir.OpenFile(p9.OREAD)
	if err != nil {
		t.Fatal(err)
	}
	defer df.Close()
	entries, err := df.ReadDir()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name)
	}
	sort.Strings(names)
	if len(names) != 2 || names[0] != "hello.txt" || names[1] != "sub" {
		t.Errorf("grafted listing = %v", names)
	}
}

// Two fids of one file, one a clone of the other, must keep independent
// open state through the namespace, through a graft, and through the
// read-only wrapper.
func TestClonedFidsIndependentThroughNamespace(t *testing.T) {
	peer := writeTree(t, map[string]string{"a.txt": "hello\n"})
	for _, tc := range []struct {
		name string
		bind func(n *Namespace) error
	}{
		{"local", func(n *Namespace) error { return n.BindFS(peer, "", "/x", Replace) }},
		{"grafted", func(n *Namespace) error {
			return n.BindFS(FromFid(pipeClient(t, peer)), "", "/x", Replace)
		}},
		{"readonly", func(n *Namespace) error {
			return n.BindFSOpts(peer, "", "/x", Replace, BindOpts{ReadOnly: true})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := New()
			if err := tc.bind(n); err != nil {
				t.Fatal(err)
			}
			root := pipeClient(t, n)
			f1, err := root.Walk("x", "a.txt")
			if err != nil {
				t.Fatal(err)
			}
			f2, err := f1.Walk() // zero names: clone
			if err != nil {
				t.Fatal(err)
			}
			file1, err := f1.OpenFile(p9.OREAD)
			if err != nil {
				t.Fatal(err)
			}
			defer file1.Close()
			file2, err := f2.OpenFile(p9.OREAD)
			if err != nil {
				t.Fatal(err)
			}
			if err := file2.Close(); err != nil {
				t.Fatal(err)
			}
			b, err := io.ReadAll(file1)
			if err != nil || string(b) != "hello\n" {
				t.Fatalf("read via original after closing clone = %q, %v", b, err)
			}
		})
	}
}

func TestWithUserNamesSyntheticDirs(t *testing.T) {
	ctx := context.Background()
	rootUid := func(n *Namespace) string {
		root, _ := n.Attach(ctx, "u", "")
		st, err := root.Stat(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return st.Uid
	}
	if got := rootUid(New(WithUser("app"))); got != "app" {
		t.Errorf("root Uid = %q, want app", got)
	}
	if got := rootUid(New()); got != DefaultUser {
		t.Errorf("default root Uid = %q, want %q", got, DefaultUser)
	}
}
