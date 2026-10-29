# 4. Declaration keywords

| Keyword | Identity | Emission | Can be referenced |
|---|---|---|---|
| `struct` | none | C# struct, embedded | no |
| `record` | none | C# class, embedded | no |
| `table` | `key` | C# class, cached | yes |
| `view … of T` | inherits T's | C# class, cached | no |

**A declaration may not be named with one of protop's built-in type names.** The set is the language's own — its scalars and its collections — and it is referred to here rather than transcribed, because whether the scalar set is closed at all is one of the things this document deliberately leaves open (see the index's *Deliberately not covered*), and a copy of it in prose is a copy that goes stale without failing anything. The rule is about reachability and not about reserved words: a type written with such a name resolves to the built-in, so no field, view member or relation could ever spell its way to the declaration. praguec reports `PROTOP0207` at the declaration keyword, which is the only line where a fix exists. Member names are untouched — a member is looked up in the declaration that carries it and never in the type system, so a field may take a built-in's name and stay legal.

**`struct` borrows FlatBuffers' split, not its layout constraint.** FlatBuffers forbids strings in a struct because its structs are memcpy'd at a fixed offset. protop emits MessagePack, where there is no fixed layout to protect, so the constraint has no reason to exist here. `struct` means: value semantics, C# struct emission, no key, embedded in whatever holds it. A struct holding a string is allowed. Value types have well-known ways to be misused and this is one of them; the language does not police it.

`Venue`'s tags start at 2. Inherited from the corpus and preserved deliberately — renumbering would break the wire.

**`record` cannot be a relation target**, because a relation resolves against a key and a record has none. That is also what makes `list<T>` unambiguous: composition and reference are told apart by whether the element type declares identity, with no second keyword.
