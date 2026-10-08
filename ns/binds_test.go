package ns

import (
	"context"
	"reflect"
	"testing"
	"time"

	p9 "github.com/sandgorgon/9p"
)

func TestBindsRecordsSpecAndCanonicalDisposition(t *testing.T) {
	n := New()
	ctx := context.Background()
	if err := n.BindFS(&memFS{name: "a", content: "A"}, "", "/boot", Replace); err != nil {
		t.Fatal(err)
	}
	if err := n.BindFSSpec(&memFS{name: "b", content: "B"}, "", "/work", Replace, `dir("/x")`); err != nil {
		t.Fatal(err)
	}
	if err := n.BindPath(ctx, []string{"/work"}, "/alias", Replace); err != nil {
		t.Fatal(err)
	}
	// Bound *before* /alias's first layer: union order is [/boot, /work],
	// but the canonical listing must still replay to that same order.
	if err := n.BindPath(ctx, []string{"/boot", "/work"}, "/u", Replace); err != nil {
		t.Fatal(err)
	}
	if err := n.BindPath(ctx, []string{"/boot"}, "/u", Before); err != nil {
		t.Fatal(err)
	}

	want := []Bind{
		// Dev is the layer's bind sequence number; a path bind (/alias, /u)
		// has none of its own and reports 0.
		{Dst: "/boot", Src: "", Disp: "replace", Dev: 1},
		{Dst: "/work", Src: `dir("/x")`, Disp: "replace", Dev: 2},
		{Dst: "/alias", Src: "/work", Disp: "replace"},
		{Dst: "/u", Src: "/boot", Disp: "replace"},
		{Dst: "/u", Src: "/boot", Disp: "after"},
		{Dst: "/u", Src: "/work", Disp: "after"},
	}
	if got := n.Binds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Binds() =\n%#v\nwant\n%#v", got, want)
	}
}

func TestBindsAfterUnbindAndReplace(t *testing.T) {
	n := New()
	ctx := context.Background()
	n.BindFSSpec(&memFS{name: "a"}, "", "/a", Replace, `dir("/a")`)
	n.BindFSSpec(&memFS{name: "b"}, "", "/b", Replace, `dir("/b")`)
	if err := n.Unbind("/a"); err != nil {
		t.Fatal(err)
	}
	// Replace drops the prior layer from the report too.
	n.BindFSSpec(&memFS{name: "c"}, "", "/b", Replace, `dir("/c")`)
	_ = ctx
	want := []Bind{{Dst: "/b", Src: `dir("/c")`, Disp: "replace", Dev: 3}}
	if got := n.Binds(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Binds() = %#v, want %#v", got, want)
	}
}

func TestBindsEmptyNamespace(t *testing.T) {
	if got := New().Binds(); len(got) != 0 {
		t.Fatalf("Binds() = %#v, want empty", got)
	}
}

func TestResolveUnionLayersTreeAndBindPoint(t *testing.T) {
	n := New()
	ctx := context.Background()
	n.BindFSSpec(&memFS{name: "a", content: "A"}, "", "/u", Replace, `dir("/one")`)
	n.BindFSSpec(&memFS{name: "b", content: "B"}, "", "/u", After, `dir("/two")`)
	n.BindFS(&memFS{name: "c"}, "", "/n/h", Replace)

	cases := []struct {
		path string
		want Resolution
	}{
		{"/u/a", Resolution{Path: "/u/a", Kind: "layer", Dst: "/u", Src: `dir("/one")`, Layer: 0, Layers: 2, Dev: 1, Inner: "/a"}},
		{"/u/b", Resolution{Path: "/u/b", Kind: "layer", Dst: "/u", Src: `dir("/two")`, Layer: 1, Layers: 2, Dev: 2, Inner: "/b"}},
		{"/u", Resolution{Path: "/u", Kind: "bindpoint", Dst: "/u", Layer: -1, Layers: 2}},
		{"/n", Resolution{Path: "/n", Kind: "tree", Dst: "/n", Layer: -1}},
		{"/n/h/c", Resolution{Path: "/n/h/c", Kind: "layer", Dst: "/n/h", Layer: 0, Layers: 1, Dev: 3, Inner: "/c"}},
		{"/", Resolution{Path: "/", Kind: "tree", Dst: "/", Layer: -1}},
	}
	for _, c := range cases {
		got, err := n.Resolve(ctx, c.path)
		if err != nil {
			t.Errorf("Resolve(%s): %v", c.path, err)
			continue
		}
		if got != c.want {
			t.Errorf("Resolve(%s) =\n%#v\nwant\n%#v", c.path, got, c.want)
		}
	}

	for _, missing := range []string{"/u/nope", "/nothing", "/u/a/deeper"} {
		if _, err := n.Resolve(ctx, missing); err == nil {
			t.Errorf("Resolve(%s) should fail", missing)
		}
	}
}

