# Unions and opaque scalars

## `uuid`, `bytes`, `raw`

Three scalars join the set:

| protop | C# | Go | On the wire |
|---|---|---|---|
| `uuid` | `Guid` | `wire.UUID` | a 36-character string, as MessagePack-CSharp writes a `Guid` member |
| `bytes` | `byte[]` | `[]byte` | MessagePack bin |
| `raw` | `ReadOnlyMemory<byte>` | `msgpack.RawMessage` | any one MessagePack value, kept undecoded |

A `uuid` header is different from a `uuid` member. Prague's `HeadersSerDe` writes a header `Guid` as its 16 `ToByteArray` bytes, and the Go runtime reads it that way.

`raw` is only allowed as a union arm's payload (PROTOP0327). On a plain member it would be a hole in the contract that nothing checks.

## Time: `timestamp`, `date`

| protop | C# | Go | On the wire |
|---|---|---|---|
| `timestamp` | `DateTime` | `wire.Timestamp` | the int64 of `DateTime.ToBinary()`, as Prague writes a `DateTime`; a reader also takes MessagePack's timestamp extension |
| `date` | `DateOnly` | `wire.Date` | the day number, days since 0001-01-01 |

`ToBinary()` carries no offset: the ticks sit in the low 62 bits and the `DateTimeKind` in the top two. A `Utc` value is an instant, and so is a `Local` one, whose ticks .NET shifts to UTC before writing. An `Unspecified` value is a wall-clock time with no zone and is read as UTC. A `timestamp` is therefore an instant, and the kind is not kept.

Both read as integers in Go -- `wire.Timestamp` is UTC ticks, `wire.Date` the day number -- so they compare, sort and key a range index as the time they are.

## `union`

A union is a value that is one of several kinds, with a kind-specific payload, discriminated by an enum:

```protop
enum SettingKind {
  NIL = 0;
  INT = 1;
  STRING = 2;
  OBJECT = 3;
}

union SettingValue of SettingKind {
  NIL;              // no payload
  INT int64;
  STRING string;
  OBJECT raw;       // kept as bytes; the consumer decodes it if it cares
}
```

On the wire it is a two-element array, `[kind, payload]`. That is the shape svc-bo-api's hand-written `SettingValue` formatter writes, and the shape a custom MessagePack-CSharp formatter writes for any tagged union. An arm without a payload writes nil in the second slot.

Rules:
- `of` names an enum: PROTOP0324.
- Each arm names a member of that enum (PROTOP0325), at most once (PROTOP0326). The enum may have members that are not arms.
- A union is referenced like a record, optionally (`SettingValue? value = 3;`), in lists, and so on.

Emission:
- **C#:** a class carrying `Kind`, one property per arm and `Raw`, plus a `[MessagePackFormatter]` that writes and reads the pair. It reads a nil union, and skips elements past the second.
- **Go:** a struct with the same fields, and a `DecodeMsgpack`. A kind with no arm in the reader's schema keeps its payload in `Raw` instead of failing, so a producer can add a kind before its consumers learn it.

An arm named `KIND` or `RAW` collides with the generated fields. This is not checked yet.
