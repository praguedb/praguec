# `[Key(N)]` on a get-only computed member: what MessagePack-CSharp v3 actually does

Answers issue #20. Primary sources and measurement only. Every mechanism claim cites a file and line in
MessagePack-CSharp at tag **v3.1.4** (commit `44aa149acce1de32881e007f53fad0f3db7dfdab`); every behavioural
claim is backed by bytes produced by a compiled probe, not by reading the generator. Claims that could not
be measured are quarantined in §8 rather than mixed in.

---

## Bottom line

**1. It writes, it does not read, and it says nothing.** The v3 source generator emits a full serialize for a
get-only `[Key(N)]` property and emits `reader.Skip()` in that slot on deserialize. The build produces **zero**
MessagePack diagnostics. The asymmetry is silent by construction, not by oversight — see §1.

**2. In explicit-key (array) mode the generated and reflection paths agree byte-for-byte.** Measured identical
output and successful cross-reads in both directions (§2). This is *not* a second divergence of the issue-#3
kind — in array mode.

**3. In map mode (`keyAsPropertyName: true`) they silently disagree.** Generated writes 30 bytes including the
computed member; reflection writes 22 and omits it. That *is* a second silent divergence, and it is the one
worth carrying forward (§2.2).

**4. A get-only member without `[Key]` on a `[MessagePackObject]` type is a hard build error**, exactly as a
settable one would be: `error MsgPack004`. There is no "ignored cleanly" path; `[IgnoreMember]` is the only
escape hatch, and it removes the member from the wire entirely (§3).

**5. `(long, long)` needs no custom formatter.** `ValueTupleFormatter<T1,T2>` serves it on both paths and encodes
it as a 2-element MessagePack array. Tuple element names never reach the wire. Tuple-ness is irrelevant to the
skip; get-only-ness is what drives it — a scalar `long Sum => A + B` behaves identically (§4).

**6. Prague puts `[Key(0)]` on every computed key — all eight of them.** The ticket says seven; the corpus at
`22331403` has **16** `[DataCache]` types, of which **8** declare a get-only expression-bodied ValueTuple
`[DataCacheKey]`, and **every one** carries `[Key(0)]` and sits on a `[MessagePackObject]` type (§5).

**7. The real hazard is not the wasted bytes, it is the constructor.** A computed `[Key(0)]` member shifts the
positional mapping used to match a deserializing constructor's parameters. On a type whose only constructor is
parameterized, this fails on both paths: the generator emits `error MsgPack007` **and** uncompilable code
(`CS7036`); reflection throws at runtime. Prague survives only because those types also declare a parameterless
constructor (§6).

**8. The slot is positionally load-bearing.** Deleting the computed member later is a wire-breaking change, because
every subsequent key shifts down one array index. Measured: a 2-element wire read back into the 3-slot type yields
`EventId=222, TicketTypeId=0` (§1.3).

---

## 0. Method and environment

- Source of truth: MessagePack-CSharp checked out at tag `v3.1.4`, commit `44aa149acce1de32881e007f53fad0f3db7dfdab`,
  in `/private/tmp/mpresearch/MessagePack-CSharp`. All `src/...` paths below are relative to that checkout.
- Probes: throwaway .NET 9 console projects under `/tmp/kp20/` referencing `MessagePack` **3.1.4** from NuGet.
  `EmitCompilerGeneratedFiles=true` with `CompilerGeneratedFilesOutputPath=generated` so the emitted formatters
  could be read directly rather than inferred. (`<Compile Remove="generated/**" />` is required, or the emitted
  files are globbed back in and the build fails with CS0579/CS0102/CS0111.)
- Three serializer configurations were compared on every type:
  - `standard` = `MessagePackSerializerOptions.Standard`
  - `generated` = `CompositeResolver.Create(GeneratedMessagePackResolver.Instance, StandardResolver.Instance)`
  - `dynamic` = `CompositeResolver.Create(BuiltinResolver, AttributeFormatterResolver, DynamicEnumResolver, DynamicGenericResolver, DynamicUnionResolver, DynamicObjectResolver)` — the reflection path with the generated resolver deliberately absent.
