# Corpus invariants

Facts established by reading the pinned corpus. Cite these rather than re-deriving
them; if a decision on the map contradicts one, the decision is wrong or the
invariant needs re-checking against a fresh pin.

## Key layout

| type | members | min | max | gaps |
|---|---|---|---|---|
| `BOBetTypeCacheItem` | 28 | 0 | 27 | — |
| `BOLiveObjectCacheItem` | 2 | 0 | 1 | — |
| `BOSearchCacheItem` | 45 | 0 | 47 | 16,28,29 |
| `BOSportGameCacheItem` | 55 | 0 | 58 | 13,41,42,43 |
| `BOVenue` | 2 | 1 | 2 | — |
| `BPBetTypeCacheItem` | 28 | 0 | 27 | — |
| `BPLiveObjectCacheItem` | 2 | 0 | 1 | — |
| `BPSearchCacheItem` | 45 | 0 | 47 | 16,28,29 |
| `BPSportGameCacheItem` | 55 | 0 | 58 | 13,41,42,43 |
| `LiveObjectCacheItem` | 2 | 0 | 1 | — |
| `SlimBetType` | 14 | 0 | 25 | 4,5,11,12,13,14,15,16,17,18,19,20 |
| `SlimOdd` | 14 | 0 | 27 | 1,2,5,7,11,14,15,16,17,18,21,22,24,25 |
| `SlimSportGame` | 41 | 0 | 58 | 6,13,16,17,22,28,30,31,41,42,43,45,46,48,49,50,51,52 |
| `Venue` | 2 | 1 | 2 | — |


- **Keys start at 0** everywhere except `BOVenue` and `Venue`, which start at 1.
  Under the settled `Key = tag - 1` mapping, tag 1 → Key 0, so those two types are
  the *only* place in the corpus where the offset decision does not simply work.
- **Max key is 58** (`BOSportGameCacheItem` / `SlimSportGame`).
- **Gaps are de facto `reserved`.** Retired fields are never reused. The `Slim*`
  types' gaps are different in kind — they are the parent's keys, deliberately
  skipped by a narrowing projection.
- MessagePack pads an int-keyed layout to a dense array of `maxKey + 1`, so
  `SlimSportGame` writes 18 nil slots per message by construction.

## Three kinds of related-message pair

The corpus has *three* alignment relationships, not one. Any key-alignment check
has to decide which of them it is checking.

1. **BO ↔ BP twins.** `BOSportGameCacheItem` / `BPSportGameCacheItem` and friends
   are the same type duplicated across two projects: identical keys, identical
   member names and types, differing only in namespace, `CacheClassName`, and
   which `[DataCacheIndex]` annotations are present. **All four pairs align
   perfectly — 0 mismatches across 130 shared keys.**
2. **Full ↔ Slim projections.** The §12 tag-preserving view, hand-maintained.
   `SlimSportGame` keeps 41 of the parent's 55 keys; `SlimBetType` keeps 14 of 28.
   Neither adds a key the parent lacks — the subset property holds.
3. **Common ↔ BO/BP.** `LiveObjectCacheItem` matches both twins exactly.

## Drift the projections have already accumulated

Mechanically found by comparing shared keys. **This is the evidence that the
key-alignment check pays for itself.**

`BOSportGameCacheItem` vs `SlimSportGame` — 41 shared keys, 4 divergent:

| key | parent | view | severity |
|---|---|---|---|
| 53 | `int? EventSource` | `EventSource? EventSource` | **wire-compatible by luck, semantically a bug.** An enum and a raw int at the same slot |
| 57 | `StaticDataRef? HomeTeam` | `StaticDataRef HomeTeam` | nullability disagreement |
| 58 | `StaticDataRef? AwayTeam` | `StaticDataRef AwayTeam` | nullability disagreement |
| 0 | `long EventId` | `long Id` | name only |

