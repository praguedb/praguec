# What MessagePack v3 actually writes for `DateTime`, structs and `required` members

Research resolving issue #19. Blocks #15 (temporal representation) and #12 (the `Venue`
struct). Everything below is **measured**, not read, unless explicitly marked *unverified*;
source citations point at the mechanism behind each measurement.

## How this was measured

A throwaway console project (`net9.0`, `PackageReference MessagePack 3.1.4`,
`<EmitCompilerGeneratedFiles>true</EmitCompilerGeneratedFiles>`) serialised each shape,
hex-dumped the bytes, ran `MessagePackSerializer.ConvertToJson` over the same bytes, and
deserialised back. Every shape was run twice: once through the **source-generated** resolver
(`[GeneratedMessagePackResolver] partial class MpProbeResolver`) and once through the
**reflection** path (`CompositeResolver` with `DynamicObjectResolver.Instance` explicitly in
front). The emitted formatters under `generated/MessagePack.SourceGenerator/` were read
directly.

| | |
|---|---|
| .NET SDK | 10.0.400 |
| Probe TFM | `net9.0`, runtime .NET 9.0.19 |
| Package | `MessagePack` 3.1.4 (pulls `MessagePack.Annotations` 3.1.4 and the `MessagePackAnalyzer` 3.1.4 source generator, `include="All"`) |
| Assembly | `MessagePack, Version=3.1.4.0, Culture=neutral, PublicKeyToken=b4a0369545f0a1be` |
| Host | macOS (Darwin 25.6.0, arm64), machine TZ `Europe/Sofia` (+02:00 on the test date) |
| Source citations | `MessagePack-CSharp` @ `44aa149acce1de32881e007f53fad0f3db7dfdab` (the v3.1.4 commit named by the nuspec) |

**Generated vs. reflection: byte-identical on every single shape below.** They are not a
distinction that matters for any question in #19 — see [Why the two paths agree](#why-the-two-paths-agree).
All hex in this document is therefore a single column.

Incidental, unrelated to the questions: `dotnet restore` reports NU1902/NU1903 advisories
(moderate + high GHSA entries) against MessagePack 3.1.4. Noted for whoever owns the pin.

---

## Part 1 — Temporal

### `DateTime` is the MessagePack timestamp extension, ext type -1

Confirmed on the wire. The extension type byte is `0xFF` = -1 as a signed byte.

```csharp
// src/MessagePack/MessagePackCode.cs:283-286
internal static class ReservedMessagePackExtensionTypeCode
{
    public const sbyte DateTime = -1;
}
```

`MessagePackWriter.Write(DateTime)` (`src/MessagePack/MessagePackWriter.cs:415-428`) hands a
15-byte span to `MessagePackPrimitives.TryWrite(Span<byte>, DateTime, out int)`
(`src/MessagePack/MessagePackPrimitives.Writers.cs:736-832`), which picks one of the three
spec encodings. `DateTimeFormatter` (`src/MessagePack/Formatters/PrimitiveFormatter.cs:1838-1855`)
is a one-line delegation to that, and `BuiltinResolver` hands it out for `DateTime`.

Test instant: `2024-03-05T06:07:08.910` (ticks 638452156289100000).

