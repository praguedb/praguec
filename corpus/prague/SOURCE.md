# Pinned corpus: the Prague caches

Verbatim copies of the MessagePack payload types this effort exists to migrate.
**Read-only reference material.** Nothing here is compiled, and nothing here is
edited — if the upstream changes, re-pin rather than patch.

## Provenance

| | |
|---|---|
| Source repo | `/Users/dimitarnanov/work/github/integratedeventapi` |
| Branch | `origin/feature/BO_Pragure_Integration` (the typo is upstream's) |
| Commit | `22331403a40c93448b4ee8a35e83dc484ba95d60` |
| Commit date | 2026-02-20 10:26:37 +0100 |
| Pinned | 2026-09-12 |
| Merged into `develop`? | **No.** The branch is 3 commits behind `origin/develop` and has never been merged |

All 28 Prague files live on that one branch — BO, BP and Common alike. An earlier
survey suspected a second BP-only branch; that was wrong, there is one branch.
The only other branch matching "prague" is `origin/bugfix/wait-prague-cache-ready`,
which touches cache readiness, not the models.

**Open with the human:** the branch is unmerged and behind `develop`. If it is
rebased or squashed before protop ships, this pin goes stale and the corpus must
be re-taken. Worth confirming this is the intended target and not an abandoned spike.

## Layout

- `payloads/` — the 14 `[MessagePackObject]` types. 1,302 LOC; 1,394 with
  `deps/`; 1,501 with `shells/` as well, which is the number to quote when
  someone asks how big the corpus is.
- `shells/` — the 11 hand-written halves of the cache classes, each a
  `public sealed partial class XCache`. **Only three of them are empty** —
  `LiveObjectCache`, `SlimBetTypeCache` and `SlimSportGameCache`. The other
  eight carry a `public void Clear()` written against members
  `B2Tech.Feed.Cache` generates: `Query()`, `Execute()`, `Remove()`. So the
  shell is not a formality a generator could emit blind and be done with. It is
  a file the human writes real code into, calling into the generated half — a
  stronger constraint on `praguec` than "emit an empty class", and the reason
  #25 owns whether `praguec` should emit a shell at all.
- `deps/` — types the payloads reference that are not themselves Prague types:
  `BaseProviderObject`, `StaticDataRef`, `IBaseLiveData`, and the
  `SportGameSlimExtensions` projection helper.

## Upstream paths

| Local | Upstream |
|---|---|
| `payloads/BO*.cs`, `payloads/BOVenue.cs` | `BO/BO.Services/Prague/Models/` |
| `payloads/BP*.cs`, `payloads/Venue.cs` | `BP/BrandProcessor.Services/Prague/Models/` |
| `payloads/LiveObjectCacheItem.cs`, `payloads/Slim*.cs` | `Common/Common.IntegratedEventApi.Cache/Prague/Models/` |
| `deps/BaseProviderObject.cs`, `deps/StaticDataRef.cs` | `Common/Common.IntegratedEventApi.Models/Branded/` |
| `deps/IBaseLiveData.cs` | `Common/Common.IntegratedEventApi.Models/LiveObject/` |

Not copied: the 19 `IBaseLiveData` implementations — large, and the interface
declaration carries everything a schema decision needs — and every enum and value
type reachable transitively.

That second half is longer than it reads, so here it is in full. Fourteen type
names appear in member positions in the copied files and are declared nowhere in
this corpus:

`BetTypeStatus`, `BookingStatus`, `Channel`, `EventPhase`, `EventScore`,
`EventSource`, `EventStatus`, `LivePeriodScore`, `Odd`, `OddStatus`, `Runner`,
`SportTypeList`, `WatchDogAction`, `WinStatus`.

Take that list as exhaustive and nothing else as resolvable: a reader working
from the shorter sentence this replaces would have concluded that `Runner` and
the six status enums could be looked up in the corpus. They cannot.

`Runner` is the one that bites. `List<Runner>` is `SportGameCacheItem` key 46,
and the spec's `SportGame` transcription writes that member as
`list<StaticDataRef> runners`. The tag is right — key 46 → tag 47 under
`Key = tag - 1` — but because `Runner` was never copied, **the pin cannot settle
whether `StaticDataRef` there is a faithful transcription, a substitution, or an
error.** Either `Runner` gets copied on the next pin or that transcription stays
unverifiable; what is no longer acceptable is the gap being silent.
