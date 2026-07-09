<p align="center">
  <img src="https://raw.githubusercontent.com/go-ruby-facter/brand/main/social/go-ruby-facter.png" alt="go-ruby-facter" width="640">
</p>

# go-ruby-facter

[![ci](https://github.com/go-ruby-facter/facter/actions/workflows/ci.yml/badge.svg)](https://github.com/go-ruby-facter/facter/actions/workflows/ci.yml)
[![coverage](https://img.shields.io/badge/coverage-100%25-brightgreen)](https://github.com/go-ruby-facter/facter/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/go-ruby-facter/facter.svg)](https://pkg.go.dev/github.com/go-ruby-facter/facter)
[![License: BSD-3-Clause](https://img.shields.io/badge/License-BSD--3--Clause-blue.svg)](LICENSE)

A pure-Go (CGO-free) implementation of the **Ruby [Facter](https://github.com/puppetlabs/facter) API semantics**, layered as an importable adapter over the [`go-facter`](https://github.com/go-facter/facter) system-inventory engine.

`go-facter` discovers the built-in structured facts (os, kernel, networking, processors, memory, filesystems, virtualisation, uptime, identity) and resolves simple custom and external facts. **This package adds the parts of Ruby Facter that a generic inventory engine does not model:**

- the module surface — `Value`/`[]`, `Fact`, `List`, `ToHash`, `Each`;
- `Add` with full **resolution semantics** — several resolutions per fact, `:weight`/`has_weight` ordering, `:confine` predicates (fact, env and block confines), `:timeout`, and **aggregate** facts (chunks + an aggregate/merge step);
- `Facter::Core::Execution`-style `Execute`/`Which` behind an injectable `Executor` seam;
- `clear`/`reset`/`flush` cache semantics;
- external- and custom-fact search-path loading, delegated to the engine.

Resolution follows Ruby Facter 4: for a fact, the highest-weight resolution whose confines all match and whose code returns a value wins; ties break by declaration order. A fact with no matching custom resolution falls back to the engine's built-in value.

## Ruby → Go API map

| Ruby | Go |
|------|-----|
| `Facter.value(:x)` / `Facter[:x]` | `(*Facter).Value("x")` |
| `Facter.fact(:x)` | `(*Facter).Fact("x")` |
| `Facter.add(:x) { … }` | `(*Facter).Add("x", Options{…}, ResolveFunc)` |
| `Facter.add(:x, :type => :aggregate)` | `(*Facter).AddAggregate("x", Options{…}, Aggregate{…})` |
| `Facter.to_hash` | `(*Facter).ToHash()` (alias of `ResolveAll`) |
| `Facter.list` / `Facter.each` | `(*Facter).List()` / `(*Facter).Each(fn)` |
| `Facter.flush` / `Facter.reset` / `Facter.clear` | `(*Facter).Flush()` / `Reset()` / `Clear()` |
| `Facter::Core::Execution.execute` / `.which` | `(*Facter).Execute` / `Which` |
| `confine :x => v` / `confine(:x){…}` / `confine{…}` | `ConfineFact` / `ConfineFactFunc` / `ConfineBlock` |

## Usage

```go
f := facter.New() // backed by a real go-facter engine

f.Add("role", facter.Options{
    Confine: []facter.Confine{facter.ConfineFact("os.family", "Debian")},
}, func(rc *facter.ResolutionContext) (any, bool) {
    return "web", true
})

v, _ := f.Value("os.name")   // built-in fact
role, _ := f.Value("role")   // custom fact
all := f.ResolveAll()        // nested map[string]any, ready to marshal
```

## Relationship to rbgo

This package has **no dependency on any Ruby runtime**. Custom resolvers are Go-typed (`ResolveFunc` + `Confine` predicates). A consumer such as [go-embedded-ruby](https://github.com/go-embedded-ruby/ruby) (rbgo) adapts a Ruby `Facter.add … do … end` block into those Go values, marshals results, and wires a Ruby `Facter` constant onto this adapter. `ResolveAll` is the documented integration point.

## License

BSD-3-Clause — see [LICENSE](LICENSE).