- Isolating the reflection path for the constructor case required a project that references `MessagePack.dll` and
  `MessagePack.Annotations.dll` directly via `<Reference>`/`<HintPath>`, because
  `<PackageReference Include="MessagePackAnalyzer" ExcludeAssets="all" />` does **not** suppress the generator —
  MessagePack pulls it transitively with `include="All"`, so the generator still runs and the compile still fails.

The probe type that mirrors Prague exactly:

```csharp
[MessagePackObject]
public sealed partial class TupleKeyItem
{
    [Key(0)] public (long EventId, long TicketTypeId) Key => (EventId, TicketTypeId);
    [Key(1)] public long EventId { get; set; }
    [Key(2)] public long TicketTypeId { get; set; }
}
```

---

## 1. Sub-question 1 — does the generator emit a write? a read? a diagnostic?

### 1.1 The emitted formatter, verbatim

From `/tmp/kp20/probe/generated/MessagePack.SourceGenerator/MessagePack.SourceGenerator.MessagePackGenerator/Formatters.MessagePack.GeneratedMessagePackResolver.Probe.TupleKeyItemFormatter.g.cs`:

```csharp
public void Serialize(ref MsgPack::MessagePackWriter writer, global::Probe.TupleKeyItem value, MsgPack::MessagePackSerializerOptions options)
{
    if (value == null) { writer.WriteNil(); return; }
    MsgPack::IFormatterResolver formatterResolver = options.Resolver;
    writer.WriteArrayHeader(3);
    MsgPack::FormatterResolverExtensions.GetFormatterWithVerify<(long EventId, long TicketTypeId)>(formatterResolver).Serialize(ref writer, value.Key, options);
    writer.Write(value.EventId);
    writer.Write(value.TicketTypeId);
}

public global::Probe.TupleKeyItem Deserialize(ref MsgPack::MessagePackReader reader, MsgPack::MessagePackSerializerOptions options)
{
    ...
    var length = reader.ReadArrayHeader();
    var ____result = new global::Probe.TupleKeyItem();
    for (int i = 0; i < length; i++)
    {
        switch (i)
        {
            case 0:
                reader.Skip();
                break;
            case 1:
                ____result.EventId = reader.ReadInt64();
                break;
            case 2:
                ____result.TicketTypeId = reader.ReadInt64();
                break;
            default:
                reader.Skip();
                break;
        }
    }
    reader.Depth--;
    return ____result;
}
```

**Write: yes. Read: no — `reader.Skip()`. Diagnostic: none.** A clean build of the probe emitted no `MsgPack0xx`
warning or error of any kind for this type.

### 1.2 Why, in the generator's own source

Three mechanisms compose to produce exactly this.

**(a) A getter alone is enough to be a member.** The only membership filter in the explicit-key path is
`src/MessagePack.SourceGenerator/CodeAnalysis/TypeCollector.cs:927-933`:

```csharp
bool isReadable = item.GetMethod is not null;
bool isWritable = item.SetMethod is not null;
bool isInitOnly = item.SetMethod?.IsInitOnly is true;
if (!isReadable && !isWritable)
{
    continue;
}
```

Only a member with *neither* accessor is dropped. The map-mode path has the identical test at
`TypeCollector.cs:839-843`.

**(b) The array length is computed from readable members, so the computed member extends it.**
`src/MessagePack.SourceGenerator/CodeAnalysis/ObjectSerializationInfo.cs:51-56`:

```csharp
public int MaxKey
{
    get
    {
        return this.Members.Where(x => x.IsReadable).Select(x => x.IntKey).DefaultIfEmpty(-1).Max();
    }
}
```

and `WriteCount` at `:36` returns `Members.Count(x => x.IsReadable)` in string-key mode, `MaxKey` otherwise.

