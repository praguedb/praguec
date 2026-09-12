# 4. Declaration keywords

| Keyword | Identity | Emission | Can be referenced |
|---|---|---|---|
| `struct` | none | C# struct, embedded | no |
| `record` | none | C# class, embedded | no |
| `table` | `key` | C# class, cached | yes |
| `view … of T` | inherits T's | C# class, cached | no |

**`struct` borrows FlatBuffers' split, not its layout constraint.** FlatBuffers forbids strings in a struct because its structs are memcpy'd at a fixed offset. protop emits MessagePack, where there is no fixed layout to protect, so the constraint has no reason to exist here. `struct` means: value semantics, C# struct emission, no key, embedded in whatever holds it. A struct holding a string is allowed. Value types have well-known ways to be misused and this is one of them; the language does not police it.

`Venue`'s tags start at 2. Inherited from the corpus and preserved deliberately — renumbering would break the wire.

**`record` cannot be a relation target**, because a relation resolves against a key and a record has none. That is also what makes `list<T>` unambiguous: composition and reference are told apart by whether the element type declares identity, with no second keyword.
