# 1. Shared repository — `prague.protop`

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