**(c) The write loop has no readability check; the read loop has a writability check.**
`src/MessagePack.SourceGenerator/Transforms/FormatterTemplate.tt:45-52` (serialize):

```
writer.WriteArrayHeader(<#= Info.MaxKey + 1 #>);
<# for (var i = 0; i <= Info.MaxKey; i++) {
  var member = Info.GetMember(i);
  if (member == null) { #>
    writer.WriteNil();
<# } else { #>
    <#= member.GetSerializeMethodString() #>;
```

`FormatterTemplate.tt:94-99` (deserialize) — the whole answer in five lines:

```
<# } else {
  if (member.IsWritable) { #>
        <#= member.GetMemberAccess("____result") #> = <#= member.GetDeserializeMethodString() #>;
<# } else { #>
        reader.Skip();
<# } #>
```

There is no diagnostic anywhere on this path. `IsWritable` selects a template branch; it never reports.

### 1.3 What that costs, measured

| type | hex | bytes | JSON |
|---|---|---|---|
| `TupleKeyItem` (Prague shape) | `93926FCCDE6FCCDE` | 8 | `[[111,222],111,222]` |
| same with `[IgnoreMember]` on the computed key | `926FCCDE` | 4 | `[111,222]` |

The computed key is **4 of 8 bytes — 50% overhead** on a two-field item, for a value that is never read back.

Three further measurements on the same type, all on both paths:

- **The wire value is ignored.** Hand-built wire `9392CD03E7CD03E76FCCDE` = `[[999,999],111,222]` deserializes to
  `Key=(111, 222)` on both the generated and the reflection path. The property recomputes from `EventId`/`TicketTypeId`;
  the transmitted tuple is discarded.
- **The slot is positionally load-bearing.** Wire `[111,222]` (2 elements) read back as `TupleKeyItem` gives
  `EventId=222, TicketTypeId=0` — slot 0 was consumed as the skipped key, slot 1 as `EventId`. Removing the computed
  member from the type later shifts every key down one index and breaks every persisted payload.
- **Key gaps become explicit nils, and the computed member does not change that.** `GapItem` (computed `[Key(0)]`,
  then keys 1 and 3) serializes to `9492050605C006` = `[[5,6],5,null,6]` — the `C0` hole is emitted by
  `FormatterTemplate.tt:48` (`writer.WriteNil()` for a missing index).

### 1.4 The default path is the generated one

Measured formatter identity (`options.Resolver.GetFormatter<T>().GetType().FullName`):

| options | formatter for `TupleKeyItem` |
|---|---|
| `MessagePackSerializerOptions.Standard` | `MessagePack.GeneratedMessagePackResolver+Probe+TupleKeyItemFormatter` |
| `generated` composite | same |
| `dynamic` composite | `MessagePack.Formatters.Probe_TupleKeyItemFormatter1` (Reflection.Emit) |

So in a v3 project that compiles the annotated types itself, **the generated formatter is what runs by default**;
the reflection path only appears when the generated resolver is deliberately excluded (or the types live in an
assembly the generator never saw).

---

## 2. Sub-question 2 — does the reflection resolver behave the same?

### 2.1 In explicit-key (array) mode: yes, byte-for-byte

| type | generated | dynamic | equal? |
|---|---|---|---|
| `TupleKeyItem` | `93926FCCDE6FCCDE` | `93926FCCDE6FCCDE` | **true** |
| `ScalarComputedKeyItem` (`[Key(0)] long Sum => A + B`) | `93070304` | `93070304` | **true** |
| `IgnoredComputedItem` (`[IgnoreMember]`) | `926FCCDE` | `926FCCDE` | **true** |

Cross-reads succeed in both directions: generated bytes → reflection reader gives `111/222`, reflection bytes →
generated reader gives `111/222`. Tampered slot 0 is ignored by both.

