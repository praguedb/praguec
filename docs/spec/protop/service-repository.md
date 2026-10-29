# 2. Service repository — `brand-data-reader.prague`

A consuming service imports the shared schema and declares only what it reads.

```protop
revision 1;

namespace BrandReader.Services.Cache;

import "prague.prague";

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
