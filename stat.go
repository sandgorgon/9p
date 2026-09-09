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

	// The following are 9P2000.u only: present on the wire, in this
	// order, only when the connection negotiated VersionU. Extension
	// holds a symlink's target when Mode has DMSYMLINK set. Nuid,
	// Ngid, and Nmuid are the numeric counterparts of Uid/Gid/Muid.
	Extension string
	Nuid      uint32
	Ngid      uint32
	Nmuid     uint32
}

// Marshal encodes s as it appears on plain 9P2000: a 2-byte size
// prefix (the length of everything that follows) followed by the
// fixed and string fields. The 9P2000.u fields are never included;
// use MarshalVersion(true) on a connection that negotiated VersionU.
func (s Stat) Marshal() []byte {
	return s.MarshalVersion(false)
}

// MarshalVersion is Marshal, but includes the 9P2000.u fields when
// unix is true.
func (s Stat) MarshalVersion(unix bool) []byte {
	var buf bytes.Buffer
	e := encoder{buf: &buf, unix: unix}
	e.stat(s)
	return buf.Bytes()
}

// UnmarshalStat decodes a single plain-9P2000 Stat blob, including
// its leading size prefix. It returns ErrTrailingBytes if b holds
// more than one Stat's worth of data. Use UnmarshalStatVersion(b,
// true) to decode a 9P2000.u-flavored blob.
func UnmarshalStat(b []byte) (Stat, error) {
	return UnmarshalStatVersion(b, false)
}

// UnmarshalStatVersion is UnmarshalStat, but expects the 9P2000.u
// fields to be present when unix is true.
func UnmarshalStatVersion(b []byte, unix bool) (Stat, error) {
	d := decoder{buf: b, unix: unix}
	s := d.stat()
	if err := d.done(); err != nil {
		return Stat{}, err
	}
	return s, nil
}

func (e *encoder) stat(s Stat) {
	body := s.marshalBody(e.unix)
	e.uint16(uint16(len(body)))
	e.bytes(body)
}

func (s Stat) marshalBody(unix bool) []byte {
	var buf bytes.Buffer
	e := encoder{buf: &buf, unix: unix}
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
	if unix {
		e.string(s.Extension)
		e.uint32(s.Nuid)
		e.uint32(s.Ngid)
		e.uint32(s.Nmuid)
	}
	return buf.Bytes()
}

func (d *decoder) stat() Stat {
	n := d.uint16()
	body := d.take(int(n))
	sd := decoder{buf: body, unix: d.unix}
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
	if sd.unix {
		s.Extension = sd.string()
		s.Nuid = sd.uint32()
		s.Ngid = sd.uint32()
		s.Nmuid = sd.uint32()
	}
	if err := sd.done(); err != nil && d.err == nil {
		d.err = err
	}
	return s
}