**Why they agree.** The reflection resolver's own readability flags are set the same way, at
`src/MessagePack/Resolvers/DynamicObjectResolver.cs:1533-1534`, and the write/read paths are filtered by
`IsReadable`/`IsWritable` at `:372`, `:378`, `:505-506`, `:531`, `:545` — the same shape as the template. The
member survives the resolver's final filter at `DynamicObjectResolver.cs:1835`:

```csharp
var membersArray = members.Where(m => m.IsExplicitContract || m.IsWrittenByConstructor || m.IsWritable).ToArray();
```

because `[Key(N)]` sets `IsExplicitContract = true` at `DynamicObjectResolver.cs:1643`. A get-only member with an
explicit key is neither writable nor constructor-written, so **the `[Key]` attribute is the only reason it is on
the wire at all** on the reflection path.

### 2.2 In map mode (`keyAsPropertyName: true`): they silently disagree

Probe type `MapModeComputedItem` — same three members, no `[Key]` attributes, `[MessagePackObject(keyAsPropertyName: true)]`:

| path | hex | bytes | JSON |
|---|---|---|---|
| generated | `83A34B6579926FCCDEA74576656E7449646FA9426574547970654964CCDE` | 30 | `{"Key":[111,222],"EventId":111,"TicketTypeId":222}` |
| dynamic | `82A74576656E7449646FA9426574547970654964CCDE` | 22 | `{"EventId":111,"TicketTypeId":222}` |

`gen==dyn bytes: False`. Different map header (`83` vs `82`), different key set. No diagnostic, no exception —
the two paths simply produce different documents for the same object.

**Cause, from source.** The map-mode template serializes **every** member with no readability filter,
`src/MessagePack.SourceGenerator/Transforms/StringKey/StringKeyFormatterTemplate.tt:57-62`:

```
writer.WriteMapHeader(<#= list.Count #>);
<# foreach (var memberAndBinary in list) {
    var member = memberAndBinary.Item1; #>
    writer.WriteRaw(GetSpan_<#= member.UniqueIdentifier #>());
    <#= member.GetSerializeMethodString() #>;
```

while its read side filters on `IsWritable` (`:86`, `:113`) — so the emitted map-mode deserializer contains
`reader.Skip(); continue;` for the `"Key"` case, exactly as in array mode. Meanwhile the reflection resolver
applies the `IsExplicitContract || IsWrittenByConstructor || IsWritable` filter at `DynamicObjectResolver.cs:1835`,
and with no `[Key]` attribute present there is no `IsExplicitContract` — so the member is dropped before the
writer ever sees it.

**This is the second silent divergence after the issue-#3 formatter-arguments one.** It does not bite the Prague
shape (explicit int keys), but it bites any type that uses `keyAsPropertyName: true` with a computed property and
mixes the two resolvers — for example a producer that compiles the type (generated) and a consumer that loads it
dynamically (reflection).

---

## 3. Sub-question 3 — a get-only member with no `[Key(N)]`

Not ignored. **Build error**, and the same error a settable unkeyed member produces. From `dotnet build` of
`/tmp/kp20/nokey`:

```
error MsgPack004: Properties and fields of MessagePackObject-attributed types require either KeyAttribute or IgnoreMemberAttribute: global::NoKey.UnkeyedComputedItem.Sum
```

The descriptor, `src/MessagePack.SourceGenerator/Analyzers/MsgPack00xMessagePackAnalyzer.cs:65-73`:

```csharp
public static readonly DiagnosticDescriptor MemberNeedsKey = new DiagnosticDescriptor(
    id: AttributeMessagePackObjectMembersId,
    title: "Attribute properties and fields of MessagePack objects",
    category: Category,
    messageFormat: "Properties and fields of MessagePackObject-attributed types require either KeyAttribute or IgnoreMemberAttribute: {0}.{1}",
    description: "Member must be marked with KeyAttribute or IgnoreMemberAttribute.",
    defaultSeverity: DiagnosticSeverity.Error,
```

