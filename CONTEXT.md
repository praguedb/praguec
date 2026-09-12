# Context

Glossary for this repo. Terms only — no design, no decisions, no implementation.
Decisions live on the [map](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/1)
and its tickets; hard-to-reverse ones get an ADR under `docs/adr/`.

## protop

The language. A strict profile of Protobuf: everything protop accepts is valid
Protobuf, but protop rejects a great deal that Protobuf permits. Named by
analogy to a dialect, not a superset — protop adds no syntax that `protoc`
would refuse.

## protopc

The compiler. Reads protop and writes C#. It is not a `protoc` plugin and does
not invoke `protoc`; it parses protop itself.

## oracle

`protoc`, used only to check protopc. Compiling the same file with both and
comparing the results is how protopc is known to be right. The oracle is a test
dependency, never a build dependency.

## tag

A field's number in protop — `string name = 3;` has tag 3. Protobuf's term,
kept deliberately.

## key

A member's `[Key(N)]` index in MessagePack. Distinct from a tag: tags and keys
are different number spaces with a fixed offset between them, and confusing the
two breaks wire compatibility silently.

## presence

Whether a field distinguishes "absent" from "set to the zero value". Explicit
presence tracks the difference; implicit presence does not. The central concept
of this effort: presence is what protop uses to decide whether a generated C#
member is nullable.

## corpus

`corpus/prague/` — the pinned, read-only sample of real payload types the design
is tested against. A decision that cannot express the corpus is not finished.
Pinned from an external repository at a fixed commit; see `corpus/prague/SOURCE.md`.

## payload type

A type whose whole purpose is to be serialized and sent. No behaviour, no
identity, no lifecycle — just fields and their wire layout. Everything protopc
generates is a payload type.

## projection

A payload type that carries a subset of another's fields **at the same keys**,
so the two are wire-compatible without conversion. The corpus's `Slim*` types
are projections of their full counterparts. A projection may drop fields; it may
not renumber them.

## twin

A payload type duplicated across two projects with identical keys, names and
types, differing only in namespace and annotations. Distinct from a projection:
a twin drops nothing.

## drift

A projection or twin silently disagreeing with its counterpart at a shared key —
different type, different name, different presence. Drift is wire corruption
waiting to happen, and it is invisible to the C# compiler.

## shell

The hand-written half of a `partial` type whose other half a source generator
writes. In the corpus, the cache shells are empty by design.

## passthrough

Carrying an annotation from protop into the generated C# verbatim, without
protopc understanding what it means. The mechanism by which protop-generated
types reach generators protopc knows nothing about.
