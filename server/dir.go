package server

import (
	"errors"
	"io"

	p9 "github.com/sandgorgon/9p"
)

// MarshalDir encodes entries as concatenated Stat blobs (each exactly
// what Stat.Marshal produces) and copies into buf the portion of that
// encoding starting at byte offset, truncated to whole entries that
// fit within len(buf). It implements the offset contract 9P2000
// directory reads require, and that this library's own
// client.File.ReadDir relies on: reads start at 0 or at an offset a
// previous call reported consuming (offset + n), and a response must
// never split an entry.
//
// entries must be in the same order on every call for a given
// directory generation — MarshalDir does not sort them, and
// reordering between calls breaks the offset contract as surely as
// mutating the directory mid-read would.
//
// A directory's File.Read can be implemented as just:
//
//	func (d *dirFile) Read(ctx context.Context, offset int64, p []byte) (int, error) {
//	    entries, err := d.list(ctx) // build/cache []p9.Stat however you like
//	    if err != nil {
//	        return 0, err
//	    }
//	    return server.MarshalDir(entries, offset, p)
//	}
//
// offset must be 0 or a value previously returned as (offset-in + n)
// against the same entries — MarshalDir doesn't track state across
// calls (File.Read is stateless per the interface) or validate that
// history, so an offset that doesn't land exactly on an entry
// boundary is reported as an error rather than silently misbehaving.
// An offset equal to the full encoded length returns (0, io.EOF), per
// File.Read's own end-of-file convention. If buf isn't big enough to
// hold even one whole entry, that is also reported as an error rather
// than a valid empty or partial read.
func MarshalDir(entries []p9.Stat, offset int64, buf []byte) (int, error) {
	if offset < 0 {
		return 0, errors.New("server: MarshalDir: negative offset")
	}

	var pos int64
	i := 0
	for ; i < len(entries); i++ {
		if pos == offset {
			break
		}
		blobLen := int64(len(entries[i].Marshal()))
		if pos+blobLen > offset {
			return 0, errors.New("server: MarshalDir: offset does not land on an entry boundary")
		}
		pos += blobLen
	}
	if i == len(entries) {
		if pos == offset {
			return 0, io.EOF
		}
		return 0, errors.New("server: MarshalDir: offset out of range")
	}

	var n int
	for _, e := range entries[i:] {
		blob := e.Marshal()
		if n+len(blob) > len(buf) {
			break
		}
		n += copy(buf[n:], blob)
	}
	if n == 0 {
		return 0, errors.New("server: MarshalDir: buffer too small for one entry")
	}
	return n, nil
}
