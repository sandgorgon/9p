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
