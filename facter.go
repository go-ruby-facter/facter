// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-facter/facter authors

package facter

import (
	"sort"
	"strings"
	"sync"

	engine "github.com/go-facter/facter"
)

// Version is the Ruby Facter major line whose semantics this adapter matches.
const Version = "4.0.0"

// Engine is the system-inventory back end the adapter resolves built-in facts
// against. *github.com/go-facter/facter.Collection satisfies it; tests inject a
// fake. It is the seam between the Ruby-Facter semantics implemented here and the
// generic fact engine underneath.
type Engine interface {
	// Value resolves a fact by dotted path (Facter.value / Facter[]).
	Value(path string) (any, bool)
	// ToHash resolves every built-in fact into one nested map.
	ToHash() map[string]any
	// Names lists the registered built-in fact names.
	Names() []string
	// LoadExternalFacts loads external (out-of-process) facts from dirs.
	LoadExternalFacts(dirs ...string) error
}

// cacheEntry memoises a resolved top-level fact for the life of a Facter, until
// Flush or Reset clears it. present distinguishes a resolved nil from absence.
type cacheEntry struct {
	value   any
	present bool
}

// factEntry is a named fact together with the custom resolutions declared for it.
// A fact may have no entry here (built-in only), an entry with resolutions
// (Facter.add blocks), or both — in which case the resolutions can override the
// built-in when their weight allows.
type factEntry struct {
	name        string
	resolutions []*resolution
}

// Facter is the Ruby Facter module surface. It owns an Engine for built-in
// facts, a registry of custom resolutions, one per-run value cache and one
// execution seam. It is the object a Ruby binding wraps to expose Facter.value,
// Facter.add, Facter.to_hash and friends. All methods are safe for concurrent
// use.
type Facter struct {
	mu        sync.Mutex
	engine    Engine
	newEngine func() Engine
	facts     map[string]*factEntry
	order     []string
	cache     map[string]cacheEntry
	exec      Executor
	seq       int
}

// New returns a Facter backed by a fresh go-facter engine with every built-in
// fact group registered against the real operating system. It is the entry point
// for production use and the object rbgo binds a Ruby Facter constant onto.
func New() *Facter {
	return NewWithEngine(func() Engine { return engine.New() })
}

// NewWithEngine returns a Facter backed by the engine produced by newEngine. The
// factory (rather than a single instance) is stored so Reset can rebuild a clean
// engine. It is the seam tests use to drive the adapter against a fake engine.
func NewWithEngine(newEngine func() Engine) *Facter {
	f := &Facter{
		newEngine: newEngine,
		engine:    newEngine(),
		facts:     map[string]*factEntry{},
		cache:     map[string]cacheEntry{},
		exec:      osExecutor{},
	}
	return f
}

// SetExecutor replaces the Facter::Core::Execution back end, so a caller (or a
// test) can supply command results without touching real binaries.
func (f *Facter) SetExecutor(e Executor) {
	f.mu.Lock()
	f.exec = e
	f.mu.Unlock()
}

// entry returns the fact entry for name, creating and registering it if absent.
// Caller must hold f.mu.
func (f *Facter) entry(name string) *factEntry {
	e, ok := f.facts[name]
	if !ok {
		e = &factEntry{name: name}
		f.facts[name] = e
		f.order = append(f.order, name)
	}
	return e
}

// addResolution appends one resolution to name's entry and invalidates any cached
// value for it. It is the shared core behind Add, AddValue, AddAggregate and
// (*Fact).Add.
func (f *Facter) addResolution(name string, r *resolution) *Fact {
	name = strings.ToLower(name)
	f.mu.Lock()
	r.seq = f.seq
	f.seq++
	e := f.entry(name)
	e.resolutions = append(e.resolutions, r)
	delete(f.cache, name)
	f.mu.Unlock()
	return &Fact{name: name, f: f}
}

// Add registers a resolution for name resolved by resolve, honouring opts
// (weight, confines, timeout). It is the Go surface behind Ruby's
// Facter.add(name) { ... }. Calling Add again for the same name adds another
// resolution; the highest-weight matching one wins. It returns a Fact handle for
// chaining further resolutions.
func (f *Facter) Add(name string, opts Options, resolve ResolveFunc) *Fact {
	return f.addResolution(name, opts.resolution(resolve, nil))
}