| shape | len | hex | `ConvertToJson` | round-trip |
|---|---:|---|---|---|
| `DateTime` Kind=**Utc** | 10 | `D7 FF D8 F5 FE 00 65 E6 B6 8C` | `"2024-03-05T06:07:08.9100000Z"` | Kind=Utc, ticks equal |
| `DateTime` Kind=**Local** | 10 | `D7 FF D8 F5 FE 00 65 E6 9A 6C` | `"2024-03-05T04:07:08.9100000Z"` | Kind=Utc, **ticks changed** (638452084289100000); instant preserved |
| `DateTime` Kind=**Unspecified** | 10 | `D7 FF D8 F5 FE 00 65 E6 B6 8C` | `"2024-03-05T06:07:08.9100000Z"` | **Kind=Utc**, ticks preserved, instant silently reinterpreted (`toUtcEqual=False`) |
| `DateTime.MinValue` | 15 | `C7 0C FF 00 00 00 00 FF FF FF F1 88 6E 09 00` | `"0001-01-01T00:00:00.0000000Z"` | equal |
| `DateTime.MaxValue` | 15 | `C7 0C FF 3B 9A C9 9C 00 00 00 3A FF F4 41 7F` | `"9999-12-31T23:59:59.9999999Z"` | equal |
| `DateTime.UnixEpoch` | **6** | `D6 FF 00 00 00 00` | `"1970-01-01T00:00:00.0000000Z"` | equal |
| `1960-01-01T00:00:00Z` (pre-epoch) | 15 | `C7 0C FF 00 00 00 00 FF FF FF FF ED 30 08 80` | `"1960-01-01T00:00:00.0000000Z"` | equal |
| `...06:07:08` + 1234 ticks (sub-ms, Utc) | 10 | `D7 FF 00 07 88 20 65 E6 B6 8C` | `"2024-03-05T06:07:08.0001234Z"` | **exact** — 100 ns fidelity survives |
| `DateTimeOffset` +02:00 | 12 | `92 D7 FF D8 F5 FE 00 65 E6 D2 AC 78` | `["2024-03-05T08:07:08.9100000Z",120]` | `EqualsExact=True` |
| `DateTimeOffset` UTC | 12 | `92 D7 FF D8 F5 FE 00 65 E6 B6 8C 00` | `["2024-03-05T06:07:08.9100000Z",0]` | exact |
| `TimeSpan` `1.02:03:04.506` | 9 | `CF 00 00 00 DA 5B EB F1 A0` | `937845060000` | exact |
| `TimeSpan` `-1500ms` | 5 | `D2 FF 1B 1E 40` | `-15000000` | exact |
| `[Key(0)] DateTime? When = null` | 2 | `91 C0` | `[null]` | null |

Payload decoding (done by hand with python3, to prove the layout rather than assume it):

- `D7` = FixExt8, `FF` = ext -1 → timestamp64. `0xD8F5FE0065E6B68C` splits as
  **nanos = 910000000** (high 30 bits) and **seconds = 1709618828** (low 34 bits) —
  `data64 = (nanoseconds << 34) | seconds`, `MessagePackPrimitives.Writers.cs:801-813`.
- The `Local` row decodes to seconds = 1709611628, exactly 7200 less: the +02:00 machine offset.
- `C7 0C FF` = Ext8, length 12, ext -1 → timestamp96: uint32 nanos then int64 seconds
  (`Writers.cs:815-829`). MinValue → nanos 0, secs -62135596800; MaxValue → nanos 999999900,
  secs 253402300799; 1960 → secs -315619200.
- `D6 FF` = FixExt4, ext -1 → timestamp32, 4 bytes of unsigned seconds
  (`Writers.cs:784-799`). Only reachable when nanos == 0 and seconds fits in uint32.

So the extension is **variable length: 6, 10 or 15 bytes**. It is emphatically *not* "one
integer-ish slot"; see the cost table below.

### `DateTimeKind` is **not** preserved. Everything comes back `Utc`.

This is the sharp edge. Three different `DateTime` values, three different failure modes:

```csharp
// src/MessagePack/MessagePackPrimitives.Writers.cs:744-750
// The spec requires UTC. Convert to UTC if we're sure the value was expressed as Local time.
// If it's Unspecified, we want to leave it alone since .NET will change the value when we convert
// and we simply don't know, so we should leave it as-is.
if (value.Kind == DateTimeKind.Local)
{
    value = value.ToUniversalTime();
}
```

| input Kind | what is written | what comes back | damage |
|---|---|---|---|
| `Utc` | the instant | `Utc`, same ticks | none |
| `Local` | instant converted to UTC | `Utc`, **different ticks** | Kind and wall-clock reading lost; instant correct |
| `Unspecified` | the ticks **verbatim, as if they were UTC** | `Utc`, same ticks | Kind lost and **the instant silently moves by the writer's UTC offset** |

