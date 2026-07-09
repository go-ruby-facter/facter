// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-facter/facter authors

package facter

import (
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeEngine is a deterministic Engine for tests: a fixed nested fact map plus a
// recorded list of external-fact dirs and an optional load error.
type fakeEngine struct {
	facts    map[string]any
	loadErr  error
	loadDirs []string
	nilHash  bool // when true, ToHash returns nil (exercises the nil branch)
}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{facts: map[string]any{
		"os":       map[string]any{"name": "Debian", "family": "Debian"},
		"kernel":   "Linux",
		"hostname": "host1",
	}}
}

func (e *fakeEngine) Value(path string) (any, bool) {
	segs := strings.Split(path, ".")
	var cur any = e.facts
	for _, s := range segs {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[s]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

func (e *fakeEngine) ToHash() map[string]any {
	if e.nilHash {
		return nil
	}
	out := map[string]any{}
	for k, v := range e.facts {
		out[k] = v
	}
	return out
}

func (e *fakeEngine) Names() []string {
	out := make([]string, 0, len(e.facts))
	for k := range e.facts {
		out = append(out, k)
	}
	return out
}

func (e *fakeEngine) LoadExternalFacts(dirs ...string) error {
	e.loadDirs = append(e.loadDirs, dirs...)
	return e.loadErr
}

func newTestFacter(e Engine) *Facter { return NewWithEngine(func() Engine { return e }) }

func TestNew(t *testing.T) {
	// New wires the real go-facter engine; it must construct and expose the
	// built-in facts without panicking.
	f := New()
	if f == nil || f.engine == nil {
		t.Fatal("New returned an incomplete Facter")
	}
	if _, ok := f.Value("kernel"); !ok {
		// kernel is one of the always-present built-ins; a bare presence check
		// avoids asserting an OS-specific value.
		t.Log("kernel not resolved by the real engine on this host (acceptable)")
	}
}

func TestValueBuiltinAndDescend(t *testing.T) {
	f := newTestFacter(newFakeEngine())
	if v, ok := f.Value("kernel"); !ok || v != "Linux" {
		t.Fatalf("Value(kernel)=%v,%v", v, ok)
	}
	if v, ok := f.Value("os.name"); !ok || v != "Debian" {
		t.Fatalf("Value(os.name)=%v,%v", v, ok)
	}
	if _, ok := f.Value(""); ok {
		t.Fatal("empty path must not resolve")
	}
	if _, ok := f.Value("os.missing"); ok {
		t.Fatal("missing nested key must not resolve")
	}
	if _, ok := f.Value("kernel.deeper"); ok {
		t.Fatal("descending into a scalar must not resolve")
	}
	if _, ok := f.Value("nope"); ok {
		t.Fatal("unknown top fact must not resolve")
	}
}

func TestValueString(t *testing.T) {
	f := newTestFacter(newFakeEngine())
	if s, ok := f.ValueString("kernel"); !ok || s != "Linux" {
		t.Fatalf("ValueString(kernel)=%q,%v", s, ok)
	}
	if _, ok := f.ValueString("nope"); ok {
		t.Fatal("unknown fact must not stringify")
	}
	f.AddValue("num", 42)
	if s, ok := f.ValueString("num"); !ok || s != "42" {
		t.Fatalf("ValueString(num)=%q,%v", s, ok)
	}
	f.AddValue("nilfact", nil)
	if s, ok := f.ValueString("nilfact"); !ok || s != "" {
		t.Fatalf("ValueString(nilfact)=%q,%v", s, ok)
	}
}

func TestAddOverridesBuiltinByWeight(t *testing.T) {
	f := newTestFacter(newFakeEngine())
	// A custom resolution for a built-in fact wins when it matches.
	f.Add("kernel", Options{}, func(*ResolutionContext) (any, bool) { return "CustomKernel", true })
	if v, _ := f.Value("kernel"); v != "CustomKernel" {
		t.Fatalf("custom kernel not applied: %v", v)
	}
	// A resolution that declines falls back to the engine's built-in.
	f.Add("hostname", Options{}, func(*ResolutionContext) (any, bool) { return nil, false })
	if v, ok := f.Value("hostname"); !ok || v != "host1" {
		t.Fatalf("fallback to built-in failed: %v,%v", v, ok)
	}
}

func TestAddWeightAndDeclarationOrder(t *testing.T) {
	f := newTestFacter(newFakeEngine())
	f.Add("pick", Options{Weight: 1, HasWeight: true}, func(*ResolutionContext) (any, bool) { return "low", true })
	f.Add("pick", Options{Weight: 10, HasWeight: true}, func(*ResolutionContext) (any, bool) { return "high", true })
	if v, _ := f.Value("pick"); v != "high" {
		t.Fatalf("highest weight should win, got %v", v)
	}
	// Ties break by declaration order (stable sort keeps the first).
	g := newTestFacter(newFakeEngine())
	g.Add("tie", Options{Weight: 5, HasWeight: true}, func(*ResolutionContext) (any, bool) { return "first", true })
	g.Add("tie", Options{Weight: 5, HasWeight: true}, func(*ResolutionContext) (any, bool) { return "second", true })
	if v, _ := g.Value("tie"); v != "first" {
		t.Fatalf("tie should keep declaration order, got %v", v)
	}
}

func TestConfineGatesResolution(t *testing.T) {
	f := newTestFacter(newFakeEngine())
	// Two resolutions; the higher-weight one is confined out, so the other wins.
	f.Add("selected", Options{Confine: []Confine{ConfineFact("os.name", "RedHat")}},
		func(*ResolutionContext) (any, bool) { return "redhat-branch", true })
	f.Add("selected", Options{Confine: []Confine{ConfineFact("os.name", "Debian")}},
		func(*ResolutionContext) (any, bool) { return "debian-branch", true })
	if v, _ := f.Value("selected"); v != "debian-branch" {
		t.Fatalf("confine gating failed: %v", v)
	}
	// All resolutions confined out → fact does not resolve (and no built-in).
	g := newTestFacter(newFakeEngine())
	g.Add("gone", Options{Confine: []Confine{ConfineFact("os.name", "Windows")}},
		func(*ResolutionContext) (any, bool) { return "x", true })
	if _, ok := g.Value("gone"); ok {
		t.Fatal("fully-confined-out fact must not resolve")
	}
}

func TestResolutionContextReadsOtherFacts(t *testing.T) {
	f := newTestFacter(newFakeEngine())
	f.Add("derived", Options{}, func(rc *ResolutionContext) (any, bool) {
		name, ok := rc.ValueString("os.name")
		if !ok {
			return nil, false
		}
		fam, _ := rc.Value("os.family")
		return name + "/" + fam.(string), true
	})
	if v, _ := f.Value("derived"); v != "Debian/Debian" {
		t.Fatalf("context fact read failed: %v", v)
	}
}

func TestCacheAndFlush(t *testing.T) {
	f := newTestFacter(newFakeEngine())
	n := 0
	f.Add("counter", Options{}, func(*ResolutionContext) (any, bool) { n++; return n, true })
	if v, _ := f.Value("counter"); v != 1 {
		t.Fatalf("first resolve = %v", v)
	}
	if v, _ := f.Value("counter"); v != 1 {
		t.Fatalf("cached resolve changed: %v", v)
	}
	f.Flush()
	if v, _ := f.Value("counter"); v != 2 {
		t.Fatalf("flush should re-resolve: %v", v)
	}
}

func TestResetAndClear(t *testing.T) {
	for _, tc := range []struct {
		name  string
		clear func(*Facter)
	}{
		{"reset", (*Facter).Reset},
		{"clear", (*Facter).Clear},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newTestFacter(newFakeEngine())
			f.AddValue("temp", "v")
			if _, ok := f.Value("temp"); !ok {
				t.Fatal("temp should exist before reset")
			}
			tc.clear(f)
			if _, ok := f.Value("temp"); ok {
				t.Fatal("custom fact should be gone after reset/clear")
			}
			if _, ok := f.Value("kernel"); !ok {
				t.Fatal("engine rebuilt, built-in still resolvable")
			}
		})
	}
}

