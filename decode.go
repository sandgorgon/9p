package p9

import "encoding/binary"

// decoder reads little-endian 9P2000 values from buf. Once a read
// runs past the end of buf, err is set and every subsequent read
// becomes a no-op returning the zero value, so an unmarshalBody
// implementation can read every field unconditionally and check err
// once at the end via done. take is the single bounds-checked
// primitive everything else is built on, which is what keeps
// Unmarshal panic-free on arbitrary/truncated input from the wire.
type decoder struct {
	buf []byte
	err error
}

func (d *decoder) take(n int) []byte {
	if d.err != nil {
		return nil
	}
	if n < 0 || n > len(d.buf) {
		d.err = ErrTruncated
		return nil
	}
	p := d.buf[:n]
	d.buf = d.buf[n:]
	return p
}

func (d *decoder) uint8() uint8 {
	p := d.take(1)
	if p == nil {
		return 0
	}
	return p[0]
}

func (d *decoder) uint16() uint16 {
	p := d.take(2)
	if p == nil {
		return 0
	}
	return binary.LittleEndian.Uint16(p)
}

func (d *decoder) uint32() uint32 {
	p := d.take(4)
	if p == nil {
		return 0
	}
	return binary.LittleEndian.Uint32(p)
}

func (d *decoder) uint64() uint64 {
	p := d.take(8)
	if p == nil {
		return 0
	}
	return binary.LittleEndian.Uint64(p)
}

func (d *decoder) bytes(n uint32) []byte {
	p := d.take(int(n))
	if p == nil {
		return nil
	}
	// Copy out: p aliases the caller-owned buffer passed to
	// Unmarshal, which the caller may reuse after this call returns.
	cp := make([]byte, len(p))
	copy(cp, p)
	return cp
}

func (d *decoder) string() string {
	n := d.uint16()
	p := d.take(int(n))
	if p == nil {
		return ""
	}
	return string(p)
}

func (d *decoder) qid() Qid {
	var q Qid
	q.Type = QidType(d.uint8())
	q.Version = d.uint32()
	q.Path = d.uint64()
	return q
}

// done reports whether the entire buffer was consumed with no
// decoding error; a non-nil result rejects both truncated messages
// and messages with unexpected trailing bytes.
func (d *decoder) done() error {
	if d.err != nil {
		return d.err
	}
	if len(d.buf) != 0 {
		return ErrTrailingBytes
	}
	return nil
}
