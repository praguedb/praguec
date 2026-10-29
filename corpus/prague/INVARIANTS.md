# Corpus invariants

Facts established by reading the pinned corpus. Cite these rather than re-deriving
them; if a decision on the map contradicts one, the decision is wrong or the
invariant needs re-checking against a fresh pin.

Two things a reader should know before citing anything below. Three claims are
marked **carried from upstream**: they arrived with the pin rather than out of
it, and nothing in `corpus/prague/` can confirm or refute them. And the type
tally and the declaration-shape counts are no longer prose alone —
`TestTheInvariantsDocumentMatchesTheCorpus` in `internal/csharp/corpus_test.go`
derives both from the `.cs` files and fails if this document disagrees, so a
number here cannot drift from the corpus in silence again.

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
   are the same type duplicated across two projects. There are **five** pairs,
   not four: `BOVenue` / `Venue` is one too, and naming it is what makes the
   tally below reproducible. **All five align — 0 mismatches across 132 shared
   keys**, reading `BOVenue?` and `Venue?` at `SportGameCacheItem` key 6 as the
   same twinned type; a mechanical string comparison reports 1 there, and that
   is the comparison's fault rather than the corpus's.
   They are not byte-identical beyond namespace and `CacheClassName`.
   `BOSportGameCacheItem` also carries a type-level `[DataCacheIndex]` its twin
   lacks, and one whole extra member — `public int FixtureIdIndex => FixtureId
   ?? 0;`, unkeyed and `[IgnoreMember]`, so present in no keyed-layout census.
   The other three pairs differ in namespace and `CacheClassName` alone.
2. **Full ↔ Slim projections.** The §12 tag-preserving view, hand-maintained.
   `SlimSportGame` keeps 41 of the parent's 55 keys; `SlimBetType` keeps 14 of 28.
   Neither adds a key the parent lacks — the subset property holds.
3. **Common ↔ BO/BP.** `LiveObjectCacheItem` matches both twins exactly. This is
   a *third* copy of one type, not a second: LiveObject is a triplet.

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

## Type tally (203 de-twinned members; 335 raw)

**The de-twinning rule, written down so the number can be reproduced.** A member
counts when it carries a `[Key(N)]`; across all 14 payload files that is 335.
The tally below drops the BO-side copy of each of the five BO ↔ BP twin pairs —
`BOBetTypeCacheItem`, `BOLiveObjectCacheItem`, `BOSearchCacheItem`,
`BOSportGameCacheItem`, `BOVenue` — and counts the remaining nine files once
each: `BPBetTypeCacheItem`, `BPLiveObjectCacheItem`, `BPSearchCacheItem`,
`BPSportGameCacheItem`, `LiveObjectCacheItem`, `SlimBetType`, `SlimOdd`,
`SlimSportGame`, `Venue`. That is 203.

The rule collapses relationship 1 and *only* relationship 1. It does not collapse
relationship 3, so `LiveObjectCacheItem`'s two keys are counted even though both
twins repeat them — it is a Common type, not another project's copy of one.
Collapsing that as well gives 201. The rule is a convention and the number moves
with it; the point of stating it is that the number can now be checked rather
than guessed at.

Entries are ordered by count, then by label:

`long` 31 · `bool` 28 · `decimal?` 18 · `string?` 16 · non-null enums 14 ·
nullable enums 13 · `string` 11 · `long?` 10 · `DateTime?` 9 · `StaticDataRef` 8 ·
`int` 8 · `int?` 8 · `bool?` 5 · collections 4 · `double` 4 · `DateTime` 3 ·
ValueTuple 3 · `IBaseLiveData?` 2 · `StaticDataRef?` 2 · `byte` 2 · `double?` 2 ·
`Odd?` 1 · `Venue?` 1

`StaticDataRef` outnumbers `StaticDataRef?` four to one, and not the other way
round: the corpus's default for an embedded reference is non-null, and the ten
uses line up with "Used by 10 members" below. Getting this pair backwards is the
one error in this document that would have produced *wrong code* rather than a
wrong number, which is why it is called out here.

The `decimal?` 18 are all money and the `double` 4 / `double?` 2 are all odds and
margins. `int` counts keyed members only: `BOSportGameCacheItem`'s
`FixtureIdIndex` is a ninth `public int` in the payload directory and is not on
the wire, so no keyed-layout census includes it. ValueTuple counts *members*, 3
of them, across 2 distinct shapes — every other entry counts members, so counting
shapes here would have been the odd one out.

