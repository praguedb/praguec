# ADR-0001 — protop is its own language, not a strict Protobuf profile

- **Status:** accepted, 2026-09-12
- **Resolves:** [#29 Does protop stay parseable by protoc?](https://github.com/praguedb/praguec/issues/29)
- **Leading document:** [`docs/spec/protop.md`](../spec/protop.md)

## Context

The effort began on the premise that *everything protop accepts is valid
Protobuf* — a strict profile, with `protoc` available as a differential-test
oracle. Two specimens were written to test that premise against the corpus:

| specimen | measured |
|---|---|
| `prototypes/lang/a-strict-protobuf/prague.proto` | compiles under `protoc` 36.1 |
| `prototypes/lang/b-native/prague.prague` | `protoc` 36.1 rejects it at `revision 1;` |

The profile survived the shapes. It did not survive them cheaply. Under spelling
A every concept the corpus actually needs arrives as a custom option: the dialect
stamp is `option (protop.revision) = REVISION_1;` because an invented keyword is
a parse error (#27); C# attributes are smuggled as string literals nothing
validates; the extension numbers need global coordination and a distributable
`protop/options.proto` (#28). The profile buys one thing — the oracle — and pays
for it in every line of every schema.

## Decision

protop is its own language with its own grammar, parsed by praguec alone.
`protoc` leaves the pipeline entirely, including as a test dependency.

## Consequences

**The oracle is gone, and it was already blind where it mattered.** The
differential harness was the phase-1 verification strategy, so this is the real
cost. But two closed findings had already measured its reach:

- `protoc`'s C# output is **byte-identical with and without `LEGACY_REQUIRED`**
  (#2), so the oracle could say nothing about the presence axis — which is this
  project's entire thesis.
- `--descriptor_set_out` carries **zero resolved features** (#4), so the harness
  proved praguec *parses* identically and nothing about whether it *resolved*
  correctly.

The oracle was sound on syntax acceptance and silent on both axes the compiler is
for. Replacing it with golden-file tests against `corpus/prague/` — real payload
types, already on the wire — tests the thing that was never testable before.

**What follows mechanically.** No `protop/options.proto` to distribute (#28
evaporates). No Editions feature resolution, so presence is `T` vs `T?` in the
grammar and `LEGACY_REQUIRED` has nothing to attach to (#8 dissolves). The
dialect stamp returns to syntax: `revision 1;`. The frozen accepted-subset
document and its `both-accept` expectation table (#18) lose their baseline and
are re-scoped to corpus golden files.

**What survives untouched.** Every decision closed on the map stands on its
merits; only the carrier changed. `Key = tag - 1`, presence as the
nullable axis, the MSBuild-task integration shape, the ban on
`[MessagePackFormatter(type, args)]`, the generated resolver as normative, and
the `auto-generated` / `.g.cs` skip trap the emitter must avoid.

**The cost that is not recoverable.** `protoc` is a mature parser with twenty
years of diagnostics behind it. praguec now owns its grammar, its error
messages and its source spans forever, for one pipeline. That is stated in the
spec's own Open section and is the largest standing risk in the project.
