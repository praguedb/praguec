# Language prototype: shapes, identity, relations, views

Two spellings of the same content, written so they can be read side by side.

| | |
|---|---|
| [`a-strict-protobuf/prague.proto`](a-strict-protobuf/prague.proto) | protop as a strict profile. **Verified: compiles under `protoc` 36.1.** |
| [`a-strict-protobuf/protop/options.proto`](a-strict-protobuf/protop/options.proto) | the option vocabulary spelling A needs. |
| [`b-native/prague.protop`](b-native/prague.protop) | protop as its own language. **Verified: `protoc` rejects it at line 43, `revision 1;`.** |

The first pass asked whether **table / record / struct** could be spelled at all.
This pass cut two constructs *out* of the language — the cache topic and the
index — and that turned out to be where the findings were.

---

## 1. The topic was never a language concept

It appeared on every `table` and every `view`, and it read like this:

```protop
view SlimSportGame of SportGame "Cache.[v:brandId].SportGame" {
```

Measured across the corpus: **12 topic-carrying types, 4 distinct topics.**

```
3 × "Cache.[v:brandId].SportGame"      3 × "Cache.[v:brandId].BetType"
3 × "Cache.[v:brandId].LiveObject"     2 × "Cache.[v:brandId].Search"
```

So two-thirds of every topic string in the corpus is restatement, and the
declaration line was carrying it. The fix is not a better default — it is
noticing that a topic is **one more C# attribute** and does not need syntax:

```protop
[DataCacheTopic("Cache.[v:brandId].SportGame")]
view SlimSportGame of SportGame {
```

That is shorter than the keyword form was, it says where the string *goes*, and
it is `passthrough` — already a term in `CONTEXT.md`. protopc copies the text
and does not read it.

The same move retires `[DataCacheFromTimestamp]`, which had earned a
`$from_timestamp` spelling it did not deserve, and it gives the long tail of
`B2Tech.Feed.Cache` attributes a home without a keyword each.

## 2. An index is not a fact about data

Dropping `index` was the larger cut. The evidence that it belongs outside the
schema is already in the corpus:

**Relations do not imply indexes.** `EventId` is a foreign key in **10** types
and indexed in only **5** — in the other five it is the identity and carries no
index at all. The two axes have been independent here all along.

**Indexes do not imply relations.** Of 30 member-level indexes:

| what is indexed | n | could a relation generate it? |
|---|---|---|
| FK-shaped (`SportTypeId`, `EventId`, `LeagueId`…) | 10 | yes |
| enum (`EventStatus`, `WatchDogAction`, `BookingStatus`…) | 10 | no |
| temporal, `range` (`EventDate`, `ProviderTimestamp`…) | 7 | no |
| bool (`IsLive`) | 2 | no |
| derived (`FixtureIdIndex`) | 1 | no |

**Two-thirds of the corpus's indexes are query-shape decisions**, not structural
ones: which enum you filter on and which timestamp you sort by is a fact about
the application's read patterns, and it changes without the data changing.

This is the honest cost of the cut, and it should be stated plainly: those 20
indexes do not come back from relations. They move out of the schema and become
the application's business — either built at runtime, or written as
`[DataCacheIndex]` passthrough by whoever wants them at compile time. The
schema stops pretending to know.

## 3. Cutting `index` deleted a whole class of computed member

The previous pass found computed members decompose into three classes, none of
which needs an expression. One of the three was the **derived index**:

```protop
index(many) int32 fixture_id_index = fixture_id else 0;   // no tag ⇒ no wire slot
```

`FixtureIdIndex => FixtureId ?? 0` is `[IgnoreMember]`. It is not on the wire.
It exists for **no reason except to be indexed** — so when the index left, the
member left with it, and `else` left with the member.

Two classes remain, and they are the two that were never about indexing:

| class | n | wire slot? | corpus |
|---|---|---|---|
| composite identity | 5 | **yes** — always `[Key(0)]` | `(long EventId, long BetTypeId) Key => (EventId, BetTypeId)` |
| alias | 1 | no — `[IgnoreMember] [JsonIgnore]` | `long BetTypeId { get => Id; set => Id = value; }` |