The read side has no choice in the matter: `TryReadDateTime`
(`src/MessagePack/MessagePackPrimitives.Readers.cs:314-370`, lines 355/361/367) rebuilds the
value as `DateTimeConstants.UnixEpoch.AddSeconds(seconds).AddTicks(nanoseconds / NanosecondsPerTick)`,
and `UnixEpoch` is `new DateTime(1970, 1, 1, 0, 0, 0, DateTimeKind.Utc)`
(`src/MessagePack/Internal/DateTimeConstants.cs:12`). Adding to a UTC `DateTime` yields a UTC
`DateTime`. There is no channel in the ext -1 payload for a Kind, so none could be carried even
in principle. (`MessagePackReader.ReadDateTime()` is at `src/MessagePack/MessagePackReader.cs:575`.)

### The bytes for a `Local` value depend on the serialising machine's time zone

Proven by re-running the same program under different `TZ`:

| `TZ` | `DateTime` Kind=Local, same ticks | hex |
|---|---|---|
| `UTC` | 2024-03-05T06:07:08.910 | `D7 FF D8 F5 FE 00 65 E6 B6 8C` |
| `Europe/Sofia` | 2024-03-05T06:07:08.910 | `D7 FF D8 F5 FE 00 65 E6 9A 6C` |
| `America/New_York` | 2024-03-05T06:07:08.910 | `D7 FF D8 F5 FE 00 65 E6 FC DC` |

Identical input, three different payloads. Any cache whose producers run in more than one zone
(or whose containers disagree with the host) will not produce byte-stable payloads for
`Kind=Local` values — and, symmetrically, `Kind=Unspecified` values will be *read back* as
different instants than they were written to mean.

### `DateTimeOffset` writes wall-clock-as-if-UTC plus offset minutes

```csharp
// src/MessagePack/Formatters/StandardClassLibraryFormatter.cs:249-280 (DateTimeOffsetFormatter)
writer.WriteArrayHeader(2);
writer.Write(new DateTime(value.Ticks, DateTimeKind.Utc)); // current ticks as is
writer.Write((short)value.Offset.TotalMinutes);            // offset is normalized in minutes
```

Note `value.Ticks`, **not** `value.UtcTicks`. The nested ext -1 timestamp in the +02:00 row
above decodes to seconds 1709626028 = `08:07:08Z`, i.e. the local wall clock stamped as if UTC.
Deserialisation reverses it with `new DateTimeOffset(utc.Ticks, TimeSpan.FromMinutes(minutes))`
and requires `count == 2`. Round-trip is exact (`EqualsExact == true`), but the nested timestamp
is *not* the instant — a non-.NET reader that ignores element 1 gets a wrong answer. Offset
resolution is one minute, range ±32767 minutes (a `short`).

### `TimeSpan` is raw int64 ticks

```csharp
// src/MessagePack/Formatters/StandardClassLibraryFormatter.cs:229-247 (TimeSpanFormatter)
writer.Write(value.Ticks);               // serialize
return new TimeSpan(reader.ReadInt64()); // deserialize
```

No extension, no unit marker: `937845060000` is meaningless to a non-.NET reader without
out-of-band knowledge that the unit is 100 ns.

### The Kind-preserving alternative that exists in the box

`NativeDateTimeResolver` (`src/MessagePack/Resolvers/NativeDateTimeResolver.cs:56-72`) maps
`DateTime` → `NativeDateTimeFormatter` and `DateTime?` → `StaticNullableFormatter<DateTime>`.
`NativeDateTimeFormatter` (`src/MessagePack/Formatters/DateTimeFormatters.cs:14-29`) is
`writer.Write(value.ToBinary())` / `DateTime.FromBinary(reader.ReadInt64())`:

