// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-facter/facter authors

//go:build !windows

package facter

import "testing"

// On Unix the production Executor is exercised against /bin/echo (present on
// Linux and macOS, and runnable under qemu-user via binfmt on the arch lanes)
// and /bin/sh for the PATH lookup.
func TestOSExecutorUnixSuccess(t *testing.T) {
	e := osExecutor{}
	if out, ok := e.Execute("/bin/echo hello world"); !ok || out != "hello world" {
		t.Fatalf("Execute(/bin/echo) = %q,%v", out, ok)
	}
	if _, ok := e.Which("sh"); !ok {
		t.Fatal("sh should resolve on PATH")
	}
}