`AttributeMessagePackObjectMembersId = "MsgPack004"` at `:17`; severity is **Error**, not Warning. Get-only-ness
buys no exemption — `TypeCollector.cs:927-933` admitted the member, and from that point it needs a key or an ignore.

`[IgnoreMember]` is the clean escape hatch and was measured to work: `IgnoredComputedItem` serializes to 4 bytes
(`926FCCDE`) on **both** paths, i.e. the member is off the wire entirely and the two resolvers agree.

---

## 4. Sub-question 4 — does `ValueTuple` change the answer?

No. Two separate facts.

**`(long, long)` is serializable with no custom formatter.** Measured on both paths, a bare `(111L, 222L)` is
`926FCCDE` = `[111,222]` — a 2-element MessagePack array. The formatter is
``MessagePack.Formatters.ValueTupleFormatter`2[System.Int64,System.Int64]`` on both paths. On the generated side
the resolver table itself contains the entry (from the emitted `GeneratedMessagePackResolver.g.cs`):

```csharp
{ typeof((long, long)), 0 },
...
case 0: return new MsgPack::Formatters.ValueTupleFormatter<long, long>();
```

On the reflection side `src/MessagePack/Resolvers/DynamicGenericResolver.cs:178-187` matches
`System.ValueTuple*` by full name and picks the arity-matching `ValueTupleFormatter<,>`. The implementation lives
at `src/MessagePack/Formatters/ValueTupleFormatter.cs:58` for the 2-arity case.

**Tuple element names are not on the wire.** `(long EventId, long TicketTypeId)` and `(long, long)` produce the same
bytes; the names are compile-time metadata (`TupleElementNamesAttribute`) and the formatter writes a positional
array. A consumer needs the same declared element order, not the same names.

**Tuple-ness is irrelevant to the skip.** `ScalarComputedKeyItem` — a get-only `long Sum => A + B` at `[Key(0)]` —
behaves identically: `93070304` = `[7,3,4]`, written, skipped on read, no diagnostic, both paths equal. The driver
is `IsWritable == false`, nothing about the member's type.

A settable tuple member is a normal round-tripping member, as the control shows: `SettableTupleItem` with
`[Key(0)] (long,long) Key { get; set; }` serializes to `9292070807` = `[[7,8],7]` and deserializes back to
`Key=(7, 8)`.

---

## 5. Sub-question 5 — what Prague actually does at `22331403`

Read-only inspection of the event API service's repository via `git show <rev>:<path>` and
`git grep <rev>` at commit `22331403a40c93448b4ee8a35e83dc484ba95d60` (branch `origin/feature/BO_Pragure_Integration`).
Nothing was written, staged, committed, or checked out there.

**The count in the ticket is low.** There are **16** `[DataCache]` types at that commit. **Eight** declare a
get-only expression-bodied `ValueTuple` `[DataCacheKey]`, and **all eight carry `[Key(0)]` on it** and sit on a
`[MessagePackObject]` type:

| # | path | member |
|---|---|---|
| 1 | `BO/BO.Services/Prague/Models/BOTicketTypeCacheItem.cs:24` | `(long EventId, long TicketTypeId) Key` |
| 2 | `BO/BO.Services/Prague/Models/BOSearchCacheItem.cs:18` | `(long EventId, Channel ChannelId) Key` |
| 3 | `BP/BrandProcessor.Services/Prague/Models/BPTicketTypeCacheItem.cs:24` | `(long EventId, long TicketTypeId) Key` |
| 4 | `BP/BrandProcessor.Services/Prague/Models/BPSearchCacheItem.cs:18` | `(long EventId, Channel ChannelId) Key` |
| 5 | `Common/Common.EventApi.Cache/Prague/Models/SlimTicketType.cs:44` | `(long EventId, long TicketTypeId) Id` |
| 6 | `BrandReader/BrandReader.Services/Models/SearchCacheItem.cs:18` | `(long EventId, Channel ChannelId) Key` |
| 7 | `Processors/EventsBackgroundProcessor/Services/Cache/TicketTypeCacheItem.cs:24` | `(long EventId, long TicketTypeId) Key` |
| 8 | `Processors/EventsBackgroundProcessor/Services/Cache/SearchCacheItem.cs:15` | `(long EventId, Channel ChannelId) Key` |

Representative, verbatim from `BO/BO.Services/Prague/Models/BOTicketTypeCacheItem.cs`:

```csharp
[MessagePackObject]
[DataCache(CacheClassName = nameof(BOTicketTypeCache))]
[DataCacheTopic("Cache.[v:brandId].TicketType")]
public sealed partial class BOTicketTypeCacheItem
{
    [IgnoreMember]
    [JsonIgnore]
    public long TicketTypeId { get => Id; set => Id = value; }