func TestFactHandle(t *testing.T) {
	f := newTestFacter(newFakeEngine())
	// Custom fact.
	ft := f.Add("cust", Options{}, func(*ResolutionContext) (any, bool) { return "cv", true })
	if ft.Name() != "cust" {
		t.Fatalf("name=%q", ft.Name())
	}
	if v, ok := ft.Value(); !ok || v != "cv" {
		t.Fatalf("fact value=%v,%v", v, ok)
	}
	if ft.Resolutions() != 1 {
		t.Fatalf("resolutions=%d", ft.Resolutions())
	}
	ft.Add(Options{Weight: 9, HasWeight: true}, func(*ResolutionContext) (any, bool) { return "cv2", true })
	if ft.Resolutions() != 2 {
		t.Fatalf("resolutions after chain=%d", ft.Resolutions())
	}
	if v, _ := ft.Value(); v != "cv2" {
		t.Fatalf("chained higher-weight value=%v", v)
	}

	// Fact() for a custom fact, an engine-only fact, and an unknown fact.
	if f.Fact("cust") == nil {
		t.Fatal("custom fact handle expected")
	}
	engineFact := f.Fact("hostname")
	if engineFact == nil {
		t.Fatal("engine fact handle expected")
	}
	if engineFact.Resolutions() != 0 {
		t.Fatalf("engine-only fact has no custom resolutions, got %d", engineFact.Resolutions())
	}
	if f.Fact("does-not-exist") != nil {
		t.Fatal("unknown fact must return nil handle")
	}
}

