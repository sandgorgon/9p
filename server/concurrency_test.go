package server_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	p9 "github.com/sandgorgon/9p"
	"github.com/sandgorgon/9p/client"
	"github.com/sandgorgon/9p/server"
)

// blockingFile is a server.File whose Stat signals on started when
// it begins, then blocks until either release is closed or ctx is
// cancelled — used to hold a MaxConcurrentRequests slot open under
// test control and to observe exactly when (or whether) a request
// actually reaches the FileSystem backend.
type blockingFile struct {
	id      int
	started chan int
	release <-chan struct{}
}

func (f *blockingFile) Qid() p9.Qid { return p9.Qid{Type: p9.QTFILE, Path: uint64(f.id)} }

func (f *blockingFile) Stat(ctx context.Context) (p9.Stat, error) {
	f.started <- f.id
	select {
	case <-f.release:
		return p9.Stat{Qid: f.Qid(), Name: fmt.Sprintf("f%d", f.id)}, nil
	case <-ctx.Done():
		return p9.Stat{}, ctx.Err()
	}
}

func (f *blockingFile) WStat(ctx context.Context, st p9.Stat) error { return nil }

func (f *blockingFile) Walk(ctx context.Context, name string) (server.File, error) {
	return nil, errors.New("server: blockingFile has no children")
}

func (f *blockingFile) Open(ctx context.Context, mode p9.Mode) error { return nil }

func (f *blockingFile) Create(ctx context.Context, name string, perm, mode p9.Mode) (server.File, error) {
	return nil, errors.New("server: blockingFile cannot create")
}

func (f *blockingFile) Read(ctx context.Context, offset int64, p []byte) (int, error) {
	return 0, io.EOF
}

func (f *blockingFile) Write(ctx context.Context, offset int64, p []byte) (int, error) {
	return len(p), nil
}

func (f *blockingFile) Remove(ctx context.Context) error { return nil }

func (f *blockingFile) Close() error { return nil }

// blockingFS attaches to a single blockingFile shared by every fid
// walked from the root, so a test can drive many concurrent Stat
// calls against distinct fids that all funnel through the same
// started/release channels.
type blockingFS struct {
	started chan int
	release <-chan struct{}
}

func (fs *blockingFS) Attach(ctx context.Context, uname, aname string) (server.File, error) {
	return &blockingFile{id: 0, started: fs.started, release: fs.release}, nil
}

func newBlockingTestClient(t *testing.T, maxConcurrent uint32) (*client.Client, chan int, chan struct{}) {
	t.Helper()
	started := make(chan int, 64)
	release := make(chan struct{})
	fs := &blockingFS{started: started, release: release}
	srv := &server.Server{FS: fs, MaxConcurrentRequests: maxConcurrent}

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
	return c, started, release
}

// awaitStarted waits up to a short deadline for n Stat calls to
// begin, failing the test if fewer arrive in time.
func awaitStarted(t *testing.T, started chan int, n int) {
	t.Helper()
	for i := range n {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for Stat #%d/%d to start", i+1, n)
		}
	}
}

// assertNoneStarted checks that no further Stat call begins within a
// short window, proving a request is genuinely parked rather than
// having reached the FileSystem backend.
func assertNoneStarted(t *testing.T, started chan int) {
	t.Helper()
	select {
	case id := <-started:
		t.Fatalf("Stat #%d started, want none to have started yet", id)
	case <-time.After(200 * time.Millisecond):
	}
}

func TestMaxConcurrentRequestsLimitsConcurrency(t *testing.T) {
	const n = 2
	c, started, release := newBlockingTestClient(t, n)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	done := make(chan error, n+1)
	for range n + 1 {
		fid, err := root.Walk()
		if err != nil {
			t.Fatalf("Walk (clone): %v", err)
		}
		go func() {
			_, err := fid.Stat()
			done <- err
		}()
	}

	// Only n requests should be able to reach the backend at once.
	awaitStarted(t, started, n)
	assertNoneStarted(t, started)

	close(release)
	for i := range n + 1 {
		if err := <-done; err != nil {
			t.Errorf("Stat #%d: %v", i, err)
		}
	}
	// The (n+1)th request should have started only after release.
	select {
	case <-started:
	default:
		t.Error("(n+1)th Stat never started after release")
	}
}