| Kind | hex | round-trip |
|---|---|---|
| Utc | `CF 48 DC 3C DA 7D B1 A8 E0` (9 B) | Kind **and** ticks exact |
| Local | `D3 88 DC 3C C9 BA 28 D8 E0` (9 B) | Kind **and** ticks exact |
| Unspecified | `CF 08 DC 3C DA 7D B1 A8 E0` (9 B) | Kind **and** ticks exact |

It round-trips perfectly and costs 9 bytes — but `ToBinary()` packs the Kind into the top two
bits of a .NET-specific tick count. It is not the msgpack timestamp and is not readable by any
non-.NET implementation. It is the only in-box option that preserves Kind.

### Cost of one instant, seven candidate slot encodings

`2024-03-05T06:07:08.910Z` (has a millisecond component):

| encoding | bytes | hex |
|---|---:|---|
| `DateTime` — what v3 writes today (ext -1, timestamp64) | 10 | `D7 FF D8 F5 FE 00 65 E6 B6 8C` |
| int64 ticks | 9 | `CF 08 DC 3C DA 7D B1 A8 E0` |
| int64 unix milliseconds | 9 | `CF 00 00 01 8E 0D 39 16 6E` |
| int64 unix microseconds | 9 | `CF 00 06 12 E3 A6 FF 9D B0` |
| int64 unix nanoseconds | 9 | `CF 17 B9 C9 44 56 7F F7 80` |
| uint32 unix seconds | 5 | `CE 65 E6 B6 8C` |
| nested `Timestamp` message `[seconds, nanos]` | 11 | `92 CE 65 E6 B6 8C CE 36 3D 7F 80` |

Whole-second instant `2024-03-05T06:07:08Z`:

| encoding | bytes | hex |
|---|---:|---|
| `DateTime` (ext -1, timestamp32) | **6** | `D6 FF 65 E6 B6 8C` |
| int64 unix milliseconds | 9 | `CF 00 00 01 8E 0D 39 12 E0` |
| uint32 unix seconds | 5 | `CE 65 E6 B6 8C` |

Note the int64 encodings are fixed 9 bytes only because these magnitudes need the full
`0xCF`/`0xD3` form; msgpack int encoding is itself variable, so a scaled integer near zero is
cheaper and one far from zero is not.

### Why the two paths agree

`DateTime`, `DateTime?`, `DateTimeOffset` and `TimeSpan` are all served by `BuiltinResolver`,
which sits **first** in both composites:

```csharp
// src/MessagePack/Resolvers/StandardResolver.cs:259-264 (StandardResolverHelper.DefaultResolvers)
BuiltinResolver.Instance, AttributeFormatterResolver.Instance,
SourceGeneratedFormatterResolver.Instance, ImmutableCollectionResolver.Instance,
CompositeResolver.Create(ExpandoObjectFormatter.Instance), DynamicGenericResolver.Instance,
DynamicUnionResolver.Instance  // then StandardResolver.cs:28-30 appends DynamicObjectResolver last
```

Formatter identity dump confirms it: both paths resolve `DateTime` → `MessagePack.Formatters.DateTimeFormatter`,
`DateTime?` → `NullableDateTimeFormatter` (`PrimitiveFormatter.cs:1857-1888`), `DateTimeOffset` →
`DateTimeOffsetFormatter`, `TimeSpan` → `TimeSpanFormatter`. Only the *object* formatters differ
(`MpProbeResolver+MpProbe+OneDateTimeFormatter` vs. the IL-emitted
`MessagePack.Formatters.MpProbe_OneDateTimeFormatter1`), and the generated object formatters
never encode a `DateTime` themselves — they delegate:

```csharp
// generated OneDateTimeFormatter.g.cs
MsgPack::FormatterResolverExtensions.GetFormatterWithVerify<global::System.DateTime>(formatterResolver)
    .Serialize(ref writer, value.When, options);
```