// AddValue registers a fact with a constant value — the degenerate custom fact
// (Facter.add(:x) { setcode { v } } with no logic).
func (f *Facter) AddValue(name string, value any) *Fact {
	return f.Add(name, Options{}, func(*ResolutionContext) (any, bool) { return value, true })
}

// AddAggregate registers an aggregate fact: a set of chunks whose results are
// combined by spec.Merge (or deep-merged when Merge is nil). It is the surface
// behind Ruby's Facter.add(:x, :type => :aggregate) with chunk / aggregate
// blocks.
func (f *Facter) AddAggregate(name string, opts Options, spec Aggregate) *Fact {
	s := spec
	return f.addResolution(name, opts.resolution(nil, &s))
}

// Fact returns a handle for name, or nil when neither a custom resolution nor a
// built-in fact provides it. It mirrors Ruby's Facter.fact / Facter[] (which
// return nil for an unknown fact).
func (f *Facter) Fact(name string) *Fact {
	name = strings.ToLower(name)
	f.mu.Lock()
	_, custom := f.facts[name]
	f.mu.Unlock()
	if custom {
		return &Fact{name: name, f: f}
	}
	if _, ok := f.engine.Value(name); ok {
		return &Fact{name: name, f: f}
	}
	return nil
}

// resolveTop resolves a single top-level fact by name, preferring a matching
// custom resolution and falling back to the engine's built-in value. The result
// is memoised until Flush or Reset. Resolution runs without f.mu held so a
// confine or resolver may safely read other facts.
func (f *Facter) resolveTop(name string) (any, bool) {
	f.mu.Lock()
	if ce, ok := f.cache[name]; ok {
		f.mu.Unlock()
		return ce.value, ce.present
	}
	e := f.facts[name]
	f.mu.Unlock()

	var v any
	var ok bool
	if e != nil {
		v, ok = f.resolveEntry(e)
	}
	if !ok {
		v, ok = f.engine.Value(name)
	}

	f.mu.Lock()
	f.cache[name] = cacheEntry{value: v, present: ok}
	f.mu.Unlock()
	return v, ok
}

// resolveEntry evaluates a fact's custom resolutions in Facter's precedence
// order — highest weight first, declaration order breaking ties — returning the
// first confined-and-matching resolution's value.
func (f *Facter) resolveEntry(e *factEntry) (any, bool) {
	f.mu.Lock()
	rs := make([]*resolution, len(e.resolutions))
	copy(rs, e.resolutions)
	f.mu.Unlock()

	sort.SliceStable(rs, func(i, j int) bool {
		return rs[i].effectiveWeight() > rs[j].effectiveWeight()
	})

	ctx := &ResolutionContext{f: f}
	for _, r := range rs {
		if !r.confinesPass(f) {
			continue
		}
		if v, ok := r.run(ctx); ok {
			return v, true
		}
	}
	return nil, false
}

// Value resolves a fact by dotted path: the first segment names a fact and any
// further segments descend into a structured fact's nested maps, so
// Value("os.name") works. It is the surface behind Ruby's Facter.value /
// Facter[].
func (f *Facter) Value(path string) (any, bool) {
	if path == "" {
		return nil, false
	}
	segs := strings.Split(path, ".")
	top, ok := f.resolveTop(strings.ToLower(segs[0]))
	if !ok {
		return nil, false
	}
	return descend(top, segs[1:])
}

// ValueString is Value coerced to a string, the common case for a Ruby caller.
// The bool reports presence, not emptiness.
func (f *Facter) ValueString(path string) (string, bool) {
	v, ok := f.Value(path)
	if !ok {
		return "", false
	}
	return stringify(v), true
}

// descend walks the remaining dotted-path segments into nested maps.
func descend(v any, segs []string) (any, bool) {
	for _, s := range segs {
		m, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok = m[s]
		if !ok {
			return nil, false
		}
	}
	return v, true
}

// stringify renders a resolved value the way a Ruby to_s would for the common
// scalar cases.
func stringify(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		return sprint(v)
	}
}

// ResolveAll resolves every known fact — built-ins from the engine plus every
// custom fact — into one nested map. It is the documented integration point a
// consumer (rbgo) marshals; ToHash is its Ruby-named alias.
func (f *Facter) ResolveAll() map[string]any {
	out := f.engine.ToHash()
	if out == nil {
		out = map[string]any{}
	}
	f.mu.Lock()
	names := append([]string(nil), f.order...)
	f.mu.Unlock()
	for _, name := range names {
		if v, ok := f.resolveTop(name); ok {
			out[name] = v
		} else {
			delete(out, name)
		}
	}
	return out
}

