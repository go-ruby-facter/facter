// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-facter/facter authors

// Package facter is a pure-Go (no cgo) implementation of the Ruby Facter API
// semantics, layered as an importable adapter over the system-inventory engine
// github.com/go-facter/facter.
//
// The go-facter engine discovers the built-in structured facts (os, kernel,
// networking, processors, memory, filesystems, virtualisation, uptime, identity)
// and resolves simple custom facts and external facts. This package adds the
// parts of Ruby's Facter that a generic inventory engine does not model:
//
//   - the module surface — Value/[] , Fact, List, ToHash, Each;
//   - Add with full resolution semantics — several resolutions per fact, :weight
//     / has_weight ordering, :confine predicates (fact, env and block confines)
//     that gate a resolution, :timeout, and aggregate facts (chunks plus an
//     aggregate/merge step);
//   - Facter::Core::Execution-style Execute / Which helpers behind an injectable
//     Executor seam;
//   - clear / reset / flush cache semantics;
//   - external- and custom-fact search-path loading, delegated to the engine.
//
// Resolution order follows Ruby Facter 4: for a given fact the resolution with
// the highest weight whose confines all match and whose code returns a value
// wins; ties break by declaration order. A fact with no matching custom
// resolution falls back to the engine's built-in value, so custom resolutions
// override built-ins exactly when their weight lets them.
//
// The package has no dependency on any Ruby runtime. Custom resolvers are
// Go-typed (a ResolveFunc plus Confine predicates); a consumer such as
// go-embedded-ruby (rbgo) adapts a Ruby "Facter.add ... do ... end" block into
// those Go values and marshals results, then wires a Ruby Facter constant onto
// this adapter. ResolveAll is the documented integration point: it returns the
// fully resolved fact set as a nested map[string]any ready for marshalling.
package facter
