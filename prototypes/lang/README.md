# Language prototype: shapes, identity, relations, views

Two spellings of the same content, written so they can be read side by side.

| | |
|---|---|
| [`a-strict-protobuf/prague.proto`](a-strict-protobuf/prague.proto) | protop as a strict profile. **Verified: compiles under `protoc` 36.1.** |
| [`a-strict-protobuf/protop/options.proto`](a-strict-protobuf/protop/options.proto) | the option vocabulary spelling A needs. |
| [`b-native/prague.protop`](b-native/prague.protop) | protop as its own language. **Verified: `protoc` rejects it at line 122, `revision 1;`.** |

The first pass asked whether **table / record / struct** could be spelled at all.
Every pass since has been a subtraction, and every subtraction has paid:

| cut | why | finding |
|---|---|---|
| `topic` | a C# attribute wearing a keyword | 1 |
| `index` | a decision about queries, not a fact about data | 2 |
| derived index member | existed only to be indexed | 3 |
| `relations { }` block | a fact about a member belongs on the member | 5 |
| `snapshot_of` | real, but nothing in phase 1 consumes it — deferred, not retired | 5a |
| cardinality keywords, `ManyToMany` | derivable from the FK and the declaring type's `key` | 5b |

What went *in* is one line: a view member may be narrowed to a view of its own
type (`SlimOdd odds;`), which the corpus has been doing all along with no way to
say so (finding 11).

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
it is `passthrough` — already a term in `CONTEXT.md`. praguec copies the text
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

