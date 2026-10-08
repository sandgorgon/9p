package server_test

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"

	p9 "github.com/sandgorgon/9p"
	"github.com/sandgorgon/9p/client"
	"github.com/sandgorgon/9p/server"
)

var errCloseFailed = errors.New("server: close failed")

// closeErrFile is a minimal server.File whose Close always fails and
// whose Remove returns whatever removeErr is set to, used to verify
// that tClunk and tRemove surface Close's error to the client rather
// than discarding it.
type closeErrFile struct {
	removeErr error
}

func (f *closeErrFile) Qid() p9.Qid { return p9.Qid{Type: p9.QTFILE, Path: 1} }

func (f *closeErrFile) Stat(ctx context.Context) (p9.Stat, error) {
	return p9.Stat{Qid: f.Qid(), Name: "closeerr"}, nil
}

func (f *closeErrFile) WStat(ctx context.Context, st p9.Stat) error { return nil }

func (f *closeErrFile) Walk(ctx context.Context, name string) (server.File, error) {
	return nil, errors.New("server: closeErrFile has no children")
}

func (f *closeErrFile) Open(ctx context.Context, mode p9.Mode) error { return nil }

func (f *closeErrFile) Create(ctx context.Context, name string, perm, mode p9.Mode) (server.File, error) {
	return nil, errors.New("server: closeErrFile cannot create")
}

func (f *closeErrFile) Read(ctx context.Context, offset int64, p []byte) (int, error) {
	return 0, io.EOF
}

func (f *closeErrFile) Write(ctx context.Context, offset int64, p []byte) (int, error) {
	return len(p), nil
}

func (f *closeErrFile) Remove(ctx context.Context) error { return f.removeErr }

func (f *closeErrFile) Close() error { return errCloseFailed }

type closeErrFS struct{ removeErr error }

func (fs closeErrFS) Attach(ctx context.Context, uname, aname string) (server.File, error) {
	return &closeErrFile{removeErr: fs.removeErr}, nil
}

func newCloseErrClient(t *testing.T, removeErr error) *client.Client {
	t.Helper()
	srv := &server.Server{FS: closeErrFS{removeErr: removeErr}}

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

func TestClunkSurfacesCloseError(t *testing.T) {
	c := newCloseErrClient(t, nil)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	err = root.Clunk()
	if err == nil {
		t.Fatal("Clunk succeeded, want the error Close returned")
	}
	if err.Error() != errCloseFailed.Error() {
		t.Errorf("Clunk error = %q, want %q", err, errCloseFailed)
	}
}

func TestRemoveSurfacesCloseErrorWhenRemoveSucceeds(t *testing.T) {
	c := newCloseErrClient(t, nil) // Remove succeeds; only Close fails.
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	err = root.Remove()
	if err == nil {
		t.Fatal("Remove succeeded, want the error Close returned")
	}
	if err.Error() != errCloseFailed.Error() {
		t.Errorf("Remove error = %q, want %q", err, errCloseFailed)
	}
}

func TestRemoveErrorTakesPriorityOverCloseError(t *testing.T) {
	removeErr := errors.New("server: remove failed")
	c := newCloseErrClient(t, removeErr)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	err = root.Remove()
	if err == nil {
		t.Fatal("Remove succeeded, want an error")
	}
	if err.Error() != removeErr.Error() {
		t.Errorf("Remove error = %q, want Remove's own error %q, not Close's", err, removeErr)
	}
}

// cloneFile records which instance each fid reaches, so the tests can
// tell a shared File from a cloned one.
type cloneFile struct {
	id     int
	opened bool
	closed bool
	fs     *cloneFS
}

type cloneFS struct {
	files []*cloneFile
	clone bool // whether files implement Cloner (see cloneOnlyFile)
}

func (fs *cloneFS) newFile() *cloneFile {
	f := &cloneFile{id: len(fs.files), fs: fs}
	fs.files = append(fs.files, f)
	return f
}

func (f *cloneFile) Qid() p9.Qid { return p9.Qid{Type: p9.QTFILE, Path: 1} }
func (f *cloneFile) Stat(ctx context.Context) (p9.Stat, error) {
	return p9.Stat{Qid: f.Qid(), Name: "clone"}, nil
}
func (f *cloneFile) WStat(ctx context.Context, st p9.Stat) error { return nil }
func (f *cloneFile) Walk(ctx context.Context, name string) (server.File, error) {
	return nil, errors.New("no children")
}
func (f *cloneFile) Open(ctx context.Context, mode p9.Mode) error { f.opened = true; return nil }
func (f *cloneFile) Create(ctx context.Context, name string, perm, mode p9.Mode) (server.File, error) {
	return nil, errors.New("cannot create")
}
func (f *cloneFile) Read(ctx context.Context, offset int64, p []byte) (int, error) {
	if !f.opened || f.closed {
		return 0, errors.New("read of unopened file")
	}
	return 0, io.EOF
}
func (f *cloneFile) Write(ctx context.Context, offset int64, p []byte) (int, error) {
	return len(p), nil
}
func (f *cloneFile) Remove(ctx context.Context) error { return nil }
func (f *cloneFile) Close() error                     { f.closed = true; return nil }

// clonerFile is a cloneFile that implements server.Cloner.
type clonerFile struct{ *cloneFile }

func (f clonerFile) Clone(ctx context.Context) (server.File, error) {
	return clonerFile{f.fs.newFile()}, nil
}

func (fs *cloneFS) Attach(ctx context.Context, uname, aname string) (server.File, error) {
	f := fs.newFile()
	if fs.clone {
		return clonerFile{f}, nil
	}
	return f, nil
}

func newCloneClient(t *testing.T, fs *cloneFS) *client.Client {
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
	return c
}

// A zero-name Twalk on a Cloner File must give the clone its own File.
func TestZeroNameWalkClonesCloner(t *testing.T) {
	fs := &cloneFS{clone: true}
	c := newCloneClient(t, fs)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	f2, err := root.Walk()
	if err != nil {
		t.Fatalf("clone: %v", err)
	}

	file1, err := root.OpenFile(p9.OREAD)
	if err != nil {
		t.Fatalf("open original: %v", err)
	}
	defer file1.Close()
	file2, err := f2.OpenFile(p9.OREAD)
	if err != nil {
		t.Fatalf("open clone: %v", err)
	}
	if err := file2.Close(); err != nil {
		t.Fatalf("close clone: %v", err)
	}

	if len(fs.files) != 2 {
		t.Fatalf("backend saw %d File instances, want 2 (original + clone)", len(fs.files))
	}
	if fs.files[0].closed {
		t.Fatal("clunking the clone closed the original File")
	}
	if !fs.files[1].closed {
		t.Fatal("clunking the clone did not close the clone's File")
	}
}

// A File that is not a Cloner keeps being shared by a clone, exactly
// as before.
func TestZeroNameWalkSharesNonCloner(t *testing.T) {
	fs := &cloneFS{}
	c := newCloneClient(t, fs)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if _, err := root.Walk(); err != nil {
		t.Fatalf("clone: %v", err)
	}
	if len(fs.files) != 1 {
		t.Fatalf("backend saw %d File instances, want 1 (shared)", len(fs.files))
	}
}