    [Key(0)]
    [DataCacheKey]
    public (long EventId, long TicketTypeId) Key => (EventId, TicketTypeId);

    [Key(1)]
    [DataCacheIndex]
    public long EventId { get; set; }
```

Note the two idioms sitting side by side in one type: a *settable* alias marked `[IgnoreMember]` (off the wire,
correct) and a *get-only* computed key marked `[Key(0)]` (on the wire, never read back). The second is the
pattern this ticket is about, and it is used consistently across all eight.

So the answer to "do those types put `[Key(N)]` on the computed key, or leave it unkeyed?" is: **`[Key(0)]`,
uniformly, in every case.** They had no choice — as §3 shows, leaving it unkeyed on a `[MessagePackObject]` type
is a compile error, and `[IgnoreMember]` is the only other legal option.

---

## 6. The consequence nobody asked about: deserializing constructors

This is the finding with teeth, and it was not among the five sub-questions.

**Int-key constructor matching is positional over key-sorted members, on both paths.** Generator, at
`src/MessagePack.SourceGenerator/CodeAnalysis/TypeCollector.cs:1244-1258`:

```csharp
IReadOnlyDictionary<int, (MemberSerializationInfo Info, ITypeSymbol TypeSymbol)> ctorParamIndexIntMembersDictionary = intMembers
    .OrderBy(x => x.Key).Select((x, i) => (Key: x.Value, Index: i))
    .ToDictionary(x => x.Index, x => x.Key);
...
if (ctorParamIndexIntMembersDictionary.TryGetValue(ctorParamIndex, out ... member))
{
    if (this.compilation.ClassifyConversion(member.TypeSymbol, item.Type) is { IsImplicit: true } && member.Info.IsReadable)
```

Reflection resolver, same mapping at `src/MessagePack/Resolvers/DynamicObjectResolver.cs:1697`.

A computed `[Key(0)]` member therefore occupies **constructor parameter slot 0**. A constructor
`Ctor(long eventId, long ticketTypeId)` is then asked to accept a `(long, long)` tuple as its first parameter, the
conversion check fails, and the candidate constructor is rejected.

**Generated path** (`/tmp/kp20/ctoronly`, type with a computed `[Key(0)]` and *only* a parameterized constructor):

```
error MsgPack007: Cannot find a public constructor
error CS7036: There is no argument given that corresponds to the required parameter 'eventId' of 'CtorOnlyTupleKeyItem.CtorOnlyTupleKeyItem(long, long)'
```

The `CS7036` is inside the emitted `.g.cs` — **the generator reports its own diagnostic and then emits
uncompilable code anyway**, the same failure shape as issue #3. Descriptor:
`MsgPack00xMessagePackAnalyzer.cs:147-155` (`messageFormat: "Cannot find a public constructor"`,
`defaultSeverity: DiagnosticSeverity.Error`), `DeserializingConstructorId = "MsgPack007"` at `:20`; reported from
`TypeCollector.cs:1348`.

**Reflection path** (`/tmp/kp20/reflectonly`, generator excluded by referencing the DLLs directly):

```
ctor-only computed-key FAILED: MessagePackSerializationException: Failed to serialize ReflectOnly.CtorOnlyTupleKeyItem value.
   inner: MessagePackDynamicObjectResolverException: can't find matched constructor. type:ReflectOnly.CtorOnlyTupleKeyItem
```

Thrown at `src/MessagePack/Resolvers/DynamicObjectResolver.cs:1802`. Note this is a **runtime** failure with no
compile-time warning — the generated path at least fails the build.

The control settles causation: an identical type with the same constructor but **no** computed member compiles
and round-trips fine on both paths (`926FCCDE`, reads back `EventId=111 TicketTypeId=222`).

**Prague survives this only by accident of style.** `SlimTicketType.cs` declares *both* a parameterless constructor
(line 12) and a parameterized one (line 17); the parameterless one wins and the computed key is harmless. Add a
computed `[Key(0)]` to a record or to a type with only a parameterized constructor and it breaks — at build time
under the generator, at first-serialize under reflection.

---

## 7. What this means for praguec

Stated as consequences, not recommendations — the design calls belong to the parent session.

1. **A computed/derived member is not expressible as a wire field without waste.** If praguec ever generates a
   convenience accessor over other fields, giving it a `[Key]` buys nothing on read and costs bytes on write.
   `[IgnoreMember]` is the correct emission for anything the generator knows is derived.
2. **If praguec emits positional records or constructor-initialized types, it must never emit a get-only
   `[Key]` member** — §6 makes that a build break. The safe emission is: every keyed member is settable or a
   constructor parameter.
3. **Key→slot identity is positional and permanent.** §1.3 shows a removed member shifts every later key. This
   reinforces the existing `FIELD_TAGS_ASCENDING` finding and the "never renumber" rule in
   `docs/research/proto-to-messagepack-csharp.md`.
4. **Map mode is not a safe default.** §2.2 is a real silent cross-resolver divergence in `keyAsPropertyName`
   mode. Explicit int keys avoid it.
5. **A migration reading the Prague corpus must expect the wasted slot.** Eight types carry a `[Key(0)]` array
   element that is written and never read. A faithful praguec schema for those types has to either keep the slot
   (wire-compatible, wasteful) or drop it and accept a breaking renumber. That is a decision, not a detail.

---

## 8. Not verified

Collected deliberately rather than mixed into the findings above.

- **Other v3 patch versions.** Everything here is measured on **3.1.4** only. 3.1.5 and later were not built or
  probed. The "`IsWritable` selects a template branch" mechanism is old and unlikely to have changed, but that is
  inference, not measurement.
- **AOT / Unity / `mpc` CLI paths.** Not probed. The v3 source generator was exercised through a normal .NET 9
  SDK build only; the Unity IL2CPP path and the standalone `mpc` code generator were not run.
- **`DynamicObjectResolverAllowPrivate`, `ContractlessStandardResolver`, and `[DataMember]`-based contracts.**
  Not probed. Only `[MessagePackObject]` with explicit int keys and with `keyAsPropertyName: true` were measured.
- **`init`-only and `required` members interacting with computed members.** `TypeCollector.cs:929` tracks
  `isInitOnly` and `StringKeyFormatterTemplate.tt:113` filters on `!x.IsInitOnly && !x.IsRequired`, so behaviour
  very likely differs — but no probe was written for it.
- **Records specifically.** §6's constructor hazard was measured on classes with explicit constructors, not on
  `record` declarations with primary constructors. The mechanism is the same code path, but the exact diagnostic
  text on a record was not captured.
- **Whether Prague's eight types are ever deserialized by a reflection-only consumer.** The divergence in §2.2
  does not apply to them (they use explicit int keys), and §2.1 shows their mode agrees — but no consumer in the
  corpus was traced end to end to confirm which resolver actually runs in production.
- **Struct (value-type) `[MessagePackObject]` targets.** `FormatterTemplate.tt` has a separate
  `throw new InvalidOperationException("typecode is null, struct not supported")` branch for the nil case; no
  struct probe was written.
