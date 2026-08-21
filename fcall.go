package p9

// Tag identifies an in-flight request so a reply can be matched to
// it. NoTag marks a message with no associated tag.
type Tag uint16

// Fid is a client-chosen handle identifying a file on the server,
// analogous to a file descriptor.
type Fid uint32

// FcallType is the one-byte message type that begins every 9P2000
// message on the wire, immediately after the 4-byte size.
type FcallType uint8

const (
	Tversion FcallType = 100
	Rversion FcallType = 101
	Tauth    FcallType = 102
	Rauth    FcallType = 103
	Tattach  FcallType = 104
	Rattach  FcallType = 105
	// 106 (Terror) is illegal and never sent on the wire.
	Rerror  FcallType = 107
	Tflush  FcallType = 108
	Rflush  FcallType = 109
	Twalk   FcallType = 110
	Rwalk   FcallType = 111
	Topen   FcallType = 112
	Ropen   FcallType = 113
	Tcreate FcallType = 114
	Rcreate FcallType = 115
	Tread   FcallType = 116
	Rread   FcallType = 117
	Twrite  FcallType = 118
	Rwrite  FcallType = 119
	Tclunk  FcallType = 120
	Rclunk  FcallType = 121
	Tremove FcallType = 122
	Rremove FcallType = 123
	Tstat   FcallType = 124
	Rstat   FcallType = 125
	Twstat  FcallType = 126
	Rwstat  FcallType = 127
)

func (t FcallType) String() string {
	switch t {
	case Tversion:
		return "Tversion"
	case Rversion:
		return "Rversion"
	case Tauth:
		return "Tauth"
	case Rauth:
		return "Rauth"
	case Tattach:
		return "Tattach"
	case Rattach:
		return "Rattach"
	case Rerror:
		return "Rerror"
	case Tflush:
		return "Tflush"
	case Rflush:
		return "Rflush"
	case Twalk:
		return "Twalk"
	case Rwalk:
		return "Rwalk"
	case Topen:
		return "Topen"
	case Ropen:
		return "Ropen"
	case Tcreate:
		return "Tcreate"
	case Rcreate:
		return "Rcreate"
	case Tread:
		return "Tread"
	case Rread:
		return "Rread"
	case Twrite:
		return "Twrite"
	case Rwrite:
		return "Rwrite"
	case Tclunk:
		return "Tclunk"
	case Rclunk:
		return "Rclunk"
	case Tremove:
		return "Tremove"
	case Rremove:
		return "Rremove"
	case Tstat:
		return "Tstat"
	case Rstat:
		return "Rstat"
	case Twstat:
		return "Twstat"
	case Rwstat:
		return "Rwstat"
	default:
		return "Tunknown"
	}
}

// Message is implemented by every 9P2000 message body (everything
// after the tag). Marshal and Unmarshal combine a Message with a Tag
// to produce or parse a complete wire message.
type Message interface {
	MsgType() FcallType
	marshalBody(e *encoder)
	unmarshalBody(d *decoder)
}

func newMessage(t FcallType) (Message, error) {
	switch t {
	case Tversion:
		return &TversionFcall{}, nil
	case Rversion:
		return &RversionFcall{}, nil
	case Tauth:
		return &TauthFcall{}, nil
	case Rauth:
		return &RauthFcall{}, nil
	case Tattach:
		return &TattachFcall{}, nil
	case Rattach:
		return &RattachFcall{}, nil
	case Rerror:
		return &RerrorFcall{}, nil
	case Tflush:
		return &TflushFcall{}, nil
	case Rflush:
		return &RflushFcall{}, nil
	case Twalk:
		return &TwalkFcall{}, nil
	case Rwalk:
		return &RwalkFcall{}, nil
	case Topen:
		return &TopenFcall{}, nil
	case Ropen:
		return &RopenFcall{}, nil
	case Tcreate:
		return &TcreateFcall{}, nil
	case Rcreate:
		return &RcreateFcall{}, nil
	case Tread:
		return &TreadFcall{}, nil
	case Rread:
		return &RreadFcall{}, nil
	case Twrite:
		return &TwriteFcall{}, nil
	case Rwrite:
		return &RwriteFcall{}, nil
	case Tclunk:
		return &TclunkFcall{}, nil
	case Rclunk:
		return &RclunkFcall{}, nil
	case Tremove:
		return &TremoveFcall{}, nil
	case Rremove:
		return &RremoveFcall{}, nil
	case Tstat:
		return &TstatFcall{}, nil
	case Rstat:
		return &RstatFcall{}, nil
	case Twstat:
		return &TwstatFcall{}, nil
	case Rwstat:
		return &RwstatFcall{}, nil
	default:
		return nil, ErrUnknownType
	}
}

