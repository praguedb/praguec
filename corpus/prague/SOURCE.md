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

- `payloads/` — the 14 `[MessagePackObject]` types. 1,501 LOC total with deps.
- `shells/` — the 11 hand-written halves of the cache classes. Every one is an
  **empty** `public sealed partial class XCache { }`. `B2Tech.Feed.Cache` writes
  the entire body; the human writes the shell. That shell is a thing `protopc`
  would also have to emit, or leave to hand-authoring.
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

Not copied: the 19 `IBaseLiveData` implementations (large, and the interface
declaration carries everything a schema decision needs), and the enum and
value types reachable transitively (`Odd`, `Channel`, `EventSource`,
`SportTypeList`, `EventPhase`, `LivePeriodScore`, `EventScore`).
