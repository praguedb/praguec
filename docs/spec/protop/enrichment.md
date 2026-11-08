# Enrichment — members written from the Kafka message

A cached member can be written from the Kafka message as well as from its value. There are two sources: a header, and the message timestamp.

```protop
table Product {
  key int64 id = 1;
  string region     = 2 [header("x-region")];  // the header x-region
  int32  tier       = 3 [header];              // the header Tier
  int64  written_at = 4 [from_timestamp];      // Unix milliseconds
}
```

These annotations belong to protop, not to a backend, because one annotation has to mean the same thing on both sides of the wire. The C# backend emits Prague's `[DataCacheHeader]`, `[DataCacheHeader("x-region")]` and `[DataCacheFromTimestamp]`, which tell the producer what to stamp. The Go backend emits an `Enrich(wire.Meta)` method that reads them back, plus an `Enrichment()` method that reports which headers and whether the timestamp it reads.

## Rules

- **Only on a field of a table or a view.** Only those are cached from a message. A record or struct is decoded inside whatever holds it and never sees the message. A composite key or an alias has no field of its own to write. Elsewhere: PROTOP0319.
- **`[header]` names the header after the member's PascalCase spelling**, which is the property name Prague's attribute defaults to. `[header("name")]` names it explicitly. The name may not be empty or contain `"` or `\`: PROTOP0405.
- **A header carries one scalar or one enum**, optional or not. A collection or a declared record is PROTOP0320.
- **`[from_timestamp]` is on an `int64`**, not optional, and takes no arguments: PROTOP0321, PROTOP0406.
- **One source per member, one member per header.** A member with two sources, or two members reading the same header, is PROTOP0322.
- **A view member inherits its source's enrichment**, including the header name: a view that renames the member does not rename the header the producer wrote. A view member cannot add enrichment on top of its source's (PROTOP0322), because the same bytes would then decode to different values in the table and the view.

## With a tag, or without

An enriched member may keep its wire slot, so the value carries it as well:

```protop
string region = 2 [header("x-region")];
```

Or it may have no tag, when it exists only to hold the header. That is C#'s `[IgnoreMember][DataCacheHeader]`:

```protop
int32 schema_version [header("schema-version")];
```

A field without a tag that is not enriched has nothing to come from: PROTOP0323.

With a tag, the value also carries the member. Enrichment runs after the value decodes and after value filters run, so a value filter sees what the value carried. Then each occurrence of the header that decodes overwrites the member, in order. One that does not decode is skipped, and a message without the header leaves the member as the value had it. This is Prague's enricher's behaviour, and the Go runtime matches it.

Header values are read as Prague's `HeadersSerDe` writes them:
- `int32` and `int64` as raw little-endian bytes (exactly 4 or 8) or as exact MessagePack. No MessagePack integer is 4 or 8 bytes long, so the two cannot be confused.
- `string` as raw UTF-8.
- Everything else as MessagePack.

## Change detection

An enriched member is part of the cached value, so a consumer's "nothing changed" test has to include what it was enriched from. The same value bytes under a new header is a change. Under a new timestamp it is also a change if the type reads the timestamp. Prague's `CacheEquals` compares enriched properties for the same reason.
