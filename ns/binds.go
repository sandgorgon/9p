package ns

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Bind is one layer of one bind point, as reported by Binds.
type Bind struct {
	Dst string `json:"dst"`
	// Src is the label the caller gave this layer ("/local",
	// `dial("h:1")`), or "" for a bind with no label.
	Src string `json:"src"`
	// Disp is the canonical disposition that reproduces the current
	// state when the layers of one Dst are replayed in order: "replace"
	// for the first, "after" for the rest. It is not the disposition the
	// layer was originally bound with — that isn't recoverable once a
	// later bind has spliced around it, and doesn't matter for state.
	Disp string `json:"disp"`
	// RO is whether the layer refuses writes (bound with the ro flag).
	RO bool `json:"ro"`
	// Dev is the id this layer stamps into Stat.Dev of every file it
	// serves, so an entry from `ls` can be matched back to its layer. 0
	// for a layer that binds an existing namespace path: it has no id of
	// its own, its files keep the id of the layer that really serves them.
	Dev uint32 `json:"dev"`
}

// Binds reports every layer currently bound anywhere in the namespace,
// ordered by when its bind point first received a layer (so replaying
// the list top to bottom respects "this bind's source was made by an
// earlier bind"), and by union order within a single Dst.
func (ns *Namespace) Binds() []Bind {
	type point struct {
		dst    string
		layers []*layer
		first  uint64
	}
	var points []point
	var walk func(n *node, path string)
	walk = func(n *node, path string) {
		n.mu.RLock()
		layers := append([]*layer(nil), n.layers...)
		names := make([]string, 0, len(n.children))
		children := make(map[string]*node, len(n.children))
		for name, c := range n.children {
			names = append(names, name)
			children[name] = c
		}
		n.mu.RUnlock()

		if len(layers) > 0 {
			first := layers[0].seq
			for _, l := range layers[1:] {
				first = min(first, l.seq)
			}
			dst := path
			if dst == "" {
				dst = "/"
			}
			points = append(points, point{dst: dst, layers: layers, first: first})
		}
		sort.Strings(names)
		for _, name := range names {
			walk(children[name], path+"/"+name)
		}
	}
	walk(ns.root, "")
	sort.SliceStable(points, func(i, j int) bool { return points[i].first < points[j].first })

	var out []Bind
	for _, p := range points {
		for i, l := range p.layers {
			disp := "after"
			if i == 0 {
				disp = "replace"
			}
			out = append(out, Bind{Dst: p.dst, Src: l.spec, Disp: disp, RO: l.ro, Dev: l.dev()})
		}
	}
	return out
}

// Resolution reports how one path resolves through the bind tree — which
// bind point and which layer of it would serve a Walk to that path.
type Resolution struct {
	Path string
	// Kind is "layer" when the path lands inside a bound filesystem,
	// "bindpoint" when it is exactly a bind point that has layers (a
	// union directory), or "tree" when it is a purely synthetic
	// directory of the bind tree (like /n when only /n/host is bound).
	Kind string
	// Dst is the bind point where resolution left the explicit tree
	// (Kind "layer"), or the path itself (the other kinds).
	Dst string
	// Src is the serving layer's source label, "" for an unlabeled bind
	// or when there is no layer. Layer is its 0-based position in
	// Dst's union order, -1 when there is none. Layers is how many
	// layers Dst has.
	Src    string
	Layer  int
	Layers int
	// Dev is the Stat.Dev the serving layer's files carry (see Bind.Dev);
	// for a layer that binds an existing path, the id of the layer that
	// really serves the file. 0 unless Kind is "layer".
	Dev uint32
	// Inner is the path within the serving layer ("/" is its root); ""
	// unless Kind is "layer".
	Inner string
	// RO is whether the serving layer is read-only; for a "bindpoint",
	// whether its first layer is (the one a create would go to).
	RO bool
}

// Resolve answers "what serves this path", by walking exactly the way
// nsFile.Walk does: explicit tree children win over layers, and at the
// first node with no such child the layers are tried in union order —
// the first one whose Walk of the next name succeeds serves the rest of
// the path, with no fallback to a later layer if a deeper element is
// missing there. Reporting the same answer a real Walk would give is
// the point, so this deliberately does not try to be smarter (a union
// directory's *listing* merges layers; a Walk into it does not).
func (ns *Namespace) Resolve(ctx context.Context, path string) (Resolution, error) {
	res, _, err := ns.resolve(ctx, path)
	return res, err
}

// resolve is Resolve plus the serving layer itself (for a "bindpoint",
// its first layer; nil for a "tree"), for callers like HostPath that need more than the report.
func (ns *Namespace) resolve(ctx context.Context, path string) (Resolution, *layer, error) {
	parts := splitPath(path)
	res := Resolution{Path: "/" + strings.Join(parts, "/"), Layer: -1}
	n := ns.root
	cur := ""
	for i, name := range parts {
		if name == ".." {
			return res, nil, errors.New("ns: '..' is not supported at a namespace bind point")
		}
		n.mu.RLock()
		child, hasChild := n.children[name]
		layers := n.layers
		n.mu.RUnlock()
		if hasChild {
			n = child
			cur += "/" + name
			continue
		}
		dst := cur
		if dst == "" {
			dst = "/"
		}
		var lastErr error
		for li, l := range layers {
			root, err := l.root(ctx)
			if err != nil {
				lastErr = err
				continue
			}
			f, err := root.Walk(ctx, name)
			if err != nil {
				lastErr = err
				continue
			}
			for _, p := range parts[i+1:] {
				if f, err = f.Walk(ctx, p); err != nil {
					return res, nil, fmt.Errorf("ns: %s: %w", res.Path, err)
				}
			}
			res.Kind, res.Dst, res.Src, res.RO = "layer", dst, l.spec, l.ro
			res.Dev = l.dev()
			if res.Dev == 0 {
				if st, err := f.Stat(ctx); err == nil {
					res.Dev = st.Dev
				}
			}
			res.Layer, res.Layers = li, len(layers)
			res.Inner = "/" + strings.Join(parts[i:], "/")
			return res, l, nil
		}
		if lastErr == nil {
			lastErr = fmt.Errorf("ns: %s: no such file", name)
		}
		return res, nil, fmt.Errorf("ns: %s: %w", res.Path, lastErr)
	}
	var first *layer
	n.mu.RLock()
	res.Layers = len(n.layers)
	if res.Layers > 0 {
		// No single serving layer here; a create at a bind point goes to
		// the first, so that is the one whose read-only flag matters.
		first = n.layers[0]
		res.RO = first.ro
	}
	n.mu.RUnlock()
	res.Kind, res.Dst = "tree", res.Path
	if res.Layers > 0 {
		res.Kind = "bindpoint"
	}
	return res, first, nil
}
