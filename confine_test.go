// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-facter/facter authors

package facter

import "testing"

func newConfineFacter() *Facter {
	f := newTestFacter(newFakeEngine())
	f.AddValue("num", 5)
	return f
}

func TestConfineFact(t *testing.T) {
	f := newConfineFacter()
	if !ConfineFact("os.name", "Debian")(f) {
		t.Fatal("matching value should pass")
	}
	if !ConfineFact("os.name", "RedHat", "Debian")(f) {
		t.Fatal("match among several should pass")
	}
	if ConfineFact("os.name", "Windows")(f) {
		t.Fatal("non-matching value should fail")
	}
	if !ConfineFact("os.name")(f) {
		t.Fatal("presence-only confine should pass when fact exists")
	}
	if ConfineFact("absent")(f) {
		t.Fatal("confine on an absent fact should fail")
	}
	// case-insensitive comparison on string forms
	if !ConfineFact("num", "5")(f) {
		t.Fatal("case/string-form comparison should pass")
	}
}

func TestConfineFactFunc(t *testing.T) {
	f := newConfineFacter()
	if !ConfineFactFunc("num", func(v any) bool { return v.(int) == 5 })(f) {
		t.Fatal("predicate true should pass")
	}
	if ConfineFactFunc("num", func(v any) bool { return v.(int) == 6 })(f) {
		t.Fatal("predicate false should fail")
	}
	if ConfineFactFunc("absent", func(any) bool { return true })(f) {
		t.Fatal("absent fact should gate out regardless of predicate")
	}
}

func TestConfineEnv(t *testing.T) {
	f := newConfineFacter()
	t.Setenv("RUBYFACTER_TEST_ENV", "prod")
	if !ConfineEnv("RUBYFACTER_TEST_ENV", "PROD")(f) {
		t.Fatal("case-insensitive env match should pass")
	}
	if ConfineEnv("RUBYFACTER_TEST_ENV", "dev")(f) {
		t.Fatal("non-matching env should fail")
	}
	if !ConfineEnv("RUBYFACTER_TEST_ENV")(f) {
		t.Fatal("set, non-empty, no-allowed confine should pass")
	}
	t.Setenv("RUBYFACTER_TEST_EMPTY", "")
	if ConfineEnv("RUBYFACTER_TEST_EMPTY")(f) {
		t.Fatal("set-but-empty with no allowed should fail")
	}
	if ConfineEnv("RUBYFACTER_UNSET_VAR_XYZ")(f) {
		t.Fatal("unset env should fail")
	}
}

func TestConfineBlock(t *testing.T) {
	f := newConfineFacter()
	if !ConfineBlock(func(*Facter) bool { return true })(f) {
		t.Fatal("true block should pass")
	}
	if ConfineBlock(func(*Facter) bool { return false })(f) {
		t.Fatal("false block should fail")
	}
}
