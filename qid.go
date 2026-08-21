package p9

import "fmt"

// QidType identifies the type of a file. It occupies the type byte
// of a Qid and mirrors the high bits of the file's permission Mode.
type QidType uint8

const (
	QTDIR    QidType = 0x80
	QTAPPEND QidType = 0x40
	QTEXCL   QidType = 0x20
	QTMOUNT  QidType = 0x10
	QTAUTH   QidType = 0x08
	QTTMP    QidType = 0x04
	QTFILE   QidType = 0x00
)

// Qid is the server's identification for a file: its type, a
// version number that changes whenever the file's contents change,
// and a path that is unique among all files served on a connection.
type Qid struct {
	Type    QidType
	Version uint32
	Path    uint64
}

// IsDir reports whether the Qid identifies a directory.
func (q Qid) IsDir() bool { return q.Type&QTDIR != 0 }

func (q Qid) String() string {
	return fmt.Sprintf("(%016x %d %s)", q.Path, q.Version, q.Type)
}

func (t QidType) String() string {
	var s string
	if t&QTDIR != 0 {
		s += "d"
	}
	if t&QTAPPEND != 0 {
		s += "a"
	}
	if t&QTEXCL != 0 {
		s += "l"
	}
	if t&QTMOUNT != 0 {
		s += "m"
	}
	if t&QTAUTH != 0 {
		s += "A"
	}
	if t&QTTMP != 0 {
		s += "t"
	}
	return s
}
