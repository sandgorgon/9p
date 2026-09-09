package client_test

import (
	"io"
	"testing"

	p9 "github.com/sandgorgon/9p"
)

// TestFileRename exercises File.Rename's Twstat wrapper: it should
// change only the file's final path element, leaving its content
// and location within its parent directory intact.
func TestFileRename(t *testing.T) {
	c := newCreateTestClient(t)
	if _, err := c.Attach("glenda", ""); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	f, err := c.Create("old.txt", 0644, p9.ORDWR)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	content := []byte("renamed content\n")
	if _, err := f.Write(content); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := f.Rename("new.txt"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if _, err := c.Open("/old.txt", p9.OREAD); err == nil {
		t.Error("Open old name after Rename succeeded, want error")
	}

	rf, err := c.Open("/new.txt", p9.OREAD)
	if err != nil {
		t.Fatalf("Open new name: %v", err)
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

// TestFileRemove exercises File.Remove's Tremove wrapper: the file
// should no longer be reachable afterward, and the fid must not be
// reused (Tremove clunks it, per spec, even though Remove doesn't
// expose a separate Close call after it).
func TestFileRemove(t *testing.T) {
	c := newCreateTestClient(t)
	if _, err := c.Attach("glenda", ""); err != nil {
		t.Fatalf("Attach: %v", err)
	}

	f, err := c.Create("doomed.txt", 0644, p9.OWRITE)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := f.Remove(); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	if _, err := c.Open("/doomed.txt", p9.OREAD); err == nil {
		t.Error("Open after Remove succeeded, want error")
	}
}
