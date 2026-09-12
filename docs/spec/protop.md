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

## 1. Shared repository — `prague.protop`

Tables, records, structs and enums live here, in one repo, owned by one team. Nothing in this file knows which services consume it.

```protop
revision 1;

namespace Common.IntegratedEventApi.Cache.Prague.Models;

// --- STRUCT -----------------------------------------------------------------
struct Venue {
  int64  id   = 2;
  string name = 3;
}

enum EventStatus {
  UNKNOWN     = 0;
  NOT_STARTED = 1;
  IN_PROGRESS = 2;
  FINISHED    = 3;
}

enum BetTypeStatus {
  UNKNOWN   = 0;
  OPEN      = 1;
  SUSPENDED = 2;
  CLOSED    = 3;
}

// --- RECORDS ----------------------------------------------------------------
record StaticDataRef {
  int64  id   = 1;
  string name = 2;
}

record Odd {
  string name        = 1;
  double odds_actual = 9;
  int64  timestamp   = 5;
}

// --- TABLE, SIMPLE KEY ------------------------------------------------------
table SportGame
  [cs: DataCache(CacheClassName = nameof(BPSportGameCache))]
  [cs: DataCacheTopic(TopicNames.BrandFromConfigs.SportGame)]
  [cs: DataCacheContract(typeof(SportGameCacheContract), Role = ContractRole.Producer)]
{
  reserved 8..14, 16..23, 25..27, 29..34, 37..46, 48..54;

  key int64 event_id = 1;

  int64 sport_type_id = 2;
  int64 country_id    = 3;
  int64 league_id     = 4;

  int64? home_team_id = 5;
  int64? away_team_id = 6;

  Venue? venue = 7;

  int32? fixture_id = 15;

  EventStatus event_status = 24;

  int64 event_date         = 28;
  int64 provider_timestamp = 35;

  decimal(4)? live_limit_manual_single = 36;

  repeated StaticDataRef runners = 47;

  StaticDataRef  sport_type = 55;
  StaticDataRef  country    = 56;
  StaticDataRef  league     = 57;
  StaticDataRef? home_team  = 58;
  StaticDataRef? away_team  = 59;
}

// --- TABLE, COMPOSITE KEY ---------------------------------------------------
table BetType
  [cs: DataCache(CacheClassName = nameof(BPBetTypeCache))]
  [cs: DataCacheTopic(TopicNames.BrandFromConfigs.BetType)]
  [cs: DataCacheContract(typeof(BetTypeCacheContract), Role = ContractRole.Producer)]
{
  reserved 5, 6, 10..21, 23, 24;

  key key = 1 (event_id, bet_type_id);

  int64 event_id = 2 [references(SportGame)];
  int64 id       = 3;

  alias bet_type_id = id;

  string name = 4;

  BetTypeStatus  status           = 7;
  BetTypeStatus? status_manual    = 8;
  BetTypeStatus? status_watch_dog = 9;

  decimal(4)? limit_manual_single = 22;

  int64 timestamp = 25 [cs: DataCacheFromTimestamp] [cs: DataCacheIgnoreEquality];

  repeated Odd odds = 26;
}
```

## 2. Service repository — `brand-data-reader.protop`

A consuming service imports the shared schema and declares only what it reads.

```protop
revision 1;

namespace BrandDataReader.Services.Cache;

import "prague.protop";

view SlimSportGame of SportGame {
  event_id as id;

  sport_type_id;
  country_id;
  league_id;
  home_team_id;

  venue;
  sport_type;
}

view SlimOdd of Odd {
  name;
  timestamp;
}

view SlimBetType of BetType {
  key as id;

  event_id;
  id as bet_type_id;

  name;

  status;
  status_manual;
  status_watch_dog;

  limit_manual_single;
  timestamp;

  SlimOdd odds;
}
```

A service may also declare tables of its own — for caches it produces and owns, which no other service reads:

```protop
table BetTypeBrandSettings
  [cs: DataCache]
{
  key int64 bet_type_id = 1;
  bool is_enabled = 2;
}
```

That is the pressure valve. The shared repo gates only what is actually shared.

---

## 3. Annotations — one position, one form

An annotation follows what it describes. There is no second position and no before-slot.

```protop
table SportGame [cs: DataCacheTopic("...")] { }      // after the declaration head
int64 timestamp = 25 [cs: DataCacheFromTimestamp];   // after the tag
alias bet_type_id = id [cs: JsonIgnore];             // after the alias
```

Legal on a table, record, struct, view, field, view member, or alias. Same rule everywhere.

