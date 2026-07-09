// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-facter/facter authors

package facter

import (
	"testing"
	"time"
)

func TestEffectiveWeight(t *testing.T) {
	explicit := Options{Weight: 7, HasWeight: true}.resolution(nil, nil)
	if explicit.effectiveWeight() != 7 {
		t.Fatalf("explicit weight = %d", explicit.effectiveWeight())
	}
	byConfines := Options{Confine: []Confine{
		ConfineBlock(func(*Facter) bool { return true }),
		ConfineBlock(func(*Facter) bool { return true }),
	}}.resolution(nil, nil)
	if byConfines.effectiveWeight() != 2 {
		t.Fatalf("confine-count weight = %d", byConfines.effectiveWeight())
	}
}

func TestRunWithTimeout(t *testing.T) {
	// d <= 0 runs inline.
	if v, ok := runWithTimeout(func() (any, bool) { return "inline", true }, 0); !ok || v != "inline" {
		t.Fatalf("inline run = %v,%v", v, ok)
	}
	// d > 0 and fn finishes first.
	if v, ok := runWithTimeout(func() (any, bool) { return "fast", true }, time.Hour); !ok || v != "fast" {
		t.Fatalf("fast run = %v,%v", v, ok)
	}
	// d > 0 and the timeout fires first (deterministic via the timer seam).
	orig := timeoutAfter
	fired := make(chan time.Time, 1)
	fired <- time.Time{}
	timeoutAfter = func(time.Duration) <-chan time.Time { return fired }
	block := make(chan struct{})
	defer func() { timeoutAfter = orig; close(block) }()
	if v, ok := runWithTimeout(func() (any, bool) { <-block; return "slow", true }, time.Millisecond); ok || v != nil {
		t.Fatalf("timed-out run should be nil,false, got %v,%v", v, ok)
	}
}

func TestResolutionRunAggregatePath(t *testing.T) {
	// run() dispatches to the aggregate compute when agg is set.
	r := Options{}.resolution(nil, &Aggregate{
		Chunks: map[string]ChunkFunc{"a": func(*ResolutionContext) (any, bool) { return "x", true }},
	})
	if v, ok := r.run(&ResolutionContext{}); !ok || v != "x" {
		t.Fatalf("aggregate run = %v,%v", v, ok)
	}
}
