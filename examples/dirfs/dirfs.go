// Package dirfs is a server.FileSystem backend that exports a real
// directory tree from the local filesystem, using only the standard
// library. Every path it touches is validated to stay within the
// configured root, so a client cannot walk ".." past it.
package dirfs

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"

	p9 "github.com/sandgorgon/9p"
	"github.com/sandgorgon/9p/server"
)

// FS exports the directory tree rooted at a local path.
type FS struct {
	root string // absolute, cleaned
}

// New returns an FS rooted at root, which must already exist and be
// a directory.
func New(root string) (*FS, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("dirfs: %s: not a directory", root)
	}
	return &FS{root: filepath.Clean(abs)}, nil
}

// Attach ignores uname and aname: every attach sees the same tree,
// rooted at the FS's configured directory.
func (d *FS) Attach(ctx context.Context, uname, aname string) (server.File, error) {
	return &file{fs: d, path: d.root}, nil
}

// within reports whether path is root itself or a descendant of it,
// guarding every path this package constructs before it touches the
// filesystem.
func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || !strings.HasPrefix(rel, "..")
}

type file struct {
	fs *FS

	mu   sync.Mutex
	path string
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
	info, err := os.Lstat(f.currentPath())
	if err != nil {
		return p9.Qid{}
	}
	return qidFor(f.currentPath(), info)
}

func (f *file) Stat(ctx context.Context) (p9.Stat, error) {
	path := f.currentPath()
	info, err := os.Lstat(path)
	if err != nil {
		return p9.Stat{}, err
	}
	return statFromInfo(path, info), nil
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
		if !within(f.fs.root, newPath) {
			return fmt.Errorf("dirfs: %q escapes root", st.Name)
		}
		if err := os.Rename(f.path, newPath); err != nil {
			return err
		}
		f.path = newPath
	}
	if st.Mode != p9.Mode(^uint32(0)) {
		if err := os.Chmod(f.path, os.FileMode(st.Mode&p9.DMPerm)); err != nil {
			return err
		}
	}
	if st.Length != ^uint64(0) {
		if err := os.Truncate(f.path, int64(st.Length)); err != nil {
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
		if path == f.fs.root {
			newPath = f.fs.root
		} else {
			newPath = filepath.Dir(path)
		}
	} else {
		newPath = filepath.Join(path, name)
	}
	if !within(f.fs.root, newPath) {
		return nil, fmt.Errorf("dirfs: %q escapes root", name)
	}
	if _, err := os.Lstat(newPath); err != nil {
		return nil, fmt.Errorf("dirfs: %s: %w", name, err)
	}
	return &file{fs: f.fs, path: newPath}, nil
}

func (f *file) Open(ctx context.Context, mode p9.Mode) error {
	path := f.currentPath()
	info, err := os.Lstat(path)
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
	osf, err := os.OpenFile(path, flag, 0)
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
	if !within(f.fs.root, newPath) {
		return nil, fmt.Errorf("dirfs: %q escapes root", name)
	}

	child := &file{fs: f.fs, path: newPath}
	if perm.IsDir() {
		if err := os.Mkdir(newPath, os.FileMode(perm&p9.DMPerm)); err != nil {
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
	osf, err := os.OpenFile(newPath, flag, os.FileMode(perm&p9.DMPerm))
	if err != nil {
		return nil, err
	}
	child.osf = osf
	return child, nil
}

func (f *file) Read(ctx context.Context, offset int64, p []byte) (int, error) {
	path := f.currentPath()
	info, err := os.Lstat(path)
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

// readDir renders path's children as a concatenation of Stat blobs,
// the wire format a 9P2000 directory read returns. os.ReadDir
// returns entries sorted by name, so repeated reads at growing
// offsets see a consistent sequence as long as the directory isn't
// concurrently modified.
func (f *file) readDir(path string, offset int64, p []byte) (int, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return 0, err
	}
	var buf []byte
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue // entry vanished between ReadDir and Info; skip it
		}
		st := statFromInfo(filepath.Join(path, e.Name()), info)
		buf = append(buf, st.Marshal()...)
	}
	if offset >= int64(len(buf)) {
		return 0, io.EOF
	}
	return copy(p, buf[offset:]), nil
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
	return os.Remove(f.currentPath())
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
