package p9

import "errors"

// Errors returned while framing or decoding a 9P2000 message. These
// indicate a malformed message on the wire — protocol-level failures
// (a file not found, permission denied, and so on) are reported by
// the peer as an RerrorFcall instead.
var (
	ErrTruncated     = errors.New("p9: message truncated")
	ErrTrailingBytes = errors.New("p9: trailing bytes after message")
	ErrMsgTooLarge   = errors.New("p9: message exceeds msize")
	ErrUnknownType   = errors.New("p9: unknown message type")
	ErrTooManyWElem  = errors.New("p9: too many walk elements")
)

// RerrorFcall is the error reply to any T-message. It implements the
// error interface so it can be returned directly from client RPC
// calls.
type RerrorFcall struct {
	Ename string
}

func (m *RerrorFcall) Error() string { return m.Ename }

func (m *RerrorFcall) MsgType() FcallType { return Rerror }

func (m *RerrorFcall) marshalBody(e *encoder) { e.string(m.Ename) }

func (m *RerrorFcall) unmarshalBody(d *decoder) { m.Ename = d.string() }