**One bracket, one annotation. Stack by juxtaposition.** No commas between annotations, because annotation contents already contain commas — `DataCacheContract(typeof(X), Role = Y)` — and a separator that competes with them buys nothing. Whitespace between brackets is insignificant, so a head carrying four annotations wraps freely across lines. protopc emits them in source order and never sorts or deduplicates.

**A prefix names the target language. No prefix means protop's own.**

| Form | Meaning |
|---|---|
| `[references(SportGame)]` | protop's own closed set. Read, checked, unknown is an error. |
| `[cs: DataCacheTopic("...")]` | Verbatim C# attribute text. Copied out, never inspected. |
| `[go: json:"sportGame"]` | Verbatim Go tag text, when a Go backend exists. |

The legal prefix set is exactly the backends protopc has. An unknown prefix is an error rather than a silent drop, because a typo that vanishes from the output is worse than the lexical rule this replaced. Adding `go:` means adding a backend, not just a keyword.

Everything after the prefix is raw text up to the matching close bracket, with nesting counted and string literals respected. This is the one place protopc parses something it does not understand, so the grammar states it explicitly.

### What this costs

protop does not validate annotation contents. Protobuf does — custom options are typed values declared by extending the descriptor messages, with globally coordinated extension numbers. protop checks position and prefix only.

That trade is right here and would not be right for Protobuf. An annotation targeting a code generator is written in that generator's language and emitted verbatim; the check that matters happens when the generated code compiles. Protobuf cannot make that trade because anyone may generate anything from a `.proto`, which is also why spelling A has to smuggle C# attributes as string literals that nothing validates.

### Why one position and not Protobuf's two

Protobuf puts field options in a bracket after the tag and type options in a body statement — `option x = y;` — which is two syntaxes for the same kind of fact, chosen by whether it describes a type or a field. The reason is parser economy: `message Foo [x] {` needs lookahead past the bracket to find the brace, and a body statement does not.

protopc is recursive-descent, where that lookahead is free. Adopting the split would mean inheriting a consequence without its cause.

The rule works here because the scope is narrow. protop targets one pipeline with known backends, which is what makes opaque namespaced passthrough safe. Protobuf's heavier machinery is not a mistake; it is what a cross-ecosystem format needs.

---

## 4. Declaration keywords

| Keyword | Identity | Emission | Can be referenced |
|---|---|---|---|
| `struct` | none | C# struct, embedded | no |
| `record` | none | C# class, embedded | no |
| `table` | `key` | C# class, cached | yes |
| `view … of T` | inherits T's | C# class, cached | no |

**`struct` borrows FlatBuffers' split, not its layout constraint.** FlatBuffers forbids strings in a struct because its structs are memcpy'd at a fixed offset. protop emits MessagePack, where there is no fixed layout to protect, so the constraint has no reason to exist here. `struct` means: value semantics, C# struct emission, no key, embedded in whatever holds it. A struct holding a string is allowed. Value types have well-known ways to be misused and this is one of them; the language does not police it.

`Venue`'s tags start at 2. Inherited from the corpus and preserved deliberately — renumbering would break the wire.

**`record` cannot be a relation target**, because a relation resolves against a key and a record has none. That is also what makes `repeated` unambiguous: composition and reference are told apart by whether the child type declares identity, with no second keyword.

---

## 5. Views

A view is a field subset of a table or record, at that source's tags. Tag, type and presence come from the source; the view line carries the name and an optional rename.

**A view may add untagged members. A view may never add a tag.** Not policy — structure. The producer writes the table, so a tag the table does not declare is never written by anyone and would deserialize to nil forever, while squatting a slot the table might later claim. Only the table extends the tag space. This is the same rule as *no tag, no wire slot*, one level down.

Protobuf's answer to the same pressure is extension ranges: an owner reserves `extensions 100 to 199;` and a foreign file claims tags inside it. That works because a *producer* can populate an extension. In a single-producer topic nobody writes outside the table, so the mechanism would delegate slots that stay empty. The asymmetry of the pipeline is what rules it out.

**A view inherits the topic.** It is a property of the table, so restating it on the view was the last piece of view-as-second-copy — and inheriting it means a view cannot point at the wrong topic.

**A view may narrow a member to another view.** `SlimOdd odds;` takes tag 26 and requires `SlimOdd` to be a view of `Odd`. A leading type asks for the narrowing; a bare name takes the source's type unchanged. A view of a view stays pinned to the original tags, so `SlimOdd` cannot quietly renumber and desync tag 26.

**A view's source is a table or a record.** `SlimOdd of Odd` narrows a record, because a view is about tags and members rather than identity.

### Computed members

Members that carry no tag and are not aliases — `StatusActual`, `IsAvailableForClient`, `IsDisabled` in the corpus — live in the hand-written partial class, not in protop. They are relocated, not deleted: a member and its attributes travel together, so moving one to the partial takes its annotations with it.

