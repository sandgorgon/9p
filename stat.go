package p9

import "bytes"

// Stat describes a file's metadata, as used in Tstat/Rstat, Twstat,
// and directory contents (a directory Read returns a concatenation
// of Stat blobs, one per entry).
type Stat struct {
	Type   uint16 // kernel use; servers should leave this zero
	Dev    uint32 // kernel use; servers should leave this zero
	Qid    Qid
	Mode   Mode
	Atime  uint32
	Mtime  uint32
	Length uint64
	Name   string
	Uid    string
	Gid    string
	Muid   string
}

// Marshal encodes s as it appears on the wire: a 2-byte size prefix
// (the length of everything that follows) followed by the fixed and
// string fields.
func (s Stat) Marshal() []byte {
	var buf bytes.Buffer
	e := encoder{buf: &buf}
	e.stat(s)
	return buf.Bytes()
}

// UnmarshalStat decodes a single Stat blob, including its leading
// size prefix. It returns ErrTrailingBytes if b holds more than one
// Stat's worth of data.
func UnmarshalStat(b []byte) (Stat, error) {
	d := decoder{buf: b}
	s := d.stat()
	if err := d.done(); err != nil {
		return Stat{}, err
	}
	return s, nil
}

func (e *encoder) stat(s Stat) {
	body := s.marshalBody()
	e.uint16(uint16(len(body)))
	e.bytes(body)
}

func (s Stat) marshalBody() []byte {
	var buf bytes.Buffer
	e := encoder{buf: &buf}
	e.uint16(s.Type)
	e.uint32(s.Dev)
	e.qid(s.Qid)
	e.uint32(uint32(s.Mode))
	e.uint32(s.Atime)
	e.uint32(s.Mtime)
	e.uint64(s.Length)
	e.string(s.Name)
	e.string(s.Uid)
	e.string(s.Gid)
	e.string(s.Muid)
	return buf.Bytes()
}

func (d *decoder) stat() Stat {
	n := d.uint16()
	body := d.take(int(n))
	sd := decoder{buf: body}
	var s Stat
	s.Type = sd.uint16()
	s.Dev = sd.uint32()
	s.Qid = sd.qid()
	s.Mode = Mode(sd.uint32())
	s.Atime = sd.uint32()
	s.Mtime = sd.uint32()
	s.Length = sd.uint64()
	s.Name = sd.string()
	s.Uid = sd.string()
	s.Gid = sd.string()
	s.Muid = sd.string()
	if err := sd.done(); err != nil && d.err == nil {
		d.err = err
	}
	return s
}
