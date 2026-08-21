package server

import (
	"context"
	"fmt"
	"io"

	p9 "p9"
)

func errReply(err error) *p9.RerrorFcall {
	if re, ok := err.(*p9.RerrorFcall); ok {
		return re
	}
	return &p9.RerrorFcall{Ename: err.Error()}
}

func errUnknownFid() *p9.RerrorFcall {
	return &p9.RerrorFcall{Ename: "unknown fid"}
}

func (c *conn) dispatch(ctx context.Context, msg p9.Message) p9.Message {
	switch m := msg.(type) {
	case *p9.TversionFcall:
		return c.tVersion(m)
	case *p9.TauthFcall:
		return &p9.RerrorFcall{Ename: "authentication not required"}
	case *p9.TattachFcall:
		return c.tAttach(ctx, m)
	case *p9.TflushFcall:
		return c.tFlush(m)
	case *p9.TwalkFcall:
		return c.tWalk(ctx, m)
	case *p9.TopenFcall:
		return c.tOpen(ctx, m)
	case *p9.TcreateFcall:
		return c.tCreate(ctx, m)
	case *p9.TreadFcall:
		return c.tRead(ctx, m)
	case *p9.TwriteFcall:
		return c.tWrite(ctx, m)
	case *p9.TclunkFcall:
		return c.tClunk(m)
	case *p9.TremoveFcall:
		return c.tRemove(ctx, m)
	case *p9.TstatFcall:
		return c.tStat(ctx, m)
	case *p9.TwstatFcall:
		return c.tWstat(ctx, m)
	default:
		return &p9.RerrorFcall{Ename: fmt.Sprintf("unexpected %v", m.MsgType())}
	}
}

func (c *conn) tAttach(ctx context.Context, m *p9.TattachFcall) p9.Message {
	f, err := c.srv.FS.Attach(ctx, m.Uname, m.Aname)
	if err != nil {
		return errReply(err)
	}
	c.putFid(m.Fid, &openFile{file: f})
	return &p9.RattachFcall{Qid: f.Qid()}
}

func (c *conn) tFlush(m *p9.TflushFcall) p9.Message {
	c.mu.Lock()
	cancel, ok := c.inflight[m.Oldtag]
	c.mu.Unlock()
	if ok {
		cancel()
	}
	return &p9.RflushFcall{}
}

// tWalk walks each element of m.Wname in turn from m.Fid, per spec:
// if the very first element fails to resolve, the whole call fails
// with Rerror and m.Fid is left untouched; if a later element fails,
// the walk stops there and succeeds with fewer qids than requested,
// repositioning m.Newfid at the last element reached.
func (c *conn) tWalk(ctx context.Context, m *p9.TwalkFcall) p9.Message {
	of, ok := c.getFid(m.Fid)
	if !ok {
		return errUnknownFid()
	}
	if len(m.Wname) == 0 {
		c.putFid(m.Newfid, &openFile{file: of.get()})
		return &p9.RwalkFcall{Wqid: []p9.Qid{}}
	}

	cur := of.get()
	qids := make([]p9.Qid, 0, len(m.Wname))
	for i, name := range m.Wname {
		next, err := cur.Walk(ctx, name)
		if err != nil {
			if i == 0 {
				return errReply(err)
			}
			break
		}
		cur = next
		qids = append(qids, cur.Qid())
	}
	c.putFid(m.Newfid, &openFile{file: cur})
	return &p9.RwalkFcall{Wqid: qids}
}

func (c *conn) tOpen(ctx context.Context, m *p9.TopenFcall) p9.Message {
	of, ok := c.getFid(m.Fid)
	if !ok {
		return errUnknownFid()
	}
	f := of.get()
	if err := f.Open(ctx, m.Mode); err != nil {
		return errReply(err)
	}
	return &p9.RopenFcall{Qid: f.Qid(), Iounit: 0}
}

func (c *conn) tCreate(ctx context.Context, m *p9.TcreateFcall) p9.Message {
	of, ok := c.getFid(m.Fid)
	if !ok {
		return errUnknownFid()
	}
	nf, err := of.get().Create(ctx, m.Name, m.Perm, m.Mode)
	if err != nil {
		return errReply(err)
	}
	// Tcreate repositions the fid onto the newly created file.
	of.set(nf)
	return &p9.RcreateFcall{Qid: nf.Qid(), Iounit: 0}
}

func (c *conn) tRead(ctx context.Context, m *p9.TreadFcall) p9.Message {
	of, ok := c.getFid(m.Fid)
	if !ok {
		return errUnknownFid()
	}
	buf := make([]byte, m.Count)
	n, err := of.get().Read(ctx, int64(m.Offset), buf)
	if err != nil && err != io.EOF {
		return errReply(err)
	}
	return &p9.RreadFcall{Data: buf[:n]}
}

func (c *conn) tWrite(ctx context.Context, m *p9.TwriteFcall) p9.Message {
	of, ok := c.getFid(m.Fid)
	if !ok {
		return errUnknownFid()
	}
	n, err := of.get().Write(ctx, int64(m.Offset), m.Data)
	if err != nil {
		return errReply(err)
	}
	return &p9.RwriteFcall{Count: uint32(n)}
}

func (c *conn) tClunk(m *p9.TclunkFcall) p9.Message {
	of, ok := c.delFid(m.Fid)
	if ok {
		of.get().Close()
	}
	return &p9.RclunkFcall{}
}

func (c *conn) tRemove(ctx context.Context, m *p9.TremoveFcall) p9.Message {
	of, ok := c.delFid(m.Fid)
	if !ok {
		return errUnknownFid()
	}
	f := of.get()
	err := f.Remove(ctx)
	f.Close()
	if err != nil {
		return errReply(err)
	}
	return &p9.RremoveFcall{}
}

func (c *conn) tStat(ctx context.Context, m *p9.TstatFcall) p9.Message {
	of, ok := c.getFid(m.Fid)
	if !ok {
		return errUnknownFid()
	}
	st, err := of.get().Stat(ctx)
	if err != nil {
		return errReply(err)
	}
	return &p9.RstatFcall{Stat: st}
}

func (c *conn) tWstat(ctx context.Context, m *p9.TwstatFcall) p9.Message {
	of, ok := c.getFid(m.Fid)
	if !ok {
		return errUnknownFid()
	}
	if err := of.get().WStat(ctx, m.Stat); err != nil {
		return errReply(err)
	}
	return &p9.RwstatFcall{}
}