Two classes of computed member remain in the language, neither of them an expression:

- **Composite identity** — `key key = 1 (event_id, bet_type_id);`. Borrowed from Cap'n Proto's `group`, a name over fields that keep their own ordinals. Cap'n Proto's group consumes no ordinal; this one does, because the corpus wire format already says so — the tuple is `[Key(0)]` and duplicates what tags 2 and 3 carry. Writing the tag makes the duplication visible and the collision checkable.
- **Alias** — `alias bet_type_id = id;`. Get and set passthrough, no wire slot. It exists so a table's API matches the view that renames the field.

---

## 6. Relations

`references(T)` names a table. Under the shared-repo model this always resolves: `SportGame` is the table and its five siblings are views of it, so there is nothing to disambiguate. Relations are annotations on members, so a view inherits the relations of the members it lists — no rule needed forbidding views from declaring their own.

**Cardinality is derived, never declared.** Compare the foreign key against the declaring type's own key:

| | |
|---|---|
| FK *is* the whole key | one-to-one — the key enforces it |
| FK is part of a composite | many-to-one — the rest of the composite distinguishes the many |
| FK is in no key | many-to-one, unconstrained |

One-to-many is the same fact read backwards and needs no declaration. A key that is exactly two foreign keys is a join table; the corpus has none.

Checked against every cached corpus type:

- `BetType` — key `(event_id, bet_type_id)`, FK is part → many BetTypes per SportGame ✅
- `LiveObject` — key `event_id`, which *is* the FK → exactly one per SportGame ✅
- `Search` — key `(event_id, channel_id)`, FK is part → many Searches per SportGame ✅
- `SportGame.league_id` — in no key → many SportGames per League ✅

**Sound, not complete.** Every one-to-one it reports is real, because the key enforces it. A many-to-one it reports may be one-to-one in truth with nothing in the schema saying so — an FK outside the key that happens to be unique. The rule is not wrong about anything it can see; it declines to promise. A declared `one_to_one` errs the other way, asserting uniqueness nothing checks, and an unchecked assertion of uniqueness is worse than a missing one because code will trust it. No `unique` keyword until the corpus shows the escaping case.

### What is not a relation

`SportTypeId`, `CountryId`, `LeagueId`, `HomeTeamId`, `AwayTeamId` — 25 members across the corpus — name no cached type, and every one of the 15 pairs among them carries a `StaticDataRef` copy. `EventId` and `BetTypeId`, 11 members, do name a cached type and carry no copy.

Exactly complementary: the copy exists because the referent does not. Each of these is the id half of an embedded pair whose other half is the `StaticDataRef` beside it. Binding the two was `snapshot_of`, now deferred.

The cost, stated exactly: the one check it bought was that both halves agree about presence, and 3 of the 15 pairs disagree. `SlimSportGame` has `long? HomeTeamId` beside a non-nullable `StaticDataRef HomeTeam = null!`. Phase 1 emits that as written and says nothing. Known gap, not an oversight.

---

## 7. Tag ownership

`reserved` is the mechanism that keeps cross-repo skew safe. Only the shared repo can retire a tag, and a removal without a reservation breaks every view at compile time — which is the correct outcome, and the reason it only works with a single owner.

This is the value of the whole design. Today six field lists drift independently: `EventId` becomes `Id`, `Key` and `Id` disagree across pairs, and one consumer carries a hand-written comment asking nobody to renumber. With one table and N views pinned to its tags, that drift stops being a matter of discipline and becomes a compile error in the repo that drifted.

---

## Out of scope

- **Topic as syntax.** It is one more C# attribute. 12 topic-carrying types, 4 distinct topics — as a keyword it was three-quarters restatement. `[cs: DataCacheTopic("...")]` is shorter and says where it goes.
- **Index.** A decision about queries, not a fact about data. Not in the language in any form.
- **Derived index members.** They existed only to be indexed; they go to the partial with the other computed members.
- **`snapshot_of`.** Deferred, with the cost stated above.

## Open

- **Read-modify-write is closed.** MessagePack-CSharp drops unknown array slots rather than preserving them the way Protobuf preserves unknown fields, so a narrow type read and written back would truncate. Producers cannot use views, so nothing narrow is ever serialized. The hazard was conditional on a case the model forbids.
- **Descriptor as IR.** Emitting a machine-readable descriptor from protopc, rather than only C#, is what lets service repos consume the shared schema without consuming its source, and what leaves the door open for a non-C# backend.
- **Diagnostics.** `revision 1;` means owning the parser and its error messages forever. Good diagnostics are most of what makes a small language tolerable and the part that always gets deferred.

---

## TBD — versioning

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