// Resolve must agree with what a real Walk does, including that it picks
// a layer by the first path element only.
func TestResolveAgreesWithWalk(t *testing.T) {
	n := New()
	ctx := context.Background()
	n.BindFS(&memFS{name: "same", content: "first"}, "", "/u", Replace)
	n.BindFS(&memFS{name: "same", content: "second"}, "", "/u", After)
	root, _ := n.Attach(ctx, "u", "")
	f := mustWalk(t, ctx, root, "u", "same")
	f.Open(ctx, 0)
	if got := readAll(t, ctx, f); got != "first" {
		t.Fatalf("walk served %q, test premise broken", got)
	}
	res, err := n.Resolve(ctx, "/u/same")
	if err != nil || res.Layer != 0 {
		t.Fatalf("Resolve = %#v, %v; want layer 0 (the one Walk served)", res, err)
	}
}

func TestLogKeepsOriginalDispositionsAndUnbinds(t *testing.T) {
	n := New()
	ctx := context.Background()
	tick := 0
	n.log.now = func() time.Time { tick++; return time.Date(2026, 1, 2, 3, 4, tick, 0, time.UTC) }

	n.BindFS(&memFS{name: "a"}, "", "/boot", Replace)
	n.BindFSSpec(&memFS{name: "b"}, "", "/work", Replace, `dir("/x")`)
	n.BindPath(ctx, []string{"/work", "/boot"}, "/u", Replace)
	n.BindPath(ctx, []string{"/boot"}, "/u", Before)
	if err := n.BindPath(ctx, []string{"/missing"}, "/u", After); err == nil {
		t.Fatal("bind of a missing source should fail")
	}
	if err := n.Unbind("/work"); err != nil {
		t.Fatal(err)
	}
	if err := n.Unbind("/never"); err == nil {
		t.Fatal("unbind of nothing should fail")
	}

	entries, dropped := n.Log()
	if dropped != 0 {
		t.Fatalf("dropped = %d", dropped)
	}
	want := []LogEntry{
		{Seq: 1, Time: "2026-01-02T03:04:01Z", Op: "bind", Dst: "/boot", Src: "", Disp: "replace"},
		{Seq: 2, Time: "2026-01-02T03:04:02Z", Op: "bind", Dst: "/work", Src: `dir("/x")`, Disp: "replace"},
		{Seq: 3, Time: "2026-01-02T03:04:03Z", Op: "bind", Dst: "/u", Src: "/work + /boot", Disp: "replace"},
		{Seq: 4, Time: "2026-01-02T03:04:04Z", Op: "bind", Dst: "/u", Src: "/boot", Disp: "before"},
		{Seq: 5, Time: "2026-01-02T03:04:05Z", Op: "unbind", Dst: "/work"},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("Log() =\n%#v\nwant\n%#v (failed operations must not be logged)", entries, want)
	}
}

func TestLogIsCappedOldestFirst(t *testing.T) {
	n := New()
	for i := 0; i < maxLogEntries+5; i++ {
		n.BindFSSpec(&memFS{name: "a"}, "", "/p", Replace, `dir("/x")`)
	}
	entries, dropped := n.Log()
	if len(entries) != maxLogEntries || dropped != 5 {
		t.Fatalf("len = %d, dropped = %d; want %d and 5", len(entries), dropped, maxLogEntries)
	}
	if entries[0].Seq != 6 || entries[len(entries)-1].Seq != maxLogEntries+5 {
		t.Fatalf("seq range = %d..%d, want 6..%d", entries[0].Seq, entries[len(entries)-1].Seq, maxLogEntries+5)
	}
}

func TestReadOnlyBindRefusesWritesButOriginalStaysWritable(t *testing.T) {
	n := New()
	ctx := context.Background()
	n.BindFS(&memFS{name: "f", content: "data"}, "", "/rw", Replace)
	if err := n.BindPathOpts(ctx, []string{"/rw"}, "/ro", Replace, BindOpts{ReadOnly: true}); err != nil {
		t.Fatal(err)
	}
	root, _ := n.Attach(ctx, "u", "")

	f := mustWalk(t, ctx, root, "ro", "f")
	if err := f.Open(ctx, p9.OREAD); err != nil {
		t.Fatalf("read open through ro bind: %v", err)
	}
	if got := readAll(t, ctx, f); got != "data" {
		t.Fatalf("read through ro = %q", got)
	}
	for name, mode := range map[string]p9.Mode{"write": p9.OWRITE, "rdwr": p9.ORDWR, "trunc": p9.OREAD | p9.OTRUNC} {
		if err := mustWalk(t, ctx, root, "ro", "f").Open(ctx, mode); err == nil {
			t.Errorf("open %s through ro bind should fail", name)
		}
	}
	if _, err := mustWalk(t, ctx, root, "ro", "f").Write(ctx, 0, []byte("x")); err == nil {
		t.Error("write through ro bind should fail")
	}
	if err := mustWalk(t, ctx, root, "ro", "f").Remove(ctx); err == nil {
		t.Error("remove through ro bind should fail")
	}
	if err := mustWalk(t, ctx, root, "ro", "f").WStat(ctx, p9.Stat{Name: "g"}); err == nil {
		t.Error("wstat through ro bind should fail")
	}
	if _, err := mustWalk(t, ctx, root, "ro").Create(ctx, "new", 0644, p9.OWRITE); err == nil {
		t.Error("create through ro bind should fail")
	}
	if st, _ := mustWalk(t, ctx, root, "ro", "f").Stat(ctx); st.Mode&0222 != 0 {
		t.Errorf("ro Stat mode = %o, write bits should be masked", st.Mode)
	}

	// The same file through the non-ro bind is untouched by any of that.
	if err := mustWalk(t, ctx, root, "rw", "f").Open(ctx, p9.OWRITE); err != nil {
		t.Errorf("open for write through the rw bind: %v", err)
	}
}

