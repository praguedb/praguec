# ADR-0002 — protopc is written in Go

- **Status:** accepted, 2026-09-12
- **Supersedes:** `docs/research/proto-to-messagepack-csharp.md` §11 ("Recommendation: C#")
- **Leading document:** [`docs/spec/protop.md`](../spec/protop.md)

## Context

§11 of the research doc recommended C#, and called it "close to
over-determined". It was — on its premises. All three have since failed:

| §11 assumed | now |
|---|---|
| protopc is a `protoc` plugin | [ADR-0001](0001-protop-is-its-own-language.md) removed `protoc` from the pipeline entirely |
| it ships *inside* a .NET build, as a Roslyn source generator reached through NuGet (§11.2, §11.3) | it runs **before** the build, the way `buf generate` does (#31, and the buf-shaped end goal on the map) |
| C# is the only backend | the spec plans several: `[go: json:"..."]` is in the annotation table at §3, *"when a Go backend exists"* |

The third is the one that decides it. protop is specified as a frontend with
pluggable backends — LLVM-shaped: source is parsed and checked once into a
resolved schema, and backends see only that. A frontend hosted in the language
of one of its backends has an asymmetry that is permanent and arbitrary.

§11's decisive argument (§11.1) was measured and remains true: `Grpc.Tools`
2.83.0 is 22.8 MiB of which **99.8% is native payload across six platform
folders**, for 48 KB of managed task code. But that argument is against
smuggling native binaries into a .NET *dependency graph*. It says nothing
against a compiler installed the way `buf` is, which is what protopc now is.

## Decision

protopc is written in Go.

## Why not Rust

A compiler front end is a cyclic graph of mutable nodes: parent pointers, scope
chains, a `references(SportGame)` that resolves to a declaration which points
back at its own file. Rust's ownership model fights that shape — `Rc<RefCell<…>>`
or arena-plus-index throughout. It is workable (rust-analyzer, swc) and the tax
lands on exactly the code that gets rewritten most while the language is still
moving.

The TypeScript compiler reached the same conclusion, and the part of its
reasoning that transfers is worth separating from the part that does not. Its
*decisive* reason was port fidelity — ~100k lines of structurally imperative
TypeScript that Go's GC and reference semantics reproduce almost 1:1, where
Rust would have forced a data-model redesign. That reason is **not** ours;
protopc is greenfield. The property underneath it is: compiler IR is a mutable
cyclic graph, and a GC makes that free.

Rust's genuine advantages here — no GC pauses, small binaries, C ABI embedding —
are ones protopc has no use for.

## Consequences

**The C# emitter produces text, with no Roslyn to check it.** This is the real
cost and it is smaller than it looks: `protoc-gen-go`, `protoc-gen-csharp` and
`grpc_csharp_plugin` all emit text, the last of them from C++. Verification was
already reassigned by ADR-0001 to golden files against `corpus/prague/`, which
checks the generated C# against types actually on the wire — a stronger test
than "it parses".

**No Roslyn source generator, so no live IDE diagnostics on `.prague` files in
Visual Studio or Rider.** The replacement is an LSP server, and that inverts the
loss: it serves every editor rather than only .NET ones, which is the right
shape for a language with several backends.

**Cross-compilation is `GOOS`/`GOARCH`** and distribution is one static binary
per platform, built in one step with no toolchain matrix, no codesigning story
for a NuGet-extracted executable, and no exec bit to lose. The six-way native
matrix §11.1 measured was a cost of the *delivery shape*, not of compiled
languages, and this delivery shape does not have it.

**Git-tag import resolution (#31) stops being novel work.** Module fetch,
immutable tags, a lock file and `go.sum`-style content hashing are the ecosystem
protopc is now written in, not a mechanism to design from scratch.

**What is unaffected.** Everything in the spec, and every measured finding about
the *output*: `Key = tag - 1`, presence as the nullable axis, the ban on
`[MessagePackFormatter(type, args)]` (#3), the generated resolver as normative,
the deserializing-constructor limit that forces mutable classes (#20), and the
`auto-generated` / `.g.cs` skip trap (#25). These are properties of the emitted
C#. The host language of the compiler cannot reach them.

**The cost that is real.** The team's language is C#, and protopc is now a Go
codebase they have to maintain, review and hire for. Nothing in this document
reduces that; it is the price of the frontend/backend split being honest.