Also worth recording: plain `MessagePackSerializerOptions.Standard` picks the **source-generated**
formatters, not the reflection ones, because `SourceGeneratedFormatterResolver` is ahead of
`DynamicObjectResolver` in the list above. It finds them via
`typeof(T).Assembly.GetCustomAttributes<GeneratedAssemblyMessagePackResolverAttribute>()`
(`src/MessagePack/Resolvers/SourceGeneratedFormatterResolver.cs:35-48`); the generator emits that
assembly attribute (`[assembly: GeneratedAssemblyMessagePackResolverAttribute(typeof(MpProbeResolver), 3, 1)]`).
No registration call is required.

---

## What this means for protop — the temporal decision (#15)

**The premise that a `DateTime` "already occupies one integer-ish slot" is false.** It occupies a
variable-length ext -1 payload of 6, 10 or 15 bytes. A scaled-integer schema representation is
therefore *not* free relative to today: for a sub-second instant it saves 1 byte (10 → 9), for a
whole-second instant in an int64 slot it costs 3 (6 → 9). The interesting comparison is against
the nested `google.protobuf.Timestamp` lowering, which is 11 bytes and an extra array header —
worse than both.

What follows for the design:

1. **The wire break, if one is taken, should be justified by semantics, not by size.** The size
   argument is a wash. The semantic argument is real and it is this: *the corpus's 12 `DateTime`
   members currently round-trip to `Kind=Utc` no matter what went in, and the nine nullable ones
   are the only members with an unambiguous "absent" encoding.* Any producer today writing
   `Kind=Unspecified` (which is what a `DateTime` parsed from a string or read from most DB
   drivers is) is writing ticks stamped as UTC — a silent offset shift that a protop-generated
   reader would inherit if it keeps `DateTime`.
2. **If protop keeps `DateTime`**, the profile must say, and the generator should enforce, that
   the only safe Kind is `Utc` — with an analyzer or runtime guard, because the failure is silent.
   Keeping `DateTime` also keeps the payload readable as a real msgpack timestamp by other
   languages, which is the one genuine advantage of the status quo and should not be given up
   lightly.
3. **If protop moves to a scaled integer**, it is a wire break on all 12 members and should pick
   its unit deliberately. ext -1 carries 100 ns fidelity today (verified: the +1234-ticks case
   round-trips exactly), so unix **milliseconds would be a lossy break** on any payload with
   sub-millisecond stamps; microseconds or 100 ns ticks would not. A scaled integer is also the
   only option that makes the type non-negotiably unambiguous at the schema level rather than
   relying on a .NET type's conventions.
4. **`google.protobuf.Timestamp` as a nested message is the worst of the three** on this evidence:
   11 bytes, a nested array, no Kind, and it still requires a custom formatter to map to/from
   `DateTime`. Nothing recommends it here except protobuf familiarity.
5. **`DateTimeOffset` should be excluded from the profile** (the corpus already contains none).
   Its encoding writes the *wall clock as if UTC* alongside the offset, so it is only
   self-consistent between two MessagePack-CSharp peers — a cross-language reader that treats
   element 0 as an instant is wrong by the offset.
6. **`TimeSpan` (also absent from the corpus) is an untagged int64 of 100 ns ticks.** If protop
   ever admits a duration type it should define the unit in the schema rather than inherit this.
7. **`NativeDateTimeResolver` is available but is a trap for a cross-language spec**: it preserves
   Kind and ticks exactly in 9 bytes, but the payload is `DateTime.ToBinary()` — .NET-only and not
   a timestamp to anyone else. Worth naming in the ADR only to rule it out explicitly.

*Unverified:* whether ext -1 payloads written by MessagePack-CSharp are accepted by the specific
non-.NET readers this project cares about (no cross-implementation test was run); and whether
`MessagePackSerializerOptions.OldSpec` mode matters here — `MessagePackWriter.cs:415-428` throws
`NotSupportedException` for `DateTime` in that mode, which was read but not exercised.

---