func TestReadOnlyShowsInBindsLogAndResolve(t *testing.T) {
	n := New()
	ctx := context.Background()
	n.BindFS(&memFS{name: "f"}, "", "/rw", Replace)
	n.BindPathOpts(ctx, []string{"/rw"}, "/ro", Replace, BindOpts{ReadOnly: true})
	n.BindFSOpts(&memFS{name: "g"}, "", "/ro2", After, BindOpts{Spec: `dir("/x")`, ReadOnly: true})

	wantBinds := []Bind{
		{Dst: "/rw", Disp: "replace", Dev: 1},
		{Dst: "/ro", Src: "/rw", Disp: "replace", RO: true},
		{Dst: "/ro2", Src: `dir("/x")`, Disp: "replace", RO: true, Dev: 3},
	}
	if got := n.Binds(); !reflect.DeepEqual(got, wantBinds) {
		t.Errorf("Binds() =\n%#v\nwant\n%#v", got, wantBinds)
	}
	entries, _ := n.Log()
	if last := entries[len(entries)-1]; last.Disp != "after" || !last.RO {
		t.Errorf("last log entry lacks disposition+ro: %#v", last)
	}
	res, err := n.Resolve(ctx, "/ro/f")
	if err != nil || !res.RO {
		t.Errorf("Resolve(/ro/f) = %#v, %v; want RO", res, err)
	}
	if res, _ := n.Resolve(ctx, "/rw/f"); res.RO {
		t.Error("Resolve(/rw/f) should not be RO")
	}
	if res, _ := n.Resolve(ctx, "/ro"); !res.RO {
		t.Error("Resolve(/ro) — the read-only bind point itself — should be RO")
	}
	if res, _ := n.Resolve(ctx, "/rw"); res.RO {
		t.Error("Resolve(/rw) should not be RO")
	}
}

func TestOnBindHookSeesSuccessfulOpsInOrder(t *testing.T) {
	n := New()
	ctx := context.Background()
	var got []LogEntry
	n.OnBind(func(e LogEntry) {
		got = append(got, e)
		n.Binds() // a hook may read the namespace: no lock may be held across it
	})
	n.BindFSSpec(&memFS{name: "a"}, "", "/w", Replace, `dir("/x")`)
	n.BindPathOpts(ctx, []string{"/w"}, "/v", After, BindOpts{ReadOnly: true})
	if err := n.BindPath(ctx, []string{"/missing"}, "/v", Replace); err == nil {
		t.Fatal("bind of a missing source should fail")
	}
	n.Unbind("/w")
	if err := n.Unbind("/never"); err == nil {
		t.Fatal("unbind of nothing should fail")
	}

	var summary []string
	for _, e := range got {
		summary = append(summary, e.Op+" "+e.Dst)
	}
	if want := []string{"bind /w", "bind /v", "unbind /w"}; !reflect.DeepEqual(summary, want) {
		t.Fatalf("hook saw %v, want %v (failed operations must not fire it)", summary, want)
	}
	if got[1].Disp != "after" || !got[1].RO || got[0].Seq != 1 || got[2].Seq != 3 {
		t.Errorf("hook entries carry the wrong details: %#v", got)
	}

	n.OnBind(nil)
	n.BindFS(&memFS{name: "b"}, "", "/z", Replace)
	if len(got) != 3 {
		t.Error("a cleared hook must not fire")
	}
}

func TestCloneDoesNotInheritTheOnBindHook(t *testing.T) {
	n := New()
	fired := 0
	n.OnBind(func(LogEntry) { fired++ })
	c := n.Clone()
	c.BindFS(&memFS{name: "a"}, "", "/private", Replace)
	if fired != 0 {
		t.Fatal("a bind in a cloned (in_ns) namespace reached the original's hook")
	}
	n.BindFS(&memFS{name: "a"}, "", "/real", Replace)
	if fired != 1 {
		t.Fatalf("the original's hook fired %d times, want 1", fired)
	}
}
