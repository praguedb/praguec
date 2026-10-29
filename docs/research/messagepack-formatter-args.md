# Can `MessagePackFormatterAttribute` carry constructor arguments?

Resolves [issue #3](https://github.com/praguedb/praguec/issues/3).

**Source of truth**: MessagePack-CSharp at tag `v3.1.4`, commit `44aa149acce1de32881e007f53fad0f3db7dfdab` (2025-06-12). All `file:line` citations below are against that tree. Everything marked **[measured]** was observed by building and running a real project against the `MessagePack` 3.1.4 NuGet package on .NET 8; the method is in §8.

---

## 1. Bottom line

**The attribute accepts arguments. The v3 source generator silently ignores them.**

`[MessagePackFormatter(typeof(ScaledDecimalFormatter), 4)]` compiles, produces no warning, and — on the source-generated path that protop will emit into — constructs the formatter as `new ScaledDecimalFormatter()`. The `4` is discarded. The reflection path (`DynamicObjectResolver`) *does* honour it. So the two paths produce **different bytes for the same object** with no diagnostic anywhere. **[measured]**

```
--- generated resolver ---   json : [-9223372036854775808,"12.3456"]   ← sentinel: scale never arrived
--- dynamic resolver  ---    json : [123456,"12.3456"]                 ← scale 4 arrived
```

The assumed emission shape is therefore **wrong and must be abandoned**. It is worse than wrong: it fails silently and only on the path protop actually uses.

**Recommendation: carry the scale in a type parameter.** One generic formatter definition, `ScaledDecimalFormatter<TScale>`, closed per scale at the use site: `[MessagePackFormatter(typeof(ScaledDecimalFormatter<Scale4>))]`. Verified working end to end on the generated path, with two scales in one message, and with `decimal?` presence round-tripping. §4.2 has the code and the measured bytes.

---

## 2. The attribute itself

`src/MessagePack.Annotations/Attributes.cs:124-141` — the full declaration:

```csharp
[AttributeUsage(AttributeTargets.Class | AttributeTargets.Struct | AttributeTargets.Interface | AttributeTargets.Enum
    | AttributeTargets.Field | AttributeTargets.Property | AttributeTargets.Parameter | AttributeTargets.ReturnValue,
    AllowMultiple = false, Inherited = true)]
public class MessagePackFormatterAttribute : Attribute
{
    public Type FormatterType { get; }
    public object?[]? Arguments { get; }

    public MessagePackFormatterAttribute(Type formatterType) { ... }
    public MessagePackFormatterAttribute(Type formatterType, params object?[]? arguments) { ... }
}
```

So: **yes**, there is a `params object?[]?` overload, and the values are retained on an `Arguments` property. `AttributeTargets` includes `Property` and `Field`, so per-member application is legal. The attribute is not documented in the README — the `## MessagePackFormatterAttribute` section (`README.md:1418-1470`) shows only the single-argument form, and no doc page anywhere in `doc/` mentions `Arguments`.

That is the whole of the attribute's contract. What happens to `Arguments` is entirely up to the consumer, and v3 has two consumers that disagree.

---

## 3. The two paths, and where they diverge

### 3.1 Reflection path — honours arguments

`src/MessagePack/Resolvers/ResolverUtilities.cs:12-33` is the shared activator:

```csharp
internal static IMessagePackFormatter ActivateFormatter(Type formatterType, object?[]? args = null)
{
    if (args == null || args.Length == 0)
    {
        if (formatterType.GetConstructor(Type.EmptyTypes) is ConstructorInfo ctor) { return ...ctor.Invoke(...); }
        else if (FetchSingletonField(formatterType) is FieldInfo instance) { return ...instance.GetValue(null)...; }
        else { throw new MessagePackSerializationException("...has no default constructor nor implements the singleton pattern."); }
    }
    else
    {
        return (IMessagePackFormatter)Activator.CreateInstance(formatterType, args)!;
    }
}
```

Type-level attributes route through it at `src/MessagePack/Resolvers/AttributeFormatterResolver.cs:36-48`:

```csharp
MessagePackFormatterAttribute? attr = typeof(T).GetTypeInfo().GetCustomAttribute<MessagePackFormatterAttribute>();
...
Formatter = (IMessagePackFormatter<T>)ResolverUtilities.ActivateFormatter(formatterType, attr.Arguments);
```

Member-level attributes are handled by `DynamicObjectResolver`, which emits IL doing the same thing. `src/MessagePack/Resolvers/DynamicObjectResolver.cs:390-453`, `BuildCustomFormatterField`:

- line 408-422 — the singleton fast path is taken **only when there are no arguments**: `if ((attr.Arguments == null || attr.Arguments.Length == 0) && ResolverUtilities.FetchSingletonField(attr.FormatterType) is FieldInfo singletonField)`;
- line 436-444 — otherwise it emits IL that re-reads the attribute off the member at runtime, pushes `attr.FormatterType` and `attr.Arguments`, and calls `Activator.CreateInstance(Type, object[])`.

The `MethodInfo` handles for both property getters are cached at `DynamicObjectResolver.cs:1393-1394`. So on this path the design intent is unambiguous: arguments are a supported feature.

The same method also **validates the formatter's generic argument against the member type** at `DynamicObjectResolver.cs:400-403`, throwing `MessagePackSerializationException` if `IMessagePackFormatter<T>`'s `T` is not equivalent to the member's declared type. That matters for `decimal?` — see §5.

### 3.2 Generated path — ignores arguments, and cannot express them

Three independent places in `MessagePack.SourceGenerator` establish this.

**(a) The argument list is never read.** `src/MessagePack.SourceGenerator/CodeAnalysis/TypeCollector.cs:756-773`:

```csharp
FormatterDescriptor? GetSpecialFormatter(ISymbol member)
{
    INamedTypeSymbol? name = member.GetAttributes()
        .FirstOrDefault(x => x.AttributeClass.ApproximatelyEqual(this.typeReferences.FormatterAttribute))
        ?.ConstructorArguments[0].Value as INamedTypeSymbol;
    ...
}
```

`ConstructorArguments[0]` is the `Type`. `ConstructorArguments[1]` — the `params` array — is never touched, here or anywhere else in the generator. The type-level check at `TypeCollector.cs:695-709` reads `ConstructorArguments[0]` too and does nothing else.

**(b) The model has nowhere to put them.** `src/MessagePack.SourceGenerator/CodeAnalysis/AnalyzerOptions.cs:211` declares `FormatterDescriptor(QualifiedNamedTypeName Name, string? InstanceProvidingMember, QualifiedTypeName InstanceTypeName, ImmutableHashSet<FormattableType> FormattableTypes)` — no argument field. `TryCreate` (`AnalyzerOptions.cs:237-240`) determines how to get an instance, and recognises exactly two shapes:

```csharp
IFieldSymbol? instanceField = type.GetMembers("Instance").OfType<IFieldSymbol>()
    .FirstOrDefault(m => m.IsStatic && m.DeclaredAccessibility == Accessibility.Public && m.IsReadOnly);
IMethodSymbol? ctor = type.InstanceConstructors.FirstOrDefault(ctor => ctor.Parameters.Length == 0 && ctor.DeclaredAccessibility >= Accessibility.Internal);
string? instanceProvidingMember = instanceField?.Name ?? ctor?.Name ?? null;
```

A `public static readonly Instance` field, or a **parameterless** constructor. Nothing else.

**(c) The emitted expression is hard-coded to zero arguments.** `AnalyzerOptions.cs:260-262`:

```csharp
public string InstanceExpression => this.InstanceProvidingMember == ".ctor"
    ? $"new {this.Name.GetQualifiedName(...)}()"
    : $"{this.Name.GetQualifiedName(...)}.{this.InstanceProvidingMember}";
```

consumed by `src/MessagePack.SourceGenerator/Transforms/FormatterTemplate.tt:19` (and `Transforms/StringKey/StringKeyFormatterTemplate.tt:23` for string-keyed objects):

```
private readonly <#= item.CustomFormatter.InstanceTypeName... #> __<#= item.Name #>CustomFormatter__ = <#= item.CustomFormatter.InstanceExpression #>;
```

The call sites are `src/MessagePack.SourceGenerator/CodeAnalysis/MemberSerializationInfo.cs:36-39` and `52-55`.

**Measured output.** For `[Key(0)] [MessagePackFormatter(typeof(ScaledDecimalFormatter), 4)] public decimal Stake { get; set; }`, the generator emitted: **[measured]**

```csharp
internal sealed class BetFormatter : MsgPack::Formatters.IMessagePackFormatter<global::Bet>
{
    private readonly global::ScaledDecimalFormatter __StakeCustomFormatter__ = new global::ScaledDecimalFormatter();
    ...
    this.__StakeCustomFormatter__.Serialize(ref writer, value.Stake, options);
```

Build succeeded. Zero warnings about the dropped `4`.

### 3.3 The failure mode when there is no parameterless constructor

If the formatter has *only* the `(int scale)` constructor — the shape the assumed design implies — `instanceProvidingMember` is `null`, and `InaccessibleDescriptor` is set to `MsgPack013` (`AnalyzerOptions.cs:245-248`; descriptor at `src/MessagePack.SourceGenerator/Analyzers/MsgPack00xMessagePackAnalyzer.cs:247-255`, severity **Warning**):

> *Formatter should declare a default constructor with at least internal visibility or a public static readonly field named Instance that returns the singleton*

The generator does not stop there. `InstanceExpression` still runs, with `InstanceProvidingMember == null`, and emits: **[measured]**

```csharp
private readonly global::CtorOnlyFormatter __CtorOnlyCustomFormatter__ = global::CtorOnlyFormatter.;
```

which fails to compile with `CS1001: Identifier expected` pointing into a generated file. **This is a generator bug in v3.1.4** — the MsgPack013 warning should be an error, or emission should be skipped. Practically it means the assumed shape does not merely lose the scale; it breaks the build with a confusing error. (Not reported upstream as part of this work.)

### 3.4 Why the divergence is dangerous rather than merely annoying

Both paths are reachable in the same process. A payload written by one and read by the other is **silently misinterpreted**: same schema, same types, no exception, wrong number. For a wallet/betting domain that is a corruption bug, not a serialization bug.

> **Correction (2026-09-12).** An earlier revision of this paragraph said `MessagePackSerializerOptions.Standard` reaches `DynamicObjectResolver` on a JIT platform, implying the reflection path is what a consumer gets by default. **That is wrong, and it inverts the risk.** Measured on 3.1.8 with a plain `[MessagePackObject]` type in the same compilation as the generator:
>
> ```
> Standard resolver type    : MessagePack.Resolvers.StandardResolver
> Poco formatter (Standard) : MessagePack.GeneratedMessagePackResolver+PocoFormatter
> Poco formatter (reflect)  : MessagePack.Formatters.PocoFormatter1
> ```
>
> `DynamicObjectResolver` *is* in `StandardResolver`'s chain, but the source-generated resolver precedes it, so whenever the generator ran for a type the **generated** formatter wins — on JIT, under the default options, with no opt-in. Note also that no `[GeneratedMessagePackResolver] partial class` declaration was needed: referencing the `MessagePack` package is enough for the generator to emit `MessagePack.GeneratedMessagePackResolver`. The reflection path is reached only by explicitly building a composite that puts `DynamicObjectResolver` first — which is what every probe in this document does — or for a type the generator never saw. The divergence and the corruption hazard both stand; what changes is which side is the default and which is the deliberate act.

protop must not emit anything whose correctness depends on which resolver is in play.

---

## 4. Alternatives, evaluated

All three were required by the ticket. Ranked by cost.

### 4.1 One generated formatter type per scale — **rejected**

`ScaledDecimal4Formatter`, `ScaledDecimal2Formatter`, … each `IMessagePackFormatter<decimal>` with a parameterless ctor or `Instance` field. This does work at the member level, but:

**It fails the build with `MsgPack009` as soon as there are two of them.** **[measured]**

```
error MsgPack009: Multiple formatters for type decimal found
```

Descriptor at `MsgPack00xMessagePackAnalyzer.cs:237-245`, reported from `MsgPack00xMessagePackAnalyzer.cs:415-419`; rationale in `doc/analyzers/MsgPack009.md`: *"All formatters in a compilation are automatically added to a source generated resolver… When two formatters implement `IMessagePackFormatter<T>` for the same `T`, it cannot be statically determined which formatter should be used."* Severity is **Error**.

The escape hatch is `[ExcludeFormatterFromSourceGeneratedResolver]` on all but one (`src/MessagePack.Annotations/AnalyzerAttributes.cs:59-71`; recognised at `AnalyzerOptions.cs:249-250`). Applying it to the second formatter made the build green and the round-trip correct. **[measured]** But the cost is real:

- protop must emit the attribute on *every* scale formatter but one, and pick which one is privileged — an arbitrary choice with a visible consequence, because the non-excluded one is the assembly's nominated formatter for bare `decimal`;
- N scales means N type declarations, N `Instance` fields, N chunks of identical code;
- the attribute is `[Conditional("NEVERDEFINED")]`, so it leaves no metadata — it is a compile-time-only contract that silently stops working if the analyzer is disabled.

Strictly worse than §4.2 on every axis.

### 4.2 Generic `ScaledDecimalFormatter<TScale>` — **recommended**

One definition; the scale is a type argument resolved at compile time.

```csharp
public interface IScale { static abstract int Scale { get; } }
public readonly struct Scale4 : IScale { public static int Scale => 4; }
public readonly struct Scale2 : IScale { public static int Scale => 2; }

public sealed class ScaledDecimalFormatter<TScale> : IMessagePackFormatter<decimal>
    where TScale : struct, IScale
{
    public static readonly ScaledDecimalFormatter<TScale> Instance = new();
    public void Serialize(ref MessagePackWriter writer, decimal value, MessagePackSerializerOptions options)
        => writer.Write((long)(value * Pow10(TScale.Scale)));
    public decimal Deserialize(ref MessagePackReader reader, MessagePackSerializerOptions options)
        => reader.ReadInt64() / (decimal)Pow10(TScale.Scale);
}
```

used as `[MessagePackFormatter(typeof(ScaledDecimalFormatter<Scale4>))]`.

Measured result with two scales and two nullable members on one message: **[measured]**

```
generated bytes: 95-CE-00-01-E2-40-CD-04-D2-CE-00-01-81-CD-C0-C0
generated json : [123456,1234,98765,null,null]
round-trip:      S4=12.3456  S2=12.34  NP=9.8765  NA=<null>  Bare=<null>
```

Why it works where §4.1 does not:

- **No `MsgPack009`.** The collision map is built one entry per *formatter type* (`AnalyzerOptions.cs:38-52`, read back at `:120`), keyed by `FormatterDescriptor.Name` — which, per the record's own doc comment at `AnalyzerOptions.cs:207`, *"if the formatter is a generic type… should **not** include any generic type parameters."* Every closed instantiation collapses onto the one open definition, so there is nothing to collide with. No `[ExcludeFormatterFromSourceGeneratedResolver]` needed anywhere. **[measured]**
- **No hijacking of bare `decimal`.** The generated resolver's lookup table contained only the message type; the custom formatter was not registered as *the* `decimal` formatter, and a plain `decimal` member on the same type still went through `GetFormatterWithVerify<decimal>(formatterResolver)` to the built-in. **[measured]**
- `TryCreate` finds the `public static readonly Instance` field, and the template emits `global::ScaledDecimalFormatter<global::Scale4>.Instance` — correct closed-generic syntax, verified in the generated file. **[measured]**
- `static abstract` members mean `TScale.Scale` is a JIT-time constant per instantiation: no field read, no indirection, and the multiplier folds. One instance per scale, allocated once in a static field.

Cost to protop: emit an `IScale` interface and one `ScaleN` struct per distinct scale actually used in the schema (a handful, and they are two lines each), plus one hand-written-once generic formatter shipped in protop's runtime support library. The scale stays in the schema — `(protop.decimal) { scale: 4 }` maps mechanically to the type argument.

### 4.3 A resolver keyed on member — **impossible**

`src/MessagePack/IFormatterResolver.cs:21-29`:

```csharp
public interface IFormatterResolver
{
    IMessagePackFormatter<T>? GetFormatter<T>();
}
```

The only key is the type `T`. A resolver cannot see which member it is being asked for, so it cannot vary by member. The only member-level hook in the entire library is `MessagePackFormatterAttribute` itself. Ruled out by the interface shape; no further investigation needed.

A degenerate variant — a distinct wrapper *type* per scale (`Scaled4 { decimal Value }`) resolved normally — is really §4.1 with an extra allocation and a changed public API surface, and it changes the C# the user writes. Not worth pursuing.

---

## 5. `decimal?`, presence, and what the formatter actually sees

**The generated path passes `decimal?` to the custom formatter unchanged. There is no `StaticNullableFormatter` wrapping.**

`MemberSerializationInfo.cs:36-39` emits `this.__XCustomFormatter__.Serialize(ref writer, value.X, options)` where `value.X` is the member as declared. Nothing in the generator inspects `Nullable<T>`. So a `decimal?` member needs `IMessagePackFormatter<decimal?>`; applying an `IMessagePackFormatter<decimal>` to it fails with **[measured]**:

```
error CS1503: Argument 2: cannot convert from 'decimal?' to 'decimal'
```

— again reported against a generated file, not as an analyzer diagnostic. (The reflection path throws a clearer `MessagePackSerializationException` from `DynamicObjectResolver.cs:400-403`, but only at runtime.)

The contrast with built-in `decimal?` is worth keeping straight: `src/MessagePack/Resolvers/BuiltinResolver.cs:87-88` registers

```csharp
{ typeof(decimal),  DecimalFormatter.Instance },
{ typeof(decimal?), new StaticNullableFormatter<decimal>(DecimalFormatter.Instance) },
```

and `StaticNullableFormatter<T>` (`src/MessagePack/Formatters/NullableFormatter.cs:43-76`) is what writes `WriteNil()` for null and does `TryReadNil()` on the way back. That wrapper is applied by the *resolver*, which never runs for a member carrying `MessagePackFormatterAttribute`. **So protop must emit a separate nullable formatter for optional decimal members** — `NullableScaledDecimalFormatter<TScale> : IMessagePackFormatter<decimal?>` — which does the nil handling itself.

Measured presence behaviour with that formatter: **[measured]**

| member | value | wire |
|---|---|---|
| `decimal?` present, scale 4 | `9.8765` | `CE 00 01 81 CD` (uint32 `98765`) |
| `decimal?` absent | `null` | `C0` (nil) |
| bare `decimal?`, no custom formatter | `null` | `C0` (nil) |

Round-trip restored `9.8765` and `null` respectively. Presence survives. A truncated payload (3 slots of 5) left the missing members at their CLR defaults — `null` for the nullable — consistent with the tolerant-deserialization finding in `proto-to-messagepack-csharp.md` §7.3. **[measured]**

**Note for the presence design**: a scaled-decimal member that is *absent* and one that is *present and zero* are distinguishable (`C0` vs `00`), which is the property protop needs. That holds only because the formatter writes nil explicitly — a formatter that wrote `0` for null would destroy it.

---

## 6. What a bare `decimal` costs today

`src/MessagePack/Formatters/StandardClassLibraryFormatter.cs:139-224`. **`decimal` is serialized as a MessagePack string** containing the invariant-culture decimal text:

```csharp
var dest = writer.GetSpan(1 + MessagePackRange.MaxFixStringLength);
if (System.Buffers.Text.Utf8Formatter.TryFormat(value, dest.Slice(1, MessagePackRange.MaxFixStringLength), out var written))
{
    dest[0] = (byte)(MessagePackCode.MinFixStr | written);   // fixstr header
    writer.Advance(written + 1);
}
else { writer.Advance(0); writer.Write(value.ToString(CultureInfo.InvariantCulture)); }
```

Deserialization is `Utf8Parser.TryParse` over the string bytes, throwing `MessagePackSerializationException` on nil or on a non-decimal string.

Measured wire cost: **[measured]**

| value | bytes | encoding |
|---|---|---|
| `12.3456` | 8 | `A7 "12.3456"` |
| `0` | 2 | `A1 "0"` |
| `1` | 2 | `A1 "1"` |
| `1.10` | 5 | `A4 "1.10"` — **trailing zero preserved** |
| `-0.0001` | 8 | `A7 "-0.0001"` |
| `decimal.MaxValue` | 30 | `BD "79228162514264337593543950335"` |

Against that, a scale-4 `sint64`: `12.3456` → `CE 00 01 E2 40`, **5 bytes**; small values reach 1-2 bytes via fixint. `writer.Write(long)` (`src/MessagePack/MessagePackWriter.cs:348-353`) picks the most compact integer encoding, unlike `WriteInt64` (`:359-364`) which always emits 9 bytes. **Use `Write`, not `WriteInt64`.**

Three consequences for the migration-cost question:

1. **The saving is real but modest** — roughly 8 bytes → 5 for a typical money value, better for small ones, dramatically better for large ones. The stronger arguments for scaling remain the semantic ones (a parse per read, and no schema-enforced scale).
2. **The deviation is total, not incremental.** String and integer are different MessagePack major types; there is no tolerant reader. A schema that switches a field from bare `decimal` to scaled `sint64` breaks every existing payload. This is a one-way door per field, exactly like the `Key = tag` vs `Key = tag - 1` question (`proto-to-messagepack-csharp.md` §7.2, open question #1).
3. **Bare `decimal` preserves scale on the wire; a scaled `sint64` does not.** `1.10m` round-trips as `1.10` today (`decimal` carries its own scale in its representation); through a scale-4 formatter it comes back as `1.1000` — numerically equal, `decimal.Equals`-equal, but not `ToString`-identical. Anything that renders a decimal by `ToString()` and compares text will see a difference. Worth a line in the protop spec.

A third built-in exists: `NativeDecimalFormatter` (`src/MessagePack/Formatters/UnsafeBinaryFormatters.cs:58-100`, wired by `src/MessagePack/Resolvers/NativeDecimalResolver.cs:29-33`) writes the raw 16-byte little-endian `decimal` and throws on big-endian hosts. Fixed 18 bytes on the wire, not portable, not a candidate.

---

## 7. What this means for protop

1. **Drop the `[MessagePackFormatter(typeof(ScaledDecimalFormatter), 4)]` shape.** It is a silent-corruption hazard on the exact path protop generates into. Nothing in the spec should depend on formatter constructor arguments.

2. **Adopt scale-as-type-parameter.** protop's runtime support library ships, hand-written and once:
   - `IScale` with `static abstract int Scale { get; }`;
   - `ScaledDecimalFormatter<TScale> : IMessagePackFormatter<decimal>`;
   - `NullableScaledDecimalFormatter<TScale> : IMessagePackFormatter<decimal?>`.

   praguec emits, per compilation unit: one `readonly struct ScaleN : IScale` per distinct scale in the schema, and `[MessagePackFormatter(typeof(ScaledDecimalFormatter<ScaleN>))]` (or the nullable variant) per decimal member. The scale lives in the `.proto`; the C# is mechanical.

3. **Presence decides which formatter.** `optional`/explicit-presence decimal → `NullableScaledDecimalFormatter<ScaleN>` on a `decimal?` member. Implicit-presence → `ScaledDecimalFormatter<ScaleN>` on a `decimal`. Getting this pairing wrong produces a `CS1503` in generated code, which is a bad error to hand a user — **praguec should validate the pairing itself** and refuse to emit, with a message naming the field.

4. **New lint rule candidate: scale must fit.** A `sint64` at scale *s* holds ±(2⁶³−1)/10ˢ. At scale 4 that is ≈ ±922,337,203,685,477. At scale 9 it is ≈ ±9.2 billion. protop declares the scale, so protop can check it against a declared magnitude bound and reject a schema that cannot represent its own domain. This is one of the six things §2.6 of the main research doc claims protop can do that protoc cannot — it is now concretely implementable. **Unverified**: whether the formatter should throw or saturate on overflow at runtime is an open design question, not settled here.

5. **Never let two non-generic formatters claim the same `T` in one assembly.** `MsgPack009` is an error, and praguec emits whole assemblies. The generic design avoids it structurally; any future per-member formatter (money, timestamps, IDs) should follow the same one-open-definition rule rather than reach for `[ExcludeFormatterFromSourceGeneratedResolver]`.

6. **Differential testing must cover both resolvers.** The oracle strategy in §10 of the main research doc compares praguec against protoc. Add a second axis: serialize every fixture through `GeneratedMessagePackResolver` *and* through `DynamicObjectResolver` and assert byte equality. That test would have caught this issue on day one, and it guards every future formatter decision.

7. **Deviating from bare `decimal` is a one-way door per field.** Bundle it with open question #1 (`Key = tag` vs `tag - 1`) and decide both before the first payload is persisted.

---

## 8. Method / reproduction

MessagePack-CSharp cloned from `github.com/MessagePack-CSharp/MessagePack-CSharp`, checked out at tag `v3.1.4` (`44aa149a`). All source citations read from that tree.

Empirical probes: four throwaway `net8.0` console projects referencing `MessagePack` 3.1.4 from NuGet, built with `EmitCompilerGeneratedFiles=true` so the generator's output could be read directly.

| probe | question | result |
|---|---|---|
| 1 | does `[MessagePackFormatter(typeof(F), 4)]` reach `F`? | generated: no (`new F()`, sentinel in output). dynamic: yes. Different bytes. |
| 2 | formatter with only `(int scale)` ctor | `MsgPack013` warning + `CS1001` from `global::F.;` |
| 2b | `ScaledDecimalFormatter<TScale>`, two scales, `decimal?` present/absent, truncated payload | all correct |
| 3 | two non-generic `IMessagePackFormatter<decimal>` types | `MsgPack009` error; fixed by `[ExcludeFormatterFromSourceGeneratedResolver]` |
| 4 | `IMessagePackFormatter<decimal>` on a `decimal?` member | `CS1503` in generated code |

The formatter used in probe 1 had **both** a parameterless constructor (setting `scale = -1`, a sentinel) and a `(int scale)` constructor, so the two paths could be told apart from their output rather than from a crash.

Probes were scratch projects and are not checked in; the code is reproduced inline above in full.