## Part 2 — Structs, `required`, `init`, `Nullable<TStruct>`

### The v3 source generator **can** deserialise a `struct` with `required` members

This is the headline answer to the second question, and it is a clean **yes**. The corpus shape
was compiled verbatim:

```csharp
[MessagePackObject]
public struct VenueRequiredStruct
{
    [Key(1)] public required long Id { get; set; }
    [Key(2)] public required string Name { get; set; }
}
```

It compiles with **zero errors and zero analyzer diagnostics**, and it round-trips. The emitted
formatter:

```csharp
// generated Formatters...VenueRequiredStructFormatter.g.cs — Deserialize
if (reader.TryReadNil())
{
    throw new global::System.InvalidOperationException("typecode is null, struct not supported");
}
options.Security.DepthStep(ref reader);
var length = reader.ReadArrayHeader();
var __Id__ = default(long);
var __Name__ = default(string);
for (int i = 0; i < length; i++)
{
    switch (i)
    {
        case 1: __Id__ = reader.ReadInt64(); break;
        case 2: __Name__ = ...GetFormatterWithVerify<string>(formatterResolver).Deserialize(ref reader, options); break;
        default: reader.Skip(); break;
    }
}
var ____result = new global::MpProbe.VenueRequiredStruct() { Id = __Id__, Name = __Name__ };
```

The mechanism: members that are `init`-only or `required` and not constructor parameters go into
`InitMembers`; that flips the formatter into "read into locals first, then construct" mode, and
construction goes through an object initializer, which is exactly what C# requires to satisfy
`required`.

```csharp
// src/MessagePack.SourceGenerator/CodeAnalysis/ObjectSerializationInfo.cs:22
public bool MustDeserializeFieldsFirst => this.ConstructorParameters.Length > 0 || this.InitMembers.Length > 0;

// src/MessagePack.SourceGenerator/CodeAnalysis/ObjectSerializationInfo.cs:83
InitMembers = members.Where(x => ((x.IsProperty && x.IsInitOnly) || x.IsRequired) && !constructorParameters.Contains(x)).ToArray(),
```

`GetConstructorString()` (`ObjectSerializationInfo.cs:97-140`) builds the
`new T(ctorArgs) { InitMember = __local__, ... }` text; the template picks between the two shapes at
`src/MessagePack.SourceGenerator/Transforms/FormatterTemplate.tt:76-82` (deferred locals vs.
`var ____result = new ...` up front) and excludes init/required/ctor members from the post-loop
assignment path (`FormatterTemplate.cs:151`). `MemberSerializationInfo`
(`src/MessagePack.SourceGenerator/CodeAnalysis/MemberSerializationInfo.cs:8-11`) carries
`IsWritable`, `IsInitOnly`, `IsRequired`.

The reflection path also handles it (`MessagePack.Formatters.MpProbe_VenueRequiredStructFormatter2`
was produced and round-tripped). So **"every protop message becomes a class" is a choice, not a
forced move.**

### But `required` is **not enforced** at deserialisation

| input bytes | type | result |
|---|---|---|
| `90` (empty fixarray) | `VenueRequiredStruct` | **succeeds**: `Id=0`, `Name=null` |
| `90` | `RequiredInitClass` (`required ... { get; init; }`) | **succeeds**: `Id=0`, `Name=null` |

A `required string` comes back `null` with no error and no warning. The generator knows and says so:

```
// src/MessagePack.SourceGenerator/CodeAnalysis/ObjectSerializationInfo.cs:127-130
// Strictly speaking, we should only be assigning these init-only properties if values for them
// was provided in the deserialized stream.
// However the C# language does not provide a means to do this, so we always assign them.
// https://github.com/dotnet/csharplang/issues/6117
```

`required` is a *construction-site* guarantee only. It buys nothing on the wire.

### Struct and class are byte-identical

Same members, same keys, same bytes:

