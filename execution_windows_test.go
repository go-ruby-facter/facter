// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-facter/facter authors

//go:build windows

package facter

import "testing"

// On Windows the production Executor is exercised against cmd, which is always
// present and resolvable on PATH.
func TestOSExecutorWindowsSuccess(t *testing.T) {
	e := osExecutor{}
	if out, ok := e.Execute("cmd /c echo hello"); !ok || out != "hello" {
		t.Fatalf("Execute(cmd) = %q,%v", out, ok)
	}
	if _, ok := e.Which("cmd"); !ok {
		t.Fatal("cmd should resolve on PATH")
	}
}
