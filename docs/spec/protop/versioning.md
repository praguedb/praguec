# TBD — versioning

Sketched, not decided. Nothing below is implemented.

### A hash is identity, not a version

A hash says two schemas differ. It cannot say which is newer, or whether the difference broke anything. Both require comparing against the previous schema, so autogeneration needs a checked-in baseline — the last published descriptor — and protopc derives the version by diffing current against it. This is what `buf breaking` does and there is no way around it.

### Versioned per table

A table is a topic is a wire contract. A consumer of `SportGame` should not see a bump because `BetType` changed.

| Change | Verdict |
|---|---|
| Append a tag | minor |
| Claim a previously-unused or reserved tag | minor |
| Add a reservation | minor |
| Narrow `T?` → `T` | minor — readers tolerate a value where they expected nil |
| Add an enum member | minor, with a caveat below |
| Change the type at a tag | **major** — silent misdecode, the worst case |
| Widen `T` → `T?` | **major** — readers get nil where they expected a value |
| Change the key | **major**, operationally distinct — identity changed, entries no longer match, flush required |
| Remove a reservation | **major** — frees a tag for reuse, the one unrecoverable mistake |
| Remove a tag | major *in source*, not on the wire |
| Rename a field | major *in source*, invisible on the wire |

### Two hashes, because breaking has two audiences

The last two rows are why one number is not enough. Removing a field harms no running reader — MessagePack skips it — but breaks every view that lists it, at compile time, in another repo. That is the outcome the design wants, and a single bump conflates it with wire damage.

- **wire hash** — tag → (type, presence), key shape, reserved set, transitively including every enum, struct and record the table references. Names, annotations and declaration order excluded.
- **source hash** — the above plus names, aliases and annotations. What a consumer compiles against.

A rename moves the source hash and not the wire hash, which tells a service repo exactly what it needs: regenerate, nothing deployed is at risk.

The transitive part carries more weight than it looks. `EventStatus` is shared, so adding a member to it must move the wire hash of every table that uses it. A per-declaration hash stopping at the type name would miss that.

### Normalization

Canonical form, then SHA-256, first 64 bits for the header. Sort members by tag, fully qualify type names, fold `alias` out (no wire slot), fold views out entirely (pinned to their table, no independent identity). Include `revision` in the hash input, so a dialect change that alters semantics moves every hash rather than silently reinterpreting old ones.

### Where the numbers live

**Build time — a lock file, not writeback into the schema.** `protop.lock` holds the last published `{major, minor, wire_hash, source_hash}` per table. CI runs a check that recomputes the diff and fails when the lock does not match what the changes imply, which makes it impossible to merge a breaking change without the major bump appearing in the diff. The compiler does not edit schema source; a derived value living in source drifts.

**Runtime — a header.** `[cs: DataCacheHeader]` already exists, so the version costs no tag and never competes for one. Emit `major` and the truncated wire hash. A consumer compares against what it compiled against and logs or rejects on mismatch — and because it is a header, a mismatched major is detectable before deserializing.

### Three open questions

- **Enum additions.** Non-breaking on the wire (an unknown value lands in the C# enum as an undefined member), breaking for any consumer switching exhaustively. Most schema languages call it minor and accept the risk. `EventStatus` drives real branching in the corpus, so consider minor but loud — a diagnostic naming the affected tables and consuming repos.
- **Repo-level rollup.** Per-table versions are authoritative, but one number for "which release of the shared schema is this" will be wanted. A rollup incrementing repo-major when any table's major did is fine, provided nobody mistakes it for a compatibility guarantee about a specific topic.
- **`revision` vs version.** `revision 1;` stamps the dialect — the language's version, not the schema's. Two things called version in one file will confuse someone. Rename one now, while it costs nothing.