| type | hex | JSON |
|---|---|---|
| `VenueStructPlain` (struct, keys 1,2) | `93 C0 07 A5 4C 65 74 6E 61` | `[null,7,"Letna"]` |
| `VenueClassPlain` (class, keys 1,2) | `93 C0 07 A5 4C 65 74 6E 61` | `[null,7,"Letna"]` |
| `VenueRequiredStruct` | `93 C0 07 A5 4C 65 74 6E 61` | `[null,7,"Letna"]` |
| `VenueRequiredClass` | `93 C0 07 A5 4C 65 74 6E 61` | `[null,7,"Letna"]` |

The leading `C0` is the nil pad for the unused key 0 — the INVARIANTS claim that MessagePack pads
an int-keyed layout to a dense array of `maxKey + 1` is confirmed on the wire, and the mechanism is
`writer.WriteArrayHeader(Info.MaxKey + 1)` with `writer.WriteNil()` for each gap
(`FormatterTemplate.tt:45-53`). `Venue`/`BOVenue` starting their keys at 1 costs exactly one wasted
byte per instance.

### `init` vs `{ get; set; }` changes the generated code but not the bytes

`InitOnlyClass`, `SetterClass`, `InitOnlyStruct` and `RequiredInitClass` all produce
`92 07 A5 4C 65 74 6E 61` (`[7,"Letna"]`). The only difference is the shape of the generated
deserialiser: plain setters assign directly inside the switch
(`____result.Id = reader.ReadInt64();`), `init`/`required` defer into locals and use an object
initializer. Both are one pass over the array.

### `Nullable<TStruct>` is byte-identical to a nullable class reference

| holder | hex | JSON |
|---|---|---|
| `HolderOfNullableStruct` (`VenueStructPlain? Venue`, `VenueStructPlain? VenueNull`) | `92 93 C0 07 A5 4C 65 74 6E 61 C0` | `[[null,7,"Letna"],null]` |
| `HolderOfNullableClass` (`VenueClassPlain?`, x2) | `92 93 C0 07 A5 4C 65 74 6E 61 C0` | `[[null,7,"Letna"],null]` |
| `HolderOfRequiredStruct` (`VenueRequiredStruct?`, x2) | `92 93 C0 07 A5 4C 65 74 6E 61 C0` | `[[null,7,"Letna"],null]` |
| bare `(VenueStructPlain?)null` | `C0` | `null` |
| bare `(VenueClassPlain?)null` | `C0` | `null` |

The generated resolver registers `new MessagePack.Formatters.NullableFormatter<VenueRequiredStruct>()`
for the `T?` closed type; `NullableFormatter<T> where T : struct`
(`src/MessagePack/Formatters/NullableFormatter.cs:13-40`) writes nil for null and otherwise
delegates to `options.Resolver.GetFormatterWithVerify<T>()`. Cost relative to a class member: one
extra formatter instance, one extra virtual call, and one extra struct copy (`value.Value`) per
element — the formatter interface takes `T value` by value, not `in`/`ref`, so the struct is copied
into `Serialize` regardless. **No boxing.** Nothing observable on the wire.

### The one real struct/class difference: a nil slot is fatal to a struct

```
// FormatterTemplate.tt:58-65
if (reader.TryReadNil())
{
    return null;                                        // class
    throw new InvalidOperationException("typecode is null, struct not supported");  // struct
}
```

Measured:

| bytes | target | result |
|---|---|---|
| `93 C0 C0 C0` (all three slots nil) | `VenueRequiredStruct` | **throws** `MessagePackSerializationException`, inner: "Unexpected msgpack code 192 (nil)" — key 1 is nil where an int64 is expected |
| `91 C0` | `HolderOfBareStruct` (non-nullable `VenueStructPlain` member) | **throws** `MessagePackSerializationException`, inner `InvalidOperationException: typecode is null, struct not supported` |
| `90` (slot simply absent) | `HolderOfBareStruct` | succeeds, member defaults (`Id=0`, `Name=null`) |

