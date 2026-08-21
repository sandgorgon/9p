package p9

import (
	"bytes"
	"encoding/binary"
)

// encoder appends the little-endian wire encoding of 9P2000 values
// to an underlying buffer.
type encoder struct {
	buf *bytes.Buffer
}

func (e *encoder) uint8(v uint8) { e.buf.WriteByte(v) }

func (e *encoder) uint16(v uint16) {
	var b [2]byte
	binary.LittleEndian.PutUint16(b[:], v)
	e.buf.Write(b[:])
}

func (e *encoder) uint32(v uint32) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	e.buf.Write(b[:])
}

func (e *encoder) uint64(v uint64) {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], v)
	e.buf.Write(b[:])
}

func (e *encoder) bytes(p []byte) { e.buf.Write(p) }

// string writes a 9P string: a 2-byte length prefix followed by the
// UTF-8 bytes. Strings longer than 65535 bytes cannot be represented
// on the wire and are truncated to fit, since the prefix must match
// the bytes actually written; callers dealing with attacker- or
// filesystem-controlled strings should not rely on names round
// tripping past that length.
func (e *encoder) string(s string) {
	if len(s) > 0xFFFF {
		s = s[:0xFFFF]
	}
	e.uint16(uint16(len(s)))
	e.buf.WriteString(s)
}

func (e *encoder) qid(q Qid) {
	e.uint8(uint8(q.Type))
	e.uint32(q.Version)
	e.uint64(q.Path)
}
