# 1. Shared repository — `prague.prague`

Tables, records, structs and enums live here, in one repo, owned by one team. Nothing in this file knows which services consume it.

```protop
revision 1;

namespace Common.EventApi.Cache.Prague.Models;

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

enum TicketTypeStatus {
  UNKNOWN   = 0;
  OPEN      = 1;
  SUSPENDED = 2;
  CLOSED    = 3;
}

// --- RECORDS ----------------------------------------------------------------
record StaticDataRef
  [cs: BsonIgnoreExtraElements]
{
  int64   id                  = 1;
  string  name                = 2;
  string? provider_source_key = 3;
  int64   provider_key        = 4;
}

record Quote {
  string name        = 1;
  double quotes_actual = 9;
  int64  timestamp   = 5;
}

// --- TABLE, SIMPLE KEY ------------------------------------------------------
table Show
  [cs: DataCache(CacheClassName = nameof(Prague.Models.BPShowCache))]
  [cs: DataCacheTopic("Cache.[v:brandId].Show")]
{
  reserved 8..14, 16..23, 25..27, 29..34, 37..46, 48..54;

  key int64 event_id = 1;

  int64 genre_id = 2;
  int64 country_id    = 3;
  int64 tour_id     = 4;

  int64? headliner_id = 5;
  int64? opener_id = 6;

  Venue? venue = 7;

  int32? listing_id = 15;

  EventStatus event_status = 24;

  int64 event_date         = 28;
  int64 provider_timestamp = 35;

  decimal(4)? live_limit_manual_single = 36;

  // Element type unverified against the pin. The corpus writes `List<Performer>`
  // in this slot and `Performer` was never copied, so the pin cannot settle whether
  // `StaticDataRef` here is faithful, a substitution, or an error.
  // corpus/prague/SOURCE.md records the gap; tracked on #38.
  list<StaticDataRef> performers = 47;

  StaticDataRef  genre = 55;
  StaticDataRef  country    = 56;
  StaticDataRef  tour     = 57;
  StaticDataRef? headliner  = 58;
  StaticDataRef? opener  = 59;
}

// --- TABLE, COMPOSITE KEY ---------------------------------------------------
table TicketType
  [cs: DataCache(CacheClassName = nameof(Prague.Models.BPTicketTypeCache))]
  [cs: DataCacheTopic("Cache.[v:brandId].TicketType")]
{
  reserved 5, 6, 10..21, 23, 24;

  key key = 1 (event_id, ticket_type_id);

  int64 event_id = 2 [references(Show)];
  int64 id       = 3;

  alias ticket_type_id = id;

  string name = 4;

  TicketTypeStatus  status           = 7;
  TicketTypeStatus? status_manual    = 8;
  TicketTypeStatus? status_watch_dog = 9;

  decimal(4)? limit_manual_single = 22;

  int64 timestamp = 25 [cs: DataCacheIndex(DataCacheIndexType.Range)] [cs: DataCacheFromTimestamp];

  list<Quote> quotes = 26;
}
```

**This section transcribes; it does not aspire.** Every annotation above is written in the form some corpus file writes it, on the kind of declaration that file writes it on: the two table heads follow `corpus/prague/payloads/BPShowCacheItem.cs` and `BPTicketTypeCacheItem.cs` down to the qualified `nameof`, the record head follows `corpus/prague/deps/StaticDataRef.cs`, and the pair on `timestamp` follows `corpus/prague/payloads/SlimTicketType.cs`. The topic is a string literal, because all eleven `[DataCacheTopic]` sites in the pin pass one and not one of them names a `TopicNames` constant. Annotations no corpus file carries are not shown here at all: `DataCacheContract` and `DataCacheIgnoreEquality` stood in this block until #43 measured them at zero occurrences corpus-wide and removed them.

The rule that produced those edits is the one worth keeping. A form protop should be able to carry, but that the pin does not yet show, belongs in a ticket and not in this section — because the emission of this block is checked against golden files, and a golden file cannot tell aspiration from transcription. It will certify either.

## Collections

`list<T>`, and `map<K, V>` when it's needed. Square brackets do exactly one job in this language — annotations — so the type syntax stays out of them.

```protop
list<StaticDataRef> performers = 47;
list<Quote>           quotes    = 26 [cs: SomeAttribute];
list<string>        groups  = 27;

map<string, Quote> quotes_by_name = 28;
```

Presence composes without ambiguity, which is the reason for the generic form over a prefix keyword:

| | |
|---|---|
| `list<Quote>` | the slot holds an array |
| `list<Quote>?` | the slot may be nil |
| `list<Quote?>` | the array may contain nils |

`repeated Quote?` cannot distinguish the last two. Nesting also just works: `list<list<int64>>`.

Ownership is unchanged. `list<StaticDataRef>` is composition because `StaticDataRef` is a record with no key, so nothing can point at it and it cannot exist apart from its parent. The line is drawn by whether the element type declares identity, never by the collection keyword.

An annotation on a collection field annotates the field, not the element. There is no position for an element annotation, and Protobuf and Cap'n Proto have the same limit.

`map` is **reserved, not implemented.** The corpus has no map. The syntax exists so that adding one later is filling a defined slot rather than inventing a third collection form. Key type restricted to scalars and enums when it lands.

**Why not `T[]`.** It would make `[]` mean two things in one line — `Quote[] quotes = 26 [cs: X]` — and disambiguating by position is exactly the rule the annotation design removed. `Quote?[]` versus `Quote[]?` is also harder to read than the generic form. No reference language uses postfix `[]`: Protobuf prefixes `repeated`, Cap'n Proto writes `List(T)`, FlatBuffers writes `[T]` and consequently puts its attributes in parens. Each assigns square brackets one job. This language assigns them to annotations.
