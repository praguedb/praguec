# Context

Glossary for this repo. Terms only — no design, no decisions, no implementation.
Decisions live on the [map](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/1)
and its tickets; hard-to-reverse ones get an ADR under `docs/adr/`.

## protop

The language. Its own grammar, parsed by protopc alone — `protoc` cannot read a
`.prague` file and is not in the pipeline at all. The name is inherited from the
period when protop was a strict profile of Protobuf, and so is the vocabulary:
*tag*, *field*, *reserved* all keep their Protobuf meanings deliberately. See
[ADR-0001](docs/adr/0001-protop-is-its-own-language.md); the language itself is
specified in [`docs/spec/protop.md`](docs/spec/protop.md).

## protopc

The compiler. Reads protop and writes C#. It is not a `protoc` plugin and does
not invoke `protoc`; it parses protop itself. Written in Go, and run *before*
the build rather than inside it — see
[ADR-0002](docs/adr/0002-protopc-is-written-in-go.md).

Split frontend/backend, the way LLVM is: the frontend parses and checks source
into one resolved **schema model**, and a **backend** turns that model into a
target language. A backend never sees source or AST. C# is the first backend,
not the only one — the spec's annotation table already reserves `go:`.

## oracle

The source of truth a protopc output is checked against. It is **the corpus** —
`corpus/prague/`, real payload types already on the wire — compared as golden
files. It was `protoc` until ADR-0001; that oracle could only ever check syntax
acceptance, and was measured blind on both presence and feature resolution.

## tag

A field's number in protop — `string name = 3;` has tag 3. Protobuf's term,
kept deliberately.

## key

A member's `[Key(N)]` index in MessagePack. Distinct from a tag: tags and keys
are different number spaces with a fixed offset between them, and confusing the
two breaks wire compatibility silently.

Never used in this repo to mean a type's *identity* — the corpus's
`[DataCacheKey]` sense. That is **identity**, below.

## identity

The member of a payload type that names the instance for the cache — what
`B2Tech.Feed.Cache` marks `[DataCacheKey]`. Distinct from **key**, which is a
position in the MessagePack array and says nothing about what a value means.

Identity takes two forms in the corpus: a stored field *is* the identity, or a
get-only tuple *over* stored fields is, in which case it occupies a position of
its own and duplicates what its components already carry.

## relation

A declared reference from one payload type to another's identity. The referring
type holds a **foreign key** — a field whose value is the other type's identity.
Distinct from containment: a related type exists independently and is referred
to, a contained one has no identity and cannot.

A relation requires a referent that exists as a payload type. Most of the
corpus's foreign-key-shaped members have none, and carry a **snapshot** instead.

## snapshot

An `Id` field paired with an inline copy of what it identifies, at its own key,
where **no payload type for the referent exists**. The corpus's `SportType`
beside `SportTypeId`: two members, two keys, two halves of one referent.

A snapshot is not a cached relation and not a join — there is nothing to join
to, which is why the copy is there at all. It is the whole representation. The
two halves can disagree, most consequentially about presence, which is a species
of **drift**.

## index

A lookup structure built over a payload type's members so the application can
find rows by something other than identity. Not a property of the data: the
same type is indexed differently by different consumers, and the choice changes
with read patterns rather than with the payload.

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
not renumber them. It *may* rename a member — names are not on the wire under
int-key MessagePack — and the corpus does: `BOSportGameCacheItem.EventId` is
`SlimSportGame.Id` at the same key.

In protop a projection is declared rather than hand-maintained; the construct is
a **view**, below.

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
types reach generators protopc knows nothing about — and the reason a concept
can be expressible without being part of the language.

## table

A protop declaration carrying identity and owning a tag space. One table is the
wire contract for one topic, held whole by a single producer.

## view

A protop declaration naming a subset of a table's or record's members, pinned to
that source's tags. The declared form of a **projection**. Read-only by
construction: a producer holds the table, so nothing narrow is ever written.

## annotation

A bracketed clause following what it describes — `[cs: DataCacheTopic("...")]`.
An unprefixed annotation is protop's own and is checked; a prefixed one names a
backend and its contents are **passthrough**, copied out unparsed.
