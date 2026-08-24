package client

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"sync"

	p9 "github.com/sandgorgon/9p"
)

// ioHeaderSize is a conservative estimate of the non-data overhead
// in a Twrite/Rread message (size, type, tag, fid, offset, count),
// subtracted from msize to bound how much payload fits in one
// message alongside that header.
const ioHeaderSize = 24

// File is an open file on the server. It implements io.Reader,
// io.Writer, io.ReaderAt, io.WriterAt, io.Seeker, and io.Closer,
// transparently chunking large reads and writes to the negotiated
// message size.
type File struct {
	fid    *Fid
	iounit uint32

	mu     sync.Mutex
	offset int64
}

// Open walks from the client's attached root (see Attach) to path
// and opens it in mode.
func (c *Client) Open(path string, mode p9.Mode) (*File, error) {
	return c.OpenContext(context.Background(), path, mode)
}

func (c *Client) OpenContext(ctx context.Context, path string, mode p9.Mode) (*File, error) {
	c.rootMu.RLock()
	root := c.root
	c.rootMu.RUnlock()
	if root == nil {
		return nil, errors.New("client: Open: not attached")
	}
	fid, err := root.WalkContext(ctx, splitPath(path)...)
	if err != nil {
		return nil, err
	}
	_, iounit, err := fid.OpenContext(ctx, mode)
	if err != nil {
		fid.ClunkContext(ctx)
		return nil, err
	}
	return &File{fid: fid, iounit: iounit}, nil
}

// Create walks from the client's attached root (see Attach) to
// path's parent directory, creates path's final element there, and
// returns a *File open for I/O on it — Open's write-side
// counterpart. perm and mode are Fid.Create's own parameters
// (Tcreate's Perm and Mode fields); see its doc for exactly what
// they mean.
func (c *Client) Create(path string, perm p9.Mode, mode p9.Mode) (*File, error) {
	return c.CreateContext(context.Background(), path, perm, mode)
}

func (c *Client) CreateContext(ctx context.Context, path string, perm p9.Mode, mode p9.Mode) (*File, error) {
	c.rootMu.RLock()
	root := c.root
	c.rootMu.RUnlock()
	if root == nil {
		return nil, errors.New("client: Create: not attached")
	}
	elems := splitPath(path)
	if len(elems) == 0 {
		return nil, errors.New("client: Create: empty path")
	}
	dir, name := elems[:len(elems)-1], elems[len(elems)-1]

	fid, err := root.WalkContext(ctx, dir...)
	if err != nil {
		return nil, err
	}
	_, iounit, err := fid.CreateContext(ctx, name, perm, mode)
	if err != nil {
		fid.ClunkContext(ctx)
		return nil, err
	}
	return &File{fid: fid, iounit: iounit}, nil
}

func splitPath(p string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(p); i++ {
		if i == len(p) || p[i] == '/' {
			if i > start {
				out = append(out, p[start:i])
			}
			start = i + 1
		}
	}
	return out
}

func (f *File) maxIO() int {
	msize := int(f.fid.c.msize.Load())
	max := msize - ioHeaderSize
	if f.iounit > 0 && int(f.iounit) < max {
		max = int(f.iounit)
	}
	if max <= 0 {
		max = 1
	}
	return max
}

func (f *File) readAt(ctx context.Context, p []byte, off int64) (int, error) {
	total := 0
	for total < len(p) {
		chunk := p[total:]
		if max := f.maxIO(); len(chunk) > max {
			chunk = chunk[:max]
		}
		reply, err := f.fid.c.rpc(ctx, &p9.TreadFcall{
			Fid:    f.fid.fid,
			Offset: uint64(off + int64(total)),
			Count:  uint32(len(chunk)),
		})
		if err != nil {
			return total, err
		}
		rr, ok := reply.(*p9.RreadFcall)
		if !ok {
			return total, mismatchErr(p9.Rread, reply)
		}
		n := copy(chunk, rr.Data)
		total += n
		if n < len(chunk) {
			return total, io.EOF
		}
	}
	return total, nil
}

func (f *File) writeAt(ctx context.Context, p []byte, off int64) (int, error) {
	total := 0
	for total < len(p) {
		chunk := p[total:]
		if max := f.maxIO(); len(chunk) > max {
			chunk = chunk[:max]
		}
		reply, err := f.fid.c.rpc(ctx, &p9.TwriteFcall{
			Fid:    f.fid.fid,
			Offset: uint64(off + int64(total)),
			Data:   chunk,
		})
		if err != nil {
			return total, err
		}
		rw, ok := reply.(*p9.RwriteFcall)
		if !ok {
			return total, mismatchErr(p9.Rwrite, reply)
		}
		if rw.Count == 0 {
			return total, io.ErrShortWrite
		}
		total += int(rw.Count)
	}
	return total, nil
}

