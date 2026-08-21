// Package server implements a 9P2000 server: given a FileSystem
// backend, it handles wire encoding, fid bookkeeping, multi-element
// walk batching, and Tflush cancellation, and exposes the backend to
// any 9P2000 client over a net.Listener or a single connection.
package server

import (
	"context"
	"io"
	"net"

	p9 "p9"
)

// FileSystem is the backend a Server serves over 9P2000. Attach is
// its only entry point; every other file is reached by walking from
// the File it returns.
type FileSystem interface {
	Attach(ctx context.Context, uname, aname string) (File, error)
}

// File is a single file or directory in a FileSystem backend. Server
// handles all wire encoding, fid bookkeeping, and multi-element walk
// batching; an implementation only needs to answer for itself and
// resolve its immediate children by name.
//
// Read follows io.ReaderAt-like conventions: returning io.EOF (with
// n possibly 0) is a normal end of file, not an error reply. Every
// other non-nil error returned by any method is reported to the
// client as an Rerror with err.Error() as the message.
//
// ctx is cancelled if the client sends a Tflush for the request
// being served; implementations that can observe cancellation mid
// operation (for example while blocked on a slow backing store)
// should check it, but are not required to.
type File interface {
	Qid() p9.Qid
	Stat(ctx context.Context) (p9.Stat, error)
	WStat(ctx context.Context, st p9.Stat) error
	Walk(ctx context.Context, name string) (File, error)
	Open(ctx context.Context, mode p9.Mode) error
	Create(ctx context.Context, name string, perm p9.Mode, mode p9.Mode) (File, error)
	Read(ctx context.Context, offset int64, p []byte) (int, error)
	Write(ctx context.Context, offset int64, p []byte) (int, error)
	Remove(ctx context.Context) error
	Close() error
}

// Server serves a FileSystem over 9P2000.
type Server struct {
	FS FileSystem

	// Msize caps the message size the server will negotiate with a
	// client. Zero means p9.DefaultMsize.
	Msize uint32
}

func (s *Server) maxMsize() uint32 {
	if s.Msize == 0 {
		return p9.DefaultMsize
	}
	return s.Msize
}

// Serve accepts connections on l, serving each with ServeConn in its
// own goroutine, until Accept returns an error.
func (s *Server) Serve(l net.Listener) error {
	for {
		nc, err := l.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer nc.Close()
			s.ServeConn(nc)
		}()
	}
}

// ServeConn serves a single connection until it errors or the peer
// closes it, then returns the error that ended it (io.EOF on a
// clean close).
func (s *Server) ServeConn(rwc io.ReadWriteCloser) error {
	return s.newConn(rwc).serve()
}
