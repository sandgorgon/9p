package client

import (
	"context"
	"fmt"

	p9 "github.com/sandgorgon/9p"
)

// Fid is a client handle for a file on the server, analogous to a
// file descriptor before it has been opened.
type Fid struct {
	c   *Client
	fid p9.Fid
}

// Walk clones f to a new Fid positioned at the path named by names,
// relative to f. Twalk messages are limited to p9.MaxWElem elements,
// so a long path is walked in batches transparently.
func (f *Fid) Walk(names ...string) (*Fid, error) {
	return f.WalkContext(context.Background(), names...)
}

func (f *Fid) WalkContext(ctx context.Context, names ...string) (*Fid, error) {
	newfid := f.c.allocFid()
	cur := f.fid

	if len(names) == 0 {
		if _, err := f.walkBatch(ctx, cur, newfid, nil); err != nil {
			return nil, err
		}
		return &Fid{c: f.c, fid: newfid}, nil
	}

	for i := 0; i < len(names); i += p9.MaxWElem {
		end := min(i+p9.MaxWElem, len(names))
		batch := names[i:end]
		rw, err := f.walkBatch(ctx, cur, newfid, batch)
		if err != nil {
			if i > 0 {
				f.c.clunkQuiet(newfid)
			}
			return nil, err
		}
		if len(rw.Wqid) < len(batch) {
			f.c.clunkQuiet(newfid)
			return nil, fmt.Errorf("client: walk: %q not found", batch[len(rw.Wqid)])
		}
		cur = newfid
	}
	return &Fid{c: f.c, fid: newfid}, nil
}

func (f *Fid) walkBatch(ctx context.Context, from, to p9.Fid, names []string) (*p9.RwalkFcall, error) {
	reply, err := f.c.rpc(ctx, &p9.TwalkFcall{Fid: from, Newfid: to, Wname: names})
	if err != nil {
		return nil, err
	}
	rw, ok := reply.(*p9.RwalkFcall)
	if !ok {
		return nil, mismatchErr(p9.Rwalk, reply)
	}
	return rw, nil
}

func (c *Client) clunkQuiet(fid p9.Fid) {
	_, _ = c.rpc(context.Background(), &p9.TclunkFcall{Fid: fid})
}

// Open prepares f for I/O in the given mode and returns its Qid and
// the server's preferred I/O unit size (0 if the server has no
// preference).
func (f *Fid) Open(mode p9.Mode) (p9.Qid, uint32, error) {
	return f.OpenContext(context.Background(), mode)
}

func (f *Fid) OpenContext(ctx context.Context, mode p9.Mode) (p9.Qid, uint32, error) {
	reply, err := f.c.rpc(ctx, &p9.TopenFcall{Fid: f.fid, Mode: mode})
	if err != nil {
		return p9.Qid{}, 0, err
	}
	ro, ok := reply.(*p9.RopenFcall)
	if !ok {
		return p9.Qid{}, 0, mismatchErr(p9.Ropen, reply)
	}
	return ro.Qid, ro.Iounit, nil
}

// Create creates name under the directory f, then repositions f
// itself onto the new file, opened in mode — matching Tcreate's wire
// semantics, this does not allocate a new Fid.
func (f *Fid) Create(name string, perm p9.Mode, mode p9.Mode) (p9.Qid, uint32, error) {
	return f.CreateContext(context.Background(), name, perm, mode)
}

func (f *Fid) CreateContext(ctx context.Context, name string, perm p9.Mode, mode p9.Mode) (p9.Qid, uint32, error) {
	reply, err := f.c.rpc(ctx, &p9.TcreateFcall{Fid: f.fid, Name: name, Perm: perm, Mode: mode})
	if err != nil {
		return p9.Qid{}, 0, err
	}
	rc, ok := reply.(*p9.RcreateFcall)
	if !ok {
		return p9.Qid{}, 0, mismatchErr(p9.Rcreate, reply)
	}
	return rc.Qid, rc.Iounit, nil
}

// OpenFile is Open, but returns an I/O-capable *File positioned at f
// instead of just a Qid — f is consumed the same way Open leaves it
// (opened in mode), just without discarding the fid to get there.
func (f *Fid) OpenFile(mode p9.Mode) (*File, error) {
	return f.OpenFileContext(context.Background(), mode)
}

func (f *Fid) OpenFileContext(ctx context.Context, mode p9.Mode) (*File, error) {
	_, iounit, err := f.OpenContext(ctx, mode)
	if err != nil {
		return nil, err
	}
	return &File{fid: f, iounit: iounit}, nil
}

// CreateFile is Create, but returns an I/O-capable *File positioned
// at the newly created child instead of just a Qid — matching
// Create's own "repositions f itself onto the new file" semantics.
func (f *Fid) CreateFile(name string, perm, mode p9.Mode) (*File, error) {
	return f.CreateFileContext(context.Background(), name, perm, mode)
}

func (f *Fid) CreateFileContext(ctx context.Context, name string, perm, mode p9.Mode) (*File, error) {
	_, iounit, err := f.CreateContext(ctx, name, perm, mode)
	if err != nil {
		return nil, err
	}
	return &File{fid: f, iounit: iounit}, nil
}

// Stat fetches f's metadata.
func (f *Fid) Stat() (p9.Stat, error) {
	return f.StatContext(context.Background())
}

func (f *Fid) StatContext(ctx context.Context) (p9.Stat, error) {
	reply, err := f.c.rpc(ctx, &p9.TstatFcall{Fid: f.fid})
	if err != nil {
		return p9.Stat{}, err
	}
	rs, ok := reply.(*p9.RstatFcall)
	if !ok {
		return p9.Stat{}, mismatchErr(p9.Rstat, reply)
	}
	return rs.Stat, nil
}

// WStat updates f's metadata. Fields that should be left unchanged
// must be set to their "don't touch" wire values (an empty string
// for the string fields, ^uint32(0)/^uint64(0) for the numeric
// ones) per the 9P2000 spec — this package does not fill those in
// automatically.
func (f *Fid) WStat(st p9.Stat) error {
	return f.WStatContext(context.Background(), st)
}

func (f *Fid) WStatContext(ctx context.Context, st p9.Stat) error {
	_, err := f.c.rpc(ctx, &p9.TwstatFcall{Fid: f.fid, Stat: st})
	return err
}

// Remove removes the file and clunks f, even if the removal fails,
// per spec.
func (f *Fid) Remove() error {
	return f.RemoveContext(context.Background())
}

func (f *Fid) RemoveContext(ctx context.Context) error {
	_, err := f.c.rpc(ctx, &p9.TremoveFcall{Fid: f.fid})
	return err
}

// Clunk releases f without removing the underlying file.
func (f *Fid) Clunk() error {
	return f.ClunkContext(context.Background())
}

func (f *Fid) ClunkContext(ctx context.Context) error {
	_, err := f.c.rpc(ctx, &p9.TclunkFcall{Fid: f.fid})
	return err
}
