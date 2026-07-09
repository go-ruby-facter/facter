// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-facter/facter authors

package facter

import "time"

// ResolveFunc produces a fact's value. It returns (value, true) when the fact
// resolves and (nil, false) when it does not on this host. It is the Go analogue
// of a Ruby resolution's setcode block; rbgo adapts a Ruby block into one of
// these.
type ResolveFunc func(ctx *ResolutionContext) (any, bool)

// ResolutionContext is handed to a resolver and to chunk functions. It is the Go
// analogue of the block self in a Ruby setcode block: it can read other facts and
// run commands through the same Facter.
type ResolutionContext struct {
	f *Facter
}

// Value reads another fact, so one resolution can depend on another.
func (rc *ResolutionContext) Value(path string) (any, bool) { return rc.f.Value(path) }

// ValueString reads another fact as a string.
func (rc *ResolutionContext) ValueString(path string) (string, bool) { return rc.f.ValueString(path) }

// Execute runs a command through the Facter's execution seam.
func (rc *ResolutionContext) Execute(command string) (string, bool) { return rc.f.Execute(command) }

// Which resolves a binary on PATH through the Facter's execution seam.
func (rc *ResolutionContext) Which(binary string) (string, bool) { return rc.f.Which(binary) }

// Options carries the per-resolution knobs of a Ruby Facter.add block.
type Options struct {
	// Weight sets an explicit resolution weight; it only takes effect when
	// HasWeight is true, mirroring Ruby's has_weight. Higher wins.
	Weight int
	// HasWeight marks Weight as explicit. When false, the resolution's weight is
	// the number of its confines, exactly as Ruby Facter defaults it.
	HasWeight bool
	// Timeout bounds how long the resolver may run; zero means no bound. A
	// resolution that times out contributes no value and the next one is tried.
	Timeout time.Duration
	// Confine lists predicates that must all pass for the resolution to be
	// considered.
	Confine []Confine
}

// resolution binds one way of resolving a fact together with its gating and
// ordering metadata. Exactly one of resolve or agg is set.
type resolution struct {
	weight    int
	hasWeight bool
	timeout   time.Duration
	confines  []Confine
	resolve   ResolveFunc
	agg       *Aggregate
	seq       int
}

// resolution builds an internal resolution from the options plus either a simple
// resolver or an aggregate spec.
func (o Options) resolution(resolve ResolveFunc, agg *Aggregate) *resolution {
	return &resolution{
		weight:    o.Weight,
		hasWeight: o.HasWeight,
		timeout:   o.Timeout,
		confines:  o.Confine,
		resolve:   resolve,
		agg:       agg,
	}
}

// effectiveWeight is the explicit weight when has_weight is set, otherwise the
// number of confines — Ruby Facter's default weighting.
func (r *resolution) effectiveWeight() int {
	if r.hasWeight {
		return r.weight
	}
	return len(r.confines)
}

// confinesPass reports whether every confine on the resolution matches.
func (r *resolution) confinesPass(f *Facter) bool {
	for _, c := range r.confines {
		if !c(f) {
			return false
		}
	}
	return true
}

// run evaluates the resolution (simple or aggregate) under its timeout.
func (r *resolution) run(ctx *ResolutionContext) (any, bool) {
	fn := func() (any, bool) {
		if r.agg != nil {
			return r.agg.compute(ctx)
		}
		return r.resolve(ctx)
	}
	return runWithTimeout(fn, r.timeout)
}

// timeoutAfter is the timer seam; tests override it to force the timeout branch
// deterministically.
var timeoutAfter = time.After

// runWithTimeout runs fn, abandoning it and returning (nil, false) if it does not
// finish within d. A zero d runs fn inline with no timer.
func runWithTimeout(fn func() (any, bool), d time.Duration) (any, bool) {
	if d <= 0 {
		return fn()
	}
	type result struct {
		v  any
		ok bool
	}
	ch := make(chan result, 1)
	go func() {
		v, ok := fn()
		ch <- result{v, ok}
	}()
	select {
	case r := <-ch:
		return r.v, r.ok
	case <-timeoutAfter(d):
		return nil, false
	}
}
