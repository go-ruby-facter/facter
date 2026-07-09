// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-facter/facter authors

package facter

import "testing"

// missingBinary is a name that must not exist on PATH on any CI lane.
const missingBinary = "definitely-not-a-real-binary-xyz123"

func TestOSExecutorErrorPaths(t *testing.T) {
	e := osExecutor{}
	if _, ok := e.Execute(""); ok {
		t.Fatal("empty command must fail")
	}
	if _, ok := e.Execute("   \t "); ok {
		t.Fatal("whitespace-only command must fail")
	}
	if _, ok := e.Execute(missingBinary + " arg"); ok {
		t.Fatal("launching a missing binary must fail")
	}
	if _, ok := e.Which(missingBinary); ok {
		t.Fatal("resolving a missing binary must fail")
	}
}