// ReadAt implements io.ReaderAt.
func (f *File) ReadAt(p []byte, off int64) (int, error) {
	return f.readAt(context.Background(), p, off)
}

// WriteAt implements io.WriterAt.
func (f *File) WriteAt(p []byte, off int64) (int, error) {
	return f.writeAt(context.Background(), p, off)
}

// Read implements io.Reader, reading from and advancing the current
// offset. Unlike ReadAt, a single call may return fewer bytes than
// len(p) without error, per io.Reader's contract.
func (f *File) Read(p []byte) (int, error) {
	f.mu.Lock()
	off := f.offset
	f.mu.Unlock()

	if max := f.maxIO(); len(p) > max {
		p = p[:max]
	}
	reply, err := f.fid.c.rpc(context.Background(), &p9.TreadFcall{
		Fid: f.fid.fid, Offset: uint64(off), Count: uint32(len(p)),
	})
	if err != nil {
		return 0, err
	}
	rr, ok := reply.(*p9.RreadFcall)
	if !ok {
		return 0, mismatchErr(p9.Rread, reply)
	}
	n := copy(p, rr.Data)

	f.mu.Lock()
	f.offset += int64(n)
	f.mu.Unlock()

	if n == 0 {
		return 0, io.EOF
	}
	return n, nil
}

// Write implements io.Writer, writing at and advancing the current
// offset.
func (f *File) Write(p []byte) (int, error) {
	f.mu.Lock()
	off := f.offset
	f.mu.Unlock()

	n, err := f.writeAt(context.Background(), p, off)

	f.mu.Lock()
	f.offset += int64(n)
	f.mu.Unlock()
	return n, err
}

// Seek implements io.Seeker. io.SeekEnd issues a Stat to learn the
// current file length.
func (f *File) Seek(offset int64, whence int) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	var newOff int64
	switch whence {
	case io.SeekStart:
		newOff = offset
	case io.SeekCurrent:
		newOff = f.offset + offset
	case io.SeekEnd:
		st, err := f.fid.Stat()
		if err != nil {
			return f.offset, err
		}
		newOff = int64(st.Length) + offset
	default:
		return f.offset, errors.New("client: Seek: invalid whence")
	}
	if newOff < 0 {
		return f.offset, errors.New("client: Seek: negative offset")
	}
	f.offset = newOff
	return f.offset, nil
}

// Stat fetches the file's metadata.
func (f *File) Stat() (p9.Stat, error) {
	return f.fid.Stat()
}

// Close clunks the file's fid.
func (f *File) Close() error {
	return f.fid.Clunk()
}

// ReadDir reads f (which must be a directory) and decodes its
// contents as a sequence of Stat entries.
func (f *File) ReadDir() ([]p9.Stat, error) {
	return f.ReadDirContext(context.Background())
}

// ReadDirContext deliberately doesn't go through readAt: a directory
// Read (server.MarshalDir in particular) only ever returns whole Stat
// entries, so a reply shorter than the requested Count is normal
// mid-listing behavior, not end-of-file — unlike a regular file read,
// where readAt's "short read means EOF" convention is exactly what
// io.ReaderAt calls for. The 9P directory-read convention is instead
// to keep reading at growing offsets until a read returns zero bytes.
func (f *File) ReadDirContext(ctx context.Context) ([]p9.Stat, error) {
	var stats []p9.Stat
	var off int64
	for {
		count := f.maxIO()
		reply, err := f.fid.c.rpc(ctx, &p9.TreadFcall{
			Fid: f.fid.fid, Offset: uint64(off), Count: uint32(count),
		})
		if err != nil {
			return stats, err
		}
		rr, ok := reply.(*p9.RreadFcall)
		if !ok {
			return stats, mismatchErr(p9.Rread, reply)
		}
		if len(rr.Data) == 0 {
			return stats, nil
		}

		chunk := rr.Data
		for len(chunk) > 0 {
			if len(chunk) < 2 {
				return stats, errors.New("client: ReadDir: truncated entry")
			}
			size := int(binary.LittleEndian.Uint16(chunk))
			total := 2 + size
			if total > len(chunk) {
				return stats, errors.New("client: ReadDir: truncated entry")
			}
			st, serr := p9.UnmarshalStat(chunk[:total])
			if serr != nil {
				return stats, serr
			}
			stats = append(stats, st)
			chunk = chunk[total:]
		}
		off += int64(len(rr.Data))
	}
}
