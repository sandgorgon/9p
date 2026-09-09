// Package dirfs is a server.FileSystem backend that exports a real
// directory tree from the local filesystem, using only the standard
// library. Every path it touches is resolved through an os.Root
// opened on the exported directory, so a client cannot walk ".."
// or follow a symlink at an intermediate path component past it.
package dirfs

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	p9 "github.com/sandgorgon/9p"
	"github.com/sandgorgon/9p/server"
)

// FS exports the directory tree rooted at a local path.
type FS struct {
	root *os.Root
	name string // base name of the root directory, reported as its own Stat name
}

// New returns an FS rooted at root, which must already exist and be
// a directory.
func New(root string) (*FS, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	r, err := os.OpenRoot(abs)
	if err != nil {
		return nil, err
	}
	info, err := r.Stat(".")
	if err != nil {
		r.Close()
		return nil, err
	}
	if !info.IsDir() {
		r.Close()
		return nil, fmt.Errorf("dirfs: %s: not a directory", root)
	}
	return &FS{root: r, name: filepath.Base(abs)}, nil
}

// Attach ignores uname and aname: every attach sees the same tree,
// rooted at the FS's configured directory.
func (d *FS) Attach(ctx context.Context, uname, aname string) (server.File, error) {
	return &file{fs: d, path: "."}, nil
}

type file struct {
	fs *FS

	mu   sync.Mutex
	path string // relative to fs.root; "." is the root itself
	osf  *os.File
}

func qidFor(path string, info fs.FileInfo) p9.Qid {
	t := p9.QTFILE
	if info.IsDir() {
		t = p9.QTDIR
	}
	h := fnv.New64a()
	h.Write([]byte(path))
	return p9.Qid{Type: t, Version: uint32(info.ModTime().Unix()), Path: h.Sum64()}
}

func statFromInfo(path string, info fs.FileInfo) p9.Stat {
	mode := p9.Mode(info.Mode().Perm())
	if info.IsDir() {
		mode |= p9.DMDIR
	}
	return p9.Stat{
		Qid:    qidFor(path, info),
		Mode:   mode,
		Mtime:  uint32(info.ModTime().Unix()),
		Length: uint64(info.Size()),
		Name:   filepath.Base(path),
		Uid:    "glenda",
		Gid:    "glenda",
		Muid:   "glenda",
	}
}

func (f *file) currentPath() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.path
}

func (f *file) Qid() p9.Qid {
	path := f.currentPath()
	info, err := f.fs.root.Lstat(path)
	if err != nil {
		return p9.Qid{}
	}
	return qidFor(path, info)
}

func (f *file) Stat(ctx context.Context) (p9.Stat, error) {
	path := f.currentPath()
	info, err := f.fs.root.Lstat(path)
	if err != nil {
		return p9.Stat{}, err
	}
	st := statFromInfo(path, info)
	if path == "." {
		st.Name = f.fs.name
	}
	return st, nil
}

// WStat supports renaming within the same directory, chmod, and
// truncation via Length; fields set to their 9P2000 "don't touch"
// sentinel (empty Name, all-ones Mode/Length) are left alone. Atime,
// Mtime, Uid, Gid, and Muid are accepted but not applied.
func (f *file) WStat(ctx context.Context, st p9.Stat) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if st.Name != "" {
		if strings.ContainsRune(st.Name, '/') {
			return fmt.Errorf("dirfs: invalid name %q", st.Name)
		}
		newPath := filepath.Join(filepath.Dir(f.path), st.Name)
		if err := f.fs.root.Rename(f.path, newPath); err != nil {
			return err
		}
		f.path = newPath
	}
	if st.Mode != p9.Mode(^uint32(0)) {
		if err := f.fs.root.Chmod(f.path, os.FileMode(st.Mode&p9.DMPerm)); err != nil {
			return err
		}
	}
	if st.Length != ^uint64(0) {
		osf, err := f.fs.root.OpenFile(f.path, os.O_WRONLY, 0)
		if err != nil {
			return err
		}
		err = osf.Truncate(int64(st.Length))
		if cerr := osf.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (f *file) Walk(ctx context.Context, name string) (server.File, error) {
	path := f.currentPath()
	if name == "" || strings.ContainsRune(name, '/') {
		return nil, fmt.Errorf("dirfs: invalid path element %q", name)
	}
	var newPath string
	if name == ".." {
		if path == "." {
			newPath = "."
		} else {
			newPath = filepath.Dir(path)
		}
	} else {
		newPath = filepath.Join(path, name)
	}
	if _, err := f.fs.root.Lstat(newPath); err != nil {
		return nil, fmt.Errorf("dirfs: %s: %w", name, err)
	}
	return &file{fs: f.fs, path: newPath}, nil
}

func (f *file) Open(ctx context.Context, mode p9.Mode) error {
	path := f.currentPath()
	info, err := f.fs.root.Lstat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}
	flag := os.O_RDONLY
	switch mode & 3 {
	case p9.OWRITE:
		flag = os.O_WRONLY
	case p9.ORDWR:
		flag = os.O_RDWR
	}
	if mode&p9.OTRUNC != 0 {
		flag |= os.O_TRUNC
	}
	osf, err := f.fs.root.OpenFile(path, flag, 0)
	if err != nil {
		return err
	}
	f.mu.Lock()
	f.osf = osf
	f.mu.Unlock()
	return nil
}

