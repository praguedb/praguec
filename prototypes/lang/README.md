# Language prototype: shapes, keys, computed members, views

Two spellings of the same content, written so they can be read side by side.
Both model the same corpus types, now including a composite-key table, all three
classes of computed member, and two views.

| | |
|---|---|
| [`a-strict-protobuf/prague.proto`](a-strict-protobuf/prague.proto) | protop as a strict profile. **Verified: compiles under `protoc` 36.1.** |
| [`a-strict-protobuf/protop/options.proto`](a-strict-protobuf/protop/options.proto) | the option vocabulary spelling A needs. |
| [`b-native/prague.protop`](b-native/prague.protop) | protop as its own language. **Verified: `protoc` rejects it at `revision 1;`.** |

The immediate motivation was the **table / record / struct** distinction, which
Protobuf has no way to spell — it has exactly one declaration form, `message`.
Writing both out turned up nine things, only two of which are about shapes.

---

## 1. The shape axis is cheap in both spellings

One `option (protop.shape) = SHAPE_STRUCT;` line per type in A, one keyword in
B. This is *not* where the cost is, and it is not on its own an argument for
leaving Protobuf. Worth saying plainly, because it was the thing that prompted
the question.

Semantics that fall out, unchanged by spelling:

- **struct** → C# `struct`. Value payload, embedded, no identity.
- **record** → C# `sealed partial class`. Payload, no identity.
- **table** → C# `sealed partial class` + the `B2Tech.Feed.Cache` attributes, with
  identity.

## 2. Presence costs 145 annotations

Editions 2023 defaults `field_presence` to **EXPLICIT** — nullable. The corpus
is the other way round:

```
333 members across 18 corpus types
188 non-nullable  (56%)
145 nullable      (44%)
```

So a protop file flips the default at file scope (`option
features.field_presence = IMPLICIT;` — features inherit, custom options do not,
which is the only reason this works at all) and then **145 members opt back in
one at a time**, each carrying `[features.field_presence = EXPLICIT]`.

In spelling B the same information is one character: `int64? home_team_id = 5;`.

This is the real cost, and it lands on the axis the whole project is about —
the thesis is *field presence is the nullable/NOT NULL axis*, and in spelling A
that axis is the noisiest thing in the file.

## 3. protop cannot demand explicitness uniformly

Measured:

```
h.proto:4:19: Message fields can't specify implicit presence.
```

A message-typed field may not carry `IMPLICIT`, and `EXPLICIT` is its only legal
value, so writing it is redundant noise. A protop rule of the form *"every field
states its presence"* — the obvious strict-profile rule — is **not
implementable** in spelling A: there is a whole class of field where protoc
rejects one value and the other is vacuous.

File-scope `IMPLICIT` inherits silently *past* message fields (rc=0, no
diagnostic), so the behaviour is correct. It just cannot be written down.

## 4. `FIRST_TAG_IS_ONE` rejects two corpus types — independent of spelling

With `Key = tag - 1`, the rule was meant to eliminate nil padding. Measuring
every corpus type's lowest key:

```
Venue      first key = 1  →  first tag = 2
BOVenue    first key = 1  →  first tag = 2
everything else            first key = 0  →  first tag = 1
```

**The only two types that break the rule are the two structs.** Renumbering them
to tag 1 moves every member one slot and breaks the wire. So one of these has to
give: the rule gains an exception, the rule becomes a warning protop cannot
have (all diagnostics are errors), or two corpus types are inexpressible — and
*"a decision that cannot express the corpus is not done."*

This has nothing to do with A vs B. It is a conflict between two already-settled
decisions that only became visible by writing a file out.

## 5. Identity is two constructs, not one

The corpus has eleven `[DataCacheKey]` members and they split cleanly:

| form | count | example |
|---|---|---|
| identity **is** a stored field | 6 | `BOSportGameCacheItem.EventId` — `[Key(0)] long` |
| identity is a **tuple over** stored fields | 5 | `BOBetTypeCacheItem.Key => (EventId, BetTypeId)` |

These are not two spellings of one thing. In the first, tag 1 holds the value.
In the second, **tag 1 holds a duplicate**: the tuple is `[Key(0)]`, it is
written to the wire, and it carries a copy of what tags 2 and 3 already carry —
while the components keep their own tags. Every composite key in the corpus is
at `[Key(0)]`, i.e. tag 1, without exception.

So the language needs both:

```protop
key int64 event_id = 1;                    // identity is the field
key key = 1 (event_id, bet_type_id);       // identity is a tuple, and owns tag 1
```

The `= 1` on the composite form is doing real work. It says which slot the
duplicate occupies, which makes the collision checkable. **Spelling A cannot
check it.** Measured — protoc compiles all of this without a word:

```proto
option (protop.composite_key) = { name: "key" tag: 1 field: "a" field: "zzz_no_such_field" };
int64 collides = 1;      // same tag the option claimed
```

