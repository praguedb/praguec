# 2. Service repository — `brand-data-reader.prague`

A consuming service imports the shared schema and declares only what it reads.

```protop
revision 1;

namespace BrandReader.Services.Cache;

import "prague.prague";

view SlimShow of Show {
  event_id as id;

  genre_id;
  country_id;
  tour_id;
  headliner_id;

  venue;
  genre;
}

view SlimQuote of Quote {
  name;
  timestamp;
}

view SlimTicketType of TicketType {
  key as id;

  event_id;
  id as ticket_type_id;

  name;

  status;
  status_manual;
  status_watch_dog;

  limit_manual_single;
  timestamp;

  SlimQuote quotes;
}
```

A service may also declare tables of its own — for caches it produces and owns, which no other service reads:

```protop
table TicketTypeBrandSettings
  [cs: DataCache]
{
  key int64 ticket_type_id = 1;
  bool is_enabled = 2;
}
```

That is the pressure valve. The shared repo gates only what is actually shared.
