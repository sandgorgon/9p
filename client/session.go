package client

import (
	"context"

	p9 "github.com/sandgorgon/9p"
)

// Attach attaches to aname on the server as uname, returning a Fid
// for the root of the attached tree. It always attaches without
// authentication (Tattach.Afid = p9.NoFid); servers that require
// authentication first are not supported by this client.
//
// The returned Fid also becomes the client's root for the Open
// convenience method.
func (c *Client) Attach(uname, aname string) (*Fid, error) {
	return c.AttachContext(context.Background(), uname, aname)
}

func (c *Client) AttachContext(ctx context.Context, uname, aname string) (*Fid, error) {
	fid := c.allocFid()
	reply, err := c.rpc(ctx, &p9.TattachFcall{Fid: fid, Afid: p9.NoFid, Uname: uname, Aname: aname})
	if err != nil {
		return nil, err
	}
	if _, ok := reply.(*p9.RattachFcall); !ok {
		return nil, mismatchErr(p9.Rattach, reply)
	}
	f := &Fid{c: c, fid: fid}
	c.rootMu.Lock()
	c.root = f
	c.rootMu.Unlock()
	return f, nil
}