`reserved 1;` and *a field at 1* are the only two things protoc understands
about a slot. "An option owns this slot" is not expressible, so in spelling A
the composite key is an unchecked assertion sitting next to the fields it
contradicts.

Note the tension with
[the get-only-member decision](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/22):
protopc was to **refuse to emit** a computed member at key 0. Five corpus types
have exactly that, and it is their identity.

## 6. Computed members decompose into three classes — and none needs an expression

[Issue #6](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/6) has
"computed-member escape hatch" down as the hardest requirement on the board, on
the assumption that arbitrary C# expressions would have to survive the trip.
Grepping the corpus for every computed member found **seven**, in three classes:

| class | n | wire slot? | corpus |
|---|---|---|---|
| composite identity | 5 | **yes** — always `[Key(0)]` | `(long EventId, long BetTypeId) Key => (EventId, BetTypeId)` |
| derived index | 1 | no — `[IgnoreMember]` | `int FixtureIdIndex => FixtureId ?? 0` |
| alias | 1 | no — `[IgnoreMember] [JsonIgnore]` | `long BetTypeId { get => Id; set => Id = value; }` |

**Not one of the three is an expression.** They are declarations wearing
expression clothes:

- the tuple is *"identity is these fields, in this order"*;
- `?? 0` is *"when absent, index as 0"* — and the class-level attribute already
  says so declaratively:
  `[DataCacheIndex(nameof(FixtureIdIndex), nameof(FixtureId), DataCacheIndexType.Many)]`;
- `get => Id; set => Id = value;` is *"this name means that field"*.

```protop
key key = 1 (event_id, bet_type_id);
index(many) int32 fixture_id_index = fixture_id else 0;   // no tag ⇒ no wire slot
alias bet_type_id = id;                                   // no tag ⇒ no wire slot
```

If the corpus is representative, **the escape hatch largely dissolves** and the
hardest requirement on the board becomes three keywords. That is a claim to test
against a second corpus before it is believed, but it is the single most
valuable thing writing these files produced.

It also gives spelling B one structural rule that pays for itself everywhere:
**no tag ⇒ no wire slot.** In spelling A that fact is invisible — the composite
key and the alias are both message-level options, and nothing about their syntax
says one is serialized and the other is not.

And the alias has a *reason*, which was not obvious: `BOBetTypeCacheItem`
exposes `BetTypeId => Id` so that its API matches `SlimBetType`, which names the
same tag-3 field `BetTypeId` outright. The alias exists to paper over a naming
drift between a type and its view.

## 7. Views: a subset at the parent's tags — and a table in its own right

A **view** carries a subset of another type's fields *at the same tags*. In
spelling A, `SlimSportGame` is simply an unrelated `message` that happens to
reuse numbers; nothing in the file says what it is a view *of*, and nothing
checks the alignment. That check is why
[the cross-message key-alignment check](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/17)
exists as a ticket at all, and why `tools/keymap.py` found **7** divergences in
the corpus rather than the 1 that was known.

Spelling A can carry the *intent* as options, and that is as far as it gets.
Measured — protoc accepts every one of these:

```proto
message V {
  option (protop.view_of) = "NoSuchMessage";                          // no such type
  string wrong_type_entirely = 1 [(protop.view_field) = "also_does_not_exist"];
}
```

No check that the source exists, that it is a table, that tag 1 is that field,
that the types agree, that presence agrees, or that the view invents no tag.
Everything that makes a view *safe* is outside what the strict profile can say.

Two things the expanded specimens turned up that the first pass missed:

**Views rename.** `BOSportGameCacheItem.EventId` at `[Key(0)]` is
`SlimSportGame.Id` at `[Key(0)]`. Under int-key MessagePack **names are not on
the wire**, so a rename is free — which is also why `Key` / `Id` / `BetTypeId`
drift across the corpus pairs with no one noticing. A view syntax therefore
needs `as`:

```protop
view SlimSportGame of SportGame "Cache.[v:brandId].SportGame" {
  event_id as id;
  index sport_type_id;
  ...
}
```

**`view` and `table` are orthogonal axes, not alternatives.** `SlimSportGame`
and `SlimBetType` each carry their own `[DataCache]`, their own
`[DataCacheTopic]`, and — this is the surprise — **their own index set**.
`SlimSportGame` indexes `SportTypeId`, `CountryId` and `LeagueId`;
`BOSportGameCacheItem` indexes none of them. So a view is not a projection of
the parent's annotations, only of its fields and their tags. The view line
carries the name, the optional rename, and its own index kind; tag, type and
presence come from the source.

## 8. View padding is structural, and neither spelling fixes it

```
SlimOdd        14 members → 28 slots   (50% nil)
SlimBetType    14 members → 26 slots   (46% nil)
SlimSportGame  41 members → 59 slots   (30% nil)
```

A view must keep the parent's tags, so it inherits the parent's *maximum* tag,
so it pays padding proportional to what it dropped. That is a MessagePack
dense-array fact, not a language fact.

## 9. What was borrowed, and from where

| loan | source | what it gave |
|---|---|---|
| `struct` vs `table` | FlatBuffers | The split, *and the rule*: a FlatBuffers struct is all-fields-present, no defaults, scalars only, no evolution ever — recommended only "for simple objects where you are very sure no changes will ever be made". `Venue` is two `required` scalars that have never changed. The keyword comes with its justification attached. |
| `key` | FlatBuffers `(key)` | A field nominated as the identity of its type. Promoted from attribute to modifier because here it is load-bearing rather than advisory (FlatBuffers uses it only to sort a vector for binary search). |
| composite `key … ( … )` | Cap'n Proto `group` | A name over fields that keep their own ordinals. The difference is instructive: a Cap'n Proto group consumes **no** ordinal of its own, and protop's composite key **must**, because the corpus wire format already spends a slot on it. |
| `$annotation` | Cap'n Proto `$name` | A spelling for the long tail of `B2Tech.Feed.Cache` attributes protop has not met yet (`[DataCacheFromTimestamp]` is the first). A keyword is what an attribute *earns* by being load-bearing; `key` and `index` earned it, this has not. |
| explicit ordinals everywhere | Cap'n Proto `@N` | Already how protop works, but worth noting the company: Cap'n Proto makes ordinals mandatory and unreusable for exactly protop's reason — the number, not the name, is the contract. |

Not borrowed, deliberately: FlatBuffers' `(id: n)` (protop's tags are always
written), `(deprecated)` (Protobuf's `reserved` already covers it), and
`(required)` (protop's presence axis is the same idea with better defaults).

Left on the table: FlatBuffers' `union` and Cap'n Proto's `union`. The corpus
has one member that needs it — `IBaseLiveData? LiveObject`, an interface with
**19 `[Union]` attributes** — and neither specimen attempts it. Protobuf's
`oneof` is the obvious strict-profile answer and it is not obviously the right
one. This is fog, not a finding.

---

## What leaving Protobuf would actually cost

Spelling B is not free, and the bill is larger than the syntax:

1. **The oracle dies.** The differential harness is descriptor equality against
   `protoc --descriptor_set_out`. `protoc` cannot read spelling B, so there is
   nothing to differentiate against, and protopc becomes a compiler checked only
   by its own tests.
2. **[The versioning contract](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/27)
   is void in its carrier half.** The whole reason the dialect stamp had to be a
   custom option — rather than a `revision 1;` statement — is that a keyword
   makes the file unparseable by `protoc`. In spelling B, `revision 1;` is just
   a statement, and the mandatory `import "protop/options.proto"` disappears
   along with [ticket #28](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/28).
3. **`CONTEXT.md`'s definition of protop changes** — *"protop adds no syntax that
   `protoc` would refuse"* is the current glossary entry, and it stops being true.
4. **Every Protobuf tool stops working**: editors, formatters, linters,
   `buf`, and anyone who wanted to generate a Go or Java reader from the same file.

## The scoreboard, now that both files are written

Options round-trip fine — the aggregate `composite_key` and the topic strings
are all present in `--descriptor_set_out`, so protopc **can** read spelling A out
of a descriptor set and never write its own parser. Spelling A's problem is not
that it cannot carry the information. It is that **protoc validates none of it**:

| construct | A carries it | A checks it | B checks it |
|---|---|---|---|
| shape | ✅ option | n/a | ✅ |
| presence | ✅ feature | ✅ | ✅ |
| simple key | ✅ option | n/a | ✅ |
| composite key | ✅ option | ❌ names, tag collision | ✅ |
| derived index | ✅ option | ❌ field name, kind | ✅ |
| alias | ✅ option | ❌ field name | ✅ |
| view source | ✅ option | ❌ existence, shape | ✅ |
| view field/tag/type/presence | ✅ option | ❌ all four | ✅ |
| decimal scale | ✅ option | ❌ applicability | ✅ |

Every ❌ is a diagnostic protopc has to implement itself. Which means the
strict-profile bet — *"lean on protoc so protopc does less"* — pays off for the
Protobuf core and **not at all** for anything protop adds. protopc writes those
checks either way; A only decides whether they run against a descriptor set or
against its own AST.

## Option C, which neither specimen is

**Write B, lower to A.** protopc parses the native spelling and emits the
strict-Protobuf form as a byproduct; the harness runs `protoc` on *that*. The
oracle survives, the versioning carrier survives, the ergonomics are B's, and
views stay checkable because the lowering is where the check happens.

What it costs: the lowering is a second emitter to write and to keep honest, and
a round-trip property to test (*lower, compile with protoc, compare descriptors*)
that does not exist today. It also means two files exist for every one the
author wrote, which has to be explained to anyone reading a stack trace.

This is the option worth discussing, and it is not on the map in any form.
