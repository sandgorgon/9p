package client

import (
	"errors"
	"sync"

	p9 "github.com/sandgorgon/9p"
)

// callMux tracks in-flight requests by tag so replies read off the
// single connection can be routed back to the goroutine waiting on
// them.
type callMux struct {
	mu       sync.Mutex
	pending  map[p9.Tag]chan p9.Message
	next     p9.Tag
	closed   bool
	closeErr error
}

func newCallMux() *callMux {
	return &callMux{pending: make(map[p9.Tag]chan p9.Message)}
}

// register allocates an unused tag and a channel that will receive
// exactly one reply for it.
func (m *callMux) register() (p9.Tag, chan p9.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, nil, m.closeErr
	}
	for range int(^p9.Tag(0)) {
		t := m.next
		m.next++
		if t == p9.NoTag {
			continue
		}
		if _, busy := m.pending[t]; busy {
			continue
		}
		ch := make(chan p9.Message, 1)
		m.pending[t] = ch
		return t, ch, nil
	}
	return 0, nil, errors.New("client: no free tags")
}

// registerTag reserves a specific tag, used only for the Tversion
// handshake, which the spec requires to use NoTag.
func (m *callMux) registerTag(t p9.Tag) (chan p9.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, m.closeErr
	}
	if _, busy := m.pending[t]; busy {
		return nil, errors.New("client: tag already in use")
	}
	ch := make(chan p9.Message, 1)
	m.pending[t] = ch
	return ch, nil
}

func (m *callMux) forget(t p9.Tag) {
	m.mu.Lock()
	delete(m.pending, t)
	m.mu.Unlock()
}

// deliver routes msg to the caller waiting on tag, if any. A tag
// with no waiter (e.g. the original reply to a request that was
// just flushed) is silently dropped, per spec.
func (m *callMux) deliver(t p9.Tag, msg p9.Message) {
	m.mu.Lock()
	ch, ok := m.pending[t]
	if ok {
		delete(m.pending, t)
	}
	m.mu.Unlock()
	if ok {
		ch <- msg
	}
}

// closeAll fails every outstanding call and marks the mux closed, so
// that future register calls fail fast instead of hanging.
func (m *callMux) closeAll(err error) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.closed = true
	m.closeErr = err
	pending := m.pending
	m.pending = nil
	m.mu.Unlock()
	for _, ch := range pending {
		close(ch)
	}
}

func (m *callMux) err() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closeErr != nil {
		return m.closeErr
	}
	return errors.New("client: connection closed")
}