A **non-nullable struct member cannot tolerate a nil in its slot**, but *can* tolerate the slot
being missing entirely (short array). That is a direct collision with the nil-padded dense-array
wire model: any key gap, and any producer that writes nil for "not set", makes a non-nullable
struct member unreadable.

---

## What this means for protop — the struct decision (#12)

1. **"Every protop message is a class" is not forced by the serializer.** A `struct` with
   `required` members compiles, generates a correct formatter, and round-trips on both the
   generated and reflection paths. If protop chooses classes it must justify it on other grounds
   (identity, mutation, size, the nil question below) — not on "the generator can't".
2. **`BOVenue`/`Venue` need no migration story for the wire.** Struct → class is a **byte-identical**
   change for these two types: `93 C0 07 A5 4C 65 74 6E 61` either way, and `Venue?` → `Venue`
   (nullable class) is byte-identical too. Whatever #12 decides, existing cached payloads stay
   readable. That removes the main cost from the decision.
3. **The decisive technical argument is nil tolerance, and it favours classes.** A protop wire
   model that pads gaps with nil and treats "nil in slot *k*" as "field *k* absent" is
   *incompatible* with a non-nullable struct member: the generated struct formatter throws
   `InvalidOperationException("typecode is null, struct not supported")`. Classes degrade to
   `null`. If protop's profile wants nil to mean "absent" uniformly — which the corpus's nil
   padding already implies — then message types should be classes, or every message-typed member
   must be declared `T?`.
4. **Do not rely on `required` for wire validation.** It is silently unenforced: a truncated or
   nil-padded payload yields `default`/`null` for `required` members with no exception
   (`ObjectSerializationInfo.cs:127-130` documents why the language makes this impossible). If
   protop wants presence guarantees, they have to be generated as explicit post-deserialisation
   checks, not delegated to `required`. This is independent of struct vs. class.
5. **`init` costs nothing on the wire** and only changes the generated deserialiser's internal
   shape (deferred locals + object initializer). The corpus's `SlimQuote`/`SlimShow.Id` usage
   is free. `init` is a fine default for generated immutable messages.
6. **Keys should start at 0.** Confirmed empirically that `Key(1)`-based `Venue`/`BOVenue` pay one
   `C0` byte per instance forever. If protop's `Key = tag - 1` rule is applied uniformly, that byte
   goes away and the two exceptions in the corpus disappear.
7. **Struct members are not free at runtime even though they are free on the wire**: `Nullable<T>`
   adds a formatter hop and a struct copy, and `IMessagePackFormatter<T>.Serialize` takes `T` by
   value, so large structs are copied on every serialise. For two-field `Venue` this is noise; it
   would stop being noise for a wide message.

*Unverified:* behaviour with `[MessagePackObject(keyAsPropertyName: true)]` / string keys (the
corpus is int-keyed throughout, so no map-key shape was probed); `readonly struct` and
`record struct` were not probed; and no test was run with `DynamicAssembly.AvoidDynamicCode`
true (AOT), where `DynamicObjectResolver` drops out of the default list entirely
(`StandardResolver.cs:259-264`) and only the generated path exists.

---

## Summary for the two blocked decisions

- **#15 temporal**: v3 writes `DateTime` as msgpack ext type -1, variable 6/10/15 bytes, at 100 ns
  fidelity, always reading back as `Kind=Utc`. `Kind` is lost; `Local` makes bytes TZ-dependent;
  `Unspecified` silently shifts the instant. A scaled-integer slot is roughly size-neutral (±1-3
  bytes), so choose on semantics; `google.protobuf.Timestamp` nesting is strictly worse (11 bytes).
- **#12 struct**: the generator handles `struct` + `required` correctly, and struct/class/nullable
  are byte-identical, so the choice is free of migration cost. The real differentiator is that a
  non-nullable struct member throws on a nil slot while a class becomes `null` — which points at
  classes for a nil-padded wire model. `required` provides no wire-level validation in either case.