type TversionFcall struct {
	Msize   uint32
	Version string
}

func (m *TversionFcall) MsgType() FcallType { return Tversion }
func (m *TversionFcall) marshalBody(e *encoder) {
	e.uint32(m.Msize)
	e.string(m.Version)
}
func (m *TversionFcall) unmarshalBody(d *decoder) {
	m.Msize = d.uint32()
	m.Version = d.string()
}

type RversionFcall struct {
	Msize   uint32
	Version string
}

func (m *RversionFcall) MsgType() FcallType { return Rversion }
func (m *RversionFcall) marshalBody(e *encoder) {
	e.uint32(m.Msize)
	e.string(m.Version)
}
func (m *RversionFcall) unmarshalBody(d *decoder) {
	m.Msize = d.uint32()
	m.Version = d.string()
}

type TauthFcall struct {
	Afid  Fid
	Uname string
	Aname string
}

func (m *TauthFcall) MsgType() FcallType { return Tauth }
func (m *TauthFcall) marshalBody(e *encoder) {
	e.uint32(uint32(m.Afid))
	e.string(m.Uname)
	e.string(m.Aname)
}
func (m *TauthFcall) unmarshalBody(d *decoder) {
	m.Afid = Fid(d.uint32())
	m.Uname = d.string()
	m.Aname = d.string()
}

type RauthFcall struct {
	Aqid Qid
}

func (m *RauthFcall) MsgType() FcallType       { return Rauth }
func (m *RauthFcall) marshalBody(e *encoder)   { e.qid(m.Aqid) }
func (m *RauthFcall) unmarshalBody(d *decoder) { m.Aqid = d.qid() }

type TattachFcall struct {
	Fid   Fid
	Afid  Fid
	Uname string
	Aname string
}

func (m *TattachFcall) MsgType() FcallType { return Tattach }
func (m *TattachFcall) marshalBody(e *encoder) {
	e.uint32(uint32(m.Fid))
	e.uint32(uint32(m.Afid))
	e.string(m.Uname)
	e.string(m.Aname)
}
func (m *TattachFcall) unmarshalBody(d *decoder) {
	m.Fid = Fid(d.uint32())
	m.Afid = Fid(d.uint32())
	m.Uname = d.string()
	m.Aname = d.string()
}

type RattachFcall struct {
	Qid Qid
}

func (m *RattachFcall) MsgType() FcallType       { return Rattach }
func (m *RattachFcall) marshalBody(e *encoder)   { e.qid(m.Qid) }
func (m *RattachFcall) unmarshalBody(d *decoder) { m.Qid = d.qid() }

type TflushFcall struct {
	Oldtag Tag
}

func (m *TflushFcall) MsgType() FcallType     { return Tflush }
func (m *TflushFcall) marshalBody(e *encoder) { e.uint16(uint16(m.Oldtag)) }
func (m *TflushFcall) unmarshalBody(d *decoder) {
	m.Oldtag = Tag(d.uint16())
}

type RflushFcall struct{}

func (m *RflushFcall) MsgType() FcallType       { return Rflush }
func (m *RflushFcall) marshalBody(e *encoder)   {}
func (m *RflushFcall) unmarshalBody(d *decoder) {}

type TwalkFcall struct {
	Fid    Fid
	Newfid Fid
	Wname  []string
}