`BOBetTypeCacheItem` vs `SlimBetType` — 14 shared keys, 3 divergent:

| key | parent | view | severity |
|---|---|---|---|
| 2 | `long Id` | `long BetTypeId` | name only; same type |
| 25 | `List<Odd> Odds` | `SlimOdd[] Odds` | **element type differs** — the projection recurses, and both collection kinds occupy one slot |
| 0 | `(long EventId, long BetTypeId) Key` | `(long EventId, long BetTypeId) Id` | name only |

Key 25 is the interesting one: a view may narrow a field's *element* type, which
means "same tag ⇒ same type" is too strong a rule as stated.

## Type tally (227 members)

`long` 31 · `bool` 28 · **`decimal?` 18 (all money)** · `string?` 16 · `string` 11 ·
`long?` 10 · `int` 9 · `int?` 8 · nullable enums 13 · non-null enums 11 ·
`DateTime?` 9 · `DateTime` 3 · `StaticDataRef?` 8 · `StaticDataRef` 2 · `bool?` 5 ·
**`double` 4 / `double?` 2 (all odds and margins)** · `byte` 2 · ValueTuple 2 ·
`IBaseLiveData?` 2 · collections 4.

Absent entirely: `Guid`, `TimeSpan`, `DateTimeOffset`, `float`, `IReadOnlyList<T>`,
`object`, and any directly-declared `Dictionary<,>` (one appears transitively via
`Odd.Specifiers`, string-keyed).

**The money/odds split is clean.** No member crosses over. That is what makes
`NO_DOUBLE_FOR_MONEY` plausible as a decidable rule and what makes per-field
`double` opt-in cheap.

## Declaration shape

- 12 of 14 payloads are `public sealed partial class`. Zero records.
- The two exceptions are `BOVenue` and `Venue`: mutable `struct`s with `required`
  members, used as `Venue?` — a nullable *value* type.
- Mostly `{ get; set; }`. `init` appears only on `SlimOdd` (all 14 members) and
  `SlimSportGame.Id`.
- No inheritance among Prague types themselves.
- `partial` is load-bearing: `B2Tech.Feed.Cache` writes the other half.

## Constructs with no protobuf equivalent

1. **`IBaseLiveData`** — an *interface* with 19 `[Union]` attributes **and 14
   declared members**. It is not a bare tagged union: every arm satisfies a shared
   contract (`SportType`, `EventScores`, `HomeScore`, …). A protobuf `oneof` gives
   you the tag and nothing else, so a `oneof` lowering loses the contract.
2. **Computed ValueTuple keys** at `[Key(0)]`, get-only, `(long, long)` and
   `(long, Channel)`. They serialize out and cannot deserialize back; their
   components are separately present at later keys.
3. **`StaticDataRef : BaseProviderObject`** — an empty subclass. The base is
   `[MessagePackObject]` with keys 0–3 and a `virtual string Name`; the derived
   type adds nothing but a name and a `[BsonIgnoreExtraElements]`. Used by 10 members.
4. **Mutable `struct` with `required`**, used as `Venue?`.

## Attribute co-tenancy

`Odd` carries MessagePack, MongoDB Bson, and System.Text.Json attributes at once.
22 `[DataCacheIndex]` annotations sit on **members**, so they cannot be relegated
to the generated half of a `partial`. Attribute passthrough is not optional.

## Nullability is advisory

`<Nullable>enable</Nullable>` is on, immediately followed by
`<NoWarn>$(NoWarn);CS8618</NoWarn>`. NRT annotations in the corpus express intent,
not an enforced invariant — which is precisely the gap the presence→nullability
thesis aims to close.

## Housekeeping observations

- No `[Obsolete]`, no V1/V2 pairs, no migration shims anywhere.
- Cross-type linkage is **by ID**; `EventId` is the universal join key.
- Relations are denormalized redundantly — a scalar FK *and* an embedded
  `StaticDataRef` copy of the same entity.
