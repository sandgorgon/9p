// Package server implements a 9P2000 server: given a FileSystem
// backend, it handles wire encoding, fid bookkeeping, multi-element
// walk batching, and Tflush cancellation, and exposes the backend to
// any 9P2000 client over a net.Listener or a single connection.
package server

import (
	"context"
	"io"
	"net"

	p9 "github.com/sandgorgon/9p"
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

	// Close's error is reported to the client as an Rerror in reply
	// to whichever of Tclunk or Tremove triggered it — but the fid is
	// released either way; a failing Close never leaves it dangling.
	Close() error
}

// Server serves a FileSystem over 9P2000.
type Server struct {
	FS FileSystem

	// Msize caps the message size the server will negotiate with a
	// client. Zero means p9.DefaultMsize.
	Msize uint32

	// MaxConcurrentRequests caps how many requests from one connection
	// may be dispatched into FS at once. Zero means unlimited, matching
	// today's behavior. 9P2000 lets a client have many tagged requests
	// outstanding on one connection at once (see Tflush); without this,
	// a single connection can drive an unbounded number of concurrent
	// calls into FS just by pipelining requests faster than it reads
	// replies.
	//
	// Once the limit is reached, a new request waits for a slot before
	// it is dispatched. That wait never stalls the connection's ability
	// to read and act on further messages in the meantime — in
	// particular Tflush, which always runs immediately regardless of
	// the limit, since it's a client's only way to free a slot held by
	// a stuck request. A request flushed before it ever acquires a slot
	// gets no reply, exactly as if it had been flushed after starting.
	//
	// This bounds concurrent work dispatched into FS, not how many
	// requests a client can have pending on the connection: a client
	// that pipelines far more requests than this limit still causes one
	// idle, FS-untouched goroutine per pending request until a slot
	// frees or it's flushed.
	//
	// Mirrors golang.org/x/net/http2.Server.MaxConcurrentStreams.
	MaxConcurrentRequests uint32

	// ConnContext, if non-nil, is called by Serve once per accepted
	// connection — right after Accept, before any 9P messages are
	// read on it — with a fresh context.Background() and the
	// connection. Its return value becomes the base context for
	// every request handled on that connection, including the
	// Attach call: a caller doing its own per-connection setup (a
	// TLS handshake in particular) can stash whatever it learns via
	// context.WithValue, and a FileSystem/File implementation
	// recovers it from ctx.
	//
	// ConnContext is not called by ServeConnContext, which is the
	// lower-level entry point for callers that build their own base
	// context directly instead of relying on this hook — see
	// ServeConnContext.
	//
	// Mirrors net/http.Server.ConnContext.
	ConnContext func(ctx context.Context, c net.Conn) context.Context
}

func (s *Server) maxMsize() uint32 {
	if s.Msize == 0 {
		return p9.DefaultMsize
	}
	return s.Msize
}

// Serve accepts connections on l, serving each in its own goroutine
// until Accept returns an error. For each connection it calls
// ConnContext (if set) to build that connection's base context, then
// serves it via ServeConnContext.
func (s *Server) Serve(l net.Listener) error {
	for {
		nc, err := l.Accept()
		if err != nil {
			return err
		}
		go func() {
			defer nc.Close()
			ctx := context.Background()
			if s.ConnContext != nil {
				ctx = s.ConnContext(ctx, nc)
			}
			s.ServeConnContext(ctx, nc)
		}()
	}
}

// ServeConn serves a single connection until it errors or the peer
// closes it, then returns the error that ended it (io.EOF on a
// clean close). ServeConn(rwc) is equivalent to
// ServeConnContext(context.Background(), rwc).
func (s *Server) ServeConn(rwc io.ReadWriteCloser) error {
	return s.ServeConnContext(context.Background(), rwc)
}

// ServeConnContext is ServeConn with an explicit base context: ctx
// becomes the base for every request handled on rwc, including the
// Attach call, exactly as if it were ConnContext's return value.
// Unlike Serve, ServeConnContext never calls ConnContext itself — it
// is the entry point for a caller that already did its own
// per-connection setup (a TLS handshake in particular) and built ctx
// by hand, bypassing Serve's accept loop entirely.
func (s *Server) ServeConnContext(ctx context.Context, rwc io.ReadWriteCloser) error {
	return s.newConn(ctx, rwc).serve()
}
