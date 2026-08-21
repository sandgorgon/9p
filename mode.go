package p9

// Mode holds either an open mode (Topen.Mode, Tcreate.Mode — one
// byte on the wire) or a permission mode (Stat.Mode, Tcreate.Perm —
// four bytes on the wire). The two share no bits in practice, so one
// type serves both roles; callers marshal it as one or four bytes
// depending on which field it appears in.
type Mode uint32

// Open-mode bits: the low two bits select the access mode, the rest
// are flags.
const (
	OREAD   Mode = 0
	OWRITE  Mode = 1
	ORDWR   Mode = 2
	OEXEC   Mode = 3
	OTRUNC  Mode = 0x10
	ORCLOSE Mode = 0x40
)

// Permission-mode bits: the top bits mark directories and other
// special files, the low 9 bits are rwxrwxrwx permissions.
const (
	DMDIR    Mode = 0x80000000
	DMAPPEND Mode = 0x40000000
	DMEXCL   Mode = 0x20000000
	DMTMP    Mode = 0x04000000
	DMPerm   Mode = 0777
)

// IsDir reports whether the permission Mode marks a directory.
func (m Mode) IsDir() bool { return m&DMDIR != 0 }
