// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-facter/facter authors

package facter

import (
	"reflect"
	"testing"
)

func TestAggregateWithMerge(t *testing.T) {
	f := newTestFacter(newFakeEngine())
	f.AddAggregate("agg", Options{}, Aggregate{
		Chunks: map[string]ChunkFunc{
			"a": func(*ResolutionContext) (any, bool) { return 1, true },
			"b": func(*ResolutionContext) (any, bool) { return 2, true },
		},
		Merge: func(chunks map[string]any) (any, bool) {
			return chunks["a"].(int) + chunks["b"].(int), true
		},
	})
	if v, ok := f.Value("agg"); !ok || v != 3 {
		t.Fatalf("merged aggregate = %v,%v", v, ok)
	}
}

func TestAggregateMergeDeclines(t *testing.T) {
	f := newTestFacter(newFakeEngine())
	f.AddAggregate("none", Options{}, Aggregate{
		Chunks: map[string]ChunkFunc{"a": func(*ResolutionContext) (any, bool) { return 1, true }},
		Merge:  func(map[string]any) (any, bool) { return nil, false },
	})
	if _, ok := f.Value("none"); ok {
		t.Fatal("aggregate whose merge declines must not resolve")
	}
}

func TestAggregateDeepMergeDefault(t *testing.T) {
	f := newTestFacter(newFakeEngine())
	f.AddAggregate("deep", Options{}, Aggregate{
		Chunks: map[string]ChunkFunc{
			"a": func(*ResolutionContext) (any, bool) {
				return map[string]any{"x": 1, "shared": map[string]any{"p": 1}}, true
			},
			"b": func(*ResolutionContext) (any, bool) {
				return map[string]any{"y": 2, "shared": map[string]any{"q": 2}}, true
			},
		},
	})
	v, ok := f.Value("deep")
	if !ok {
		t.Fatal("deep aggregate must resolve")
	}
	want := map[string]any{"x": 1, "y": 2, "shared": map[string]any{"p": 1, "q": 2}}
	if !reflect.DeepEqual(v, want) {
		t.Fatalf("deep merge = %v, want %v", v, want)
	}
}

func TestAggregateDeepMergeSkipsDeclinedChunk(t *testing.T) {
	// A declining chunk in the default (no-Merge) deep-merge path must be skipped
	// while the contributing chunks still merge.
	f := newTestFacter(newFakeEngine())
	f.AddAggregate("mix", Options{}, Aggregate{
		Chunks: map[string]ChunkFunc{
			"a": func(*ResolutionContext) (any, bool) { return map[string]any{"x": 1}, true },
			"b": func(*ResolutionContext) (any, bool) { return nil, false },
			"c": func(*ResolutionContext) (any, bool) { return map[string]any{"y": 2}, true },
		},
	})
	v, ok := f.Value("mix")
	if !ok {
		t.Fatal("aggregate with one declining chunk must still resolve")
	}
	if !reflect.DeepEqual(v, map[string]any{"x": 1, "y": 2}) {
		t.Fatalf("deep merge skipping declined chunk = %v", v)
	}
}

func TestAggregateEmptyDeclines(t *testing.T) {
	f := newTestFacter(newFakeEngine())
	f.AddAggregate("empty", Options{}, Aggregate{
		Chunks: map[string]ChunkFunc{"a": func(*ResolutionContext) (any, bool) { return nil, false }},
	})
	if _, ok := f.Value("empty"); ok {
		t.Fatal("aggregate with no contributing chunk must not resolve")
	}
}

func TestAggregateOrderRespectsRequires(t *testing.T) {
	var seq []string
	agg := &Aggregate{
		Chunks: map[string]ChunkFunc{
			"first":  func(*ResolutionContext) (any, bool) { seq = append(seq, "first"); return 1, true },
			"second": func(*ResolutionContext) (any, bool) { seq = append(seq, "second"); return 2, true },
		},
		// "first" requires "second", and a requirement naming a non-chunk which
		// must be ignored.
		Requires: map[string][]string{"first": {"second", "not-a-chunk"}},
	}
	if _, ok := agg.compute(&ResolutionContext{}); !ok {
		t.Fatal("aggregate should compute")
	}
	if !reflect.DeepEqual(seq, []string{"second", "first"}) {
		t.Fatalf("dependency order not honoured: %v", seq)
	}
}

func TestAggregateOrderCycleMakesProgress(t *testing.T) {
	agg := &Aggregate{
		Chunks: map[string]ChunkFunc{
			"a": func(*ResolutionContext) (any, bool) { return 1, true },
			"b": func(*ResolutionContext) (any, bool) { return 2, true },
		},
		Requires: map[string][]string{"a": {"b"}, "b": {"a"}}, // cycle
	}
	order := agg.order()
	if len(order) != 2 {
		t.Fatalf("cycle order must still emit every chunk: %v", order)
	}
}

func TestDeepMerge(t *testing.T) {
	// slices concatenate
	if got := deepMerge([]any{1, 2}, []any{3}); !reflect.DeepEqual(got, []any{1, 2, 3}) {
		t.Fatalf("slice concat = %v", got)
	}
	// scalar: b wins
	if got := deepMerge(1, 2); got != 2 {
		t.Fatalf("scalar merge = %v", got)
	}
	// map + non-map: b wins
	if got := deepMerge(map[string]any{"a": 1}, 9); got != 9 {
		t.Fatalf("mixed merge = %v", got)
	}
	// nested map recursion with a non-shared and a shared key
	got := deepMerge(map[string]any{"a": 1}, map[string]any{"a": 2, "b": 3})
	if !reflect.DeepEqual(got, map[string]any{"a": 2, "b": 3}) {
		t.Fatalf("map merge = %v", got)
	}
}
