# 6. Relations

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