func TestMaxConcurrentRequestsTflushCancelsQueuedRequest(t *testing.T) {
	c, started, release := newBlockingTestClient(t, 1)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	// Clone both fids up front, before the semaphore's only slot is
	// occupied — Twalk is itself semaphore-gated, so cloning a fid
	// after the slot is taken would queue right alongside the Stat
	// this test means to test, with no context to unstick it.
	firstFid, err := root.Walk()
	if err != nil {
		t.Fatalf("Walk (clone): %v", err)
	}
	secondFid, err := root.Walk()
	if err != nil {
		t.Fatalf("Walk (clone): %v", err)
	}

	// First Stat takes the only slot and holds it.
	firstDone := make(chan error, 1)
	go func() { _, err := firstFid.Stat(); firstDone <- err }()
	awaitStarted(t, started, 1)

	// Second Stat has nowhere to run: it should queue on the
	// semaphore without ever reaching the backend.
	ctx, cancel := context.WithCancel(context.Background())
	secondDone := make(chan error, 1)
	go func() {
		_, err := secondFid.StatContext(ctx)
		secondDone <- err
	}()
	assertNoneStarted(t, started)

	// Flushing it before it ever started must unblock it without
	// ever dispatching into the backend.
	cancel()
	select {
	case err := <-secondDone:
		if err == nil {
			t.Error("queued Stat succeeded, want it cancelled by flush")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for queued Stat to be flushed — server likely deadlocked")
	}
	assertNoneStarted(t, started)

	// The first request is still running and holding the slot; let
	// it finish normally.
	close(release)
	if err := <-firstDone; err != nil {
		t.Errorf("first Stat: %v", err)
	}
}

func TestMaxConcurrentRequestsTflushReachesInFlightRequestAtCap(t *testing.T) {
	// Regression test for the deadlock in the original design: with
	// the semaphore full, a Tflush for an already-running request
	// must not get stuck behind a second, still-queued request on
	// the same connection's single read loop.
	c, started, release := newBlockingTestClient(t, 1)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	// Clone both fids up front, before the semaphore's only slot is
	// occupied — see the comment in the queued-flush test above for
	// why cloning after the slot is taken would itself deadlock.
	firstFid, err := root.Walk()
	if err != nil {
		t.Fatalf("Walk (clone): %v", err)
	}
	secondFid, err := root.Walk()
	if err != nil {
		t.Fatalf("Walk (clone): %v", err)
	}

	// First Stat takes the only slot.
	firstCtx, firstCancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() { _, err := firstFid.StatContext(firstCtx); firstDone <- err }()
	awaitStarted(t, started, 1)

	// Second Stat queues behind it, waiting for the slot — and stays
	// queued for the rest of the test.
	secondDone := make(chan error, 1)
	go func() { _, err := secondFid.Stat(); secondDone <- err }()
	assertNoneStarted(t, started)

	// Cancelling the first request's context sends a Tflush for it.
	// Under the old blocking-read-loop design, that Tflush would
	// never be read (the loop is stuck trying to seat the second
	// request), and this would time out.
	firstCancel()
	select {
	case err := <-firstDone:
		if err == nil {
			t.Error("first Stat succeeded, want it cancelled by flush")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out flushing the in-flight request — read loop likely wedged behind the queued one")
	}

	// Its slot should now free up for the queued second request.
	awaitStarted(t, started, 1)
	close(release)
	if err := <-secondDone; err != nil {
		t.Errorf("second Stat: %v", err)
	}
}

func TestMaxConcurrentRequestsZeroIsUnlimited(t *testing.T) {
	const n = 8
	c, started, release := newBlockingTestClient(t, 0)
	root, err := c.Attach("glenda", "")
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}

	done := make(chan error, n)
	for range n {
		fid, err := root.Walk()
		if err != nil {
			t.Fatalf("Walk (clone): %v", err)
		}
		go func() {
			_, err := fid.Stat()
			done <- err
		}()
	}

	// All n should start concurrently with no limit in effect.
	awaitStarted(t, started, n)
	close(release)
	for i := range n {
		if err := <-done; err != nil {
			t.Errorf("Stat #%d: %v", i, err)
		}
	}
}
