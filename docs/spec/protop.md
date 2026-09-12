# protop — language specification, revision 1

> **Status: leading document.** This is what protopc implements. It supersedes
> the two specimens under [`prototypes/lang/`](../../prototypes/lang/), which
> argued whether protop should stay parseable by `protoc`; this document is the
> native spelling and answers that *no* — see
> [ADR-0001](../adr/0001-protop-is-its-own-language.md).
>
> **Settled here:** shapes (`table` / `record` / `struct` / `view`), presence as
> `T` vs `T?`, identity and composite keys, aliases, relations and derived
> cardinality, annotation passthrough, tag ownership via `reserved`.
>
> **Deliberately not covered.** Four things the corpus contains and this document
> does not spell. Each is a tracker ticket, not an oversight:
> unions (`IBaseLiveData`'s 19 arms), maps, a temporal type (the corpus and this
> document both carry instants as `int64`), and the closed set of scalar type
> keywords.

**The rule everything else falls out of: tables are written, views are read.**

A table is the wire contract for a topic. Its producer holds the whole table and is the only writer. Consumers declare views — field subsets pinned to the table's tags. A producer cannot use a view.

Read that rule first and most of the language needs no further justification.

---

## The document

Split by section, so that a task reads what it needs and no more. Sections keep
the numbers they had when this was one file, so a citation like *§3* still
resolves.

| § | File | Covers | Read it when |
|---|---|---|---|
| 1 | [`shared-repository.md`](protop/shared-repository.md) | `prague.protop` in full: revision, namespace, struct, enum, record, and two tables | you need the written form of a declaration, a field, a tag or presence |
| 2 | [`service-repository.md`](protop/service-repository.md) | the consuming side: `import`, views, and a service's own table | you are working on views or on what a service repo may declare |
| 2 | [`import-resolution.md`](protop/import-resolution.md) | where `import "prague.protop";` resolves from — git, by tag | you are working on module resolution, the lock file or distribution |
| 3 | [`annotations.md`](protop/annotations.md) | one position, one form; the prefix rule and passthrough | you are parsing or emitting `[...]` |
| 4 | [`declaration-keywords.md`](protop/declaration-keywords.md) | `struct` / `record` / `table` / `view`: identity, emission, referenceability | you need to know what a declaration keyword means |
| 5 | [`views.md`](protop/views.md) | field subsets, narrowing, computed members, composite identity, aliases | you are working on views, `key` or `alias` |
| 6 | [`relations.md`](protop/relations.md) | `references(T)` and derived cardinality | you are working on relations or name resolution |
| 7 | [`tag-ownership.md`](protop/tag-ownership.md) | `reserved`, and why single ownership is what makes it safe | you are working on `reserved` or cross-repo skew |
| TBD | [`versioning.md`](protop/versioning.md) | sketched, not decided: two hashes, per-table versions, `protop.lock` | you are working on versioning or the breaking check |

## Out of scope

- **Topic as syntax.** It is one more C# attribute. 12 topic-carrying types, 4 distinct topics — as a keyword it was three-quarters restatement. `[cs: DataCacheTopic("...")]` is shorter and says where it goes.
- **Index.** A decision about queries, not a fact about data. Not in the language in any form.
- **Derived index members.** They existed only to be indexed; they go to the partial with the other computed members.
- **`snapshot_of`.** Deferred, with the cost stated above.

## Open

- **Read-modify-write is closed.** MessagePack-CSharp drops unknown array slots rather than preserving them the way Protobuf preserves unknown fields, so a narrow type read and written back would truncate. Producers cannot use views, so nothing narrow is ever serialized. The hazard was conditional on a case the model forbids.
- **Descriptor as IR.** Emitting a machine-readable descriptor from protopc, rather than only C#, is what lets service repos consume the shared schema without consuming its source, and what leaves the door open for a non-C# backend.
- **The tool is buf-shaped, eventually.** `protopc` compiling schemas to C# is
  the phase-1 deliverable, not the destination. The destination is what `buf` is
  for Protobuf: `lint` against a rule ladder, `breaking` against a checked-in
  baseline, module resolution over git tags, formatting. Three of those four are
  already sketched in this document — the lint rules as structural consequences,
  the breaking check as the versioning section's diff-against-baseline, and
  resolution just above. Naming the shape now keeps phase 1 from painting into a
  corner it would have to be cut out of.
- **Diagnostics.** `revision 1;` means owning the parser and its error messages forever. Good diagnostics are most of what makes a small language tolerable and the part that always gets deferred.