func (m *TwalkFcall) MsgType() FcallType { return Twalk }
func (m *TwalkFcall) marshalBody(e *encoder) {
	e.uint32(uint32(m.Fid))
	e.uint32(uint32(m.Newfid))
	e.uint16(uint16(len(m.Wname)))
	for _, n := range m.Wname {
		e.string(n)
	}
}
func (m *TwalkFcall) unmarshalBody(d *decoder) {
	m.Fid = Fid(d.uint32())
	m.Newfid = Fid(d.uint32())
	n := d.uint16()
	if n > MaxWElem {
		if d.err == nil {
			d.err = ErrTooManyWElem
		}
		return
	}
	m.Wname = make([]string, n)
	for i := range m.Wname {
		m.Wname[i] = d.string()
	}
}

type RwalkFcall struct {
	Wqid []Qid
}

func (m *RwalkFcall) MsgType() FcallType { return Rwalk }
func (m *RwalkFcall) marshalBody(e *encoder) {
	e.uint16(uint16(len(m.Wqid)))
	for _, q := range m.Wqid {
		e.qid(q)
	}
}
func (m *RwalkFcall) unmarshalBody(d *decoder) {
	n := d.uint16()
	if n > MaxWElem {
		if d.err == nil {
			d.err = ErrTooManyWElem
		}
		return
	}
	m.Wqid = make([]Qid, n)
	for i := range m.Wqid {
		m.Wqid[i] = d.qid()
	}
}

type TopenFcall struct {
	Fid  Fid
	Mode Mode
}

func (m *TopenFcall) MsgType() FcallType { return Topen }
func (m *TopenFcall) marshalBody(e *encoder) {
	e.uint32(uint32(m.Fid))
	e.uint8(uint8(m.Mode))
}
func (m *TopenFcall) unmarshalBody(d *decoder) {
	m.Fid = Fid(d.uint32())
	m.Mode = Mode(d.uint8())
}

type RopenFcall struct {
	Qid    Qid
	Iounit uint32
}

func (m *RopenFcall) MsgType() FcallType { return Ropen }
func (m *RopenFcall) marshalBody(e *encoder) {
	e.qid(m.Qid)
	e.uint32(m.Iounit)
}
func (m *RopenFcall) unmarshalBody(d *decoder) {
	m.Qid = d.qid()
	m.Iounit = d.uint32()
}

type TcreateFcall struct {
	Fid  Fid
	Name string
	Perm Mode
	Mode Mode
}

func (m *TcreateFcall) MsgType() FcallType { return Tcreate }
func (m *TcreateFcall) marshalBody(e *encoder) {
	e.uint32(uint32(m.Fid))
	e.string(m.Name)
	e.uint32(uint32(m.Perm))
	e.uint8(uint8(m.Mode))
}
func (m *TcreateFcall) unmarshalBody(d *decoder) {
	m.Fid = Fid(d.uint32())
	m.Name = d.string()
	m.Perm = Mode(d.uint32())
	m.Mode = Mode(d.uint8())
}

type RcreateFcall struct {
	Qid    Qid
	Iounit uint32
}

func (m *RcreateFcall) MsgType() FcallType { return Rcreate }
func (m *RcreateFcall) marshalBody(e *encoder) {
	e.qid(m.Qid)
	e.uint32(m.Iounit)
}
func (m *RcreateFcall) unmarshalBody(d *decoder) {
	m.Qid = d.qid()
	m.Iounit = d.uint32()
}

type TreadFcall struct {
	Fid    Fid
	Offset uint64
	Count  uint32
}

func (m *TreadFcall) MsgType() FcallType { return Tread }
func (m *TreadFcall) marshalBody(e *encoder) {
	e.uint32(uint32(m.Fid))
	e.uint64(m.Offset)
	e.uint32(m.Count)
}
func (m *TreadFcall) unmarshalBody(d *decoder) {
	m.Fid = Fid(d.uint32())
	m.Offset = d.uint64()
	m.Count = d.uint32()
}

