package p9

import (
	"reflect"
	"testing"
)

func TestMessageRoundTrip(t *testing.T) {
	longName := make([]byte, 300)
	for i := range longName {
		longName[i] = 'a' + byte(i%26)
	}
	long := string(longName)

	cases := []struct {
		name string
		tag  Tag
		msg  Message
	}{
		{"Tversion", NoTag, &TversionFcall{Msize: 8192, Version: Version}},
		{"Rversion", NoTag, &RversionFcall{Msize: 8192, Version: Version}},
		{"Rversion-unknown", NoTag, &RversionFcall{Msize: 8192, Version: VersionUnknown}},
		{"Tauth", 1, &TauthFcall{Afid: 5, Uname: "glenda", Aname: ""}},
		{"Rauth", 1, &RauthFcall{Aqid: Qid{Type: QTAUTH, Version: 0, Path: 1}}},
		{"Tattach", 1, &TattachFcall{Fid: 0, Afid: NoFid, Uname: "glenda", Aname: "/"}},
		{"Rattach", 1, &RattachFcall{Qid: Qid{Type: QTDIR, Version: 1, Path: 1}}},
		{"Rerror", 1, &RerrorFcall{Ename: "file not found"}},
		{"Rerror-empty", 1, &RerrorFcall{Ename: ""}},
		{"Tflush", 2, &TflushFcall{Oldtag: 1}},
		{"Rflush", 2, &RflushFcall{}},
		{"Twalk-empty", 3, &TwalkFcall{Fid: 0, Newfid: 1, Wname: []string{}}},
		{"Twalk-one", 3, &TwalkFcall{Fid: 0, Newfid: 1, Wname: []string{"usr"}}},
		{"Twalk-max", 3, &TwalkFcall{Fid: 0, Newfid: 1, Wname: repeatStrings("x", MaxWElem)}},
		{"Rwalk-empty", 3, &RwalkFcall{Wqid: []Qid{}}},
		{"Rwalk-max", 3, &RwalkFcall{Wqid: repeatQid(MaxWElem)}},
		{"Topen", 4, &TopenFcall{Fid: 1, Mode: OREAD}},
		{"Topen-truncrclose", 4, &TopenFcall{Fid: 1, Mode: OWRITE | OTRUNC | ORCLOSE}},
		{"Ropen", 4, &RopenFcall{Qid: Qid{Path: 1}, Iounit: 8168}},
		{"Tcreate", 5, &TcreateFcall{Fid: 1, Name: "file", Perm: 0644, Mode: ORDWR}},
		{"Tcreate-dir", 5, &TcreateFcall{Fid: 1, Name: "dir", Perm: DMDIR | 0755, Mode: OREAD}},
		{"Rcreate", 5, &RcreateFcall{Qid: Qid{Path: 2}, Iounit: 0}},
		{"Tread-zero", 6, &TreadFcall{Fid: 1, Offset: 0, Count: 0}},
		{"Tread", 6, &TreadFcall{Fid: 1, Offset: 4096, Count: 8168}},
		{"Rread-empty", 6, &RreadFcall{Data: []byte{}}},
		{"Rread", 6, &RreadFcall{Data: []byte("hello, plan 9")}},
		{"Twrite-empty", 7, &TwriteFcall{Fid: 1, Offset: 0, Data: []byte{}}},
		{"Twrite", 7, &TwriteFcall{Fid: 1, Offset: 10, Data: []byte("some data")}},
		{"Rwrite", 7, &RwriteFcall{Count: 9}},
		{"Tclunk", 8, &TclunkFcall{Fid: 1}},
		{"Rclunk", 8, &RclunkFcall{}},
		{"Tremove", 9, &TremoveFcall{Fid: 1}},
		{"Rremove", 9, &RremoveFcall{}},
		{"Tstat", 10, &TstatFcall{Fid: 1}},
		{"Rstat", 10, &RstatFcall{Stat: sampleStat("afile", QTFILE, 0644)}},
		{"Rstat-longname", 10, &RstatFcall{Stat: sampleStat(long, QTFILE, 0644)}},
		{"Twstat", 11, &TwstatFcall{Fid: 1, Stat: sampleStat("afile", QTFILE, 0600)}},
		{"Rwstat", 11, &RwstatFcall{}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := Marshal(c.tag, c.msg)
			gotTag, gotMsg, err := Unmarshal(raw)
			if err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if gotTag != c.tag {
				t.Errorf("tag = %v, want %v", gotTag, c.tag)
			}
			if gotMsg.MsgType() != c.msg.MsgType() {
				t.Errorf("type = %v, want %v", gotMsg.MsgType(), c.msg.MsgType())
			}
			if !reflect.DeepEqual(gotMsg, c.msg) {
				t.Errorf("round trip mismatch:\n got  %#v\n want %#v", gotMsg, c.msg)
			}
		})
	}
}

func TestStatRoundTrip(t *testing.T) {
	s := sampleStat("afile", QTFILE, 0644)
	raw := s.Marshal()
	got, err := UnmarshalStat(raw)
	if err != nil {
		t.Fatalf("UnmarshalStat: %v", err)
	}
	if !reflect.DeepEqual(got, s) {
		t.Errorf("round trip mismatch:\n got  %#v\n want %#v", got, s)
	}
}

func TestUnmarshalTruncated(t *testing.T) {
	raw := Marshal(1, &TattachFcall{Fid: 0, Afid: NoFid, Uname: "glenda", Aname: "/"})
	for n := range len(raw) {
		if _, _, err := Unmarshal(raw[:n]); err == nil {
			t.Errorf("Unmarshal(raw[:%d]) succeeded, want error", n)
		}
	}
}

func TestUnmarshalTrailingBytes(t *testing.T) {
	raw := Marshal(1, &TclunkFcall{Fid: 1})
	raw = append(raw, 0xFF)
	// Fix up the size prefix so ReadMessage-style framing would
	// still treat this as one message with a bad body length, to
	// exercise the trailing-bytes-inside-body case rather than a
	//framing mismatch.
	raw[0] = byte(len(raw))
	raw[1] = byte(len(raw) >> 8)
	if _, _, err := Unmarshal(raw); err == nil {
		t.Error("Unmarshal with trailing byte succeeded, want error")
	}
}

func TestUnmarshalUnknownType(t *testing.T) {
	raw := Marshal(1, &TclunkFcall{Fid: 1})
	raw[4] = 106 // Terror: illegal, never a valid type byte
	if _, _, err := Unmarshal(raw); err == nil {
		t.Error("Unmarshal with unknown type succeeded, want error")
	}
}

func TestTwalkTooManyElements(t *testing.T) {
	raw := Marshal(1, &TwalkFcall{Fid: 0, Newfid: 1, Wname: repeatStrings("x", MaxWElem+1)})
	if _, _, err := Unmarshal(raw); err == nil {
		t.Error("Unmarshal with > MaxWElem walk names succeeded, want error")
	}
}

func repeatStrings(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func repeatQid(n int) []Qid {
	out := make([]Qid, n)
	for i := range out {
		out[i] = Qid{Type: QTFILE, Version: uint32(i), Path: uint64(i)}
	}
	return out
}

func sampleStat(name string, t QidType, perm Mode) Stat {
	return Stat{
		Qid:    Qid{Type: t, Version: 1, Path: 42},
		Mode:   perm,
		Atime:  1000,
		Mtime:  2000,
		Length: 123,
		Name:   name,
		Uid:    "glenda",
		Gid:    "glenda",
		Muid:   "glenda",
	}
}