Absent entirely: `Guid`, `TimeSpan`, `DateTimeOffset`, `float`, `IReadOnlyList<T>`,
`object`, and `Dictionary<,>` — which appears in no `.cs` file under
`corpus/prague/` at all, directly or otherwise.

**Carried from upstream, not re-derivable from this pin:** a `Dictionary<,>` is
reported to appear transitively and string-keyed via `Odd.Specifiers`. `Odd` is
one of the types `SOURCE.md` records as not copied, so the pin can neither
confirm nor refute it. Evidence for #24, which is unruled.

**The money/odds split is clean.** No member crosses over. That is what makes
`NO_DOUBLE_FOR_MONEY` plausible as a decidable rule and what makes per-field
`double` opt-in cheap.

## Declaration shape

- 11 of 14 payloads are `public sealed partial class`. Zero records.
- The exceptions are `BOVenue`, `Venue` and `SlimOdd`. `BOVenue` and `Venue` are
  mutable `struct`s with `required` members, used as `Venue?` — a nullable
  *value* type. `SlimOdd` is a bare `public class`: neither sealed nor partial,
  carrying two hand-written constructors, and it is the type `SlimBetType`
  key 25 recurses into.
- Mostly `{ get; set; }`. `init` appears only on `SlimOdd` (all 14 members) and
  `SlimSportGame.Id`.
- No inheritance among Prague types themselves.
- `partial` is load-bearing where it appears — `Prague.Feed.Cache` writes the
  other half — but it is not universal. The three exceptions have no generated
  half at all, so "a payload type is `partial`" is a rule with three counterexamples
  out of fourteen rather than an invariant.

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
   type adds nothing but a name and a `[BsonIgnoreExtraElements]`. Used by 10
   members. All four of its keyed members are inherited, so a tally taken by
   reading `StaticDataRef.cs` alone gets zero. #11 is closed and ruled: protop
   flattens, and the forked C# hierarchy is an accepted cost.
4. **Mutable `struct` with `required`**, used as `Venue?`.

## Attribute co-tenancy

22 `[DataCacheIndex]` annotations sit on **members** under the de-twinning rule
above, so they cannot be relegated to the generated half of a `partial`.
Attribute passthrough is not optional.

That 22 is the one figure here that genuinely depends on which twin you keep, so
take the raw count as the real one: across all 14 payload files there are 31 —
30 on members and one on a type, `BOSportGameCacheItem`'s
`[DataCacheIndex(nameof(FixtureIdIndex), …)]`, which the rule above drops along
with the BO copy. Keeping the BO side instead gives 23 member-level and that one
type-level. Grep for it with `--include='*.cs'`: an unrestricted grep over
`corpus/prague/` also counts this document's own prose about it.

**Carried from upstream, not re-derivable from this pin:** `Odd` is reported to
carry MessagePack, MongoDB Bson and System.Text.Json attributes at once. `Odd`
was not copied, so the pin cannot check the co-tenancy claim — only the fact that
`StaticDataRef` pairs MessagePack with `[BsonIgnoreExtraElements]`, which it does.

## Nullability is advisory

**Carried from upstream, not re-derivable from this pin:**
`<Nullable>enable</Nullable>` is on, immediately followed by
`<NoWarn>$(NoWarn);CS8618</NoWarn>`. NRT annotations in the corpus express intent,
not an enforced invariant — which is precisely the gap the presence→nullability
thesis aims to close.

The pin holds `.cs` files only: there is no `.csproj`, `.sln`, `.props` or
`.targets` anywhere in this repository, and the two lines quoted above appear
nowhere except in this paragraph. The thesis rests on them, so if it is going to
be load-bearing, pin the project file and this section stops being hearsay.

## Housekeeping observations

- No `[Obsolete]`, no V1/V2 pairs, no migration shims anywhere.
- Cross-type linkage is **by ID**, and `EventId` is the join key wherever a type
  has one. It is not universal: `BOVenue`, `Venue`, `SlimOdd` and the 41-member
  `SlimSportGame` carry no `EventId` at all — `SlimSportGame`'s key 0 is
  `long Id`, which the drift table above already records.
- Relations are denormalized redundantly — a scalar FK *and* an embedded
  `StaticDataRef` copy of the same entity.