type RreadFcall struct {
	Data []byte
}

func (m *RreadFcall) MsgType() FcallType { return Rread }
func (m *RreadFcall) marshalBody(e *encoder) {
	e.uint32(uint32(len(m.Data)))
	e.bytes(m.Data)
}
func (m *RreadFcall) unmarshalBody(d *decoder) {
	n := d.uint32()
	m.Data = d.bytes(n)
}

type TwriteFcall struct {
	Fid    Fid
	Offset uint64
	Data   []byte
}

func (m *TwriteFcall) MsgType() FcallType { return Twrite }
func (m *TwriteFcall) marshalBody(e *encoder) {
	e.uint32(uint32(m.Fid))
	e.uint64(m.Offset)
	e.uint32(uint32(len(m.Data)))
	e.bytes(m.Data)
}
func (m *TwriteFcall) unmarshalBody(d *decoder) {
	m.Fid = Fid(d.uint32())
	m.Offset = d.uint64()
	n := d.uint32()
	m.Data = d.bytes(n)
}

type RwriteFcall struct {
	Count uint32
}

func (m *RwriteFcall) MsgType() FcallType       { return Rwrite }
func (m *RwriteFcall) marshalBody(e *encoder)   { e.uint32(m.Count) }
func (m *RwriteFcall) unmarshalBody(d *decoder) { m.Count = d.uint32() }

type TclunkFcall struct {
	Fid Fid
}

func (m *TclunkFcall) MsgType() FcallType       { return Tclunk }
func (m *TclunkFcall) marshalBody(e *encoder)   { e.uint32(uint32(m.Fid)) }
func (m *TclunkFcall) unmarshalBody(d *decoder) { m.Fid = Fid(d.uint32()) }

type RclunkFcall struct{}

func (m *RclunkFcall) MsgType() FcallType       { return Rclunk }
func (m *RclunkFcall) marshalBody(e *encoder)   {}
func (m *RclunkFcall) unmarshalBody(d *decoder) {}

type TremoveFcall struct {
	Fid Fid
}

func (m *TremoveFcall) MsgType() FcallType       { return Tremove }
func (m *TremoveFcall) marshalBody(e *encoder)   { e.uint32(uint32(m.Fid)) }
func (m *TremoveFcall) unmarshalBody(d *decoder) { m.Fid = Fid(d.uint32()) }

type RremoveFcall struct{}

func (m *RremoveFcall) MsgType() FcallType       { return Rremove }
func (m *RremoveFcall) marshalBody(e *encoder)   {}
func (m *RremoveFcall) unmarshalBody(d *decoder) {}

type TstatFcall struct {
	Fid Fid
}

func (m *TstatFcall) MsgType() FcallType       { return Tstat }
func (m *TstatFcall) marshalBody(e *encoder)   { e.uint32(uint32(m.Fid)) }
func (m *TstatFcall) unmarshalBody(d *decoder) { m.Fid = Fid(d.uint32()) }

type RstatFcall struct {
	Stat Stat
}

func (m *RstatFcall) MsgType() FcallType       { return Rstat }
func (m *RstatFcall) marshalBody(e *encoder)   { e.stat(m.Stat) }
func (m *RstatFcall) unmarshalBody(d *decoder) { m.Stat = d.stat() }

type TwstatFcall struct {
	Fid  Fid
	Stat Stat
}

func (m *TwstatFcall) MsgType() FcallType { return Twstat }
func (m *TwstatFcall) marshalBody(e *encoder) {
	e.uint32(uint32(m.Fid))
	e.stat(m.Stat)
}
func (m *TwstatFcall) unmarshalBody(d *decoder) {
	m.Fid = Fid(d.uint32())
	m.Stat = d.stat()
}

type RwstatFcall struct{}

func (m *RwstatFcall) MsgType() FcallType       { return Rwstat }
func (m *RwstatFcall) marshalBody(e *encoder)   {}
func (m *RwstatFcall) unmarshalBody(d *decoder) {}