// ToHash is the Ruby Facter.to_hash surface; it returns the same map as
// ResolveAll.
func (f *Facter) ToHash() map[string]any { return f.ResolveAll() }

// List returns every known fact name — built-in and custom — sorted and
// de-duplicated. It is the surface behind Ruby's Facter.list.
func (f *Facter) List() []string {
	set := map[string]struct{}{}
	for _, n := range f.engine.Names() {
		set[n] = struct{}{}
	}
	f.mu.Lock()
	for _, n := range f.order {
		set[n] = struct{}{}
	}
	f.mu.Unlock()
	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Each resolves every fact and calls fn with each name and value, in sorted name
// order. It is the surface behind Ruby's Facter.each.
func (f *Facter) Each(fn func(name string, value any)) {
	all := f.ResolveAll()
	names := make([]string, 0, len(all))
	for n := range all {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fn(n, all[n])
	}
}

// LoadExternalFacts loads external facts from dirs, delegating file discovery and
// parsing to the engine. It is the surface behind Facter's external-fact search
// path.
func (f *Facter) LoadExternalFacts(dirs ...string) error {
	err := f.engine.LoadExternalFacts(dirs...)
	f.Flush()
	return err
}

// Flush clears the per-run value cache so the next query re-resolves. It is the
// surface behind Ruby's Facter.flush. Custom resolutions stay registered.
func (f *Facter) Flush() {
	f.mu.Lock()
	f.cache = map[string]cacheEntry{}
	f.mu.Unlock()
}

// Reset forgets every custom resolution, clears the cache and rebuilds a clean
// engine. It is the surface behind Ruby's Facter.reset.
func (f *Facter) Reset() {
	f.mu.Lock()
	f.facts = map[string]*factEntry{}
	f.order = nil
	f.cache = map[string]cacheEntry{}
	f.seq = 0
	f.engine = f.newEngine()
	f.mu.Unlock()
}

// Clear is Ruby's Facter.clear: it resets the collection and flushes cached
// values. Here it is equivalent to Reset.
func (f *Facter) Clear() { f.Reset() }

// Execute runs command through the execution seam and returns its trimmed
// stdout, or ("", false) on failure. It is the surface behind
// Facter::Core::Execution.execute.
func (f *Facter) Execute(command string) (string, bool) {
	f.mu.Lock()
	e := f.exec
	f.mu.Unlock()
	return e.Execute(command)
}

// Which resolves a binary on PATH, or ("", false) when absent. It is the surface
// behind Facter::Core::Execution.which.
func (f *Facter) Which(binary string) (string, bool) {
	f.mu.Lock()
	e := f.exec
	f.mu.Unlock()
	return e.Which(binary)
}

// ToJSON marshals the fully resolved fact set as JSON, reusing the engine's
// encoder.
func (f *Facter) ToJSON() (string, error) { return engine.MarshalJSON(f.ResolveAll()) }

// ToYAML marshals the fully resolved fact set as YAML, reusing the engine's
// encoder.
func (f *Facter) ToYAML() string { return engine.MarshalYAML(f.ResolveAll()) }

// Fact is a handle to a named fact, the Go analogue of Ruby's Facter::Util::Fact.
// It resolves through the owning Facter, so it always reflects the current
// registry and cache.
type Fact struct {
	name string
	f    *Facter
}

// Name returns the fact's name.
func (ft *Fact) Name() string { return ft.name }

// Value resolves the fact and returns its value and presence.
func (ft *Fact) Value() (any, bool) { return ft.f.Value(ft.name) }

// Add appends another resolution to this fact and returns the handle for
// chaining, mirroring repeated Facter.add blocks for the same fact.
func (ft *Fact) Add(opts Options, resolve ResolveFunc) *Fact {
	return ft.f.addResolution(ft.name, opts.resolution(resolve, nil))
}

// Resolutions reports how many custom resolutions are registered for the fact.
func (ft *Fact) Resolutions() int {
	ft.f.mu.Lock()
	defer ft.f.mu.Unlock()
	if e, ok := ft.f.facts[ft.name]; ok {
		return len(e.resolutions)
	}
	return 0
}
