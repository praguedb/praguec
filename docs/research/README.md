# Research

Primary-source investigations backing the [protopc phase 0+1 map](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/1).
Each document resolves one research ticket and is cited from that ticket's resolution comment.

Every claim in these documents is either cited to a file and line in a primary source, or measured —
compiled, run, and the bytes dumped. Unverified claims are collected in a labelled section at the end
of each document rather than mixed in with the findings.

| Document | Ticket | The finding that mattered |
| --- | --- | --- |
| [`proto-to-messagepack-csharp.md`](proto-to-messagepack-csharp.md) | — | The original substrate (1,675 lines). Authoritative except §11.3's shipping-shape ranking, superseded by the MSBuild-task decision. |
| [`legacy-required-scalars.md`](legacy-required-scalars.md) | [#2](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/2) | `LEGACY_REQUIRED` is legal on every singular type — but protoc's C# backend emits **byte-identical** output with and without it, so protoc cannot be an oracle on the presence axis. Go and Java enforce transitively. |
| [`messagepack-formatter-args.md`](messagepack-formatter-args.md) | [#3](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/3) | `[MessagePackFormatter(type, args)]` is honoured by the reflection resolver and **silently ignored by the v3 source generator**. Different bytes, zero warnings — on exactly protop's path. |
| [`editions-2023-features.md`](editions-2023-features.md) | [#4](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/4) | `--descriptor_set_out` carries **zero resolved features**. The differential harness therefore cannot check feature resolution at all. |
| [`messagepack-unions.md`](messagepack-unions.md) | [#5](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/5) | An unregistered runtime type serializes to **nil, silently, on both paths** — including any subclass of a listed arm. Total write-side data loss with no diagnostic. |
| [`b2tech-feed-cache.md`](b2tech-feed-cache.md) | [#6](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/6) | The generator skips any file containing `auto-generated` in its first 500 bytes. A compiler that writes real files before `csc` must not emit the conventional banner. |
| [`messagepack-computed-members.md`](messagepack-computed-members.md) | [#20](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/20) | A `[Key(N)]` on a get-only computed property **is written and never read** — it extends the array and is `Skip()`ped on deserialization. It also breaks deserializing-constructor matching on both paths. |
| [`messagepack-datetime-structs.md`](messagepack-datetime-structs.md) | [#19](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/19) | `DateTime` is the variable-length timestamp extension (6/10/15 B), not one slot — and `DateTimeKind` is not preserved. `required` structs round-trip, but `required` guarantees nothing at deserialization. |

## Recurring themes

Four findings independently landed on the same fault line: **the generated resolver and the reflection
resolver are not interchangeable.** Formatter arguments diverge in behaviour, union discriminators
diverge in encoding, `[Union]` on a concrete class diverges in whether it throws, and a computed member
diverges in whether it appears at all under map mode. Built-in types (`DateTime` among them) do *not*
diverge, because `BuiltinResolver` precedes both — and array mode, which protop uses, is exempt from the
fourth. Tracked as
[Which MessagePack resolver is normative for protop?](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/22).

A third: **two separate bugs produce uncompilable generated code rather than a diagnostic** — a formatter
with only a non-default constructor (`CS1001`), and a computed member at key 0 shifting deserializing-constructor
parameters (`MsgPack007` plus `CS7036`). Both fail at `csc`, not in the generator, which means the error
points at source nobody wrote.

A second theme: **several constructs declare an intent that nothing enforces.** `LEGACY_REQUIRED` in C#,
C# `required` at deserialization, and `DateTimeKind` on the wire are all promises the runtime discards.
Each one, if protop honours it, is honoured by protopc's own generated code and by nothing else.
