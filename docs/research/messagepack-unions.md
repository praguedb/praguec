# MessagePack-CSharp v3 unions: mechanics and constraints

Primary-source research for [issue #5](https://github.com/praguedb/praguec/issues/5). Every claim cites either a file and line in MessagePack-CSharp at **tag `v3.1.4`** (commit `44aa149acce1de32881e007f53fad0f3db7dfdab`) or a byte string produced by a throwaway probe project referencing `MessagePack` **3.1.4** on `net8.0`. Claims that could not be verified against a primary source are labelled **unverified** in place; there are six of them.

**Method.** Two probe projects were built and run, then deleted. The positive probe declared a `[Union]` interface, an abstract-class union, a struct arm, a partial-split union, a nested union, a sparse-key union and an unlisted implementor, and dumped every payload as both hex and `MessagePackSerializer.ConvertToJson`, once through `MessagePackSerializer.DefaultOptions` (which reaches the source generator) and once through a hand-built `CompositeResolver` ending in `DynamicUnionResolver.Instance` (the reflection path). The negative probe compiled seven deliberately-invalid declarations one at a time and captured the compiler diagnostic or the runtime exception chain. Where the two resolver paths disagree, both results are given.

---

## Bottom line

**1. A union is a two-element MessagePack array — `[key, payload]` — and nothing else.** No type name, no assembly-qualified string, no length prefix. `Serialize<IAnimal>(new Dog { Name = "Rex", Legs = 4 })` is `92 D2 00 00 00 00 92 A3 52 65 78 04`, which is `[0, ["Rex", 4]]`. The payload is exactly the bytes the concrete type's own formatter would have written standalone (`92 A3 52 65 78 04`), wrapped in one extra two-element array. The envelope costs **two bytes plus the key** on the reflection path and **six bytes** on the source-generated path (§1).

**2. The two resolver paths produce different bytes for the same object, and this is a real hazard.** The AOT template calls `writer.WriteInt32(key)`, which always emits the 5-byte `0xD2` int32 form; the reflection resolver calls `writer.Write(int)`, which emits the compact form. So the same `Dog` is 12 bytes through the source generator and 8 through `DynamicUnionResolver`. Both deserialize identically — the reader is `ReadInt32()` on both paths — so they are **interoperable but not byte-identical**. Any protop rule that hashes, dedupes or byte-compares a payload containing a union is therefore resolver-sensitive (§1.2).

**3. Union keys and `[Key(N)]` are wholly independent number spaces, and — unlike `[Key(N)]` — union keys have no density penalty whatsoever.** `[0, ["Rex", 4]]` has a union key `0` alongside member keys `0` and `1` in the same payload with no interaction. A union key of `1000` costs the same 6 bytes as a union key of `0` on the AOT path (`92 D2 00 00 03 E8 …`), and `int.MinValue`…`int.MaxValue` are all legal. This is the exact opposite of the int-keyed object layout, which pads a dense array to `maxKey + 1` with one literal `WriteNil()` per gap (see `proto-to-messagepack-csharp.md` §7.2). Sparse union keys are free; sparse member keys are not (§3, §8).

**4. An unknown union key deserializes to `null`, silently, on both paths.** No exception, no diagnostic, no callback. The payload is `Skip()`ped and the enclosing object survives with the member defaulted: a `Holder` carrying an arm with key `99` round-trips as `EventId = 77, Animal = null`. This is a genuinely forward-compatible read story for a 20th subtype — old readers do not crash — but it is **lossy**: the unknown arm is discarded, not preserved, so an old binary that reads and re-writes a record destroys the new arm (§4).

**5. The sharpest hazard is the write side, not the read side: an unregistered runtime type serializes to `C0` (nil) with no error at all.** An interface implementor that is not listed in any `[Union]` attribute, *and* any subclass of a listed arm, both write one byte of nil. Dispatch is by exact `RuntimeTypeHandle`, not by assignability (`DynamicUnionResolver.cs:180-223` builds a `Dictionary<RuntimeTypeHandle, KeyValuePair<int,int>>`; the AOT template does the same). A forgotten arm is therefore **silent, total data loss on write**, and no analyzer catches it. Any protop union construct must make the arm list exhaustive by construction (§6).

**6. `[Union]` must sit on the base type, and the base must be an interface or an abstract class — but the source generator only treats *interfaces* as generation roots.** `[AttributeUsage(AttributeTargets.Interface | AttributeTargets.Class, AllowMultiple = true, Inherited = false)]` (`Attributes.cs:83`) rules out structs at compile time (`CS0592`). `Inherited = false` plus attribute lookup on the *static* type means `[Union]` on a derived type does nothing — `GetFormatter<IBase>()` returns `null` and serialization throws `FormatterNotRegisteredException`. The generator's syntax predicate is `node is InterfaceDeclarationSyntax` (`MessagePackGenerator.cs:26`), so an abstract-class union is only reached transitively, when some `[MessagePackObject]` type references it (§2, §5).

**7. `[Union]` attributes may be split across `partial` halves of the same declaration.** Verified: `[Union(0, typeof(SplitArm0))]` in one file and `[Union(1, typeof(SplitArm1))]` in another, on the same `partial interface ISplit`, both serialize and round-trip. The mechanism is that Roslyn's `INamedTypeSymbol.GetAttributes()` merges all partial declarations (`TypeCollector.cs:396`) and `FullModel.Combine` dedupes the resulting `UnionSerializationInfo` through an `ImmutableSortedSet` (`FullModel.cs:46, 55`). **Partial declarations cannot span assemblies**, so this permits `praguec` and a third-party generator to each contribute arms *within one project*, and nothing more (§2.3).

**8. A union-typed member nested inside a union arm works; a union arm whose declared type is itself a union interface does not.** `IOuter → OuterArm { int Tag; IAnimal Inner }` round-trips as `[0, [9, [0, ["Rex", 4]]]]`. But `[Union(0, typeof(IMid))]` where `IMid` is itself a union interface serializes any `IMid` instance to `C0`, because the dispatch lookup is on the concrete runtime type, which is never `IMid`. Union hierarchies are one level deep, always (§6).

**9. Duplicate union keys and duplicate subtypes are not diagnosed cleanly.** Two arms sharing key `0` either crash the generator at build time (`CS8785: Generator 'MessagePackGenerator' failed to generate source … 'An item with the same key has already been added. Key: 0'`) or — on a clean single-file build — compile with **zero warnings** and then fail at first use with `MessagePackSerializationException` ← `TypeInitializationException` ← `ArgumentException: An item with the same key has already been added. Key: 0`. The same subtype under two keys fails the same way with `Key: System.RuntimeTypeHandle`. `TypeCollector.CollectUnion` performs no duplicate validation at all (`TypeCollector.cs:394-430`); the reflection resolver does, and throws a clear `MessagePackDynamicUnionResolverException` (`DynamicUnionResolver.cs:119, 124`). **praguec must own uniqueness checking**, because the target does not (§9).

**10. On the protop question (issue #9), the evidence favours shape B — a dedicated closed-type construct.** Union keys are an independent, sparse-tolerant, zero-cost number space that a 19-arm `oneof` would needlessly entangle with the parent's field numbering and its dense-array layout; and a `oneof` produces 19 *fields in the parent*, not a substitutable member, so it would have to be reinterpreted rather than translated. Shape B maps 1:1 onto `[Union(n, typeof(T))]` and lets praguec enforce the four things the target leaves unenforced: key uniqueness, subtype uniqueness, arm exhaustiveness, and arm sealedness (§10).

**Open questions requiring a decision:**

| # | Question | Where | Reversibility |
|---|---|---|---|
| 1 | Do union keys start at `0` or `1`, and are they independent of the arm messages' field numbers? | §8, §10.4 | **None** once a payload is persisted. |
| 2 | Is "unknown arm reads as `null`" acceptable domain behaviour, or must protop emit a catch-all arm? | §4, §10.5 | Cheap now (add a reserved arm at key `N`); impossible later without a version flag. |
| 3 | Does protop require union arms to be `sealed`? | §7.3, §10.4 | Tightening later breaks existing schemas; the default should be yes. |
| 4 | Does protop pin a resolver (AOT vs reflection), given the byte divergence? | §1.2 | Cheap; but any byte-level invariant written now assumes an answer. |

---

## 1. The wire format

### 1.1 The envelope

Both implementations write the identical structure. The AOT template is the clearest statement of it:

```csharp
// src/MessagePack.SourceGenerator/Transforms/UnionTemplate.tt:36-38
writer.WriteArrayHeader(2);
writer.WriteInt32(keyValuePair.Key);
// … then the concrete formatter serializes the value
```

and the reflection resolver emits the same two calls in IL (`DynamicUnionResolver.cs:258-267`, with the method handles bound at `:483-484`). The README states it in one line: *"Unions are internally serialized to two-element arrays."* (`README.md:511`).

Measured, with `Dog { Name = "Rex", Legs = 4 }` as union key `0` of `IAnimal`, and `Cat { Name = "Tom" }` as key `1`:

| Expression | Bytes | JSON |
|---|---|---|
| `Serialize<Dog>(dog)` — no envelope | `92 A3 52 65 78 04` | `["Rex",4]` |
| `Serialize<IAnimal>(dog)` — source generator | `92 D2 00 00 00 00 92 A3 52 65 78 04` | `[0,["Rex",4]]` |
| `Serialize<IAnimal>(cat)` — source generator | `92 D2 00 00 00 01 91 A3 54 6F 6D` | `[1,["Tom"]]` |
| `Serialize<IAnimal>(dog)` — `DynamicUnionResolver` | `92 00 92 A3 52 65 78 04` | `[0,["Rex",4]]` |

Reading left to right on the source-generated form: `92` is fixarray(2); `D2 00 00 00 00` is int32 `0`, the union key; `92 A3 52 65 78 04` is the unchanged standalone encoding of `Dog`. The envelope is purely additive — **the arm's own layout is untouched**, which means an arm message's `[Key]` numbering is a completely separate concern from its union key.

### 1.2 The two key encodings, and why they differ

`MessagePackWriter` has two integer writers with different contracts:

```csharp
// src/MessagePack/MessagePackWriter.cs:288  — compact: fixint / int8 / int16 / int32 as needed
public void Write(int value)
// src/MessagePack/MessagePackWriter.cs:299  — always 5 bytes: 0xD2 + 4
public void WriteInt32(int value)
```

The AOT template calls `WriteInt32` (`UnionTemplate.tt:37`). The reflection resolver binds `MessagePackWriterTypeInfo.WriteInt32` to `GetRuntimeMethod(nameof(MessagePackWriter.Write), new[] { typeof(int) })` (`DynamicUnionResolver.cs:484`) — the field is *named* `WriteInt32` but resolves to the **compact** `Write(int)`. Hence the 12-vs-8-byte divergence above.

Both readers call `reader.ReadInt32()` (`UnionTemplate.tt:69`; `DynamicUnionResolver.cs` deserialize path), which accepts any msgpack integer encoding. So the formats are read-compatible in both directions. What is *not* safe is treating the byte string as canonical: a payload produced under `StandardResolver` with a source-generated formatter present is four bytes longer per union envelope than the same payload produced with only `DynamicUnionResolver` in play.

Which path runs is decided by resolver order:

```csharp
// src/MessagePack/Resolvers/StandardResolver.cs:261-263
public static readonly IFormatterResolver[] DefaultResolvers = DynamicAssembly.AvoidDynamicCode
    ? [BuiltinResolver.Instance, AttributeFormatterResolver.Instance, SourceGeneratedFormatterResolver.Instance, …, DynamicGenericResolver.Instance]
    : [BuiltinResolver.Instance, AttributeFormatterResolver.Instance, SourceGeneratedFormatterResolver.Instance, …, DynamicGenericResolver.Instance, DynamicUnionResolver.Instance];
```

`SourceGeneratedFormatterResolver` precedes `DynamicUnionResolver` unconditionally. So if the generator ran — and it runs whenever the `MessagePack` package is referenced with a `[Union]` interface in the compilation — the AOT formatter wins, and the fixed 5-byte key is what actually ships. Confirmed at runtime: `IAnimal formatter (default) = MessagePack.GeneratedMessagePackResolver+Probe+IAnimalFormatter`, versus `MessagePack.Formatters.Probe_IAnimalFormatter1` through the hand-built composite.

### 1.3 Interaction with the int-keyed dense array

The envelope nests as **one slot** of the parent's positional array. With `Holder { [Key(0)] long EventId; [Key(1)] IAnimal? Animal }`:

```
Holder { EventId = 1, Animal = dog }
92 01 92 D2 00 00 00 00 92 A3 52 65 78 04
└┬┘ └┬┘ └──────────────┬───────────────┘
 │   │                 └─ slot 1: the union envelope
 │   └─ slot 0: EventId
 └─ fixarray(2) — the Holder itself

json: [1,[0,["Rex",4]]]
```

and with a null member, `92 01 C0` → `[1,null]`.

The padding rule from `proto-to-messagepack-csharp.md` §7.2 applies unchanged — the union slot is just a slot. With `[Key(0)]` and `[Key(5)]`:

```
SparseHolder { EventId = 1, Animal = dog }   // Animal is [Key(5)]
96 01 C0 C0 C0 C0 92 D2 00 00 00 00 92 A3 52 65 78 04
└┬┘    └────┬────┘
 │          └─ four literal nils for the gap at keys 1..4
 └─ fixarray(6) = maxKey + 1

json: [1,null,null,null,null,[0,["Rex",4]]]
```

So a union member inherits the parent's density economics and contributes none of its own.

### 1.4 Nil

A `null` union-typed value writes one byte, `C0`, and `C0` reads back as `null`. On the write side this is explicit in both implementations — `UnionTemplate.tt` emits `writer.WriteNil()` when the value is null **or when its runtime type is not in the map** (see §6); the reflection resolver does the same (`DynamicUnionResolver.cs`, serialize path). On the read side, `if (reader.TryReadNil()) return null;` (`UnionTemplate.tt:57`).

---

## 2. Where `[Union(n, typeof(T))]` must be declared

### 2.1 The attribute itself

```csharp
// src/MessagePack.Annotations/Attributes.cs:83-84
[AttributeUsage(AttributeTargets.Interface | AttributeTargets.Class, AllowMultiple = true, Inherited = false)]
public class UnionAttribute : Attribute
```

Three facts follow directly. `AllowMultiple = true` — many arms on one declaration, as expected. `Inherited = false` — an arm list is not inherited by a derived base. And the target set excludes `AttributeTargets.Struct`, so `[Union]` on a struct is a **compile error**:

```
error CS0592: Attribute 'Union' is not valid on this declaration type.
              It is only valid on 'class, interface' declarations.
```

The attribute has two constructors — `(int key, Type subType)` at `:101` and `(int key, string subType)` at `:112`, the latter resolving via `Type.GetType(subType, throwOnError: true)`. The string overload is **unverified** here; the AOT analyzer flags it (`AotUnionAttributeRequiresTypeArg`), so it is presumably reflection-only.

### 2.2 It must be on the base, and the base must be abstract

The reflection resolver is explicit:

```csharp
// src/MessagePack/Resolvers/DynamicUnionResolver.cs:101-110
UnionAttribute[] unionAttrs = ti.GetCustomAttributes<UnionAttribute>().OrderBy(x => x.Key).ToArray();
…
if (!ti.IsInterface && !ti.IsAbstract)
{
    throw new MessagePackDynamicUnionResolverException("Union can only be interface or abstract class. Type:" + type.Name);
}
```

matching the README: *"Only interfaces and abstracts classes are allowed to be annotated with `Union` attributes. Unique union keys are required."* (`README.md:463`).

Two negative cases pin the behaviour down, and they disagree by path:

**`[Union]` on a concrete class.** The source generator ignores it entirely (its root predicate is interface-only, §5.1), the class gets an ordinary `[MessagePackObject]` formatter, and serializing a derived instance through the base **silently drops the derived members**:

```
Serialize<ConcreteBase>(new Derived { Z = 1, W = 2 })  →  91 01  →  [1]
Deserialize<ConcreteBase>(…)                           →  ConcreteBase   // not Derived; W lost
```

The same call through `DynamicUnionResolver` throws `MessagePackDynamicUnionResolverException: Union can only be interface or abstract class. Type:ConcreteBase`. **The AOT path is silently lossy where the reflection path is loud.**

**`[Union]` on the derived type only.** Because lookup is on the static type and `Inherited = false`, the base has no arm list:

```
MessagePackSerializer.DefaultOptions.Resolver.GetFormatter<IBase>()  →  null
Serialize<IBase>(new ArmA { V = 1 })
  → MessagePackSerializationException: Failed to serialize Neg.IBase value.
  → FormatterNotRegisteredException: Neg.IBase is not registered in resolver: MessagePack.Resolvers.StandardResolver
```

### 2.3 Splitting across `partial` halves — this works

Verified end to end. `SplitA.cs` carries `[Union(0, typeof(SplitArm0))] public partial interface ISplit {}` and `SplitB.cs` carries `[Union(1, typeof(SplitArm1))] public partial interface ISplit {}`. Both arms serialize and round-trip:

```
ISplit = SplitArm0   92 D2 00 00 00 00 91 01   [0,[1]]
ISplit = SplitArm1   92 D2 00 00 00 01 91 02   [1,[2]]
roundtrip arm1 (declared in the other half)    OK → SplitArm1
```

The mechanism is worth stating precisely, because it is what makes the guarantee robust rather than accidental:

- `TypeCollector.CollectUnion` reads `type.GetAttributes()` on the `INamedTypeSymbol` (`TypeCollector.cs:396`). Roslyn's symbol model merges attributes from **all** partial declarations into one list, so the collector never sees "halves".
- Both syntax nodes fire `ForAttributeWithMetadataName`, so the pipeline produces the type twice; `FullModel.Combine` unions the results into `ImmutableSortedSet.CreateBuilder<UnionSerializationInfo>(ResolverRegisterInfoComparer.Default)` (`FullModel.cs:46, 55`), which dedupes rather than emitting two formatters.

**The hard limit: `partial` cannot span assemblies.** So `praguec` and a third-party generator can each contribute arms only if they emit into the *same project*. Cross-assembly arm contribution is impossible by construction, and there is no `[UnionAdd]`-style escape hatch in v3.1.4.

---

## 3. Union keys vs `[Key(N)]`: independent number spaces

Verified directly. `Dog` is union key `0` and has members `[Key(0)] Name` and `[Key(1)] Legs`:

```
92 D2 00 00 00 00 92 A3 52 65 78 04   →   [0,["Rex",4]]
   └── union key 0 ──┘ └─ member keys 0 and 1 ─┘
```

There is no interaction, no offsetting and no collision check across the two. Structurally this is inevitable: the union key lives in the envelope array, the member keys index the payload array, and the two arrays are disjoint.

The one place the README warns about key reuse is a *different* concern — members inherited across a class hierarchy: *"you cannot reuse the same keys in derived types that are already present in the parent type, as internally a single flat array or map will be used and thus cannot have duplicate indexes/keys."* (`README.md:550`). That is about `[Key(N)]` on a base class and its subclass sharing one flat payload array, which the abstract-class union probe confirms: `Shape` declares `[Key(0)] Z` and `Circle` adds `[Key(1)] R`, producing a single flat `92 01 CB 40 00 …` = `[1,2]` payload inside the envelope `[0,[1,2]]`. It says nothing about union keys.

---

## 4. Unknown union keys, and malformed envelopes

### 4.1 Unknown key → `null`, silently

```csharp
// src/MessagePack.SourceGenerator/Transforms/UnionTemplate.tt:70-84
if (!this.keyToJumpMap.TryGetValue(key, out key)) { key = -1; }
switch (key)
{
    …
    default:
        reader.Skip();
        break;
}
```

with `result` left at its `null` initial value. `DynamicUnionResolver.cs:352-356` does the same thing in IL, guarded by `IsZeroStartSequential` (§8).

Measured, on both paths:

| Injected key | Bytes | Source generator | `DynamicUnionResolver` |
|---|---|---|---|
| `2` (one past the last arm) | `92 D2 00 00 00 02 92 A5 5A 65 62 72 61 04` | `null` | `null` |
| `99` | `92 D2 00 00 00 63 92 A5 …` | `null` | `null` |
| `-5` | `92 D2 FF FF FF FB 92 A5 …` | `null` | `null` |

And the enclosing object is unharmed — a `Holder` whose `Animal` slot carries key `99` deserializes to `EventId = 77, Animal = null`. The unknown payload is skipped as a unit, so the parent's array cursor stays aligned.

**Forward-compatibility verdict.** Adding a 20th subtype is safe for old *readers*: they see `null`, not an exception. It is not safe for old *round-trippers*: the arm is discarded, and re-serializing writes `C0`. This mirrors the asymmetry already recorded for unknown fields (`proto-to-messagepack-csharp.md` §1.6) — protobuf preserves, MessagePack-CSharp discards.

### 4.2 Malformed envelopes do throw

```csharp
// src/MessagePack.SourceGenerator/Transforms/UnionTemplate.tt:62-63
if (reader.ReadArrayHeader() != 2)
{
    throw new global::System.InvalidOperationException("Invalid Union data was detected. Type:<DataType>");
}
```

(identically at `DynamicUnionResolver.cs:339`). Measured:

| Malformation | Outcome |
|---|---|
| `93 …` — array header 3 | `MessagePackSerializationException: Failed to deserialize Probe.IAnimal value.` ← `InvalidOperationException: Invalid Union data was detected. Type:global::Probe.IAnimal` |
| `91 …` — array header 1 | same |
| `05` — a bare positive fixint | `MessagePackSerializationException` ← `MessagePackSerializationException: Unexpected msgpack code 5 (positive fixint) encountered.` |
| `92 A3 … ` — string where the key should be | ← `Unexpected msgpack code 161 (fixstr) encountered.` |
| `C0` — nil | **no throw**, returns `null` |

So the envelope *shape* is validated and the envelope *key* is not. That asymmetry is the whole forward-compatibility story in one sentence.

---

## 5. Source generator vs reflection resolver

### 5.1 Only interfaces are generation roots

```csharp
// src/MessagePack.SourceGenerator/MessagePackGenerator.cs:24-27
var unionTypes = context.SyntaxProvider.ForAttributeWithMetadataName(
    $"{AttributeNamespace}.{MessagePackUnionAttributeName}",
    predicate: static (node, _) => node is InterfaceDeclarationSyntax,
    transform: static (context, _) => (ITypeSymbol)context.TargetSymbol);
```

An **abstract class** carrying `[Union]` is never a root. It is still handled, but only transitively: `TypeCollector.CollectCore` accepts `TypeKind.Class && IsAbstract` with a `[Union]` attribute (`TypeCollector.cs:262`) when it is *reached* from a `[MessagePackObject]` type that references it. Verified — an abstract `Shape` union produces a working `ShapeFormatter` because a `ShapeHolder` references it:

```
Shape = Circle          92 D2 00 00 00 00 92 01 CB 40 00 00 00 00 00 00 00   [0,[1,2]]
ShapeHolder{Circle}     91 92 D2 00 00 00 00 92 01 CB …                      [[0,[1,2]]]
roundtrip Shape         OK → Circle Z=1 R=2
```

**Consequence for protop:** if a union base is generated as an abstract class and nothing in the compilation references it from a `[MessagePackObject]` member, no formatter is generated and serialization fails at runtime with `FormatterNotRegisteredException`. An **interface** base is always a root and has no such dependency. protop should emit interfaces.

### 5.2 The complete list of behavioural differences

| | Source generator (`UnionTemplate.tt`) | `DynamicUnionResolver` |
|---|---|---|
| Key encoding | `WriteInt32` — always `D2` + 4 bytes | `Write(int)` — compact |
| Root discovery | interfaces only; abstract classes transitively | any interface or abstract type, on demand |
| Non-abstract base | ignored; falls through to an object formatter, **silently lossy** | throws `MessagePackDynamicUnionResolverException` |
| Duplicate key | generator crash (`CS8785`) or runtime `ArgumentException` | `MessagePackDynamicUnionResolverException: Same union key has found.` (`:119`) |
| Duplicate subtype | runtime `ArgumentException` on `RuntimeTypeHandle` | `MessagePackDynamicUnionResolverException: Same union subType has found.` (`:124`) |
| Key dispatch | always a `Dictionary<int,int>` lookup (`UnionTemplate.tt:70`) | jump table when `IsZeroStartSequential`, else dictionary (`:352`) |
| Depth accounting | calls `options.Security.DepthStep(ref reader)` (`:67`) | **does not** |
| AOT / trimming | works | requires `Reflection.Emit` |

### 5.3 The depth difference is observable

`MessagePackSecurity.MaximumObjectGraphDepth` defaults to `500` (`MessagePackSecurity.cs:87`). Because the AOT union formatter steps depth and the reflection one does not, **a union envelope consumes a graph-depth level on the AOT path only**. Measured with the limit forced to 2 and a union-inside-a-union-arm payload:

```
source generator:  THROW MessagePackSerializationException ← InsufficientExecutionStackException:
                   This msgpack sequence has an object graph that exceeds the maximum depth allowed of 2.
DynamicUnionResolver:  OK → OuterArm
```

At the default of 500 this is irrelevant for a 19-arm flat union; it matters only if protop ever allows deeply recursive schemas *and* a consumer lowers the limit.

---

## 6. Nesting and nullability

### 6.1 A union-typed member inside an arm: works

`IOuter` with arm `OuterArm { [Key(0)] int Tag; [Key(1)] IAnimal? Inner }`:

```
92 D2 00 00 00 00 92 09 92 D2 00 00 00 00 92 A3 52 65 78 04
                                           
json: [0,[9,[0,["Rex",4]]]]
roundtrip: Tag=9 Inner=Dog     OK
```

Envelopes nest cleanly; there is no arity or depth restriction beyond `MaximumObjectGraphDepth`.

### 6.2 An arm whose declared type is itself a union interface: does **not** work

`[Union(0, typeof(IMid))] [Union(1, typeof(TopLeaf))] interface ITop`, where `IMid` is itself `[Union(…)]`:

```
ITop = MidLeaf (an IMid implementor)   C0     null      ← silently nil
ITop = TopLeaf (an exact arm type)     92 D2 00 00 00 01 91 03    [1,[3]]
roundtrip ITop = MidLeaf               OK → null
```

Because dispatch is `value.GetType().TypeHandle` against a dictionary of registered arm types, and the runtime type is `MidLeaf`, never `IMid`, nothing matches and the nil branch runs. **A union arm must be a concrete type.** Union hierarchies cannot be composed.

### 6.3 Nullability is fully representable

```
Holder with a null Animal    92 03 C0     → Animal is null
Serialize<IAnimal>(null)     C0           → null
```

A `IBaseLiveData?` member is therefore exactly representable: `C0` on the wire, `null` in C#, and there is no ambiguity with any arm encoding (every arm is a 2-element array).

---

## 7. Constraints on subtypes

### 7.1 Arms must be `[MessagePackObject]`

Verified — an arm without the attribute is a **build error**, not a runtime failure:

```
error MsgPack003: Type must be marked with MessagePackObjectAttribute: global::Neg.PlainArm
```

(`MsgPack00xMessagePackAnalyzer.cs:125-133`.) This is the one constraint the toolchain enforces well.

### 7.2 An arm must actually implement the base — but the diagnostic is bad

A `[Union(0, typeof(Unrelated))]` where `Unrelated` does not implement the base produces no analyzer diagnostic. Instead the *generated* file fails to compile:

```
…IBaseFormatter.g.cs(37,130): error CS0030: Cannot convert type 'Neg.IBase' to 'Neg.Unrelated'
…IBaseFormatter.g.cs(73,15):  error CS0030: Cannot convert type 'Neg.Unrelated' to 'Neg.IBase'
```

Loud, but the error points at machine-generated source rather than at the declaration. praguec should reject this in its own front end.

### 7.3 Arms need not be `sealed` — but they should be

Not required, and not checked. But because dispatch is by exact runtime type, a subclass of a registered arm is **not** a registered arm:

```
IOpen = OpenArm (the exact registered arm)      92 D2 00 00 00 00 91 01     [0,[1]]
IOpen = DerivedOpenArm (a subclass of it)       C0                          null
```

One byte of nil, no error, on both resolver paths. The same applies to an interface implementor that was simply never listed (`IAnimal = Ghost` → `C0`). **Sealing every arm converts a silent runtime data-loss bug into a compile error at the point where someone tries to extend one.**

### 7.4 A struct may be an arm

Not a union *base* (§2.1, `CS0592`), but a perfectly legal arm:

```
IMixed = StructArm { V = 7 }          92 D2 00 00 00 00 91 07     [0,[7]]
roundtrip                             OK → StructArm V=7
(DynamicUnionResolver) 92 00 91 07    OK → StructArm V=7
```

The reflection resolver handles this with explicit `Box`/`Unbox_Any` in the generated IL. Note that a struct arm reintroduces the hazard already recorded for the struct tier (`proto-to-messagepack-csharp.md` §8): `default(T)` is indistinguishable from a legitimately-zero value — but it is at least distinguishable from *absence*, because absence is `C0` at the envelope level.

---

## 8. Sparse, negative and large union keys

Measured against a four-arm union with keys `-1`, `5`, `1000` and `int.MaxValue`:

| Key | Source generator | `DynamicUnionResolver` |
|---|---|---|
| `-1` | `92 D2 FF FF FF FF 91 09` (8 bytes) | — |
| `5` | `92 D2 00 00 00 05 91 09` (8 bytes) | — |
| `1000` | `92 D2 00 00 03 E8 91 09` (8 bytes) | `92 CD 03 E8 91 09` (6 bytes) |
| `int.MaxValue` | `92 D2 7F FF FF FF 91 09` (8 bytes) | — |

On the AOT path the cost of a union key is **constant at 5 bytes regardless of its value**. On the reflection path it is the compact-encoding cost: 1 byte for `0..127`, 2 for `int8`/`uint8` range, 3 for `uint16`, 5 above that. Negative keys are legal on both.

The only structural cost of non-contiguity is a dispatch-strategy switch, and only on the reflection path:

```csharp
// src/MessagePack/Resolvers/DynamicUnionResolver.cs:352
if (!IsZeroStartSequential(infos))
{
    // … Dictionary<int,int> lookup instead of a direct switch jump table
}
```

`IsZeroStartSequential` is at `:405`. The source generator does not have this optimisation at all — it always performs the `keyToJumpMap.TryGetValue` lookup (`UnionTemplate.tt:70`). So on the path that actually ships, **there is no cost difference whatsoever between dense and sparse union keys**, in bytes or in dispatch.

This is the single most important asymmetry with `[Key(N)]`, and it is worth stating as a rule: *bound the member-key span, but place no constraint at all on union keys.*

---

## 9. The failure-mode inventory

Every negative case run, with the exact diagnostic. This is the checklist praguec's front end must implement, because the target enforces only two of the eight.

| # | Declaration defect | When it surfaces | Diagnostic |
|---|---|---|---|
| 1 | Arm lacks `[MessagePackObject]` | build | `error MsgPack003: Type must be marked with MessagePackObjectAttribute: global::Neg.PlainArm` |
| 2 | Two arms share a union key | build **or** first use | `CSC : warning CS8785: Generator 'MessagePackGenerator' failed to generate source … 'An item with the same key has already been added. Key: 0'`, or at runtime `MessagePackSerializationException` ← `TypeInitializationException` ← `ArgumentException: An item with the same key has already been added. Key: 0` |
| 3 | `[Union]` on a concrete class | **never (AOT)** | AOT: silently emits a base-only object formatter — `Serialize<ConcreteBase>(derived)` → `91 01`, derived members lost. Reflection: `MessagePackDynamicUnionResolverException: Union can only be interface or abstract class. Type:ConcreteBase` |
| 4 | `[Union]` on the derived type instead of the base | first use | `FormatterNotRegisteredException: Neg.IBase is not registered in resolver: MessagePack.Resolvers.StandardResolver` |
| 5 | Arm does not implement the base | build, in generated source | `error CS0030: Cannot convert type 'Neg.IBase' to 'Neg.Unrelated'` in `…IBaseFormatter.g.cs` |
| 6 | `[Union]` on a struct | build | `error CS0592: Attribute 'Union' is not valid on this declaration type. It is only valid on 'class, interface' declarations.` |
| 7 | Same subtype under two keys | first use | `ArgumentException: An item with the same key has already been added. Key: System.RuntimeTypeHandle` |
| 8 | **An implementor is simply not listed** | **never** | `C0`. Total silent data loss on write. |

Row 8 has no diagnostic at any layer and is the one that will actually bite a hand-maintained 19-arm union.

---

## 10. What this means for protop

Issue #9 puts three shapes against each other for `LiveObjectCacheItem[Key(1)] IBaseLiveData?` and its 19 subtypes. The mechanics above decide between them more cleanly than expected.

### 10.1 Shape A — a 19-arm `oneof` — is the wrong translation

A protobuf `oneof` is *n* fields inlined in the parent message, sharing the parent's field-number space and a single presence discriminator. The target construct is a *substitutable member of an interface type*, whose discriminator lives in its own independent number space one array level down. Three concrete mismatches:

1. **Number-space entanglement.** A `oneof` arm's number is a parent field number, reserved forever against reuse. A union key is not a field number of anything. Mapping 19 oneof arms onto 19 union keys means either burning 19 parent field numbers that never appear in the parent's array (`proto-to-messagepack-csharp.md` §7.2: the layout is padded to `maxKey + 1`, so a 19-arm oneof in a 2-field message would be catastrophic if the arms were taken literally as `[Key]`s), or inventing a second numbering that is *derived from but not equal to* the field numbers — at which point the `oneof` is decoration, not source of truth.
2. **The C# shape is wrong.** A `oneof` generates a discriminated set of properties plus a `Case` enum. MessagePack needs `IBaseLiveData? LiveObject` — one property, interface-typed, dispatching on runtime type. Any generator would have to *reinterpret* the oneof rather than translate it, which means the `.proto` no longer reads as what it produces.
3. **It does not survive a 20th subtype well.** A new oneof arm is a new parent field number; combined with the union-key derivation, adding an arm perturbs whichever numbering is derived. Union keys have no such coupling — §8 shows a 20th key can be literally anything, including `1000`, at zero cost.

`oneof` remains the right construct for genuine *inline alternatives within one message*. It is not the right construct for a closed polymorphic type.

### 10.2 Shape C — refusal — is coherent but concedes the wrong thing

Refusing polymorphism keeps `LiveObjectCacheItem` hand-written and lets praguec generate the other eight corpus types. The argument for it is row 8 of §9: an unlisted implementor is silent, total data loss, and a schema language that cannot prevent that is not obviously better than a hand-written file.

But that argument cuts the other way. **A hand-maintained 19-arm union is precisely the artefact most exposed to row 8**, and praguec is the only thing that could make the arm list exhaustive by construction. Refusing leaves the single riskiest type in the corpus ungoverned while governing the eight safe ones. Shape C is the right *fallback* if shape B cannot be specified in phase 1, not the right answer on the merits.

### 10.3 Shape B — a closed-type construct — maps 1:1 and is what the evidence supports

The target's semantics are: *a named base type, closed over an explicitly-keyed set of concrete implementors, where the keys are an independent int32 space with no density or ordering requirement.* That is a schema construct with no protobuf equivalent, and inventing one is cheaper than bending `oneof` into it — the same reasoning that already settled `(protop.decimal)` rather than `google.type.Decimal` (`proto-to-messagepack-csharp.md` §2.1).

Sketch (illustrative; the syntax is not the finding):

```proto
message BaseLiveData {
  option (protop.closed) = {
    implementor: { key: 0, message: "RockLiveData" }
    implementor: { key: 1, message: "JazzLiveData" }
    // … 17 more
  };
}
```

emitting

```csharp
[Union(0, typeof(RockLiveData))]
[Union(1, typeof(JazzLiveData))]
// … 17 more
public partial interface IBaseLiveData { }
```

### 10.4 What praguec must enforce, because the target does not

Directly from §9 and §7:

1. **Union keys unique within a closed type.** The target's failure is a generator crash or a `TypeInitializationException` at first use (§9 row 2). Non-negotiable; check it in the front end.
2. **Each implementor listed exactly once.** Same failure class (§9 row 7).
3. **The listed set is exhaustive** — every type that implements the base in the emitted assembly appears in the list. This is the only defence against row 8, and it is the whole value proposition over a hand-written file.
4. **Every arm `sealed`.** §7.3: a subclass of an arm serializes to nil silently. Sealing converts that into a compile error at the extension site.
5. **The base emits as an `interface`, not an abstract class.** §5.1: only interfaces are generator roots; an abstract-class base whose formatter is never transitively reached fails at runtime.
6. **Keys are frozen at first publication, like field numbers.** They are wire identity. A `reserved`-equivalent for retired union keys follows for free from the same reasoning.
7. **Arms may not themselves be closed types.** §6.2: a union arm whose declared type is another union interface silently serialises to nil. The construct is exactly one level deep.

Note that (1), (2), (4) and (7) are all things the target will happily compile and then fail on. The compiler's whole job here is to be the layer that says no.

### 10.5 The 20th subtype

Adding an arm is a **key-space append**: pick the next unused key, add an `implementor` entry, ship. Because union keys are sparse-tolerant and constant-cost (§8), the key need not be `19` and need not be contiguous — retiring key `7` and adding key `20` is free.

What old binaries do with it: **read it as `null`, silently, with the enclosing record intact** (§4.1). No crash, no exception, no log. Whether that is acceptable is a domain decision, not a mechanics one — for a *cache* item, "the live object is absent" may degrade acceptably; for anything with a write-back path it is silent destruction, because an old reader that re-serializes writes `C0`.

Two mitigations exist and both belong in the phase-1 decision, not later:
- A reserved **catch-all arm** at a fixed key carrying `bytes`, so an unknown arm can at least be preserved through a round trip. This must exist from day one; it cannot be retrofitted.
- A **schema-version field** on the parent, so a reader can detect that it is out of date rather than inferring absence.

This is open question #2 in the table at the top.

### 10.6 Two byte-level facts protop must not forget

- **Union payloads are not canonical across resolvers** (§1.2). 12 bytes source-generated, 8 bytes via reflection, for the same object. Any protop invariant expressed in bytes — a golden-file test, a content hash, a dedupe key — must pin the resolver, and the differential-test oracle (`proto-to-messagepack-csharp.md` §10) must compare *decoded values*, not bytes, wherever a union is in play.
- **The envelope is additive and the payload is untouched** (§1.1). An arm message's own layout, `[Key]` numbering and view-compatibility story are entirely unaffected by being an arm. So the existing view design (preserve the parent's tags, skip the rest) composes with unions for free.

---

## Unverified claims

Six things were not checked against a primary source and are flagged here rather than asserted above:

1. The `UnionAttribute(int key, string subType)` overload (`Attributes.cs:112`) was not exercised. The AOT analyzer's `AotUnionAttributeRequiresTypeArg` suggests it is reflection-only, but that was not tested. **unverified**
2. Generic types as union arms were not tested. **unverified**
3. Behaviour under `MessagePackSerializer.Typeless` / `TypelessFormatter` (a different polymorphism mechanism entirely) was not examined. **unverified**
4. Whether a union key collides usefully with `MessagePackObject(keyAsPropertyName: true)` string-keyed arms was not tested; all probes used int keys. **unverified**
5. The exact count of arms at which the generator's `switch` degrades (if at all) was not measured; probes used at most four arms, not nineteen. **unverified**
6. Cross-assembly behaviour — a base in assembly A with arms in assembly B, where B's arms are listed on A's declaration — was reasoned about from `partial`'s language rules but not built. **unverified**