func (f *file) Create(ctx context.Context, name string, perm p9.Mode, mode p9.Mode) (server.File, error) {
	path := f.currentPath()
	if name == "" || name == ".." || strings.ContainsRune(name, '/') {
		return nil, fmt.Errorf("dirfs: invalid name %q", name)
	}
	newPath := filepath.Join(path, name)

	child := &file{fs: f.fs, path: newPath}
	if perm.IsDir() {
		if err := f.fs.root.Mkdir(newPath, os.FileMode(perm&p9.DMPerm)); err != nil {
			return nil, err
		}
		return child, nil
	}

	flag := os.O_RDONLY | os.O_CREATE | os.O_EXCL
	switch mode & 3 {
	case p9.OWRITE:
		flag = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	case p9.ORDWR:
		flag = os.O_RDWR | os.O_CREATE | os.O_EXCL
	}
	osf, err := f.fs.root.OpenFile(newPath, flag, os.FileMode(perm&p9.DMPerm))
	if err != nil {
		return nil, err
	}
	child.osf = osf
	return child, nil
}

func (f *file) Read(ctx context.Context, offset int64, p []byte) (int, error) {
	path := f.currentPath()
	info, err := f.fs.root.Lstat(path)
	if err != nil {
		return 0, err
	}
	if info.IsDir() {
		return f.readDir(path, offset, p)
	}

	f.mu.Lock()
	osf := f.osf
	f.mu.Unlock()
	if osf == nil {
		return 0, errors.New("dirfs: read of unopened file")
	}
	return osf.ReadAt(p, offset)
}

// readDir lists path's children as Stat entries, sorted by name so
// repeated reads at growing offsets see a consistent sequence as
// long as the directory isn't concurrently modified, and hands them
// to server.MarshalDir to satisfy the directory Read contract
// (whole entries only, never split across a call).
func (f *file) readDir(path string, offset int64, p []byte) (int, error) {
	dirf, err := f.fs.root.Open(path)
	if err != nil {
		return 0, err
	}
	defer dirf.Close()
	dirEntries, err := dirf.ReadDir(-1)
	if err != nil {
		return 0, err
	}
	sort.Slice(dirEntries, func(i, j int) bool { return dirEntries[i].Name() < dirEntries[j].Name() })

	entries := make([]p9.Stat, 0, len(dirEntries))
	for _, e := range dirEntries {
		info, err := e.Info()
		if err != nil {
			continue // entry vanished between ReadDir and Info; skip it
		}
		entries = append(entries, statFromInfo(filepath.Join(path, e.Name()), info))
	}
	return server.MarshalDir(entries, offset, p)
}

func (f *file) Write(ctx context.Context, offset int64, p []byte) (int, error) {
	f.mu.Lock()
	osf := f.osf
	f.mu.Unlock()
	if osf == nil {
		return 0, errors.New("dirfs: write of unopened file")
	}
	return osf.WriteAt(p, offset)
}

func (f *file) Remove(ctx context.Context) error {
	f.Close()
	return f.fs.root.Remove(f.currentPath())
}

func (f *file) Close() error {
	f.mu.Lock()
	osf := f.osf
	f.osf = nil
	f.mu.Unlock()
	if osf != nil {
		return osf.Close()
	}
	return nil
}
