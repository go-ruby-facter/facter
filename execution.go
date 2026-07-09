// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-facter/facter authors

package facter

import (
	"os/exec"
	"strings"
)

// Executor is the Facter::Core::Execution back end: it runs a command and locates
// a binary. It is an injectable seam so custom facts that shell out stay testable
// without invoking real binaries. SetExecutor replaces it.
type Executor interface {
	// Execute runs command and returns its trimmed stdout, or ("", false) on any
	// failure (empty command, launch error, non-zero exit).
	Execute(command string) (string, bool)
	// Which returns the resolved path of binary on PATH, or ("", false) when it is
	// not found.
	Which(binary string) (string, bool)
}

// osExecutor is the production Executor: it runs commands with os/exec and
// resolves binaries with exec.LookPath.
type osExecutor struct{}

// Execute splits command on whitespace and runs it, capturing stdout. It performs
// no shell interpretation; a caller needing shell semantics supplies its own
// Executor.
func (osExecutor) Execute(command string) (string, bool) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return "", false
	}
	out, err := exec.Command(fields[0], fields[1:]...).Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

// Which resolves binary on PATH.
func (osExecutor) Which(binary string) (string, bool) {
	p, err := exec.LookPath(binary)
	if err != nil {
		return "", false
	}
	return p, true
}
