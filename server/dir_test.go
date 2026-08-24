package server_test

import (
	"encoding/binary"
	"io"
	"testing"

	p9 "github.com/sandgorgon/9p"
	"github.com/sandgorgon/9p/server"
)

func testEntries(names ...string) []p9.Stat {
	entries := make([]p9.Stat, len(names))
	for i, name := range names {
		entries[i] = p9.Stat{Name: name, Uid: "glenda", Gid: "glenda", Muid: "glenda"}
	}
	return entries
}

func TestMarshalDirEmpty(t *testing.T) {
	n, err := server.MarshalDir(nil, 0, make([]byte, 64))
	if n != 0 || err != io.EOF {
		t.Fatalf("MarshalDir(nil, 0, buf) = %d, %v, want 0, io.EOF", n, err)
	}
}

func TestMarshalDirWholeInOneCall(t *testing.T) {
	entries := testEntries("a", "b", "c")
	var want int
	for _, e := range entries {
		want += len(e.Marshal())
	}

	buf := make([]byte, want)
	n, err := server.MarshalDir(entries, 0, buf)
	if err != nil {
		t.Fatalf("MarshalDir: %v", err)
	}
	if n != want {
		t.Fatalf("n = %d, want %d", n, want)
	}

	got := decodeAll(t, buf[:n])
	assertNames(t, got, "a", "b", "c")

	n2, err := server.MarshalDir(entries, int64(n), buf)
	if n2 != 0 || err != io.EOF {
		t.Fatalf("MarshalDir at end = %d, %v, want 0, io.EOF", n2, err)
	}
}

// TestMarshalDirChunked drives MarshalDir the way client.File.ReadDir
// drives a real directory Read: sequential calls at offset 0, then at
// whatever offset the previous call reported consuming, using a
// buffer too small to hold every entry at once so a boundary lands
// mid-directory.
func TestMarshalDirChunked(t *testing.T) {
	entries := testEntries("alpha", "bravo", "charlie", "delta", "echo")

	var all []p9.Stat
	var off int64
	buf := make([]byte, 150) // fits roughly two entries, not all five
	for {
		n, err := server.MarshalDir(entries, off, buf)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("MarshalDir at offset %d: %v", off, err)
		}
		if n == 0 {
			t.Fatalf("MarshalDir at offset %d returned n=0 with no error", off)
		}
		all = append(all, decodeAll(t, buf[:n])...)
		off += int64(n)
	}

	assertNames(t, all, "alpha", "bravo", "charlie", "delta", "echo")
}

func TestMarshalDirOffsetNotOnBoundary(t *testing.T) {
	entries := testEntries("alpha", "bravo")
	_, err := server.MarshalDir(entries, 1, make([]byte, 64))
	if err == nil {
		t.Fatal("MarshalDir at a mid-entry offset succeeded, want error")
	}
}

func TestMarshalDirOffsetBeyondEnd(t *testing.T) {
	entries := testEntries("alpha")
	total := len(entries[0].Marshal())
	_, err := server.MarshalDir(entries, int64(total)+1, make([]byte, 64))
	if err == nil {
		t.Fatal("MarshalDir past end of directory succeeded, want error")
	}
}

func TestMarshalDirNegativeOffset(t *testing.T) {
	entries := testEntries("alpha")
	_, err := server.MarshalDir(entries, -1, make([]byte, 64))
	if err == nil {
		t.Fatal("MarshalDir with negative offset succeeded, want error")
	}
}

func TestMarshalDirBufferTooSmall(t *testing.T) {
	entries := testEntries("a-long-enough-name-to-not-fit")
	_, err := server.MarshalDir(entries, 0, make([]byte, 1))
	if err == nil {
		t.Fatal("MarshalDir with an undersized buffer succeeded, want error")
	}
}

// decodeAll mirrors client.File.ReadDirContext's own chunk parser
// (client/file.go): a 2-byte little-endian size prefix followed by
// that many bytes, repeated until chunk is exhausted. A short or
// split entry is exactly the failure MarshalDir must never produce.
func decodeAll(t *testing.T, chunk []byte) []p9.Stat {
	t.Helper()
	var stats []p9.Stat
	for len(chunk) > 0 {
		if len(chunk) < 2 {
			t.Fatalf("truncated entry: %d bytes left", len(chunk))
		}
		size := int(binary.LittleEndian.Uint16(chunk))
		total := 2 + size
		if total > len(chunk) {
			t.Fatalf("truncated entry: need %d bytes, have %d", total, len(chunk))
		}
		st, err := p9.UnmarshalStat(chunk[:total])
		if err != nil {
			t.Fatalf("UnmarshalStat: %v", err)
		}
		stats = append(stats, st)
		chunk = chunk[total:]
	}
	return stats
}

func assertNames(t *testing.T, got []p9.Stat, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i].Name != name {
			t.Errorf("entry %d name = %q, want %q", i, got[i].Name, name)
		}
	}
}
