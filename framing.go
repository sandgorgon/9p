package p9

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// minMsgSize is the smallest legal message: a 4-byte size, a 1-byte
// type, and a 2-byte tag, with an empty body.
const minMsgSize = 7

// ReadMessage reads one complete 9P2000 message from r: a 4-byte
// little-endian size prefix followed by size-4 bytes of body. The
// returned slice includes the size prefix, as Unmarshal expects. If
// msize is non-zero, messages larger than msize are rejected without
// being read into memory, which bounds how much a misbehaving peer
// can force a reader to allocate.
func ReadMessage(r io.Reader, msize uint32) ([]byte, error) {
	var szb [4]byte
	if _, err := io.ReadFull(r, szb[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(szb[:])
	if size < minMsgSize {
		return nil, ErrTruncated
	}
	if msize != 0 && size > msize {
		return nil, ErrMsgTooLarge
	}
	buf := make([]byte, size)
	copy(buf, szb[:])
	if _, err := io.ReadFull(r, buf[4:]); err != nil {
		return nil, err
	}
	return buf, nil
}

// WriteMessage writes raw, as produced by Marshal, to w.
func WriteMessage(w io.Writer, raw []byte) error {
	_, err := w.Write(raw)
	return err
}

// Marshal encodes tag and m into a complete wire message, including
// the leading 4-byte size prefix.
func Marshal(tag Tag, m Message) []byte {
	var buf bytes.Buffer
	buf.Write([]byte{0, 0, 0, 0})
	buf.WriteByte(byte(m.MsgType()))
	var tb [2]byte
	binary.LittleEndian.PutUint16(tb[:], uint16(tag))
	buf.Write(tb[:])
	e := encoder{buf: &buf}
	m.marshalBody(&e)
	out := buf.Bytes()
	binary.LittleEndian.PutUint32(out[0:4], uint32(len(out)))
	return out
}

// Unmarshal parses a complete wire message, as produced by
// ReadMessage, into its tag and typed Message.
func Unmarshal(raw []byte) (Tag, Message, error) {
	if len(raw) < minMsgSize {
		return 0, nil, ErrTruncated
	}
	size := binary.LittleEndian.Uint32(raw[0:4])
	if int(size) != len(raw) {
		return 0, nil, ErrTruncated
	}
	t := FcallType(raw[4])
	tag := Tag(binary.LittleEndian.Uint16(raw[5:7]))
	m, err := newMessage(t)
	if err != nil {
		return tag, nil, err
	}
	d := decoder{buf: raw[7:]}
	m.unmarshalBody(&d)
	if err := d.done(); err != nil {
		return tag, nil, fmt.Errorf("p9: decoding %v: %w", t, err)
	}
	return tag, m, nil
}