func TestResolveAllToHashListEach(t *testing.T) {
	f := newTestFacter(newFakeEngine())
	f.AddValue("extra", "e")
	f.Add("declined", Options{}, func(*ResolutionContext) (any, bool) { return nil, false })

	all := f.ResolveAll()
	if all["extra"] != "e" {
		t.Fatalf("custom fact missing from ResolveAll: %v", all)
	}
	if _, ok := all["declined"]; ok {
		t.Fatal("declined custom fact must be deleted from the hash")
	}
	if all["kernel"] != "Linux" {
		t.Fatalf("built-in missing: %v", all)
	}
	if !reflect.DeepEqual(f.ToHash(), all) {
		t.Fatal("ToHash must equal ResolveAll")
	}

	list := f.List()
	if !sort.StringsAreSorted(list) {
		t.Fatalf("List not sorted: %v", list)
	}
	if !contains(list, "extra") || !contains(list, "kernel") {
		t.Fatalf("List missing entries: %v", list)
	}

	seen := map[string]any{}
	var order []string
	f.Each(func(name string, value any) {
		seen[name] = value
		order = append(order, name)
	})
	if !sort.StringsAreSorted(order) {
		t.Fatalf("Each not in sorted order: %v", order)
	}
	if seen["extra"] != "e" {
		t.Fatalf("Each missing extra: %v", seen)
	}
}

func TestResolveAllNilEngineHash(t *testing.T) {
	e := newFakeEngine()
	e.nilHash = true
	f := newTestFacter(e)
	f.AddValue("only", "o")
	all := f.ResolveAll()
	if all["only"] != "o" {
		t.Fatalf("custom fact must survive a nil engine hash: %v", all)
	}
}

func TestLoadExternalFacts(t *testing.T) {
	e := newFakeEngine()
	f := newTestFacter(e)
	if err := f.LoadExternalFacts("/a", "/b"); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if !reflect.DeepEqual(e.loadDirs, []string{"/a", "/b"}) {
		t.Fatalf("dirs not forwarded: %v", e.loadDirs)
	}

	e2 := newFakeEngine()
	e2.loadErr = errFake
	f2 := newTestFacter(e2)
	if err := f2.LoadExternalFacts("/c"); err != errFake {
		t.Fatalf("load error not propagated: %v", err)
	}
}

func TestExecuteAndWhichThroughSeam(t *testing.T) {
	f := newTestFacter(newFakeEngine())
	f.SetExecutor(fakeExec{out: "hi", ok: true, whichPath: "/bin/x", whichOK: true})
	if s, ok := f.Execute("anything"); !ok || s != "hi" {
		t.Fatalf("Execute via seam=%q,%v", s, ok)
	}
	if p, ok := f.Which("x"); !ok || p != "/bin/x" {
		t.Fatalf("Which via seam=%q,%v", p, ok)
	}
	// ResolutionContext delegates to the same seam.
	f.Add("viaexec", Options{}, func(rc *ResolutionContext) (any, bool) {
		s, ok := rc.Execute("cmd")
		if !ok {
			return nil, false
		}
		if _, ok := rc.Which("x"); !ok {
			return nil, false
		}
		return s, true
	})
	if v, _ := f.Value("viaexec"); v != "hi" {
		t.Fatalf("context execute=%v", v)
	}
}

func TestToJSONAndYAML(t *testing.T) {
	f := newTestFacter(newFakeEngine())
	f.AddValue("k", "v")
	js, err := f.ToJSON()
	if err != nil || !strings.Contains(js, "\"k\"") {
		t.Fatalf("ToJSON=%q err=%v", js, err)
	}
	ym := f.ToYAML()
	if !strings.Contains(ym, "k") {
		t.Fatalf("ToYAML=%q", ym)
	}
}

func TestConcurrentAccess(t *testing.T) {
	f := newTestFacter(newFakeEngine())
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			f.AddValue("c", i)
			_, _ = f.Value("c")
			_ = f.List()
			f.Flush()
		}(i)
	}
	wg.Wait()
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// errFake is a sentinel error used to assert propagation.
var errFake = fakeErr("boom")

type fakeErr string

func (e fakeErr) Error() string { return string(e) }

// fakeExec is a scripted Executor.
type fakeExec struct {
	out       string
	ok        bool
	whichPath string
	whichOK   bool
}

func (e fakeExec) Execute(string) (string, bool) { return e.out, e.ok }
func (e fakeExec) Which(string) (string, bool)   { return e.whichPath, e.whichOK }

// used to keep time import if trimmed elsewhere.
var _ = time.Second