[Issue #6](https://github.com/praguedb/praguec/issues/6) has
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

This is what pays for `snapshot_of`: with the pair declared as one thing,
`home_team` does not get to have an opinion about presence separate from the id
it accompanies.

### …and the copy exists *because the referent does not*

The obvious next question is why the copy is there at all — why not point at a
`SportType` and fetch it? Because **there is no `SportType`**. Measured across
every corpus type that carries `[DataCacheKey]`:

| foreign-key-shaped member | n | names a cached type? | carries a `StaticDataRef` copy? |
|---|---|---|---|
| `EventId`, `BetTypeId` | 11 | **yes** | **never** |
| `SportTypeId`, `CountryId`, `LeagueId`, `HomeTeamId`, `AwayTeamId` | 25 | **no** | **always** — all 15 pairs |
| `StatisticsCenterId`, `PlayerId` | 4 | no | no — opaque ids, nothing attached |

The two sets are **exactly complementary**, and that is not a coincidence:

```csharp
public class StaticDataRef : BaseProviderObject { }   // empty
public class BaseProviderObject {
    [Key(0)] public long Id;   [Key(1)] public virtual string Name;
    [Key(2)] public string? ProviderSourceKey;   [Key(3)] public long ProviderKey;
}
```

`StaticDataRef` is an **empty subclass**, and it is the *identical type* for a
sport, a country and a team. So `long SportTypeId` + `StaticDataRef SportType` is
not a foreign key beside a cached row that could be fetched instead. **It is the
whole representation.** The alternative to the copy is not a join — it is nothing.

Two consequences, and the first cost me four declarations:

**The specimens had `table SportType`, `Country`, `League` and `Team` in them.**
None exists in the corpus. They were invented so `references(SportType)` would
have somewhere to point — inventing corpus to fit a feature, which is the exact
failure the "a construct the corpus never exercises is not tested" rule exists to
catch. Deleted from both spellings.

**`references` and `snapshot_of` are not two flavours of one thing.** They were
merged in the block form, which let a single `relation` carry `target:` *and*
`inline:`. The corpus says they never co-occur. `snapshot_of` binds two members
of the same message and needs no target type to exist — which is why it survives
this finding intact, and why its presence check still catches the live bug.

**Scope limit, stated plainly:** the corpus is the cache's payload types, not the
whole system. A `SportType` entity may well exist upstream — in a database, in a
service. What is measured here is that *nothing in the cache protop generates for*
is pointable-at. Whether that holds beyond the sample is a question for someone
who knows the wider system.

**And the one real relation is ambiguous.** `BetType.EventId` does resolve — to
**six types at once**. `BOSportGameCacheItem`, `BPSportGameCacheItem`,
`BOLiveObjectCacheItem`, `BPLiveObjectCacheItem`, `LiveObjectCacheItem` and
`SlimSportGame` all declare `EventId` as their identity, because they are twins
and projections of one another sharing an identity *and* a key space. So
`references(SportGame)` does not pick one out, in either spelling. Unanswered.

## 5. An annotation absorbs a unary fact for free; a binary one costs a name

Relations first went in as a block, on the argument that field lines should stay
pure — type, name, tag, presence, the wire contract and nothing else:

```protop
relations {
  sport_type_id -> SportType inline sport_type;
  home_team_id  -> Team      inline home_team;
}
```

That argument does not survive its own file. The previous pass had just
established `[Attribute]` as *the* way a fact attaches to a member; the block is
a second declarative form for one kind of fact. And it is the expensive form.
Measured, in `prague.proto`:

| | member-naming strings |
|---|---|
| relations as a block | **11** — `field: "sport_type_id"`, `inline: "sport_type"` |
| relations on the member | **6** |

Every one of the 11 is a string naming a member of the same message, and protoc
checks **none** of them (measured — see the scoreboard). Halving them is not the
finding, though. The finding is *which five went*, because the split is exact:

**`references` is unary** — "this member holds `Team`'s identity" is a fact about
one member. Written on that member, it has no field name left to get wrong:

```protop
int64? home_team_id = 5 [references(Team)];
```

An orphaned `references` is now **unrepresentable**, not merely unchecked. That
is a strictly better failure mode, and spelling A gets it too. (What it does
*not* fix is `references` naming a type that does not exist — see finding 4,
where that turned out to be 25 of the corpus's 36 foreign-key-shaped members.)

**`snapshot_of` was binary** — it pairs a foreign key with its denormalized copy,
which are two different members at two different tags. One of the two must name
the other, and no choice of where to write the annotation changes that:

```protop
StaticDataRef? home_team = 58 [snapshot_of(home_team_id)];
```

It is **deferred out of phase 1** (see finding 5a), but it is the cleanest
example of the binary case and so is kept here as one.

The rule generalises across the whole file. Every member-naming string left in
`prague.proto` is binary — 4 composite-key components, 1 alias target, 2 view
renames — and every one that vanished was unary. **A relation between two
members always costs a name; a property of one member never does.**

Three things survive unchanged from the block form:

**`references(SportType)` names a table, not a field.** It resolves to that
table's *declared key*, so arity and type are checked, and a composite-key target
needs no new syntax at all.

**`repeated` vs a relation is a real line, drawn by the data.** `Runner`, `Odd`
and `SlimOdd` declare no `[DataCacheKey]`, so nothing can point at them and they
cannot exist apart from their row. `repeated` is composition; a relation is
reference; the test is whether the child type declares a key.

**A view inherits relations with its fields**, and now does so by construction.
Under the block form this had to be a *rule* — "a view may not declare relations
of its own". On the member it is simply what happens: the annotation travels with
the member the view lists, or the member is not there.

**Many-to-many needed no message-level form after all.** It was the one relation
with no single owning member, and so looked like the one that had to stay on the
message. Finding 5b removes it entirely: a type whose key is exactly two foreign
keys *is* the join table, and that is already written down. The corpus has no
such type, so it stays fog — but fog with a derivation waiting for it rather than
an unchecked option standing in.

## 5a. `snapshot_of` is deferred, and the gap it leaves is exact

Deferred out of phase 1. The pairing is real and measured — 15 corpus pairs — but
nothing in phase 1 consumes it, and a construct nothing consumes is a construct
nothing tests.

What deferring costs, stated precisely rather than waved at: the one check it
bought was **that the two halves agree about presence**, and **3 of the 15 pairs
do not**. `SlimSportGame` carries

```csharp
public long? HomeTeamId { get; set; }
public StaticDataRef HomeTeam { get; set; } = null!;   // not nullable
```

protop phase 1 emits that as written and says nothing. Known, accepted gap.

Spelling A could barely have checked it anyway: a message field may not carry
`IMPLICIT` presence (protoc errors) and `EXPLICIT` is its only legal value, so
all five snapshot lines look identical whatever the truth is. The check was
always going to be B's, or praguec's on top of A.

Extension `50041` is **held, not retired** — the fact it named did not stop being
true.

## 5b. Cardinality is derivable, so it is not declared

The question that prompted it: `SportGame` has `league_id`; how does anyone know
one League is in many Games?

Compare the foreign key against **the declaring type's own `key`**:

| FK vs. the declaring type's key | cardinality |
|---|---|
| FK **is** the whole key | one-to-one — at most one row per target row, *enforced by the key* |
| FK is **part of** a composite key | many-to-one — the rest of the composite is exactly what distinguishes the many |
| FK is **not in** the key | many-to-one, unconstrained |

One-to-many is that same fact read backwards, so it needs no declaration either.
Many-to-many likewise: a key that is exactly two foreign keys *is* a join table.

Checked against the corpus:

| type | key | `event_id` | derived | right? |
|---|---|---|---|---|
| `BOBetTypeCacheItem` | `(EventId, BetTypeId)` | part of it | many BetTypes per game | ✅ |
| `LiveObjectCacheItem` | `EventId` | **is** it | exactly one per game | ✅ |
| `BOSearchCacheItem` | `(EventId, ChannelId)` | part of it | many per game | ✅ |
| `SportGame.league_id` | `EventId` | not in it | many games per league | ✅ |

**The rule is sound, not complete**, and the two are different promises. Every
one-to-one it derives is enforced by a key and cannot be wrong. A many-to-one it
derives *may* be one-to-one in truth — an FK outside the key that happens to be
unique — with nothing in the schema saying so.

Note which way that errs. The derivation never invents a guarantee; it only
declines to promise one. A declared `one_to_one` errs the other way, asserting a
uniqueness **nothing checks** — strictly worse, because code will trust it. That
is the same failure that took `topic` out of the language: unchecked restatement
of something the file already carries.

The **extension table** is the case that looks like a counterexample and is not.
A type existing only to carry more columns for one `SportGame` declares
`key event_id`, case one fires, and one-to-one is derived correctly with no
keyword at all.

So: no cardinality vocabulary in either spelling. Spelling A's `Cardinality` enum
and its message-level `ManyToMany` are both gone; `50022` is retired.

## 6. Position tells the two kinds of annotation apart, and a name cannot

protop's brackets carry two unrelated things: **protop's own options**, which
praguec reads and checks, and **verbatim C# attribute text**, which praguec
copies without reading. Something has to distinguish them.

The first attempt was lexical — `[lowercase(...)]` checked, `[PascalCase(...)]`
verbatim. It works, and it is wrong: it makes capitalization load-bearing, so a
typo in a protop annotation silently becomes a passthrough attribute and fails at
**C# compile time instead of protop compile time**. protop has no warnings; a
mistyped annotation must be an error.

Position is free, structural, and cannot be mistyped into the other meaning:

```protop
[DataCacheTopic("Cache.[v:brandId].SportGame")]     // C#'s slot    → copied
table SportGame {
  int64 sport_type_id = 2 [references(SportType)];  // Protobuf's slot → checked

  [DataCacheFromTimestamp]                          // both on one member,
  int64 timestamp = 25;                             // no collision
}
```

Each annotation sits where its **own language** already puts it: C# attributes
before the declaration, options after the field. Nothing new is invented, and the
reader never inspects a name to learn whether a bracket is compiled or copied.

**Why the C# attribute is *before* and not after** — the reason is not symmetry
with C#, it is **verbatim-ness**. The passthrough has to survive a round trip:
paste the text out of the C# into protop, and back out again, byte for byte. Two
measurements say a bracket cannot host it.

*The text contains brackets and quotes.* The corpus's topic attribute is
`[DataCacheTopic("Cache.[v:brandId].SportGame")]` — a `[`, a `]` and two `"`
inside the thing that would go inside a `[…]`. Inside a bracket that already
means something it must become a **string**, and a string must be escaped.
Spelling A is the proof:

```proto
option (protop.type_attribute) = "[DataCacheTopic(\"Cache.[v:brandId].SportGame\")]";
```

*Attributes stack.* **55 of the corpus's 356 attribute-carrying sites carry two
or more**, up to four:

| attributes on one member | sites |
|---|---|
| 1 | 301 |
| 2 | 43 |
| 3 | 11 |
| 4 | 1 |

C# stacks them one per line; so does B. A bracket list after the tag would crowd
them onto the field line beside the wire contract — the one line that has to stay
legible.

So the field line carries everything praguec **understands** (type, name, tag,
presence, checked options) and the text it does not understand sits above,
quoted by nothing and escaped by nothing. The cost is that a reader learns two
positions — one sentence, and the same sentence in both of the languages the
file mixes.

**Spelling A cannot have this rule** — it has exactly one slot, so its checked
options and its verbatim C# text share the bracket:

```proto
int64 timestamp = 25 [(protop.field_attribute) = "[DataCacheFromTimestamp]"];
```

Worse, A cannot even have one *name* for passthrough. Measured:

```
protop/options.proto:100:19: "attribute" is already defined in "protop".
```

Extension field names live in a **flat package namespace**, so one name cannot
extend both `MessageOptions` and `FieldOptions`. A needs `type_attribute` and
`field_attribute`, and every author must know which context they are in. B has
one bracket per slot, and the slot says which.

**The rule extends to the type level, where it costs nothing to settle now.** The
annotation follows what it describes, so on a type it goes after the declaration
head:

```protop
table Foo [something] { ... }
```

One rule at both levels. The slot is **defined and empty**: protop has no
type-level annotation of its own today — `topic` became C# passthrough, `shape`
is a keyword, and cardinality turned out to be derivable (finding 5b). Defining
it anyway is deliberate. It is where the next one goes, and settling that while
nothing depends on it costs nothing, where discovering it later invites a third
syntax. Protobuf's own answer at that level is a body statement (`option x = y;`)
— B does **not** borrow that, because it would put the same kind of fact in two
different places depending only on whether it describes a type or a field.

Recorded as a cost: nothing yet exercises the type-level slot, so the rule above
a field is asserted and not tested.

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
[the get-only-member decision](https://github.com/praguedb/praguec/issues/22):
praguec was to **refuse to emit** a computed member at key 0. Five corpus types
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

### A view member can itself be narrowed to a view — and the corpus does it

Measured, at the same tag:

```csharp
BOBetTypeCacheItem.Odds   is   List<Odd>
SlimBetType.Odds          is   SlimOdd[]
```

A view of a view, and no version of the syntax could say so until now. The
spelling falls out of what a view line already is — a bare member name takes the
source's type unchanged, so writing a **type before the name** is what asks for
the narrowing:

```protop
view SlimBetType of BetType {
  name;              // tag 4, BetType's type
  SlimOdd odds;      // tag 26, narrowed — and SlimOdd must be a view of Odd
}
```

That last clause is the whole check, and it is worth naming what it buys: the
view stays pinned to the *source's* tags however deep the nesting goes, so
`SlimOdd` cannot quietly renumber and desync tag 26.

`SlimOdd` also settles a smaller question by existing: **a view's source need not
be a table.** `Odd` is a record — no key, nothing points at it — and it has a
view all the same, because a view is about tags and members, not identity.

**Spelling A cannot express the narrowing at all.** It writes
`repeated SlimOdd odds = 26;`, which is indistinguishable from a *type
disagreement at a shared tag* — which is drift, the exact thing views exist to
prevent. protoc reads an unrelated message with an unrelated field at tag 26 and
says nothing.

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

So praguec **can** read spelling A out of a descriptor set and never write its
own parser. Spelling A's problem is not that it cannot carry the information:

| construct | A carries it | A checks it | B checks it |
|---|---|---|---|
| shape | ✅ option | n/a | ✅ |
| presence | ✅ feature | ✅ | ✅ |
| simple key | ✅ option | n/a | ✅ |
| composite key | ✅ option | ❌ names, tag collision | ✅ |
| relation target | ✅ field option | ❌ exists, is a table, has a key | ✅ |
| relation FK type | ✅ field option | ❌ matches target's key | ✅ |
| relation owner | ✅ **by construction** | ✅ **unrepresentable if wrong** | ✅ |
| snapshot pairing | *deferred (5a)* | — | — |
| cardinality | *derived, not carried (5b)* | n/a | n/a |
| alias | ✅ option | ❌ field name | ✅ |
| view source | ✅ option | ❌ existence, shape | ✅ |
| view field/tag/type/presence | ✅ option | ❌ all four | ✅ |
| view member narrowed to a view | ❌ **inexpressible** — reads as drift | ❌ | ✅ `SlimOdd odds;` |
| decimal scale | ✅ option | ❌ applicability | ✅ |
| attribute placement | ⚠️ two option names | ❌ | ✅ one slot each |
| checked vs verbatim | ⚠️ one shared bracket | ❌ | ✅ by position |
| type-level annotation slot | ✅ `option` in the body | n/a | ✅ `table Foo [x]` — defined, empty |

Every ❌ is a diagnostic praguec has to implement itself. The strict-profile bet
— *"lean on protoc so praguec does less"* — pays off for the Protobuf core and
**not at all** for anything protop adds. praguec writes those checks either way;
A only decides whether they run against a descriptor set or against its own AST.

Cutting the topic and the index made spelling A's ledger *worse*, not better:
both were unchecked strings, and what replaced them — relations — is the most
check-hungry construct in the language. The `snapshot_of` presence check alone
would have caught a live bug in the corpus.

One row moved the other way, and it is the only ✅ in A's check column that was
*earned* rather than inherited from protoc. Putting `references` on the member it
describes deleted the field name it used to carry, so "this relation names a
member that does not exist" stopped being a diagnostic anybody has to write. The
cheapest check is the one made unrepresentable — and it is available in **both**
spellings, because it is a fact about where an annotation sits, not about which
parser reads it.

## What leaving Protobuf would cost

1. **The oracle dies.** The differential harness is descriptor equality against
   `protoc --descriptor_set_out`. `protoc` cannot read spelling B, so there is
   nothing to differentiate against.
2. **[The versioning contract](https://github.com/praguedb/praguec/issues/27)
   is void in its carrier half.** The dialect stamp had to be a custom option
   precisely because a keyword makes the file unparseable by `protoc`. In B,
   `revision 1;` is just a statement, and the mandatory
   `import "protop/options.proto"` disappears along with
   [ticket #28](https://github.com/praguedb/praguec/issues/28).
3. **`CONTEXT.md`'s definition of protop changes** — *"protop adds no syntax that
   `protoc` would refuse"* stops being true.
4. **Every Protobuf tool stops working**: editors, formatters, linters, `buf`,
   and anyone who wanted a Go or Java reader from the same file.

## Option C, which neither specimen is

**Write B, lower to A.** praguec parses the native spelling and emits the
strict-Protobuf form as a byproduct; the harness runs `protoc` on *that*. The
oracle survives, the versioning carrier survives, the ergonomics are B's, and
relations and views stay checkable because the lowering is where the check
happens.

What it costs: a second emitter to write and keep honest, and a round-trip
property to test (*lower, compile with protoc, compare descriptors*) that does
not exist today. And two files exist for every one the author wrote, which has
to be explained to anyone reading a stack trace.

This is the option worth discussing, and it is not on the map in any form.
