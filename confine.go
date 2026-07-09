// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-facter/facter authors

package facter

import (
	"fmt"
	"os"
	"strings"
)

// Confine gates a resolution: it is consulted against the owning Facter and must
// return true for the resolution to be eligible. It is the Go analogue of a Ruby
// "confine" declaration.
type Confine func(f *Facter) bool

// sprint renders any value the way Ruby's to_s would for the common scalar cases,
// used for case-insensitive confine and value comparison.
func sprint(v any) string { return fmt.Sprint(v) }

// matchAny reports whether v equals any of allowed, compared case-insensitively
// on their string forms — matching how Ruby confines compare a fact to its
// allowed values.
func matchAny(v any, allowed []any) bool {
	s := sprint(v)
	for _, a := range allowed {
		if strings.EqualFold(s, sprint(a)) {
			return true
		}
	}
	return false
}

// ConfineFact confines a resolution to hosts where fact name resolves to one of
// allowed. It is the Go analogue of Ruby's confine :name => value (or an array of
// values). With no allowed values it confines to name merely being present.
func ConfineFact(name string, allowed ...any) Confine {
	return func(f *Facter) bool {
		v, ok := f.Value(name)
		if !ok {
			return false
		}
		if len(allowed) == 0 {
			return true
		}
		return matchAny(v, allowed)
	}
}

// ConfineFactFunc confines on an arbitrary predicate over fact name's value. It is
// the Go analogue of Ruby's confine(:name) { |val| ... }. The resolution is gated
// out when the fact is absent.
func ConfineFactFunc(name string, pred func(v any) bool) Confine {
	return func(f *Facter) bool {
		v, ok := f.Value(name)
		if !ok {
			return false
		}
		return pred(v)
	}
}

// ConfineEnv confines on an environment variable's value being one of allowed
// (case-insensitively). With no allowed values it confines to the variable being
// set and non-empty.
func ConfineEnv(key string, allowed ...string) Confine {
	return func(f *Facter) bool {
		v, ok := os.LookupEnv(key)
		if !ok {
			return false
		}
		if len(allowed) == 0 {
			return v != ""
		}
		for _, a := range allowed {
			if strings.EqualFold(v, a) {
				return true
			}
		}
		return false
	}
}

// ConfineBlock confines on an arbitrary predicate over the Facter, the Go
// analogue of Ruby's confine { ... } block form.
func ConfineBlock(pred func(f *Facter) bool) Confine {
	return func(f *Facter) bool { return pred(f) }
}
