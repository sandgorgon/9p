package p9

import "testing"

// FuzzUnmarshal exercises the one function in this package that
// parses untrusted network input. It must never panic, regardless of
// how malformed or adversarial raw is — a rejected message should
// come back as an error, not a crash.
func FuzzUnmarshal(f *testing.F) {
	seeds := []Message{
		&TversionFcall{Msize: 8192, Version: Version},
		&TauthFcall{Afid: 5, Uname: "glenda", Aname: ""},
		&TattachFcall{Fid: 0, Afid: NoFid, Uname: "glenda", Aname: "/"},
		&RerrorFcall{Ename: "file not found"},
		&TflushFcall{Oldtag: 1},
		&TwalkFcall{Fid: 0, Newfid: 1, Wname: []string{"usr", "glenda"}},
		&RwalkFcall{Wqid: repeatQid(MaxWElem)},
		&TopenFcall{Fid: 1, Mode: ORDWR | OTRUNC},
		&TcreateFcall{Fid: 1, Name: "file", Perm: DMDIR | 0755, Mode: OREAD},
		&TreadFcall{Fid: 1, Offset: 4096, Count: 8168},
		&RreadFcall{Data: []byte("hello, plan 9")},
		&TwriteFcall{Fid: 1, Offset: 10, Data: []byte("some data")},
		&TclunkFcall{Fid: 1},
		&RstatFcall{Stat: sampleStat("afile", QTFILE, 0644)},
		&TwstatFcall{Fid: 1, Stat: sampleStat("afile", QTFILE, 0600)},
	}
	for i, m := range seeds {
		f.Add(Marshal(Tag(i), m))
	}
	// A handful of adversarial byte patterns not shaped like any
	// valid message, to seed the corpus beyond well-formed mutants.
	f.Add([]byte{})
	f.Add([]byte{0, 0, 0, 0})
	f.Add([]byte{0xFF, 0xFF, 0xFF, 0xFF, 100, 0, 0})
	f.Add([]byte{7, 0, 0, 0, 110, 0, 0})

	f.Fuzz(func(t *testing.T, raw []byte) {
		tag, msg, err := Unmarshal(raw)
		if err != nil {
			return
		}
		// A successful decode must re-encode to something that
		// decodes back to an equal message, tag included.
		raw2 := Marshal(tag, msg)
		tag2, msg2, err2 := Unmarshal(raw2)
		if err2 != nil {
			t.Fatalf("re-marshal of accepted message failed to decode: %v (orig raw=%x)", err2, raw)
		}
		if tag2 != tag || msg2.MsgType() != msg.MsgType() {
			t.Fatalf("re-marshal round trip mismatch: got (%v,%v) want (%v,%v)", tag2, msg2.MsgType(), tag, msg.MsgType())
		}
	})
}