[Issue #6](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/6) has
the "computed-member escape hatch" down as the hardest requirement on the board,
on the assumption that arbitrary C# expressions must survive the trip. It is now
**two keywords**, neither of them an expression — `key … ( … )` and `alias`.

## 4. FK + denormalized snapshot is the corpus's real relational shape

Grepping for every `*Id` field that has a same-named sibling found **15 pairs**,
5 distinct relations across 3 types:

```
SportTypeId  long   <->  SportType   StaticDataRef
CountryId    long   <->  Country     StaticDataRef
LeagueId     long   <->  League      StaticDataRef
HomeTeamId   long?  <->  HomeTeam    StaticDataRef?
AwayTeamId   long?  <->  AwayTeam    StaticDataRef?
```

The navigation member is not a pointer — it is a **denormalized copy** of the
target, on the wire, at its own tag. One relation, two members, two slots.

**And they drift.** In `SlimSportGame`:

```csharp
public long? HomeTeamId { get; set; }              // nullable
[Key(57)]
public StaticDataRef HomeTeam { get; set; } = null!;   // NOT nullable
```

where `BOSportGameCacheItem` has `StaticDataRef? HomeTeam`. The foreign key
admits it may be absent; its own snapshot, in the same type, does not. That is
a nil slot waiting to become a `NullReferenceException`, and the C# compiler
cannot see it because `= null!` is exactly the syntax for telling it not to.

This is what pays for a relations block: with the pair declared as one thing,
`home_team` does not get to have an opinion about presence.

## 5. Relations as a block, not a modifier

Field lines stay pure — type, name, tag, presence — which is exactly the wire
contract and nothing else. Relational structure goes in one place:

```protop
table SportGame {
  key int64 event_id = 1;

  int64  sport_type_id = 2;
  int64? home_team_id  = 5;

  StaticDataRef  sport_type = 55;
  StaticDataRef? home_team  = 58;

  relations {
    sport_type_id -> SportType inline sport_type;
    home_team_id  -> Team      inline home_team;
  }
}
```

Three things fall out of this shape:

**`-> SportType` names a table, not a field.** It resolves to that table's
*declared key*, so arity and type are checked, and a composite-key target needs
no new syntax at all — the key is already declared, once, at the target.

**`repeated` vs `relations` is a real line, drawn by the data.** `Runner`, `Odd`
and `SlimOdd` declare no `[DataCacheKey]`, so nothing can point at them and they
cannot exist apart from their row. `repeated` is composition; `relations` is
reference; the test is whether the child type declares a key.

**A view inherits relations with its fields** and declares none of its own — it
has no fields of its own to relate.

**Fog, marked as such:** the corpus has **no many-to-many**. The syntax carries
`many … <-> … through …` because it was asked for, but nothing here grounds it,
and an ungrounded construct is a guess. *A decision that cannot express the
corpus is not done* — the inverse also holds: a construct the corpus never
exercises is not tested.

## 6. Annotations should be spelled the way they are emitted

The target is a C# attribute. Spelling it as a C# attribute costs the reader
nothing and costs protopc nothing, because protopc does not read it:

```protop
[DataCacheTopic("Cache.[v:brandId].BetType")]
table BetType {
  [DataCacheFromTimestamp]
  int64 timestamp = 25;
}
```

Spelling A cannot have one name for this. Measured:

```
protop/options.proto:100:19: "attribute" is already defined in "protop".
```

Extension field names live in a **flat package namespace**, so one name cannot
extend both `MessageOptions` and `FieldOptions`. Spelling A needs
`type_attribute` and `field_attribute`, and every author has to know which
context they are in. Spelling B has one bracket, wherever it goes.

A related small finding: **retired extension numbers cannot be reserved.**
`reserved` is a message-level statement and protoc rejects it inside `extend`,
so `50020 was cache_topic` can only ever be a comment — in a language whose
entire pitch is that numbers are the contract.

## 7. Presence still costs 145 annotations

Unchanged by this pass, and still the largest single cost in spelling A.
Editions 2023 defaults `field_presence` to **EXPLICIT** — nullable. The corpus
is the other way round:

```
333 members across 18 corpus types
188 non-nullable  (56%)
145 nullable      (44%)
```

A protop file flips the default at file scope (`option
features.field_presence = IMPLICIT;` — features inherit, custom options do not,
which is the only reason this works) and **145 members opt back in one at a
time**. In spelling B the same information is one character: `int64?`.

This lands on the axis the whole project is about — *field presence is the
nullable/NOT NULL axis* — and in spelling A that axis is the noisiest thing in
the file.

## 8. protop cannot demand explicitness uniformly

```
h.proto:4:19: Message fields can't specify implicit presence.
```

A message-typed field may not carry `IMPLICIT`, and `EXPLICIT` is its only legal
value, so writing it is redundant noise. The obvious strict-profile rule —
*every field states its presence* — is **not implementable** in spelling A.
File-scope `IMPLICIT` inherits silently past message fields (rc=0, no
diagnostic), so the behaviour is correct. It just cannot be written down.

## 9. `FIRST_TAG_IS_ONE` rejects two corpus types — independent of spelling

```
Venue      first key = 1  →  first tag = 2
BOVenue    first key = 1  →  first tag = 2
everything else            first key = 0  →  first tag = 1
```

**The only two types that break the rule are the two structs.** Renumbering them
moves every member one slot and breaks the wire. So the rule gains an exception,
becomes a warning protop cannot have (all diagnostics are errors), or two corpus
types are inexpressible. A conflict between two settled decisions, visible only
once a file was written out.

## 10. Identity is two constructs, not one

Eleven `[DataCacheKey]` members, splitting cleanly:

| form | count | example |
|---|---|---|
| identity **is** a stored field | 6 | `BOSportGameCacheItem.EventId` — `[Key(0)] long` |
| identity is a **tuple over** stored fields | 5 | `BOBetTypeCacheItem.Key => (EventId, BetTypeId)` |

In the second, **tag 1 holds a duplicate**: the tuple is `[Key(0)]`, written to
the wire, carrying a copy of what tags 2 and 3 already carry. Every composite
key in the corpus is at tag 1, without exception.

```protop
key int64 event_id = 1;                    // identity is the field
key key = 1 (event_id, bet_type_id);       // identity is a tuple, and owns tag 1
```

The `= 1` says which slot the duplicate occupies, which makes the collision
checkable. **Spelling A cannot check it** — measured, protoc compiles this
without a word:

```proto
option (protop.composite_key) = { name: "key" tag: 1 field: "zzz_no_such_field" };
int64 collides = 1;      // same tag the option claimed
```

`reserved 1;` and *a field at 1* are the only two things protoc understands
about a slot. "An option owns this slot" is not expressible.

Note the tension with
[the get-only-member decision](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/22):
protopc was to **refuse to emit** a computed member at key 0. Five corpus types
have exactly that, and it is their identity.

## 11. Views rename, and view padding is structural

`BOSportGameCacheItem.EventId` at `[Key(0)]` is `SlimSportGame.Id` at `[Key(0)]`.
Under int-key MessagePack **names are not on the wire**, so a rename is free —
which is also why `Key` / `Id` / `BetTypeId` drift across the corpus pairs with
nobody noticing. A view syntax therefore needs `as`.

```
SlimOdd        14 members → 28 slots   (50% nil)
SlimBetType    14 members → 26 slots   (46% nil)
SlimSportGame  41 members → 59 slots   (30% nil)
```

A view keeps the parent's tags, so it inherits the parent's *maximum* tag, so it
pays padding proportional to what it dropped. A MessagePack dense-array fact,
not a language fact, and neither spelling fixes it.

One thing this pass changed: with the topic and the index set both gone from the
declaration, a view is no longer a second copy of its source. `SlimSportGame`
indexed `SportTypeId`, `CountryId` and `LeagueId` where `BOSportGameCacheItem`
indexed none of them — that divergence was the loudest thing in the old view
syntax, and it was never schema in the first place.

## 12. What was borrowed, and from where

| loan | source | what it gave |
|---|---|---|
| `struct` vs `table` | FlatBuffers | The split, *and the rule*: a FlatBuffers struct is all-fields-present, no defaults, scalars only, no evolution ever — recommended only "for simple objects where you are very sure no changes will ever be made". `Venue` is two `required` scalars that have never changed. The keyword comes with its justification attached. |
| `key` | FlatBuffers `(key)` | A field nominated as the identity of its type. Promoted from attribute to modifier because here it is load-bearing rather than advisory (FlatBuffers uses it only to sort a vector for binary search). |
| composite `key … ( … )` | Cap'n Proto `group` | A name over fields that keep their own ordinals. The difference is instructive: a Cap'n Proto group consumes **no** ordinal of its own, and protop's composite key **must**, because the corpus wire format already spends a slot on it. |
| `[Attribute]` | C# itself | The passthrough spelling. Replaces the `$annotation` loan from Cap'n Proto, which was a third syntax for a thing that already has two. |
| explicit ordinals everywhere | Cap'n Proto `@N` | Already how protop works, but worth noting the company: Cap'n Proto makes ordinals mandatory and unreusable for exactly protop's reason — the number, not the name, is the contract. |

Not borrowed, deliberately: FlatBuffers' `(id: n)` (protop's tags are always
written), `(deprecated)` (Protobuf's `reserved` already covers it), and
`(required)` (protop's presence axis is the same idea with better defaults).

Left on the table: FlatBuffers' and Cap'n Proto's `union`. The corpus has one
member that needs it — `IBaseLiveData? LiveObject`, an interface with **19
`[Union]` attributes** — and neither specimen attempts it. Protobuf's `oneof` is
the obvious strict-profile answer and it is not obviously the right one. Fog,
not a finding.

---

## The scoreboard

Options round-trip fine. Measured on a deliberately broken file — a relation
naming a table that does not exist, a field that does not exist, an `inline`
member that does not exist, and an attribute string that is not valid C#:

```
protoc: ACCEPTED ALL OF IT (rc=0)
ThisIsNotEven    PRESENT in the descriptor
NoSuchTable      PRESENT
no_such_field    PRESENT
no_such_member   PRESENT
```

So protopc **can** read spelling A out of a descriptor set and never write its
own parser. Spelling A's problem is not that it cannot carry the information:

| construct | A carries it | A checks it | B checks it |
|---|---|---|---|
| shape | ✅ option | n/a | ✅ |
| presence | ✅ feature | ✅ | ✅ |
| simple key | ✅ option | n/a | ✅ |
| composite key | ✅ option | ❌ names, tag collision | ✅ |
| relation target | ✅ option | ❌ exists, is a table, has a key | ✅ |
| relation FK type | ✅ option | ❌ matches target's key | ✅ |
| relation `inline` | ✅ option | ❌ exists, type, **presence agrees** | ✅ |
| alias | ✅ option | ❌ field name | ✅ |
| view source | ✅ option | ❌ existence, shape | ✅ |
| view field/tag/type/presence | ✅ option | ❌ all four | ✅ |
| decimal scale | ✅ option | ❌ applicability | ✅ |
| attribute placement | ⚠️ two option names | ❌ | ✅ one spelling |

Every ❌ is a diagnostic protopc has to implement itself. The strict-profile bet
— *"lean on protoc so protopc does less"* — pays off for the Protobuf core and
**not at all** for anything protop adds. protopc writes those checks either way;
A only decides whether they run against a descriptor set or against its own AST.

Cutting the topic and the index made spelling A's ledger *worse*, not better:
both were unchecked strings, and what replaced them — relations — is the most
check-hungry construct in the language. The `inline` presence check alone would
have caught a live bug in the corpus.

## What leaving Protobuf would cost

1. **The oracle dies.** The differential harness is descriptor equality against
   `protoc --descriptor_set_out`. `protoc` cannot read spelling B, so there is
   nothing to differentiate against.
2. **[The versioning contract](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/27)
   is void in its carrier half.** The dialect stamp had to be a custom option
   precisely because a keyword makes the file unparseable by `protoc`. In B,
   `revision 1;` is just a statement, and the mandatory
   `import "protop/options.proto"` disappears along with
   [ticket #28](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/28).
3. **`CONTEXT.md`'s definition of protop changes** — *"protop adds no syntax that
   `protoc` would refuse"* stops being true.
4. **Every Protobuf tool stops working**: editors, formatters, linters, `buf`,
   and anyone who wanted a Go or Java reader from the same file.

## Option C, which neither specimen is

**Write B, lower to A.** protopc parses the native spelling and emits the
strict-Protobuf form as a byproduct; the harness runs `protoc` on *that*. The
oracle survives, the versioning carrier survives, the ergonomics are B's, and
relations and views stay checkable because the lowering is where the check
happens.

What it costs: a second emitter to write and keep honest, and a round-trip
property to test (*lower, compile with protoc, compare descriptors*) that does
not exist today. And two files exist for every one the author wrote, which has
to be explained to anyone reading a stack trace.

This is the option worth discussing, and it is not on the map in any form.
