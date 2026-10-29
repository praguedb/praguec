# ADR-0003 — Collections are generic types, not field labels

- **Status:** accepted, 2026-09-12
- **Leading document:** [`docs/spec/protop.md`](../spec/protop.md)

## Context

protop inherited `repeated` from Protobuf along with *tag*, *field* and
*reserved*, and the word worked for as long as a collection was the only thing
a field could be other than a single value. It stops working the moment
presence is applied to one.

`repeated` is a label on the field. `?` binds to the type. So there is exactly
one slot for a `?` on a repeated field and two things it could mean — a list
that may itself be absent, and a list whose elements may be absent — and
`repeated Odd?` has no way to say which. Presence is the whole thesis of this
project; a construct that cannot express it is not a small gap.

Square brackets are also already spoken for. Section 3 gives annotations one
position and one form, `[cs: X]`, and the argument for that rule was that
disambiguating a bracket by its position is a thing the language should not ask
a reader to do.

## Decision

Repetition is a **type**, written `list<T>`, not a label on the field. `Field.Repeated`
is deleted; `TypeRef` grows `Args []TypeRef`, and a collection's arguments are
parsed by recursing into the same type-parsing function.

Presence then composes with no new rule:

| | |
|---|---|
| `list<Odd>` | the slot holds an array |
| `list<Odd>?` | the slot may be nil |
| `list<Odd?>` | the array may contain nils |

Nesting follows from the same recursion: `list<list<int64>>` needs no further
machinery.

`map<K, V>` is **reserved, not implemented.** The corpus has no map. The syntax
is defined so that adding one later fills a slot that already exists rather than
inventing a third collection form; the key type will be restricted to scalars
and enums when it lands. protopc parses a `map` in full and reports
`PROTOP0900 NotYetImplemented` against it, which is the standing rule for a
construct the spec defines and the compiler is behind on.

**`T[]` was rejected.** It would make `[]` mean two things in one line —
`Odd[] odds = 26 [cs: X]` — which is exactly the rule the annotation design
removed. `Odd?[]` versus `Odd[]?` is also harder to read than the generic form.
No reference language uses postfix `[]`: Protobuf prefixes `repeated`, Cap'n
Proto writes `List(T)`, FlatBuffers writes `[T]` and consequently puts its
attributes in parens. Each of them assigns square brackets one job. This
language assigns them to annotations.

## Consequences

**Ownership is unchanged.** `list<StaticDataRef>` is composition because
`StaticDataRef` is a record with no key, so nothing can point at it and it
cannot exist apart from its parent. The line between composition and reference
is drawn by whether the element type declares identity, and was never drawn by
the collection keyword — which is why replacing that keyword costs nothing here.

**An annotation on a collection field annotates the field, not the element.**
There is no position for an element annotation and none is being invented;
Protobuf and Cap'n Proto have the same limit.

**The cost is a lexer token kind.** `<` and `>` did not lex, and now do, as
`KindLAngle` and `KindRAngle`. The frontend conventions call adding a token kind
rare and suspicious, and the suspicion is warranted: the lexer's completeness is
what makes growing the language a parser change alone, and every addition to it
spends that property once. It is spent here because angle brackets are
punctuation the language genuinely did not have. They are punctuation and
nothing more — `list` and `map` still lex as `KindIdent`, and a field may still
be named either one, because the parser decides by position and there are still
no keyword token kinds.

One thing falls out of it for free: `[cs: Foo<Bar>]` now lexes. Annotation
contents are scanned out of the token stream after the whole file has been
tokenised, so a character the lexer rejected was rejected even inside an
annotation, and a C# attribute carrying a generic used to cost two
`PROTOP0003` diagnostics.

**Two diagnostic codes join the 0300 group.** `PROTOP0306
TypeArgumentCountWrong` for `list<A, B>` or a bare `list`, and `PROTOP0307
TypeArgumentsNotAllowed` for `int64<string>`. Both are shipped and keep their
numbers.

**`list` and `map` are not scalars.** They live in their own table with their
arities. Whether protop's scalar set is closed at all is a separate, open
question (#32), and a type constructor has nothing to say about it.
