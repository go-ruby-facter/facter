// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-facter/facter authors

package facter

import "sort"

// ChunkFunc computes one chunk of an aggregate fact. It returns (value, true)
// when the chunk produces data and (nil, false) when it has nothing to
// contribute. It is the Go analogue of a Ruby aggregate "chunk" block.
type ChunkFunc func(ctx *ResolutionContext) (any, bool)

// Aggregate describes an aggregate fact: named chunks, optional inter-chunk
// ordering, and a merge step. It is the Go analogue of Ruby's
// Facter::Core::Aggregate (chunk / aggregate blocks).
type Aggregate struct {
	// Chunks maps a chunk name to the function that computes it.
	Chunks map[string]ChunkFunc
	// Requires maps a chunk name to the chunk names that must run before it. It is
	// the Go analogue of a chunk's :require option and only affects ordering.
	Requires map[string][]string
	// Merge combines the resolved chunk results into the fact's value. When nil,
	// the results are deep-merged. It is the Go analogue of the aggregate block;
	// returning ok=false means the aggregate produced no value.
	Merge func(chunks map[string]any) (any, bool)
}

// compute runs the chunks in dependency order, collects the results and merges
// them.
func (a *Aggregate) compute(ctx *ResolutionContext) (any, bool) {
	results := map[string]any{}
	for _, name := range a.order() {
		if v, ok := a.Chunks[name](ctx); ok {
			results[name] = v
		}
	}
	if a.Merge != nil {
		return a.Merge(results)
	}
	if len(results) == 0 {
		return nil, false
	}
	var merged any
	first := true
	for _, name := range a.order() {
		v, ok := results[name]
		if !ok {
			continue
		}
		if first {
			merged = v
			first = false
			continue
		}
		merged = deepMerge(merged, v)
	}
	return merged, true
}

// order returns the chunk names in a deterministic, dependency-honouring order:
// names are considered alphabetically, and a name is emitted once all its
// still-pending requirements have been emitted. A dependency cycle (or a chunk
// requiring a missing chunk that never becomes ready) falls back to emitting the
// lowest remaining name so progress is always made.
func (a *Aggregate) order() []string {
	names := make([]string, 0, len(a.Chunks))
	for n := range a.Chunks {
		names = append(names, n)
	}
	sort.Strings(names)

	emitted := map[string]bool{}
	out := make([]string, 0, len(names))
	remaining := append([]string(nil), names...)
	for len(remaining) > 0 {
		pick := -1
		for i, n := range remaining {
			if a.ready(n, emitted) {
				pick = i
				break
			}
		}
		if pick == -1 {
			pick = 0 // cycle or unsatisfiable dependency: make progress anyway
		}
		n := remaining[pick]
		out = append(out, n)
		emitted[n] = true
		remaining = append(remaining[:pick], remaining[pick+1:]...)
	}
	return out
}

// ready reports whether every requirement of n that is itself a chunk has already
// been emitted. Requirements naming a non-chunk are ignored.
func (a *Aggregate) ready(n string, emitted map[string]bool) bool {
	for _, dep := range a.Requires[n] {
		if _, isChunk := a.Chunks[dep]; isChunk && !emitted[dep] {
			return false
		}
	}
	return true
}

// deepMerge merges b into a: two maps merge key by key (recursing on shared
// keys), two slices concatenate, and anything else takes b as the winner. It is
// the default aggregate merge.
func deepMerge(a, b any) any {
	am, aok := a.(map[string]any)
	bm, bok := b.(map[string]any)
	if aok && bok {
		out := map[string]any{}
		for k, v := range am {
			out[k] = v
		}
		for k, v := range bm {
			if existing, ok := out[k]; ok {
				out[k] = deepMerge(existing, v)
			} else {
				out[k] = v
			}
		}
		return out
	}
	as, aok := a.([]any)
	bs, bok := b.([]any)
	if aok && bok {
		return append(append([]any{}, as...), bs...)
	}
	return b
}
