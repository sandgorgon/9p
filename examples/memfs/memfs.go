// Package memfs is an in-memory server.FileSystem backend: a tree of
// files and directories held entirely in memory, useful both as a
// demo server and as a test fixture for the server package.
package memfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"sync"
	"sync/atomic"

	p9 "github.com/sandgorgon/9p"
	"github.com/sandgorgon/9p/server"
)

type node struct {
	name     string
	dir      bool
	qid      p9.Qid
	perm     p9.Mode // permission bits only; DMDIR is derived from dir
	atime    uint32
	mtime    uint32
	data     []byte
	children map[string]*node
	parent   *node
}

// FS is an in-memory 9P2000 filesystem. The zero value is not
// usable; construct one with New.
type FS struct {
	mu       sync.RWMutex
	root     *node
	nextPath atomic.Uint64
}

// New returns an empty filesystem: a single root directory.
func New() *FS {
	fs := &FS{}
	fs.root = fs.newNode("/", true, 0755)
	return fs
}

func (fs *FS) newNode(name string, dir bool, perm p9.Mode) *node {
	qtype := p9.QTFILE
	if dir {
		qtype = p9.QTDIR
	}
	n := &node{name: name, dir: dir, perm: perm, qid: p9.Qid{Type: qtype, Path: fs.nextPath.Add(1)}}
	if dir {
		n.children = make(map[string]*node)
	}
	return n
}

// Attach ignores uname and aname: every attach sees the same tree,
// rooted at "/".
func (fs *FS) Attach(ctx context.Context, uname, aname string) (server.File, error) {
	return &file{fs: fs, n: fs.root}, nil
}

type file struct {
	fs *FS
	n  *node
}

func (f *file) Qid() p9.Qid {
	f.fs.mu.RLock()
	defer f.fs.mu.RUnlock()
	return f.n.qid
}

func (f *file) statLocked() p9.Stat {
	n := f.n
	mode := n.perm
	if n.dir {
		mode |= p9.DMDIR
	}
	return p9.Stat{
		Qid:    n.qid,
		Mode:   mode,
		Atime:  n.atime,
		Mtime:  n.mtime,
		Length: uint64(len(n.data)),
		Name:   n.name,
		Uid:    "glenda",
		Gid:    "glenda",
		Muid:   "glenda",
	}
}

func (f *file) Stat(ctx context.Context) (p9.Stat, error) {
	f.fs.mu.RLock()
	defer f.fs.mu.RUnlock()
	return f.statLocked(), nil
}

// WStat applies the fields of st that are not set to their 9P2000
// "don't touch" sentinel: the empty string for Name, and all-ones
// for Mode/Mtime/Length. Atime, Uid, Gid, and Muid are accepted but
// ignored.
func (f *file) WStat(ctx context.Context, st p9.Stat) error {
	f.fs.mu.Lock()
	defer f.fs.mu.Unlock()
	n := f.n

	if st.Name != "" && st.Name != n.name {
		if n.parent == nil {
			return errors.New("memfs: cannot rename root")
		}
		if _, exists := n.parent.children[st.Name]; exists {
			return fmt.Errorf("memfs: %s: already exists", st.Name)
		}
		delete(n.parent.children, n.name)
		n.name = st.Name
		n.parent.children[st.Name] = n
	}
	if st.Mode != p9.Mode(^uint32(0)) {
		n.perm = st.Mode &^ p9.DMDIR
	}
	if st.Length != ^uint64(0) && !n.dir {
		n.data = resize(n.data, int(st.Length))
		n.qid.Version++
	}
	return nil
}

func (f *file) Walk(ctx context.Context, name string) (server.File, error) {
	f.fs.mu.RLock()
	defer f.fs.mu.RUnlock()
	n := f.n
	if !n.dir {
		return nil, errors.New("memfs: not a directory")
	}
	if name == ".." {
		if n.parent == nil {
			return &file{fs: f.fs, n: n}, nil
		}
		return &file{fs: f.fs, n: n.parent}, nil
	}
	child, ok := n.children[name]
	if !ok {
		return nil, fmt.Errorf("memfs: %s: no such file", name)
	}
	return &file{fs: f.fs, n: child}, nil
}

func (f *file) Open(ctx context.Context, mode p9.Mode) error {
	if mode&p9.OTRUNC == 0 {
		return nil
	}
	f.fs.mu.Lock()
	defer f.fs.mu.Unlock()
	if f.n.dir {
		return nil
	}
	f.n.data = f.n.data[:0]
	f.n.qid.Version++
	return nil
}

func (f *file) Create(ctx context.Context, name string, perm p9.Mode, mode p9.Mode) (server.File, error) {
	f.fs.mu.Lock()
	defer f.fs.mu.Unlock()
	n := f.n
	if !n.dir {
		return nil, errors.New("memfs: not a directory")
	}
	if _, exists := n.children[name]; exists {
		return nil, fmt.Errorf("memfs: %s: already exists", name)
	}
	child := f.fs.newNode(name, perm.IsDir(), perm&^p9.DMDIR&p9.DMPerm)
	child.parent = n
	n.children[name] = child
	return &file{fs: f.fs, n: child}, nil
}

func (f *file) Read(ctx context.Context, offset int64, p []byte) (int, error) {
	f.fs.mu.RLock()
	defer f.fs.mu.RUnlock()
	n := f.n
	if n.dir {
		return readDir(n, offset, p)
	}
	if offset >= int64(len(n.data)) {
		return 0, io.EOF
	}
	return copy(p, n.data[offset:]), nil
}

// readDir lists n's children as Stat entries, sorted by name so that
// repeated reads at growing offsets see a consistent sequence, and
// hands them to server.MarshalDir to satisfy the directory Read
// contract (whole entries only, never split across a call).
func readDir(n *node, offset int64, p []byte) (int, error) {
	names := make([]string, 0, len(n.children))
	for name := range n.children {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]p9.Stat, len(names))
	for i, name := range names {
		c := n.children[name]
		mode := c.perm
		if c.dir {
			mode |= p9.DMDIR
		}
		entries[i] = p9.Stat{
			Qid: c.qid, Mode: mode, Atime: c.atime, Mtime: c.mtime,
			Length: uint64(len(c.data)), Name: c.name,
			Uid: "glenda", Gid: "glenda", Muid: "glenda",
		}
	}
	return server.MarshalDir(entries, offset, p)
}

func (f *file) Write(ctx context.Context, offset int64, p []byte) (int, error) {
	f.fs.mu.Lock()
	defer f.fs.mu.Unlock()
	n := f.n
	if n.dir {
		return 0, errors.New("memfs: cannot write to a directory")
	}
	end := offset + int64(len(p))
	if end > int64(len(n.data)) {
		n.data = resize(n.data, int(end))
	}
	copy(n.data[offset:], p)
	n.qid.Version++
	return len(p), nil
}

func (f *file) Remove(ctx context.Context) error {
	f.fs.mu.Lock()
	defer f.fs.mu.Unlock()
	n := f.n
	if n.parent == nil {
		return errors.New("memfs: cannot remove root")
	}
	if n.dir && len(n.children) > 0 {
		return errors.New("memfs: directory not empty")
	}
	delete(n.parent.children, n.name)
	return nil
}

func (f *file) Close() error { return nil }

func resize(b []byte, n int) []byte {
	if n <= len(b) {
		return b[:n]
	}
	grown := make([]byte, n)
	copy(grown, b)
	return grown
}
