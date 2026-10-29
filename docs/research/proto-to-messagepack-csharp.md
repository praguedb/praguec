# protop: protobuf as a schema language for C# / MessagePack

Primary-source research. Every claim below cites the source that owns it — a spec section, an official doc page, or a file and symbol in real source code. Claims that could not be verified against a primary source are labelled **unverified** in place; there are four of them.

Scope note: this document assumes the settled architecture — **`praguec` compiles protop directly to C#, in one step. protoc is not in the pipeline.** Material that only bore on the rejected alternative has been removed rather than retained as background.

---

## Bottom line

**1. The architecture is settled and the research supports it.** Under a direct compiler, the two things that make a protoc-based pipeline painful both evaporate: protop never has to allocate extension field numbers defensively (it resolves `(protop.entity)` by name in its own symbol table, §4.1), and it never has to re-parse custom options back out of a serialized descriptor with a bootstrapped `ExtensionRegistry`. protoc's remaining role is as a **differential-test oracle**, and that role is important enough to build on day one (§10).

**2. Protobuf is a genuinely capable schema language, with exactly two holes that matter to this domain.** It has field numbers as stable identity, an evolution model that maps cleanly onto database migrations, and — in Editions 2023+, contrary to the usual proto3 framing — a real required/optional axis via `features.field_presence`, including `LEGACY_REQUIRED` on message fields (§1.3). What it does not have: **no decimal**, and **no way to say "this string is a foreign key to that message"**. Those two gaps are the entire justification for a metadata vocabulary, and they are not incidental — they are precisely the two things a betting/wallet domain needs most.

**3. Decimal is the load-bearing case, and the answer is a scaled integer with a schema-declared scale** — `sint64` plus `(protop.decimal) { scale: 4 }` — not `google.type.Money` (currency-coupled, unit/nano-split, cannot express a fee rate) and not `google.type.Decimal` (a string; every arithmetic operation is a parse). The official protobuf guidance is "use a string", which is an admission rather than a design (§2.1). A schema-declared scale lets protop do six things protoc cannot, including reject `double` for money outright and check that two fields being compared share a scale (§2.6).

**4. Field presence is the nullable / NOT NULL axis, and it is the thesis of the project.** `optional` on a proto3 field creates a *synthetic oneof*, which is invisible in the source and visible in the descriptor — a trap for any tool that walks `oneof_decl` (§1.4). Getting presence right is what turns a proto file into a schema rather than a message format, and it is what makes the generated C# record's nullability annotations mean something.

**5. MessagePack-CSharp's int-key layout is a dense array padded to `maxKey + 1`, and the AOT generator emits one literal `writer.WriteNil()` per gap** (§7.2). So sparse keys degrade catastrophically, not gracefully. The rule that falls out: **bound the key *span*; never require density.** A density lint rule would be the single worst phase-one mistake available, because it would make tag-preserving views (§12) illegal by construction.

**6. Deserialization is tolerant in both directions** — extra array slots are skipped, missing ones are left defaulted (§7.3). That is what makes a view wire-compatible with its parent for free, and it is why the settled view design (keep the parent's tags, skip the dropped ones) is correct. One asymmetry to know: protobuf *preserves* unknown fields on round-trip; MessagePack-CSharp **discards** them, so a protop record is a lossy projection of its wire form (§1.6).

**7. Constructor deserialization matches parameters by ordinal over key-sorted members** (§7.5). Therefore a generated positional record's parameter order is the tag order whether or not the author wrote them that way — which is the mechanical justification for the `FIELD_TAGS_ASCENDING` rule, and the reason that rule belongs in phase 1 rather than phase 2 (§14).

**8. There is no meaningful direct prior art for proto → MessagePack codegen.** There is excellent prior art for *custom options as schema metadata* — protoc-gen-validate/protovalidate, protoc-gen-gorm, entproto, and Google's own `field_behavior`/`resource` (§3) — and the most valuable finding in it is a negative one: the options file itself **must be proto2** (§3.2), and the biggest lesson is PGV → protovalidate, which broke its vocabulary deliberately and shipped an automated rewriter to do it (§3.6).

**9. buf has no field-presence rule and no field-number rule of any kind, and `buf breaking` does not look at custom options at all** (§9). That is a verified gap, not an oversight protop should be shy about occupying — but it also means `buf breaking` cannot detect a `csharp_kind` flip from `RECORD` to `STRUCT`, which is a wire-and-API break. protop must check that itself.

**10. The struct tier pays off twice, but unevenly.** FlatBuffers' `struct`/`table` split is the right model and the right vocabulary to borrow. In C# the payoff is substantial (a real value type, no allocation, fixed layout). In MessagePack the payoff is a **density guarantee, not a byte saving** — there is no memcpy path, unlike FlatSharp's `fs_valueStruct`/`MemoryMarshal.Cast` (§8). The hazard has no fix: `default(T)` is all-zero and indistinguishable from a legitimately-zero value, which forces the rule that a protop struct may contain only scalars, enums and other structs, and that `csharp_kind` is immutable once published.

**11. Writing the parser is realistic but not small: 6,000–10,000 lines of C#** for a strict, messages-only dialect, against ≈27,000 Go for `protocompile` and ≈8,000 C# for `protobuf-net.Reflection` (§10). The hard parts are name resolution, option interpretation, and the fact that the message-literal grammar is not context-free without resolved types. **The risk that actually materialises is feature lag, not parser size** — which is why the frozen-subset document in phase 0 matters.

**12. Implement in C#** (§11). Go's one decisive advantage — `protogen` + `dynamicpb` for re-linking unknown extensions — is exactly the thing a direct compiler removes the need for, since protop's option surface is closed and it owns its own parse. Distribution decides it: Grpc.Tools 2.83.0 ships 23,904,464 bytes of which 24,576 is managed code — 99.8% native payload across six platform folders, paid twice for a plugin. A managed toolchain ships in half a megabyte and `protobuf-net.BuildTools` is the existence proof that a Roslyn source generator can consume `.proto` via `AdditionalFiles`.

**13. You should not just use TypeSpec** (§13), and the reason is structural rather than aesthetic: `@typespec/protobuf` **cannot emit a custom option at all** — the `options` field on its field AST node is dead code, never written. It also cannot emit `oneof`, errors on `decimal`, and has no `.proto` parser anywhere in the monorepo, so existing files cannot be onboarded. Tier 1 is unreachable. Its ~600-line `.proto` text emitter is nonetheless the best prior art to read if a `.proto` emitter is ever wanted.

**14. Views are deferred but already constrain phase one.** Settled: a view preserves its parent's tags and skips the rest, so wire compatibility is free. Two phase-one recommendations exist solely to protect that (no density rule; `MAX_TAG_BOUND` bounds the span instead), and views are the **first construct where Tier-2 syntax genuinely pays for itself** — because `struct` merely *annotates* a message that exists, while a view *generates a type that does not* (§12).

**Open questions requiring your decision**, all collected in one place:

| # | Question | Where | Reversibility |
|---|---|---|---|
| 1 | `Key = tag` or `Key = tag - 1`? | §7.2 | **None** once a payload is persisted. Decide before phase 1 ships. |
| 2 | Should the lint engine and `(protop.decimal)` move into phase 1? | §14 | Cheap now; expensive later — lint rules added late invalidate existing files. |
| 3 | Does a derived table name pluralise? | §5.5 | Cheap only until the first migration is generated. No primary source can settle it; the lower-risk default is no. |
| 4 | Which protobuf subset is frozen as "what protop accepts"? | §14 | Widening later is free; narrowing is not. Write the list before the parser, not after. |
| 5 | Does the struct tier ship at all? | §8 | The payoff is real in C# and marginal on the wire, and `csharp_kind` is immutable once published. Deferring costs nothing. |

---

## 1. What protobuf can and cannot express as a schema language

protop's premise is that a `.proto` file can carry a domain/data schema — entities, relationships, keys, constraints. This section is the honest inventory of what the base language gives you for free, what it actively forbids, and what has to be invented on top. Everything here was checked against the language spec and, where the docs are silent or ambiguous, against **protoc 36.1** directly; compiler output is marked `[protoc 36.1]`.

### 1.1 What the language has

**Fifteen scalar types**, and that is the entire closed set: `double`, `float`, `int32`, `int64`, `uint32`, `uint64`, `sint32`, `sint64`, `fixed32`, `fixed64`, `sfixed32`, `sfixed64`, `bool`, `string`, `bytes` (<https://protobuf.dev/programming-guides/proto3/#scalar>). Plus: nested messages, enums, `repeated`, `map`, `oneof`, `reserved`, packages, and custom options.

C# mapping is fixed and documented (<https://learn.microsoft.com/en-us/aspnet/core/grpc/protobuf>): `double→double, float→float, int32/sint32/sfixed32→int, int64/sint64/sfixed64→long, uint32/fixed32→uint, uint64/fixed64→ulong, bool→bool, string→string, bytes→ByteString`. The same page states the constraint that bites a nullable-column mapping: *"Scalar values always have a default value and can't be set to `null`. This constraint includes `string` and `ByteString` which are C# classes."*

**Enums must start at zero and are open by default.** `[protoc 36.1]` on `enum E { E_ONE = 1; }` under proto3: `The first enum value must be zero for open enums.` The wording is deliberate — Editions makes openness configurable (`features.enum_type = OPEN | CLOSED`), and `descriptor.proto` records `edition_defaults = { edition: EDITION_PROTO3, value: "OPEN" }`. **Consequence for a domain schema: an open enum field can legally hold a value your schema has never heard of.** A `BetStatus` read off the wire may be `7` when the generated record only knows `0..5`. A generated C# `enum` will happily hold it.

**`oneof` is a labelled sum type over fields, not a union over types.** It discriminates on field number, so two members may share a type — `[protoc 36.1]` accepts `oneof v { int64 a = 1; int64 b = 2; }` without complaint. Restrictions, quoted from <https://protobuf.dev/programming-guides/proto3/#oneof>: *"You can add fields of any type, except `map` fields and `repeated` fields"*; *"A oneof cannot be `repeated`"*; *"if multiple values are set, the last set value as determined by the order in the proto will overwrite all previous ones"*. Verified: `oneof v { repeated int64 a = 1; }` → `Fields in oneofs must not have labels (required / optional / repeated).`; `oneof v { map<string,int64> a = 1; }` → `Map fields are not allowed in oneofs.`

This is genuinely useful for protop — a `oneof` maps cleanly onto a closed C# hierarchy or a discriminated-union-shaped record, driven by the generated `XOneofCase` enum. But note the documented evolution hazard: *"If checking the value of a oneof returns `None`/`NOT_SET`, it could mean that the oneof has not been set or it has been set to a field in a different version of the oneof. There is no way to tell the difference."*

**`map` key types are restricted.** Grammar (<https://protobuf.dev/reference/protobuf/proto3-spec/#map_field>) permits only `int32, int64, uint32, uint64, sint32, sint64, fixed32, fixed64, sfixed32, sfixed64, bool, string`. `[protoc 36.1]` on `map<double, string>`: `Key in map fields cannot be float/double, bytes or message types.` **So `map<CurrencyCode, Money>` with an enum key is illegal**; you get `map<string, Money>` and lose key-type safety. Also documented: *"Map fields cannot be `repeated`"*, *"Wire format ordering and map iteration ordering of map values is undefined"*, *"if there are duplicate map keys the last key seen is used"*.

### 1.2 What the language does not have

| Capability | Status | Evidence |
|---|---|---|
| Inheritance / type extension | **Absent** | `messageBody = "{" { field \| enum \| message \| option \| oneof \| mapField \| reserved \| emptyStatement } "}"` — no extension clause (<https://protobuf.dev/reference/protobuf/proto3-spec/#message_definition>) |
| Generics / type parameters | **Absent** | `type = "double" \| … \| messageType \| enumType` — a closed set with no parameterization |
| Sum type over scalars | `oneof` is close but field-discriminated, not type-discriminated | §1.1 |
| Field-level constraints / validation | **Absent from the language** | No constraint production in the grammar. The only validation feature anywhere is `features.utf8_validation`, which is about string encoding. Every constraint system in existence (protovalidate, PGV) is third-party custom options. |
| Non-null guarantee on a message field | **Absent in proto3**; expressible but unenforced in Editions | §1.3 |
| UUID | **Absent** | Not a scalar; not in the WKT index (<https://protobuf.dev/reference/protobuf/google.protobuf/>); **no `uuid.proto` in `google/type/`** (directory verified) |
| Date/time | Only `Timestamp` + `Duration` in core | `google/type/` (googleapis, *not* core protobuf) adds `date`, `datetime`, `timeofday`, `interval`, `dayofweek`, `month`, `calendar_period` |
| Decimal | **Absent** | §2 |
| Explicit default values | **Absent in proto3; present in Editions** | `[protoc 36.1]` proto3: `Explicit default values are not allowed in proto3.` Edition 2023 `int32 x = 1 [default = 5];` → accepted |
| Primary key, foreign key, uniqueness, index, cross-field invariant | **Absent** | Not in the grammar at any production |

Everything in the bottom half of that table is protop's reason to exist. Note that **none of it requires new syntax** — all of it is expressible as custom options, which is the Tier-1 claim.

### 1.3 Non-null message fields: a correction to the usual framing

The received wisdom is "proto3 message fields are always optional, so you cannot say a relation is mandatory". That is **true for proto3 syntax and false for Editions 2023+.**

Proto3 is unambiguous:

> In proto3, message-type fields already have field presence. Because of this, adding the `optional` modifier doesn't change the field presence for the field.
> — <https://protobuf.dev/programming-guides/proto3/#field-presence>

and `required` is prohibited — `descriptor.proto` on `LABEL_REQUIRED`: *"The required label is only allowed in google.protobuf. In proto3 and Editions it's explicitly prohibited. In Editions, the `field_presence` feature can be used to get this behavior."* `[protoc 36.1]` confirms: `Required fields are not allowed in proto3.`

But that last sentence is the door. Under Edition 2023, `[protoc 36.1]` gives:

| Input (edition 2023) | Result |
|---|---|
| `M1 a = 1 [features.field_presence = LEGACY_REQUIRED];` | **accepted**; descriptor carries `options { features { field_presence: LEGACY_REQUIRED } }` on a `TYPE_MESSAGE` field |
| `M1 a = 1 [features.field_presence = IMPLICIT];` | `Message fields can't specify implicit presence.` |

**However, the C# backend does not enforce it.** `protoc --csharp_out` on that exact schema emits a plain nullable property with no hasser, no `IsInitialized`, no guard:

```csharp
public const int AFieldNumber = 1;
private global::T3.M1 a_;
public global::T3.M1 A { get { return a_; } set { a_ = value; } }
```

**Net for protop:** "mandatory relation" is expressible as descriptor metadata under Editions and is runtime-unenforced in stock C#. protop should read `field.options.features.field_presence == LEGACY_REQUIRED` and emit the non-nullable property, the required constructor parameter, and the guard itself. This is a concrete, immediate win that does not need a custom option at all — it uses a first-class protobuf feature that the reference C# generator ignores. *(Whether C++/Java enforce it at runtime is unverified; only the C# backend was checked.)*

### 1.4 Presence as a schema feature — this is the nullable/NOT NULL axis

The premise in the brief is correct and worth stating sharply. From the proto3 guide on implicit fields:

> the field is set to the default (zero) value. It will not be serialized to the wire. **In fact, you cannot determine whether the default (zero) value was set or parsed from the wire or not provided at all.**
> — <https://protobuf.dev/programming-guides/proto3/#field-labels>

and from the presence app note:

> "Empty" length-delimited values (such as empty strings) can be validly represented in serialized values: the field is "present," in the sense that it appears in the wire format. However, **if the generated API does not track presence, then these values may not be re-serialized; i.e., the empty field may be "not present" after a serialization round-trip.**
> — <https://protobuf.dev/programming-guides/field_presence/>

The presence table for proto3:

| Field type | `optional` | Explicit presence |
|---|---|---|
| Singular numeric | No / Yes | — / ✔️ |
| Singular enum | No / Yes | — / ✔️ |
| Singular string or bytes | No / Yes | — / ✔️ |
| Singular message | No / Yes | ✔️ / ✔️ |
| Repeated | N/A | — |
| Oneofs | N/A | ✔️ |
| Maps | N/A | — |

protobuf.dev itself recommends what protop should mandate:

> We recommend always adding the `optional` label for proto3 basic types. This provides a smoother path to editions, which uses explicit presence by default.

**Recommendation, and it is a strong one: yes — `optional string table = 1;` becomes C# `string?` and a nullable column; bare `string table = 1;` becomes non-nullable `string` with a NOT NULL column and a default of `""`.** This is the natural reading, it matches what the wire actually distinguishes, and it makes `optional` load-bearing rather than decorative.

Two caveats to write into the design:

- **`repeated` and `map` can never track presence, in any dialect.** "Empty list" and "no list" are permanently indistinguishable. A nullable collection column is not expressible; don't pretend otherwise.
- The mapping means a user who omits `optional` everywhere (the common proto3 habit) gets an all-NOT-NULL schema. That is a *feature* if diagnosed loudly and a trap if silent. See the strictness rules in §9.

**Descriptor mechanics protop must handle.** `optional` in proto3 is implemented as a synthetic oneof. From `descriptor.proto` field 17:

```proto
  // If true, this is a proto3 "optional". When a proto3 field is optional, it
  // tracks presence regardless of field type.
  //
  // When proto3_optional is true, this field must belong to a oneof ... This oneof
  // is known as a "synthetic" oneof, and this field must be its sole member
  // ... Synthetic oneofs exist in the descriptor only, and do not generate any API.
  // Synthetic oneofs must be ordered after all "real" oneofs.
  optional bool proto3_optional = 17;
```

`[protoc 36.1]` on `message P { string table = 1; optional string opt_table = 2; }`:

```
field { name: "table"     number: 1 label: LABEL_OPTIONAL type: TYPE_STRING }
field { name: "opt_table" number: 2 label: LABEL_OPTIONAL type: TYPE_STRING
        oneof_index: 0 proto3_optional: true }
oneof_decl { name: "_opt_table" }
```

**If protop enumerates `oneof_decl` naively, every `optional` scalar masquerades as a discriminated union.** A synthetic oneof is identified by having exactly one member whose `proto3_optional` is true (conventionally named `_<field>`). This is a real bug waiting to happen and it costs one line to avoid. It applies equally to protop's own parser: the parser must *produce* the same model, and must not conflate the two.

**Editions flips the default the right way.** `descriptor.proto`:

```proto
  optional FieldPresence field_presence = 1 [
    retention = RETENTION_RUNTIME,
    targets = TARGET_TYPE_FIELD, targets = TARGET_TYPE_FILE,
    feature_support = { edition_introduced: EDITION_2023 },
    edition_defaults = { edition: EDITION_LEGACY, value: "EXPLICIT" },
    edition_defaults = { edition: EDITION_PROTO3, value: "IMPLICIT" },
    edition_defaults = { edition: EDITION_2023,   value: "EXPLICIT" }
  ];
```

Editions 2023/2024/2026 all default to `EXPLICIT`. **This is the strongest argument for protop targeting Editions rather than proto3 as its recommended dialect**: the sane nullability default flips, `LEGACY_REQUIRED` becomes available, and `[default = …]` comes back. It is also the more conservative choice for a tool being written in 2026. *(Open question for the user — see the end.)*

### 1.5 Evolution: field numbers, `reserved`, and how it maps to migrations

Quoted from <https://protobuf.dev/programming-guides/proto3/#assigning>:

> You must give each field ... a number between **`1` and `536,870,911`** ... Field numbers **`19,000` to `19,999` are reserved** ... This number **cannot be changed once your message type is in use** ... Field numbers **should never be reused**.

> field numbers in the range 1 through 15 take one byte to encode. Field numbers in the range 16 through 2047 take two bytes.

`[protoc 36.1]` confirms both limits: `19000` → `Field numbers 19000 through 19999 are reserved for the protocol buffer library implementation.`; `536870912` → `Field numbers cannot be greater than 536870911.`

The consequences of reuse are stated in unusually blunt terms for a spec, and are worth repeating verbatim in protop's diagnostics:

> Encoding a field using one definition and then decoding that same field with a different definition can lead to: Developer time lost to debugging; A parse/merge error (best case scenario); **Leaked PII/SPII**; **Data corruption**.
>
> Common causes of field number reuse: **renumbering fields** (sometimes done to achieve a more aesthetically pleasing number order for fields) ... **deleting a field and not reserving the number**.

`reserved` protects two different things, which is why both forms exist:

> you **must** reserve the deleted field number ... You should also **reserve the field name** to allow JSON and TextFormat encodings of your message to continue to parse.
>
> **Reserved names affect only the protoc compiler behavior and not runtime behavior** ... Note that you **can't mix field names and field numbers in the same `reserved` statement.**

So: **reserved numbers protect the binary wire format; reserved names protect JSON/TextFormat.** protop should emit and require both, and should refuse to compile a schema that reuses either.

**Mapping onto database migrations.** The analogy is exact and useful:

| Protobuf concept | Database analogue |
|---|---|
| field number | column identity (the thing a migration must preserve) |
| field name | column name (renameable; only affects JSON/TextFormat and generated code) |
| `reserved 7;` | a dropped column whose ordinal must never be reused |
| `reserved "old_name";` | a dropped column name that must not come back with different semantics |
| adding a field | `ALTER TABLE ADD COLUMN ... NULL` (always safe) |
| removing a field | dropping a column — safe on the wire, breaks readers' code |
| changing a field number | drop + add, i.e. **data loss**, never a rename |

The three documented change classes, condensed (<https://protobuf.dev/programming-guides/proto3/#updating>):

- **Wire-unsafe (breaking):** changing a field number; moving an existing field into an existing `oneof`.
- **Wire-safe:** adding fields; removing fields (number never reused); adding enum values; moving an explicit-presence field into a *new* `oneof`; collapsing a single-member `oneof` to a plain field; turning a field into an extension of the same number and type.
- **Wire-compatible but lossy:** `int32↔uint32↔int64↔uint64↔bool` (*"if a 64-bit number is read as an int32, it will be truncated"*); `sint32↔sint64` (*"not compatible with the other integer types"*); `string↔bytes` (if valid UTF-8); message↔`bytes`; `fixed32↔sfixed32`; singular↔`repeated` **for `string`/`bytes`/message only** (*"not generally safe for numeric types ... Repeated fields of numeric types are serialized in the packed format by default"*); `enum↔int32/…`; `map<K,V>↔repeated` entry message (*"clients using the `map` field definition may reorder entries and drop entries with duplicate keys"*).

The spec's own warning about the third class is the one protop should adopt as policy: *"If your schema is published outside of your organization, you should generally not make wire-compatible changes."*

### 1.6 Unknown fields: a direct hit on the mapper design

> Proto3 messages preserve unknown fields and include them during parsing and in the serialized output ...
>
> Some actions can cause unknown fields to be lost. For example ... **Iterate over all of the fields in a message to populate a new message.**
> — <https://protobuf.dev/programming-guides/proto3/#unknowns>

That sentence describes exactly what a proto→C#-record mapper does. Combined with the MessagePack finding in §7 — MessagePack-CSharp's int-key layout skips unknown array slots but does **not** retain them — the conclusion is unavoidable and should be stated in protop's docs rather than discovered:

**A protop-generated record is a lossy projection of the wire message. Read-modify-write through one drops any field written by a newer schema version.** This is fine for a system that owns both ends of its own schema (which is protop's stated domain) and is a serious hazard in a multi-writer pipeline. If it ever matters, the record needs an explicit "unknown bytes" member; that is a design decision, not a bug fix.

---

## 2. Decimal and money — the motivating case for the whole options mechanism

The domain is betting and wallets. `Bet`, `wallet.Account`, stakes, balances, odds. Money is the type protobuf does not have, and the gap is entirely on the protobuf side — **MessagePack-CSharp has no decimal problem at all** (see §7.7). This makes decimal the cleanest possible justification for protop's metadata layer: the information needed to generate a correct `decimal` is knowable, is not expressible in the base language, and rides perfectly on a custom option.

### 2.1 There is no decimal, and the official answer is "use a string"

The 15-scalar table has no decimal, no fixed-point type, no arbitrary-precision integer. Microsoft states it plainly (<https://learn.microsoft.com/en-us/aspnet/core/grpc/protobuf>):

> **Decimals** — Protobuf doesn't natively support the .NET `decimal` type, just `double` and `float`. There's an ongoing discussion in the Protobuf project about the possibility of adding a standard decimal type to the Well-Known Types ... Nothing has been implemented yet.

And there is no protobuf.dev page about decimal or currency at all. *(Verified by grepping the source markdown of the proto3 guide, the editions guide, the features reference, the WKT reference and the proto3 spec; the WKT index has no Decimal or Money entry. Not an exhaustive site crawl — strong evidence, not proof.)*

The maintainer position lives in an issue, not the docs. **protocolbuffers/protobuf#4406, "Decimal Type?"** (<https://github.com/protocolbuffers/protobuf/issues/4406>) — opened 2018, 161 reactions, closed 2025-12-31 as duplicate. Verbatim:

> As noted above, we have no intention of pursuing this in the near term. The language specific aspects of this make it a very large amount of effort to implement across our entire surface. **I recommend that users use a string field with appropriate parsing in their own code.**

> While you are welcome to create a PR, the odds that it is adopted are low. As mentioned, any sort of increased precision datatype needs to be handled in the full set of languages supported consistently which makes this a substantially larger amount of effort then most anticipate.

A prototype existed — PR #7847, "DO NOT MERGE: Prototype decimal message types", by JamesNK (author of the Microsoft gRPC/protobuf doc), opened 2020, closed unmerged 2021. *(Which issue #4406 was marked duplicate of could not be determined — the timeline API returned no resolvable target. Unverified.)*

### 2.2 The four options, measured

Wire sizes below were **measured** with `protoc --encode | wc -c` on protoc 36.1, not estimated. Sizes are for the field at top level; nesting a submessage adds ~2 bytes.

| | (i) `string` | (ii) scaled `sint64` | (iii) `google.type.Money` | (iv) `google.type.Decimal` |
|---|---|---|---|---|
| Precision | Unbounded | Exactly the declared scale *s*; ≤19 significant digits | **Fixed scale 9** | Unbounded |
| Range | Unbounded | ±9.22×10¹⁸ / 10ˢ (s=4 → ±922,337,203,685,477.5807) | ±9.223372036854775807×10¹⁸ + 0.999999999 | Unbounded |
| Scale visible in the schema? | ✗ | ✓ **only if you declare it** — protobuf cannot | ✓ hardcoded 9 | ✗ (explicitly pushed to prose) |
| Trailing zeros / scale preserved | ✓ | ✗ (scale fixed and implicit) | ✗ (`1.50` → `1.5`) | ✓ |
| Currency carried | ✗ | ✗ | ✓ (unvalidated `string`) | ✗ |
| **1234.56** on the wire | 9 B | **4 B** | 14 B | 11 B |
| **−1234.56** on the wire | 10 B | **4 B** (`int64` would be **11 B**) | 14 B | 12 B |
| zero / default | 0 B (implicit presence!) | 0 B (implicit presence!) | 5 B | 2 B |
| max | 32 B | 11 B | 21 B | 34 B |
| Parse cost in C# | `decimal.Parse`, allocates | **none** — integer shift, zero-alloc | one division | `decimal.Parse` + submessage alloc |
| ProtoJSON | string | **string** (int64 is emitted as a string) | object | object |

Two measured facts that are easy to get wrong:

1. **Negative scaled `int64` costs 11 bytes; `sint64` costs 4.** A wallet ledger is full of negative amounts — debits, voids, chargebacks, adjustments. The docs say it (*"Inefficient for encoding negative numbers – if your field is likely to have negative values, use sint32/sint64 instead"*) and measurement confirms: `int64 = -123456` → 11 B, `sint64 = -123456` → 4 B. **If protop generates or recommends scaled integers, the default must be `sint64`.** And the choice is irreversible: *"`sint32` and `sint64` are compatible with each other but are **not** compatible with the other integer types."*
2. **ProtoJSON emits int64 as a string anyway** (*"ProtoJSON serializers emit int64s as strings to ensure no precision loss"*, <https://protobuf.dev/programming-guides/json/#int64-strings>), so option (ii) gains nothing over (i)/(iv) in JSON — its advantage is purely binary size and parse cost.

### 2.3 `google.type.Money` — and why it is the wrong ledger type

Full definition (<https://github.com/googleapis/googleapis/blob/master/google/type/money.proto>):

```proto
message Money {
  // The three-letter currency code defined in ISO 4217.
  string currency_code = 1;
  // The whole units of the amount.
  int64 units = 2;
  // Number of nano (10^-9) units of the amount.
  // The value must be between -999,999,999 and +999,999,999 inclusive.
  // If `units` is positive, `nanos` must be positive or zero.
  // If `units` is zero, `nanos` can be positive, zero, or negative.
  // If `units` is negative, `nanos` must be negative or zero.
  // For example $-1.75 is represented as `units`=-1 and `nanos`=-750,000,000.
  int32 nanos = 3;
}
```

**The `{units, nanos}` convention is a doc comment, not a constraint.** Nothing validates the sign-consistency rule — not the wire format, not protoc, not any generated code. `units = 5, nanos = -750000000` parses cleanly and means nothing. `currency_code` is a bare implicit-presence `string`, so `""` and absent are indistinguishable, and ISO-4217 membership is unchecked.

The hard limits are visible in the only real C# binding, `Google.Api.CommonProtos/Type/MoneyPartial.cs` (googleapis/gax-dotnet), which is hand-written, not generated:

```csharp
public decimal DecimalValue
{
    get => Units + (Nanos / 1_000_000_000M);
    set
    {
        decimal integralPart = Math.Truncate(value);
        long units;
        try { units = (long)integralPart; }
        catch (OverflowException ex) { throw new ArgumentOutOfRangeException("...must be a valid System.Int64 value.", ex); }
        decimal fractionalPart = (value - integralPart) * 1_000_000_000M;
        if (fractionalPart != Math.Truncate(fractionalPart))
            throw new ArgumentOutOfRangeException("...Money type supports a maximum of 9 digits after the decimal.");
        Units = units; Nanos = (int)fractionalPart;
    }
}
```

So `Money` **throws** above 9 decimal places and above `long.MaxValue` units, and the getter reconstructs by division, losing scale. Its range ceiling is 9.22×10¹⁸ against `System.Decimal`'s 7.92×10²⁸ — **narrower than `decimal` in both directions**. Microsoft's own suggested alternative in the gRPC docs, `DecimalValue { int64 units = 1; sfixed32 nanos = 2; }`, has the identical scale-9 ceiling and the page says so: *"which isn't the full range of a `decimal`"*.

### 2.4 `google.type.Decimal` — a string with a spec attached

```proto
message Decimal {
  // The decimal value, as a string. ...
  //     DecimalString = '' | [Sign] Significand [Exponent];
  //     Sign = '+' | '-';
  //     Significand = Digits ['.'] [Digits] | [Digits] '.' Digits;
  //     Exponent = ('e' | 'E') [Sign] Digits;
  string value = 1;
}
```

The comment is long and good — it defines an EBNF, mandates `.` as the separator, forbids thousand separators, prescribes normalization (`+2.5`→`2.5`, `.5`→`0.5`, `2.5e8`→`2.5E+8`, `2.5E0`→`2.5`), and permits trailing-zero preservation. But the load-bearing sentence for a *schema* language is this one:

> Services **should** clearly document the range of supported values, the maximum supported precision (total number of digits), and, if applicable, the scale (number of digits after the decimal point) ...

**`google.type.Decimal` deliberately pushes range, precision and scale out of the schema and into per-service prose.** As a schema artifact it expresses nothing about the value domain. That is precisely the hole protop fills.

Also present and relevant to betting: `google/type/fraction.proto` — `Fraction { int64 numerator = 1; int64 denominator = 2; }`. Fractional odds (5/2) are exactly representable here where decimal odds are not always. "Must be positive" is, again, a comment. The full `google/type/` inventory is `calendar_period, color, date, datetime, dayofweek, decimal, expr, fraction, interval, latlng, localized_text, money, month, phone_number, postal_address, quaternion, timeofday` — **no `uuid`**.

### 2.5 Mapping to `System.Decimal`, precisely

`System.Decimal` is 128 bits: a 96-bit integer, a sign, and a scale 0–28, i.e. `((-2⁹⁶ to 2⁹⁶) / 10^(0 to 28))`, range ±79,228,162,514,264,337,593,543,950,335, 28–29 significant digits (<https://learn.microsoft.com/en-us/dotnet/api/system.decimal>). Critically: *"The scaling factor also preserves any trailing zeros in a `Decimal` number."*

**The round-trip string format is the default `"G"`, not `"R"`.** From the standard numeric format strings reference: for `Decimal`, `"G"` with no precision specifier means *"Smallest round-trippable number of digits to represent the number"*, and *"if the number is a `Decimal` and the precision specifier is omitted, fixed-point notation is always used and **trailing zeros are preserved**"*. Meanwhile `"R"` is *"Supported by: `Single`, `Double`, and `BigInteger`"* — **not `Decimal`**. So the correct pair is `value.ToString(CultureInfo.InvariantCulture)` / `decimal.Parse(s, NumberStyles.AllowExponent | AllowLeadingSign | AllowDecimalPoint, CultureInfo.InvariantCulture)`. That is exactly what `Google.Api.CommonProtos/Type/Decimal.cs` does:

```csharp
public static Decimal FromClrDecimal(decimal value) =>
    new Decimal { Value = value.ToString(CultureInfo.InvariantCulture) };

public decimal ToClrDecimal() =>
    decimal.Parse(Value, NumberStyles.AllowExponent | NumberStyles.AllowLeadingSign
                       | NumberStyles.AllowDecimalPoint, CultureInfo.InvariantCulture);
```

Overflow behaviour on parse: *"A `Decimal` has 29 digits of precision. If `s` represents a number that has more than 29 digits, but has a fractional part and is within the range of `MaxValue` and `MinValue`, the number is **rounded, not truncated**, to 29 digits using rounding to nearest."* So a `google.type.Decimal` carrying 35 significant digits **silently rounds** in C#, or throws `OverflowException` if out of range — and the schema gives the generator no basis on which to emit a guard. *(The gax-dotnet XML doc says "truncated towards zero" where Learn says "rounded, not truncated" — an upstream doc inconsistency, not resolved here. Unverified.)*

### 2.6 The recommendation, and why it is the argument for custom options

**For wallet and ledger amounts: scaled `sint64` with a schema-declared scale.** It is the only option that is exact, allocation-free on both sides, and cheapest on the wire including negatives. Reserve `string` / `google.type.Decimal` for values whose scale is genuinely open — odds, exchange rates, RTP. Do **not** use `google.type.Money` for the ledger.

But scaled `sint64` has one fatal property as raw protobuf: **the scale is invisible.** `sint64 balance = 3;` tells a reader, a generator, and a migration tool nothing. Worse, per §1.5, **changing the scale is wire-*compatible*** — going from scale 2 to scale 4 parses cleanly and turns every historical amount into 1/100th of its value. Protobuf offers zero protection.

This is exactly what a custom option fixes, and it is the cleanest motivating example in the whole design:

```proto
// protop/options.proto  (syntax = "proto2")
message DecimalSpec {
  optional uint32 scale     = 1;  // digits after the point; required
  optional uint32 precision = 2;  // total significant digits; informational + guard
  optional string currency_field = 3;  // name of the sibling field carrying ISO-4217
}
extend google.protobuf.FieldOptions {
  optional DecimalSpec decimal = 50001;
}
```

```proto
message Account {
  string id = 1;
  string currency = 2;
  sint64 balance = 3 [(protop.decimal) = { scale: 4 precision: 19 currency_field: "currency" }];
}
```

What protop can then do that protoc cannot:

1. **Emit `decimal Balance` instead of `long Balance`**, with the conversion generated inline and allocation-free — `new decimal(lo, mid, hi, isNegative, scale)` rather than a division. The user writes `sint64`, gets `decimal`, and the wire stays 4 bytes.
2. **Emit a range guard** derived from `precision` and `scale` — the one thing `google.type.Decimal` explicitly declines to specify.
3. **Reject `scale > 28`** at compile time, because `System.Decimal` cannot represent it.
4. **Refuse a scale change in place** across a baseline comparison, with the diagnostic that protobuf's own compatibility rules cannot produce: *"scale of `Account.balance` changed 2 → 4; this is wire-compatible and silently corrupting. Allocate a new field number."*
5. **Check that `currency_field` names a real sibling field** of type `string` — a cross-field check protoc has no vocabulary for.
6. Emit the SQL column type, `DECIMAL(19,4)`, from the same declaration.

None of that requires new syntax; the file above compiles under stock protoc unchanged. **If the options mechanism earned its place on one example, this is the example.**

---

## 3. Prior art: custom options as schema metadata

Five bodies of prior art were read at source. None of them is a proto→MessagePack generator (no such prior art exists), but all five are *exactly* protop's Tier-1 problem: hang domain metadata off protobuf declarations using custom options, then generate or validate from it.

### 3.1 The five, at a glance

| Project | Options file | Syntax | Ext. numbers | Scopes covered | Consumption |
|---|---|---|---|---|---|
| protoc-gen-validate (PGV) | `validate/validate.proto` | **proto2** | 1071 (registered), 1072 (not) | message, oneof, field | protoc plugin → generated `Validate()` |
| protovalidate | `proto/protovalidate/buf/validate/validate.proto` | **proto2** | 1159, 1160 (registered block 1157–1166) | message, oneof, field | **runtime reflection**, no codegen |
| protoc-gen-gorm | `proto/options/gorm.proto` | proto3 | 52119 on all 5 extendees (unregistered, private band) | file, message, field, service, method | Go plugin → GORM models |
| protoc-gen-ent | `entproto/cmd/protoc-gen-ent/options/ent/opts.proto` | **proto2** | 150119, 150120 (unregistered) | message, field | Go plugin → ent schema |
| googleapis | `google/api/field_behavior.proto`, `google/api/resource.proto` | proto3 | 1052, 1053, 1055 (registered block 1048–1056) | file, message, field | linters + client generators |

Sources: <https://github.com/bufbuild/protoc-gen-validate/blob/main/validate/validate.proto>, <https://github.com/bufbuild/protovalidate/blob/main/proto/protovalidate/buf/validate/validate.proto>, <https://github.com/infobloxopen/protoc-gen-gorm/blob/master/proto/options/gorm.proto>, <https://github.com/ent/contrib/blob/master/entproto/cmd/protoc-gen-ent/options/ent/opts.proto>, <https://github.com/googleapis/googleapis/blob/master/google/api/field_behavior.proto>, <https://github.com/googleapis/googleapis/blob/master/google/api/resource.proto>.

### 3.2 The single most important finding: the options file must be proto2

PGV, protovalidate and protoc-gen-ent all declare `syntax = "proto2"` and mark **every** rule field `optional`. This is not legacy drag; it is load-bearing. With explicit presence, `min_len = 0` is distinguishable from "no `min_len` stated". With proto3 implicit presence it is not, and every boolean and numeric knob in the vocabulary silently acquires a meaning for its zero value.

protoc-gen-gorm used proto3 and consequently cannot distinguish `not_null: false` from "nothing said about nullability" — its `GormTag` is a 24-field flat bag of `bool` and `string` where absence and false are the same value.

**Recommendation for protop: the `protop/options.proto` vocabulary file is `syntax = "proto2"`, every field `optional`.** This is orthogonal to what the *user's* schema files are; they can be proto3 or editions.

### 3.3 Extension-number discipline

protovalidate reuses **one number (1159) across three different extendees** (`MessageOptions`, `OneofOptions`, `FieldOptions`). Extension numbers are scoped per extendee, so this is legal and it means the whole vocabulary burns exactly one slot from the global registry regardless of how many scopes it covers.

The global registry lives at <https://github.com/protocolbuffers/protobuf/blob/main/docs/options.md>. Relevant allocations: `Protoc-gen-validate` → 1071; `Protokt (pre 1.0.0)` → 1072; `Buf` → 1157–1166; `API client generators` → 1048–1056. PGV's 1072 on `MessageOptions` is **not** registered to it — 1072 belongs to Protokt. (Whether it actually collides depends on Protokt's extendee; *unverified*.) protoc-gen-gorm's file carries a self-diagnosing comment that was never acted on:

```proto
// TODO: The option number 52119 lies within the internally reserved extension
// range. I believe a publicly unique number should be requested.
```

Official guidance (<https://protobuf.dev/programming-guides/proto2/#customoptions>): 50,000–99,999 is "reserved for internal use within individual organizations"; anything published needs a registry entry.

**Recommendation:** while protop is private, use one number in 50,000–99,999, reused across extendees. If it is ever published, request a small block. Under the settled architecture the numbers are inert for protop itself — it resolves `(protop.entity)` by name against the `extend` it parsed — so they matter only for the protoc-compatibility invariant.

### 3.4 Vocabulary shapes worth copying

**A `oneof` for mutually exclusive facets.** protoc-gen-gorm's relation options:

```proto
message GormFieldOptions {
    GormTag tag = 1;
    bool drop = 2;
    oneof association {
        HasOneOptions     has_one      = 3;
        BelongsToOptions  belongs_to   = 4;
        HasManyOptions    has_many     = 5;
        ManyToManyOptions many_to_many = 6;
    }
}
```

Cardinality cannot be two things at once, and protoc enforces it. protovalidate uses the same shape for per-type rule bags (`oneof type { FloatRules float = 1; ... }`).

**A repeated plain enum for composable facets.** `google/api/field_behavior.proto` is the cheapest useful vocabulary in existence:

```proto
extend google.protobuf.FieldOptions {
  repeated google.api.FieldBehavior field_behavior = 1052 [packed = false];
}
```

used as

```proto
google.protobuf.Timestamp expire_time = 1
  [(google.api.field_behavior) = OUTPUT_ONLY,
   (google.api.field_behavior) = IMMUTABLE];
```

`[packed = false]` is mandatory because it extends a proto2 message. Values: `OPTIONAL`, `REQUIRED`, `OUTPUT_ONLY`, `INPUT_ONLY`, `IMMUTABLE`, `UNORDERED_LIST`, `NON_EMPTY_DEFAULT`, `IDENTIFIER`. The file carries two comments worth adopting verbatim in spirit:

> Note: This enum **may** receive new values in the future.

> This **does not** change the behavior in protocol buffers itself; it only denotes the behavior and may affect how API tooling handles the field.

**Splitting definition from reference.** googleapis puts `resource` on the message (the entity definition) and `resource_reference` on the field (the pointer), each its own extension. protop's `Entity`/`Relation` split is the same idea and is right.

**Recursive rule nesting for `repeated`/`map`.** protovalidate's `RepeatedRules.items` and `MapRules.keys`/`values` are themselves `FieldRules`, with the inapplicable subset handled by documentation plus a lint rule, not by the type system:

> Note that the `required` rule does not apply. Repeated items cannot be unset.

**Tombstones on every removal.** protovalidate reserves both number and name, every time:

```proto
message FieldRules {
  ...
  reserved 24, 26;
  reserved "skipped", "ignore_empty";
}
enum Ignore {
  IGNORE_UNSPECIFIED = 0;
  IGNORE_IF_ZERO_VALUE = 1;
  IGNORE_ALWAYS = 3;
  reserved 2;
  reserved "IGNORE_EMPTY", "IGNORE_DEFAULT", "IGNORE_IF_DEFAULT_VALUE", "IGNORE_IF_UNPOPULATED";
}
```

Four reserved names for one number — that enum value was renamed three times. By contrast `google.api.ResourceDescriptor` jumps from `singular = 6` to `style = 10` with no `reserved` at all; 7–9 are simply gone and unrecoverable.

**Explicit `plural`/`singular` rather than inflection.** `ResourceDescriptor` requires both — "The plural form is required even for singleton resources" — and `name_field` with a documented default ("If omitted, this is assumed to be `name`"). An explicit escape hatch beats a convention nobody can override.

**Evolution metadata inside the vocabulary.** `ResourceDescriptor.History` records *that the shape changed and in which direction*:

```proto
enum History {
  HISTORY_UNSPECIFIED = 0;
  ORIGINALLY_SINGLE_PATTERN = 1;
  FUTURE_MULTI_PATTERN = 2;
}
```

Directly applicable to `Entity` — a way to record "this table was renamed" or "this key was originally single-column" that survives in the schema rather than in a migration folder.

**A file-level definition escape hatch.** `repeated ResourceDescriptor resource_definition = 1053` on `FileOptions` exists for "a resource definition without a corresponding message" — join tables, views, legacy tables. protop will want the equivalent.

### 3.5 What the prior art got wrong

**1. Stringly-typed cross-type references.** `google.api.ResourceReference.type` is a `string` naming another resource in a namespace (`{service}/{Kind}`) disjoint from protobuf's own. Consequences visible in the design itself: a regex and a 100-character limit written into a doc comment; a wildcard `type: "*"` because the string space has no type system to express "any"; a second field `child_type` whose only job is to invert the direction of the reference, because a string cannot carry one; and validation pushed entirely out to an external linter (`linter.aip.dev`), with `Style.DECLARATIVE_FRIENDLY` existing to switch it on.

This is precisely the `references: "wallet.Account"` pattern, and it is a warning, not a model. **protoc-gen-gorm shows the better answer**: its relation options never name the target type at all — the field's own message type is the reference, and protoc checks it.

```proto
// gorm: the target of the relation is the field's message type, not a string
Account account = 3 [(gorm.field).belongs_to = {foreignkey: "account_id"}];
```

**2. Embedded mini-languages inside string fields.** `ResourceDescriptor.pattern` carries a path-template grammar whose EBNF is written in the comment. protoc-gen-gorm expresses composite indexes by *two fields sharing the same index-name string*:

```proto
message GormTag {
    string index = 10;          // index NAME as a string
    string unique_index = 11;   // composite index = share the same name string
}
```

It therefore **cannot express column order in a composite index** — a fatal gap for a DB schema language, and one that a `repeated Index { string name = 1; repeated string columns = 2; }` shape would not have.

**3. Cardinality inferred from the field label with an empty-message opt-in.** In protoc-gen-gorm, `repeated Foo x = 5;` is has-many by default but `repeated Foo x = 5 [(gorm.field).many_to_many = {}];` is M2M — presence of an empty message literal is the switch. In protoc-gen-ent, a singular message field is to-one, `repeated` is to-many, and `ref: "owner"` marks the inverse side by naming a field on the *other* message with nothing checking it. Better: state cardinality explicitly and **lint that it agrees with the field label**.

**4. Generating schema elements that do not appear in the schema.** protoc-gen-gorm synthesizes foreign-key fields that are not in the `.proto`: "foreign keys pointing to primary keys (association keys) are automatically created if they don't exist in proto messages". This breaks "the .proto is the schema" and makes any descriptor-level compatibility check meaningless.

**5. Documentation drift.** protoc-gen-gorm's README names the knobs `references`, `joinForeignKey`, `joinReferences`; the `.proto` calls them `association_foreignkey`, `jointable_foreignkey`, `association_jointable_foreignkey`. Generate the docs from the vocabulary file.

**6. Reusing a type name across a semantic break.** PGV's `MessageRules` was a *field*-level message about message-typed fields; protovalidate's `MessageRules` is the *message*-level option type. Same identifier, incompatible meaning.

**7. `ON DELETE` does not exist in either ORM generator.** Neither gorm nor ent expresses SQL referential actions at all; gorm's `disable_association_autoupdate` / `replace` / `append` / `clear` are ORM write-time behaviours. This is a genuine gap in the prior art, not a considered omission — an `on_delete` enum is an improvement on both, but there is no vocabulary to borrow.

### 3.6 The PGV → protovalidate architectural break — the biggest lesson

PGV was a protoc plugin that emitted `Validate()` methods. protovalidate is pure runtime reflection over the descriptor:

> Protovalidate performs all validations using reflection, so there's no `protoc` plugin to use or manage, and it doesn't require any code generation.
> — <https://protovalidate.com/migration-guides/migrate-from-protoc-gen-validate/>

Even better, **the vocabulary file carries its own semantics**. Standard rules are defined as CEL expressions attached to the option fields themselves — 320 occurrences of `(predefined).cel` in `validate.proto`:

```proto
optional uint64 min_items = 1 [(predefined).cel = {
  id: "repeated.min_items"
  expression: "uint(this.size()) < rules.min_items ? 'must contain at least %d item(s)'.format([rules.min_items]) : ''"
}];
optional bool unique = 3 [(predefined).cel = {
  id: "repeated.unique"
  message: "repeated value must contain unique items"
  expression: "!rules.unique || this.unique()"
}];
```

One executable spec drives five language runtimes with no per-language rule table. `protovalidate-go` later added native (non-CEL) implementations for speed, toggled by `WithDisableNativeRules`, and conformance-tests the two paths against each other — CEL remains normative, the fast path is an optimization that must agree with it.

**This does not directly apply to protop** (protop *is* a code generator; it has nothing to reflect over at runtime). But the derived lesson does: **put as much of the vocabulary's meaning into the vocabulary file as possible**, so that adding a rule does not mean editing the generator.

They also **shipped a rewriter with the vocabulary break**: `go run ./tools/protovalidate-migrate -w /path/to/protos`, with `--remove-legacy` / `--replace-protovalidate`, backed by golden files at `proto/protovalidate-testing/tests/migrate/*.{source,migrated,removed}.proto`. Budget for one from day one; it is what made the break survivable.

### 3.7 Distribution of a vocabulary file

protovalidate ships the same `validate.proto` four ways: as a BSR module (`buf.build/bufbuild/protovalidate`, added to consumers' `buf.yaml` `deps:`), as a raw-URL vendoring one-liner for plain protoc users

```sh
curl --create-dirs -O --output-dir ./vendor/github.com/bufbuild/protovalidate/buf/validate \
  https://raw.githubusercontent.com/bufbuild/protovalidate/refs/heads/main/proto/protovalidate/buf/validate/validate.proto
```

as a BSR-generated-code SDK (the Go runtime depends on `buf.build/gen/go/bufbuild/protovalidate/protocolbuffers/go` because it needs the compiled descriptors to resolve the extensions), and as a Bazel/BCR artifact.

For protop the relevant point is narrower: **under the settled architecture, praguec can embed `protop/options.proto` in its own binary** and make it importable without the user vendoring anything — the same way protoc ships `google/protobuf/descriptor.proto` internally. The user only needs a physical copy of the file if they also want stock protoc to compile their schema, which is exactly the compatibility invariant. Ship it in the NuGet package as content, and document the `curl` one-liner.

---

## 4. Custom options: the metadata carrier

### 4.1 What `descriptor.proto` allows

Source: `google/protobuf/descriptor.proto` (<https://github.com/protocolbuffers/protobuf/blob/main/src/google/protobuf/descriptor.proto>). Every options message — `FileOptions`, `MessageOptions`, `FieldOptions`, `OneofOptions`, `EnumOptions`, `EnumValueOptions`, `ExtensionRangeOptions`, plus the service/method ones — ends with:

```proto
  extensions 990 to 998;                                  // internal / reserved
  repeated UninterpretedOption uninterpreted_option = 999;
  extensions 1000 to max;                                 // <- custom options live here
```

| Range | Meaning |
|---|---|
| 1 – 989 | protobuf's own option fields |
| 990 – 998 | reserved (internal) |
| 999 | `uninterpreted_option` |
| 1000 – max | available for custom options |

The language guide asks that extension numbers be registered globally to avoid collisions (<https://protobuf.dev/programming-guides/proto2/#customoptions>; registry at `docs/options.md` in the protobuf repo). The user's `50010`/`50011` are legal but unregistered.

**Under the settled architecture (praguec parses `.proto` itself; protoc is not in the pipeline) the extension *numbers* carry no weight for protop.** protop resolves `(entity)` by name against the `extend google.protobuf.MessageOptions { Entity entity = 50011; }` declaration it parsed. The numbers exist solely so that stock protoc also accepts the file. Recommendation: **reserve one contiguous block (e.g. `50000`–`50099`), document it in the repo, assign numbers once, never reuse.** Cheap insurance; zero runtime cost.

### 4.2 The option-literal grammar — the real parser cost

The syntax protop must parse:

```proto
message Bet {
  option (entity) = { table: "bets" primary_key: "id" };
  int64  id = 1;
  string account_id = 2 [(relation) = {
    references: "wallet.Account"
    cardinality: MANY_TO_ONE
    on_delete: RESTRICT
  }];
}
```

This is protobuf's **message-literal (aggregate) value** syntax — a near-relative of text format. Enumerated from a working reference implementation (`src/protobuf-net.Reflection/Parsers.cs`, `ReadOption<T>` at ~line 3559), the features a parser owes:

- `option (ext) = ...;` at statement level vs `[(ext) = ...]` inside a field's bracket list — **different punctuation for the same construct**.
- Bare identifiers for field names inside a literal (`table:`), but bracket syntax `[fully.qualified.ext]: value` for an *extension* field inside a literal.
- **Enum values are bare identifiers** (`MANY_TO_ONE`) — indistinguishable from anything else lexically. You must know the field's declared type to interpret them. This is the one place the option grammar is not context-free from the parser's view, and it is why the `extend` declarations must be resolved before option values can be interpreted.
- Repeated fields either as repeated entries (`primary_key: "id" primary_key: "seq"`) **or** list syntax (`primary_key: ["id","seq"]`), and repeated messages as `[{...},{...}]`.
- Separators optional: commas, semicolons, or nothing between entries.
- Nested braces for submessages; `<` `>` accepted as an alternative to `{` `}`.
- `default` and `json_name` are special-cased pseudo-options, not real fields.

That list is the honest measure. It is a few hundred lines of careful code, not a research project — but it is the fiddliest part of the front end, and **protop can legitimately narrow it**: accept only `{ }`, require `:` after every field name, accept both repeated forms (they appear in real code), and reject `<` `>`. Every narrowing is safe under the "protop accepts a subset" rule.

### 4.3 Two-pass interpretation is unavoidable

protoc's own model is instructive even though protop does not use it: unresolved option values are parked in `UninterpretedOption` and only later re-encoded as real extension fields once the `extend` declaration is known. protobuf-net does the same in two passes (`ResolveOptionsImpl` → `AppendOptions(..., resolve: true, ...)` then `AppendOptions(..., apply: false, ...)`).

protop needs the same shape: **parse option values into a loosely-typed literal tree, then type-check and coerce them against the resolved option message in a second pass.** Doing it in one pass is not possible (bare-identifier enums, forward references to option messages declared later or in another file).

---

## 5. Option vocabulary design

This section synthesises A (what proto can express), B (decimal) and C (what the prior art did) into a concrete first vocabulary. Every recommendation below is traceable to a finding in one of those three sections; where it is a judgement call with no primary-source backing, it says so.

### 5.1 The five structural decisions, settled

| Decision | Choice | Why (source) |
|---|---|---|
| Syntax of the options file | **`syntax = "proto2"`, every field `optional`** | §3.2. PGV, protovalidate and protoc-gen-ent all do this. proto3's implicit presence makes `scale: 0` indistinguishable from "unstated" — and gorm's 24-field proto3 `GormTag` demonstrates the resulting damage. |
| Extension numbers | **one contiguous block, e.g. 50000–50099, recorded in a comment** | §4.1. Under the settled architecture the numbers are inert for protop — it resolves `(protop.entity)` by name against the `extend` it parsed. They exist only so stock protoc also accepts the file. Avoid the internal range that bit gorm (`// TODO: The option number 52119 lies within the internally reserved extension range`). |
| Distribution of `options.proto` | **embedded in the praguec binary**, resolvable as `import "protop/options.proto";` without the file existing on disk | §3.7. protoc does exactly this for `descriptor.proto` and the WKTs; protobuf-net.Reflection embeds `google/protobuf/*.proto` the same way. Also write it to disk on request (`praguec --emit-options-proto`) so stock protoc users can compile the same files. |
| Message-typed vs scalar options | **message-typed, one option per concern** | §3.4. `(protop.entity) = { ... }` groups related settings, is extensible without burning extension numbers, and reads like a declaration. |
| Evolution of the vocabulary itself | **tombstone both number and name on removal** | §3.4 — googleapis reserves both; §3.5 — protovalidate reused the name `MessageRules` across an incompatible break, which is the failure to avoid. |

### 5.2 The vocabulary

```proto
// protop/options.proto
syntax = "proto2";
package protop;
import "google/protobuf/descriptor.proto";

// ---------------------------------------------------------------- message

message Entity {
  optional string table       = 1;  // defaults to snake_case(message name), pluralised
  optional string schema      = 2;
  repeated string primary_key = 3;  // field names, in key order; order is significant
  repeated Index  index       = 4;
  repeated Unique unique      = 5;
}

message Index {
  optional string name    = 1;             // defaults to ix_<table>_<columns joined by _>
  repeated Column column  = 2;             // ORDER IS SIGNIFICANT - see D.4
  optional bool   unique  = 3 [default = false];
  optional string where   = 4;             // partial-index predicate; opaque to protop, see D.5
}

message Column {
  optional string    field     = 1;        // a field name on this message
  optional Direction direction = 2 [default = ASC];
}
enum Direction { ASC = 0; DESC = 1; }

message Unique { repeated string field = 1; }

// ---------------------------------------------------------------- field

message Relation {
  // Target entity. OPTIONAL when the field is message-typed: the field's own
  // type is the reference and protoc itself checks it. REQUIRED (and checked
  // by protop) when the field is a bare scalar foreign key.
  optional string      references  = 1;
  optional Cardinality cardinality = 2;
  repeated string      target_key  = 3;    // defaults to the target's primary_key
  optional OnDelete    on_delete   = 4 [default = RESTRICT];
  optional string      inverse     = 5;    // field name on the target, for bidirectional
}
enum Cardinality { CARDINALITY_UNSPECIFIED = 0; ONE_TO_ONE = 1; MANY_TO_ONE = 2; ONE_TO_MANY = 3; }
enum OnDelete    { RESTRICT = 0; CASCADE = 1; SET_NULL = 2; SET_DEFAULT = 3; NO_ACTION = 4; }

message DecimalSpec {              // see B.6
  optional uint32 scale          = 1;
  optional uint32 precision      = 2;
  optional string currency_field = 3;
}

message ColumnSpec {
  optional string column      = 1;         // override the derived column name
  optional uint32 max_length  = 2;         // VARCHAR(n); also a generated guard
  optional bool   generated   = 3 [default = false];
}

enum CSharpKind { KIND_UNSPECIFIED = 0; RECORD = 1; STRUCT = 2; }

extend google.protobuf.MessageOptions {
  optional Entity     entity      = 50000;
  optional CSharpKind csharp_kind = 50001;
}
extend google.protobuf.FieldOptions {
  optional Relation    relation = 50010;
  optional DecimalSpec decimal  = 50011;
  optional ColumnSpec  column   = 50012;
}
extend google.protobuf.FileOptions {
  optional string default_schema  = 50020;
  optional string csharp_base_ns  = 50021;
}
```

Three shapes copied deliberately from the prior art (§3.4): the **definition-vs-reference split** (an `Entity` is declared on the message, referenced by name from a `Relation` — as googleapis splits `ResourceDescriptor` from `ResourceReference`); **explicit rather than inferred cardinality**, because gorm inferring it from the field label (§3.5) cannot express a one-to-one; and **file-level defaults** as an escape hatch, like googleapis' `resource_definition`.

One shape deliberately **not** copied: gorm's composite index by shared name, which §3.5 established **cannot express column order** — a composite index `(account_id, placed_at)` is a different index from `(placed_at, account_id)`, and a schema language that cannot distinguish them is broken for its primary purpose. Hence `repeated Column`, ordered.

And one genuine gap in both ORM generators, filled here: **`ON DELETE`**. §3.5 found it absent from protoc-gen-gorm and protoc-gen-ent alike — not a considered omission, just missing. A relation without a deletion rule is an under-specified relation.

### 5.3 The worked example

Before — the user's original, which stock protoc accepts and understands nothing about:

```proto
syntax = "proto3";
package betting;

message Bet {
  option (entity) = { table: "bets" primary_key: "id" };
  int64  id         = 1;
  string account_id = 2 [(relation) = { references: "wallet.Account" cardinality: MANY_TO_ONE }];
  double stake      = 3;
  string status     = 4;
}
```

Five defects, each caught by a rule from the validation section: `double stake` for money (§2 — binary floating point cannot represent 0.1); `string status` where an enum belongs; `string account_id` referencing an entity whose primary key is `int64` (a type mismatch nothing checks); no presence anywhere, so every column silently becomes NOT NULL (§1.4); and no `on_delete`.

After:

```proto
edition = "2023";
package betting;

import "protop/options.proto";
import "wallet/account.proto";

option (protop.default_schema) = "betting";

enum BetStatus {
  BET_STATUS_UNSPECIFIED = 0;
  PENDING                = 1;
  SETTLED                = 2;
  VOID                   = 3;
}

message Bet {
  option (protop.entity) = {
    table:       "bets"
    primary_key: "id"
    index: {
      name:   "ix_bets_account_placed"
      column: { field: "account_id" }
      column: { field: "placed_at" direction: DESC }
    }
  };

  int64  id         = 1;
  int64  account_id = 2 [(protop.relation) = {
                           references:  "wallet.Account"
                           cardinality: MANY_TO_ONE
                           on_delete:   RESTRICT
                           inverse:     "bets"
                         }];
  sint64 stake      = 3 [(protop.decimal) = { scale: 4 precision: 19 currency_field: "currency" }];
  string currency   = 4 [(protop.column)  = { max_length: 3 }];
  BetStatus status  = 5;
  google.protobuf.Timestamp placed_at = 6 [features.field_presence = LEGACY_REQUIRED];
  string settled_note = 7 [features.field_presence = EXPLICIT];   // -> string? / NULL
}
```

Every change is still plain protobuf. `edition = "2023"` is chosen over `syntax = "proto3"` for the reason in §1.4: it gives per-field control of presence through `features.field_presence` without the synthetic-oneof trap, and `LEGACY_REQUIRED` is accepted on a message field — which is what makes `placed_at` a genuinely mandatory nested message (§1.3). Note the caveat from §1.3: the stock C# backend emits `placed_at` as a plain property with no guard, so **protop reads the feature and emits the guard itself**. That is the entire thesis of this project in one field.

What protop emits from that (shape only — see the MessagePack section for the exact attribute rules):

```csharp
[MessagePackObject]
public sealed record Bet(
    [property: Key(0)] long                     Id,
    [property: Key(1)] long                     AccountId,
    [property: Key(2)] decimal                  Stake,
    [property: Key(3)] string                   Currency,
    [property: Key(4)] BetStatus                Status,
    [property: Key(5)] DateTimeOffset           PlacedAt,
    [property: Key(6)] string?                  SettledNote);
```

Note `[Key(0..6)]` against tags 1..7: keys are zero-based dense positions, tags are one-based. The mapping is `Key = tag - 1` **only while tags are gapless**; the general rule and its cost are in the MessagePack section.

### 5.4 Rules the vocabulary makes checkable

The full diagnostic catalogue is in the validation section. Three are worth calling out here because they are properties of *this vocabulary's shape*, not of schemas written in it:

1. **`primary_key` is `repeated`, and order is the key order.** A single-component key is the one-element case, not a special case. This avoids googleapis' `"*"`-wildcard-and-`child_type` accretion (§3.5): when the general form is the only form, there is nothing to bolt on later.
2. **`Index.column` is a repeated message, not a repeated string.** Per-column direction has somewhere to live from day one. Adding `direction` later to a `repeated string columns` would require either a parallel array (which can desynchronise) or an embedded mini-language (`"placed_at DESC"`) — and §3.5 identified embedded mini-languages as one of the prior art's clearest mistakes.
3. **`Relation.references` is optional when the field is message-typed.** This is gorm's one decisive win (§3.5): make the reference a protobuf type reference wherever possible, so name resolution checks it for free, in protop *and* in stock protoc, and go-to-definition works in the IDE.

### 5.5 Where to stop

Two deliberate non-features, on the prior art's evidence:

- **No embedded expression language in v1.** `Index.where` above is the one place a predicate could go, and it is specified as *opaque to protop* — passed through to the DDL emitter, unvalidated, explicitly labelled as such in the diagnostic output. If constraint expressions are ever wanted, §3.6 is the map: protovalidate replaced PGV's fixed rule set with CEL, and the migration needed a **shipped rewriter** (`tools/protovalidate-migrate`) to carry users across. Do not start down that road in v1; if you do later, start with CEL rather than inventing.
- **No `required`-style validation vocabulary** (`min_len`, `pattern`, …). That is protovalidate's job, protovalidate already exists, and its options compose with protop's because protobuf options are independent extensions of the same `*Options` messages. Two vocabularies on one field is normal and correct.

**Open question for the user** (§5.2, no primary source can settle it): whether the derived table name should pluralise. gorm pluralises; ent does not. Pluralisation in code is a lexical guess that is wrong often enough to matter (`Status` → `Statuses`, `Bonus` → `Bonuses`, `Data` → ?). The lower-risk default is **no pluralisation, explicit `table:` when you want it**, but this is a taste call, and it is cheap to reverse only before the first migration is generated.

---

## 6. Stringly-typed references: the validation opportunity

This is the single clearest place where protop earns its existence over plain protoc, and it is worth being precise about what is checkable and how.

### 6.1 The problem, in the user's own example

```proto
message Bet {
  option (entity) = { table: "bets" primary_key: "id" };
  int64  id         = 1;
  string account_id = 2 [(relation) = { references: "wallet.Account" cardinality: MANY_TO_ONE }];
  sint64 stake      = 3;
}
```

To protoc, `"id"` and `"wallet.Account"` are opaque string literals. protoc type-checks that `primary_key` is a `string` field on the `Entity` option message and stops. Rename `id` to `bet_id` and the schema still compiles; delete `wallet.Account` entirely and it still compiles. **Every one of these is a silent build-time failure that surfaces as a runtime or migration failure later.**

googleapis hit the identical wall with `google.api.ResourceReference.type` and the design scars are visible: a naming regex and a 100-character limit written into a doc comment; a wildcard `type: "*"` because the string space has no way to say "any"; a second field `child_type` that exists only to invert the reference's direction; and validation delegated entirely to an external linter (`linter.aip.dev`), switched on by an opt-in `Style.DECLARATIVE_FRIENDLY` flag. (See §3.5.)

### 6.2 What a generator can check that protoc cannot

Every one of these is a pure function of the resolved schema graph. None needs new syntax, a database connection, or user annotation beyond the options themselves.

| Check | Example diagnostic |
|---|---|
| `primary_key` names a field that exists on this message | `bets.proto(3,32): error PROTOP0101: Entity 'Bet' declares primary_key "id" but no such field exists. Did you mean 'bet_id'?` |
| every `primary_key` component is a legal key type (not `repeated`, not `map`, not a message, not `bytes` unless permitted) | `PROTOP0102: primary_key component "tags" is repeated; a key component must be singular.` |
| `primary_key` components have non-nullable presence | `PROTOP0103: primary_key component "id" is 'optional'; key components must not be nullable.` |
| `references` resolves to a message in the schema graph | `PROTOP0110: relation on 'Bet.account_id' references "wallet.Account", which is not defined or imported.` |
| the referenced message is itself an entity | `PROTOP0111: "wallet.Account" is not declared as an entity; add option (protop.entity).` |
| the referencing field's type matches the referent's primary-key type | `PROTOP0112: 'Bet.account_id' is 'string' but the primary key of 'wallet.Account' is 'int64'.` |
| composite-key arity matches | `PROTOP0113: 'wallet.Account' has a 2-component primary key; a single scalar field cannot reference it.` |
| `cardinality` agrees with the field label | `PROTOP0114: cardinality MANY_TO_ONE declared on a 'repeated' field; did you mean ONE_TO_MANY?` |
| `on_delete` is legal for the cardinality and nullability | `PROTOP0115: on_delete = SET_NULL requires the referencing field to be 'optional'.` |
| index components name real fields, and column order is explicit | `PROTOP0120: index "ix_bets_account" names column "acct_id"; no such field.` |
| no reference cycle that makes insertion order impossible | `PROTOP0130: mandatory relation cycle: Bet -> Account -> Bet.` |
| the inverse side of a bidirectional relation agrees | `PROTOP0131: 'Account.bets' declares inverse "account_id" but 'Bet.account_id' references 'wallet.Ledger'.` |
| `decimal` scale ≤ 28; `currency_field` names a sibling `string` | §2.6 |

The taxonomy is deliberately the same as a compiler's: **existence, kind, type-compatibility, arity, agreement, and acyclicity.** That is the whole list; there is nothing exotic here.

### 6.3 Source locations: how to point at the right characters

To emit `bets.proto(3,32)` protop needs the span of the *string literal inside the option value*, not of the field. Two facts matter.

**If protop keeps its own AST** — which under the settled architecture it does — this is free: the lexer already has a `(line, column)` for every token, and the option-literal tree (§4.2) just needs to carry the span of each scalar value alongside the value. **Do this from the first commit.** Retrofitting spans into a literal tree that was built without them is a miserable refactor, and the diagnostics are the product.

**If protop ever reads descriptors instead**, the best available granularity is `SourceCodeInfo.Location`, and it is coarser than you want. From `descriptor.proto`:

> Always has exactly three or four elements: start line, start column, end line (optional, otherwise assumed same as start line), end column ... **line and column numbers are zero-based** -- typically you will want to add 1 to each before displaying to a user.

That gives you the span of the *option*, not of a string inside its aggregate value. Pointing at the exact `"wallet.Account"` would not be possible.

**Watch the off-by-one.** `SourceCodeInfo` is zero-based; MSBuild, Visual Studio, and every editor are one-based. The canonical MSBuild diagnostic format is

```
Origin: Subcategory Category Code: Text
bets.proto(3,32): error PROTOP0110: relation references "wallet.Account", which is not defined.
```

and `TaskLoggingHelper.LogError(subcategory, errorCode, helpKeyword, file, lineNumber, columnNumber, endLineNumber, endColumnNumber, message, messageArgs)` takes one-based line and column. If protop emits that format, Visual Studio's error list, `dotnet build`, and GitHub Actions all light up for free.

Emit at least three formats, as buf does: the default `path(line,col): error CODE: message` (MSVS-style, which MSBuild parses natively — buf's own default text format omits the rule ID, which is a real usability wart worth not copying), plus JSON Lines for tooling, plus a paste-ready suppression block (see §9.5).

### 6.4 The deeper design lesson: prefer the type system to the string

protoc-gen-gorm gets one thing decisively right that googleapis gets wrong: **its relation options never name the target type.** The field's own message type *is* the reference, and protoc checks it:

```proto
// The target of the relation is the field's declared message type. protoc checks it.
Account account = 3 [(gorm.field).belongs_to = { foreignkey: "account_id" }];
```

Wherever protop can make a reference be a protobuf type reference rather than a string, it should — the check then happens in name resolution for free, in *both* protop and stock protoc, and the IDE's go-to-definition works. Strings should be reserved for things that genuinely are not types: column names, index names, field names within a message.

That said, a foreign key in a wire-schema is frequently **just the scalar key**, with no message field pointing at the parent at all (`string account_id = 2;` and no `Account account`). protop cannot make that a type reference. So the honest position is:

- **`references` on a scalar FK field stays a string** — and protop resolves and checks it, which is the whole point.
- **`references` is optional and inferred when the field is message-typed**, matching gorm.
- **`primary_key`, `index.columns`, `currency_field` and inverse-side names stay strings** — they name fields, not types — and every one of them is checked.

A stringly-typed reference is only a mistake if nothing checks it. Being the thing that checks it is protop's job description.

---

## 7. MessagePack-CSharp: the facts that constrain codegen

Evidence base: local clone of `github.com/MessagePack-CSharp/MessagePack-CSharp` at commit `b0e5cee3e30e75004ea95e154ec3832027a46090` (latest tag `v3.1.8`). License MIT.

### 7.1 Attribute surface

`src/MessagePack.Annotations/Attributes.cs`:

| Attribute | Signature | Notes |
|---|---|---|
| `[MessagePackObject]` | `MessagePackObjectAttribute(bool keyAsPropertyName = false)` | plus `SuppressSourceGeneration`, `AllowPrivate` properties |
| `[Key]` | `KeyAttribute(int x)` / `KeyAttribute(string x)` | int → array layout; string → map layout |
| `[IgnoreMember]` | — | excludes a member |
| `[SerializationConstructor]` | — | picks the deserializing ctor |
| `[Union]` | `UnionAttribute(int key, Type subType)` | `AttributeTargets.Interface \| Class`, `AllowMultiple = true` |
| `[MessagePackFormatter]` | `(Type formatterType, params object[] args)` | per-member custom formatter |

### 7.2 Int keys produce a dense positional array padded to `maxKey + 1`

This is the single most important fact for tag→key mapping, and it is identical on both code paths.

**Reflection.Emit path** — `src/MessagePack/Resolvers/DynamicObjectResolver.cs` ~line 505:

```csharp
var maxKey = info.Members.Where(x => x.IsReadable).Select(x => x.IntKey).DefaultIfEmpty(-1).Max();
var intKeyMap = info.Members.Where(x => x.IsReadable).ToDictionary(x => x.IntKey);
var len = maxKey + 1;
il.EmitLdc_I4(len);
il.EmitCall(MessagePackWriterTypeInfo.WriteArrayHeader);
for (int i = 0; i <= maxKey; i++)
{
    if (intKeyMap.TryGetValue(i, out var member)) { EmitSerializeValue(...); }
    else { argWriter.EmitLoad(); il.EmitCall(MessagePackWriterTypeInfo.WriteNil); }   // "Blanc"
}
```

**AOT source-generator path** — `src/MessagePack.SourceGenerator/Transforms/FormatterTemplate.tt` line 43 onward:

```
writer.WriteArrayHeader(<#= Info.MaxKey + 1 #>);
<# for (var i = 0; i <= Info.MaxKey; i++) {
  var member = Info.GetMember(i);
  if (member == null) { #>
writer.WriteNil();
<# } else { #>
<#= member.GetSerializeMethodString() #>;
<# } #>
<# } #>
```

Consequences, stated precisely:

- **Wire cost of a gap is exactly 1 byte per missing index** (msgpack `nil` = `0xc0`), plus the array header growing (fixarray ≤ 15 elements = 1 byte; `array16` ≤ 65535 = 3 bytes; `array32` = 5 bytes — msgpack spec, <https://github.com/msgpack/msgpack/blob/master/spec.md>). A type with `[Key(1)]` and `[Key(7)]` writes `WriteArrayHeader(8)` then nil,v1,nil,nil,nil,nil,nil,v7 — **6 wasted bytes per instance**.
- **There is no threshold and no strategy switch.** I grepped `DynamicObjectResolver.cs`, `ObjectSerializationInfo.cs` and the templates for any density/magnitude guard: there is none. It never falls back to a map. The only two layouts are "all members have int keys → array" and "all members have string keys → map" (`ObjectSerializationInfo.IsStringKey => !this.IsIntKey`; template selection at `src/MessagePack.SourceGenerator/MessagePackGenerator.Emit.cs:45`).
- **The AOT generator degrades catastrophically, not gracefully, with large keys.** The T4 loop emits one literal `writer.WriteNil();` statement *per gap index* into the generated C#. A field numbered 50010 (the user's example extension range) would emit ~50 000 statements into a single method — far past practical method-size limits. The Reflection.Emit path is less explosive but still allocates `new DeserializeInfo[maxKey + 1]` (line 869) and a `OpCodes.Switch` jump table of that size.
- **Practical upper bound to warn users about:** keep the maximum field number in any single message in the low hundreds. Under ~64 the array header stays cheap and IL/source size is trivial. Gaps of a few dozen are a few bytes per message. Gaps of thousands are a build-time hazard on the AOT path. Protobuf itself permits 1..536 870 911 with 19 000–19 999 reserved (`descriptor.proto` / language guide), so **protop must diagnose the mismatch; MessagePack will not.**

The README documents the wire consequence in the same terms: with `[Key(3)]` and `[Key(10)]` you get `[null,null,null,0,null,null,null,null,null,null,0]`, and "when the values of indexed keys 'jump' a lot, leaving gaps in the sequence, it will negatively affect the binary size, as `null` placeholders will be inserted into the resulting arrays" (<https://github.com/MessagePack-CSharp/MessagePack-CSharp#object-serialization>).

### 7.3 Deserialization is tolerant in both directions — projections work

This is what makes tag-preserving views viable at runtime.

**Longer payload than the type declares → extra elements are skipped, not an error.** `DynamicObjectResolver.cs:915` `BuildDeserializeInternalDeserializeLoopIntKey` emits a `for (i = 0; i < length; i++)` over the *wire* length, with `OpCodes.Switch` over `infoList` and a default arm that calls `reader.Skip()`. The AOT template is the same shape in source:

```csharp
var length = reader.ReadArrayHeader();
for (int i = 0; i < length; i++)
{
    switch (i)
    {
        case 1: ... break;
        case 7: ... break;
        default: reader.Skip(); break;      // <-- unknown / dropped indices
    }
}
```

**Shorter payload than the type declares → remaining members keep their defaults.** The AOT template guards every post-loop member assignment:

```csharp
if (length <= <#= memberIndex #>) { goto MEMBER_ASSIGNMENT_END; }
```

So: a payload written for `Bet` deserializes into `BetSummary` (a tag-preserving projection) without error, and a payload written for an older, shorter `Bet` deserializes into a newer one. **MessagePack-CSharp's int-key layout has the same "ignore unknown, default missing" tolerance protobuf has.** This is verified from the generator source, not inferred.

Note the one asymmetry with protobuf: protobuf *preserves* unknown fields on round-trip by default; MessagePack-CSharp **discards** them (`reader.Skip()`), so a read-modify-write through a projection loses the dropped fields.

### 7.4 String keys (map layout) — the escape hatch, and its price

`src/MessagePack.SourceGenerator/Transforms/StringKey/StringKeyFormatterTemplate.tt`:

```
writer.WriteMapHeader(<#= list.Count #>);
writer.WriteRaw(GetSpan_<#= member.UniqueIdentifier #>());   // pre-encoded UTF-8 key bytes
...
var length = reader.ReadMapHeader();
```

- Exactly `N` entries for `N` members — **no padding, gaps cost nothing**.
- Price: the UTF-8 property name on the wire for every field of every instance (1 length byte + name bytes for names ≤ 31 chars), plus key matching on read (an automata/trie dispatch rather than a jump table).
- `[MessagePackObject(keyAsPropertyName: true)]` is the "map mode" shorthand; `MsgPack018` (`CollidingMemberNamesInForceMapModeId`) enforces unique names in that mode.

**Recommendation for protop:** int keys by default (tags → `[Key(N)]`), with a per-message option to switch a type to string keys. The switch is the mitigation for any type whose tag span is genuinely wide — including future view types derived from a message with high tags. Cheap to hold open now: it is a codegen flag, not an architectural commitment.

### 7.5 Records, init-only, and constructor matching

Records are first-class and tested: `tests/MessagePack.Tests/DynamicObjectResolverRecordsTests.cs` covers positional records, records with `init` properties, inherited positional records, and generic positional records:

```csharp
[MessagePackObject] public record PersonPositional([property: Key(0)] string FirstName, [property: Key(1)] string LastName);
[MessagePackObject] public record StudentPositional(string FirstName, string LastName, [property: Key(2)] int Grade) : PersonPositional(FirstName, LastName);
[MessagePackObject] public record Person { [Key(0)] public string FirstName { get; init; } [Key(1)] public string LastName { get; init; } }
```

Constructor rules, from `DynamicObjectResolver.cs`:

- **Selection** (line ~1674): the ctor marked `[SerializationConstructor]` if exactly one exists; otherwise ctors are tried in descending parameter count.
- **Parameter matching for int keys** (line 1697) — the rule a generator must obey:
  ```csharp
  IReadOnlyDictionary<int, EmittableMember> ctorParamIndexIntMembersDictionary =
      intMembers.OrderBy(x => x.Key).Select((x, i) => (Key: x.Value, Index: i)).ToDictionary(x => x.Index, x => x.Key);
  ```
  Parameters are matched **by ordinal position against members sorted ascending by int key** — *not* by name, and *not* by key value. **Therefore a generated positional record's parameters must be emitted in ascending tag order.** With protop's ascending-tag strictness rule, declaration order, tag order and ctor order all coincide — the rule pays for itself here.
- Value types: `var isStruct = type.IsValueType;` (line 1456) and `if (ctor == null && !isStruct) throw ...` (line 1689) — a struct with no public ctor is legal (it is default-constructed and members assigned); a class without one is an error.

Analyzer rules that gate this (`src/MessagePack.SourceGenerator/Analyzers/MsgPack00xMessagePackAnalyzer.cs`): `MsgPack007` family — `NoDeserializingConstructor`, `DeserializingConstructorParameterTypeMismatch`, `DeserializingConstructorParameterIndexMissing` ("The deserializing constructor parameter count must meet or exceed the number of serialized members"), name missing/duplicate. Also `MsgPack011` partial-type-required, `MsgPack015` `AllowPrivate`, `MsgPack016` KeyAttribute-derived unsupported in AOT, `MsgPack017` init accessor with initializer, `MsgPack018` map-mode name collisions.

### 7.6 `struct` / `record struct` are supported

Verified rather than assumed:

- `src/MessagePack.SourceGenerator/Transforms/TransformUtilities.cs:36` emits nesting declarations for `TypeKind.Struct when nestingType.IsRecord => "record struct"`, alongside `class`, `record`, `struct`, `interface`.
- `tests/MessagePack.SourceGenerator.Tests/MsgPack00xAnalyzerTests.cs` has `AddAttributesToMembersOfRecordStruct` and `AddAttributesToMembersOfRecordStructWithPrimaryCtor` — the analyzer *and* its code fix handle `record struct` in both member and primary-constructor form, producing `[property: Key(0)]` on positional parameters.
- `tests/MessagePack.SourceGenerator.Tests/PrivateMemberAccessTests.cs` and the resource snapshot directory `Resources/FormatterForClassWithPrivateMembers(record struct)/` confirm generated formatters for record structs.
- One caveat from `FormatterTemplate.tt`: for a value type, a `nil` on the wire throws — `throw new global::System.InvalidOperationException("typecode is null, struct not supported")`. A struct-shaped message therefore cannot be null on the wire; a nullable field of that type needs `T?` (`StaticNullableFormatter<T>`).

### 7.7 `decimal`

`decimal` is supported out of the box by the built-in formatter (string-ish encoding) and there is an opt-in `NativeDecimalResolver` / `NativeDecimalFormatter` (`src/MessagePack/Resolvers/NativeDecimalResolver.cs`) that writes the CLR-native representation, with `StaticNullableFormatter<decimal>` for `decimal?`. So **the C# side has no problem carrying `decimal`; the gap is entirely on the protobuf side** (§2.2).

### 7.8 Union

`[Union(key, type)]` on an interface or abstract class encodes as a two-element array `[key, value]` (README, "Union"). Keys may not be reused across a hierarchy. Relevant only if protop ever maps `oneof` or entity inheritance to a polymorphic C# type — both currently out of scope.

---

## 8. struct vs record: two decisions in one declaration

The hypothesis to test was that a FlatBuffers-style `struct`/`table` split pays off twice — once in the C# shape emitted, once in the MessagePack encoding chosen. The evidence says **the first half is real and valuable, the second half is real but much smaller, and the language-design lesson is the most valuable part of all**.

### 8.1 What FlatBuffers actually means by `struct`

`docs/source/schema.md`:

> Similar to a table, `structs` consist of fields are required (so no defaults either), and **fields may not be added or be deprecated.**

> Structs may only contain scalars or other structs. **Use this for simple objects where you are very sure no changes will ever be made** (as quite clear in the example `Vec3`). Structs use less memory than tables and are even faster to access (they are always stored in-line in their parent object, and use no virtual table).

The compiler enforces the corollaries in `src/idl_parser.cpp`: `"structs may contain only scalar or struct fields"`, `"default values are not supported for struct fields, table fields, or in structs."`, `"can't deprecate fields in a struct"` (line 1253), `"struct fields are always required"` (line 1164, under the comment `// setting required or optional on a struct field is meaningless`).

The mechanical reason is one line of `Parser::ConformTo` (`idl_parser.cpp:4534`), which is what `flatc --conform` runs:

```cpp
if (field.value.offset != field_base->value.offset)
  return "offsets differ for field: " + qualified_field_name;
```

For a **struct**, `value.offset` is a **byte offset**; for a **table** it is a **vtable slot**. Same field, same check, two meanings — inserting a field into a struct shifts every subsequent byte offset and trips the check, while appending to a table just allocates a new slot. That dual meaning *is* the design.

Worth noting honestly: `docs/source/evolution.md`, the page dedicated to compatibility, **never states that modifying a struct is a breaking change.** The prohibition is expressed as a schema-language rule; the wire consequence is only implied by the inline layout described in `internals.md`. The rule is load-bearing but under-documented — a mistake protop should not repeat.

### 8.2 The C# half: what FlatSharp does, and what it declined to do

`github.com/jamescourtney/FlatSharp` (Apache-2.0) is the only serious C# FlatBuffers implementation, and it is the closest existing precedent for exactly this decision. Three findings matter.

**1. The value-type mapping is opt-in per declaration, not implied by `struct`.** `src/FlatSharp.Compiler/SchemaModel/ValueStructSchemaModel.cs` requires both `@struct.IsStruct` *and* the FlatSharp-invented metadata key `fs_valueStruct` (`MetadataKeys.cs`: *"Indicates that a struct is to be generated as a value type."*). Without it, `ReferenceStructSchemaModel` emits a `partial class`. **An fbs `struct` is a C# class by default.** The value tier is a third tier, deliberately.

**2. The value tier is not a `record struct`.** `ValueStructSchemaModel.OnWriteCode`:

```csharp
writer.AppendLine($"[FlatBufferStruct({memMarshalBehavior})]");
writer.AppendLine($"[System.Runtime.InteropServices.StructLayout(System.Runtime.InteropServices.LayoutKind.Explicit{size})]");
writer.AppendLine($"public partial struct {this.Name}");
...
writer.AppendLine($"[System.Runtime.InteropServices.FieldOffset({field.Offset})]");
writer.AppendLine($"{field.Visibility} {field.TypeName} {field.Name};");
```

Public **mutable fields**, explicit layout, no constructor, not `readonly`. And with `--gen-methods` FlatSharp **hand-rolls** `Equals`, `op_==`, `GetHashCode` and a `ToString` carrying the comment `// This matches C# records` — it reimplements record-struct semantics rather than using `record struct`, because `LayoutKind.Explicit` + `[FieldOffset]` is incompatible with auto-properties. That constraint is FlatSharp's, driven by its `MemoryMarshal.Cast` fast path; **protop has no such constraint**, because MessagePack never memcpys a struct. protop is free to emit `readonly record struct`, and should.

**3. The payoff FlatSharp gets is one protop cannot get.** `FlatBufferStructAttribute`'s `MemoryMarshalBehavior` is documented as *"FlatSharp will opportunistically serialize and deserialize a value-type struct with a call to `MemoryMarshal.Cast<TFrom, TTo>(Span<TFrom>)`"*, gated on `UnsafeSizeOf(this.ClrType) == this.inlineSize`. That is a whole-struct memcpy. **MessagePack has no equivalent** — a MessagePack array is length-prefixed and per-element type-tagged; there is no blittable layout to cast to.

### 8.3 So does the distinction pay off twice? Partly.

| | C# shape | MessagePack encoding |
|---|---|---|
| Does the declaration change anything? | **Yes, substantially** | **Marginally** |
| What changes | `readonly record struct` vs `sealed record`: no heap allocation, no GC pressure, value equality without a reference compare, `default(T)` reachable | Nothing in the wire bytes. Both emit `WriteArrayHeader(n)` + n values. |
| What you can additionally do | forbid nullable members, forbid `repeated`/`map`, forbid nested reference types — making the type genuinely blittable-shaped | **guarantee tag density**, since a frozen type can never gain a field, so `[Key(N)]` is dense by construction and the Nil-padding hazard cannot arise |
| Measured benefit | allocation elimination on hot paths; real for `Money`, `Odds`, `Stake`, point-like types | zero bytes saved; the win is a *guarantee*, not a saving |

So the honest statement is: **the struct tier pays off once in C#, and once more as a schema-level guarantee rather than an encoding change.** That second payoff is still worth having — it is the only construct in the language that lets protop *prove* a message's key space is dense — but it should not be sold as an encoding optimisation, because it isn't one.

`TransformUtilities.cs:36` in MessagePack-CSharp confirms `record struct` is supported by the AOT generator, so there is no tooling obstacle.

### 8.4 The .NET guidance points away from a large struct tier

The Framework Design Guideline, as quoted on Microsoft Learn:

> AVOID defining a struct unless the type has all of the following characteristics: It logically represents a single value… **It has an instance size under 16 bytes.** It is immutable. It will not have to be boxed frequently.

Learn's performance guidance phrases the copy-cost threshold as **"three words or less."** (The commonly cited "24 bytes" figure is **not** on Learn — flagged because it is easy to repeat.) Two `long`s and a `byte` is already at the edge. **A protop `struct` that holds more than about three scalars is outside the guideline**, and protop should say so in a warning rather than silently emitting a 64-byte value type that gets copied on every `in` parameter and every array index.

Related trap, since a `readonly record struct` passed by `in` to a non-readonly member causes a **defensive copy**: emit the whole type `readonly`, so every member is readonly and no defensive copy is ever inserted.

### 8.5 The `default(T)` hazard, which has no fix

Microsoft Learn, on parameterless struct constructors:

> the default value expression ignores a parameterless constructor and produces the default value of the structure type

and array instantiation does the same. So `readonly record struct Money(string Currency, long Units)` has a reachable instance where `Currency is null` — created by `default(Money)`, by `new Money[10]`, by a field of a class, and by MessagePack itself when a payload is short. No constructor, no `required` modifier, and no nullable annotation closes that hole.

**FlatBuffers closes it by forbidding strings in structs.** That is not incidental — it is the only available fix, and protop should copy it:

> **Rule: a protop `struct` may contain only scalars, enums, and other protop `struct`s.** No `string`, no `bytes`, no message fields, no `repeated`, no `map`, no presence.

This is exactly FlatBuffers' member rule, arrived at from a completely different direction (C#'s `default(T)` rather than inline byte layout), which is a good sign the rule is right.

### 8.6 Declaration: Tier-1 option, not Tier-2 keyword

```proto
message Money {
  option (protop.csharp_kind) = STRUCT;
  string currency_code = 1;   // rejected by the member rule above
  sint64 units         = 2;
  uint32 scale         = 3;
}
```

An option is the correct mechanism here and a new keyword is not, for a reason that generalises: **`struct` merely annotates an existing message — it changes how one declared type is emitted, not what types exist.** A Tier-2 keyword would buy three characters of syntax at the cost of making the file unreadable to stock protoc, for zero expressive gain. Contrast a *view*, which generates a type that does not otherwise exist; that is the first construct where Tier-2 syntax is genuinely justified (see the roadmap section). **Annotation → option. Generation → syntax.** That is the whole test, and it is worth writing down because it will be applied repeatedly.

### 8.7 The evolution hazard, stated as a rule

A struct is a frozen contract. Concretely, for protop:

- Adding a field to a `STRUCT` message **must** be a breaking-change error against a baseline, not a warning. Under `[Key(N)]` a new field is merely a longer array and would deserialize fine — which is exactly why it needs a *schema-level* prohibition: the wire will not stop you, so the compiler must.
- Removing or renumbering a field is likewise an error.
- Changing `csharp_kind` from `STRUCT` to `RECORD` is source-breaking for every consumer (`Money x = default;` stops compiling; `null` starts being possible) though wire-compatible. Changing `RECORD` to `STRUCT` is worse, because it silently introduces the `default(T)` hole into code that was written against a reference type.
- **Recommendation: `csharp_kind` is immutable once published.** Say it in the diagnostic, not just the docs — FlatBuffers' failure to write the consequence down anywhere in `evolution.md` is the counter-example.

Note that buf cannot check any of this: `buf breaking` explicitly excludes custom options, so a `csharp_kind` flip is invisible to it. Entity-level and kind-level breaking-change detection is protop's own work.

---

## 9. Strictness rules worth imposing

Reference implementation: `buf lint` (`github.com/bufbuild/buf` @ `05bb117580e9f40e011564a3cb46ff58bfded71b`, v1.73.0, Apache-2.0). It is the right model for three reasons: its rules are organised as a **strictness ladder of nested categories**, its suppression mechanism is in-source and per-rule, and its output formats are designed to be consumed by other tools. All three are worth copying directly.

### 9.1 What buf's ladder looks like

From `private/bufpkg/bufcheck/bufcheckserver/internal/bufcheckserverbuild/bufcheckserverbuild.go` (lines 893–989):

| Category | Verbatim purpose | Contains |
|---|---|---|
| `MINIMAL` | "Checks that bare-minimum lint rules are followed." | 5 package/directory rules |
| `BASIC` | "Checks that basic lint rules are followed." | ⊇ MINIMAL + naming, `FIELD_NOT_REQUIRED`, `IMPORT_USED`, `SYNTAX_SPECIFIED`, `PACKAGE_SAME_CSHARP_NAMESPACE` |
| `STANDARD` | "Checks that standard lint rules are followed." **(the default)** | ⊇ BASIC + `ENUM_VALUE_PREFIX`, `ENUM_ZERO_VALUE_SUFFIX`, `PACKAGE_VERSION_SUFFIX`, `PROTOVALIDATE`, RPC naming |
| `COMMENTS` | "Checks that all types have comments." | opt-in, independent |
| `UNARY_RPC` | "Checks that all RPCs are unary." | opt-in, independent |

`DEFAULT` is marked `Deprecated: true, ReplacementIDs: []string{"STANDARD"}` — a small but instructive detail: buf ships *machine-readable deprecation* for its own rule IDs, so a config referencing an old ID gets redirected rather than failing. Worth copying if protop's rule set will churn.

### 9.2 The two gaps protop should occupy

I enumerated every buf lint rule ID. Two absences are significant and verified:

- **No field-presence rule exists.** Grep for `presence` across the lint rule spec returns nothing. The only adjacent rule, `FIELD_NOT_REQUIRED`, forbids `required` — the *opposite* direction. Nothing in buf warns about proto3 implicit presence, which is the single most consequential default in the language for a schema that maps to nullable database columns (§1.4).
- **No field-number rule of any kind exists.** No `FIELD_NUMBER_*` rule. buf will not complain about sparse numbering, gaps, jumping past the 1-byte-tag boundary at 15, or anything else protoc itself accepts.

Those are precisely the two axes that matter for protop, because presence drives nullability and field numbers drive `[Key(N)]`. **This is a genuine, verified gap** — not a claim that buf is deficient for its own purpose, but a statement that protop's strictness domain is largely disjoint from buf's.

### 9.3 The rules to impose

Organised as a ladder, most valuable first.

| Rule | Level | Rationale |
|---|---|---|
| **`FIELD_TAGS_ASCENDING`** — field numbers must be declared in ascending order | error | worked below |
| **`PRESENCE_EXPLICIT`** — every singular field must state its presence (`optional`, or an explicit `features.field_presence`) | error | §1.4. Implicit presence silently makes every column NOT NULL with a `""`/`0` default. The most expensive default in the language to get wrong. |
| **`ENUM_CLOSED_HANDLED`** — a message with an enum field must handle the unknown case | warning | §1.1: proto3 enums are open; a wire value outside the schema is legal. A generated C# `switch` must have a default arm. |
| **`FIRST_TAG_IS_ONE`** — the lowest field number in a message must be 1 | error | Without it, `[Key(N)]` starts with Nil padding at position 0 for no reason. |
| **`STRUCT_MEMBERS_SCALAR`** — a `STRUCT` message may contain only scalars, enums and other structs | error | the `default(T)` hazard; see the struct section |
| **`STRUCT_SIZE`** — a `STRUCT` message wider than ~3 machine words | warning | Framework Design Guidelines' 16-byte / "three words or less" |
| **`DECIMAL_SCALE_RANGE`** — `(protop.decimal).scale` ≤ 28 | error | §2.5: `System.Decimal` cannot represent more |
| **`NO_DOUBLE_FOR_MONEY`** — `double`/`float` on a field whose name or option suggests money | warning | §2. Heuristic, hence a warning, hence suppressible |
| **`MAX_TAG_BOUND`** — highest field number in a message exceeds a configurable bound (default ~512) | warning | the Nil-padding cost; see below |
| all buf `BASIC` naming rules | error | no reason to re-litigate settled conventions |

**Deliberately NOT a rule: field-number density.** A rule of the form "field numbers must be contiguous" or "gaps may not exceed N" is tempting given the Nil-padding cost, and it is wrong, because a **view preserves its parent's field numbers and simply omits the ones it drops** (see the roadmap section). A density rule would make every non-trivial view illegal. `MAX_TAG_BOUND` above is the correct shape instead: it bounds the *span*, which is what actually costs bytes, without requiring contiguity. Views of a message whose tags stay in the low hundreds pass it comfortably.

### 9.4 Worked rule: `FIELD_TAGS_ASCENDING`

**Statement.** Within a message body, each field's number must be strictly greater than the number of the textually preceding field. Fields inside a `oneof` participate in the same ordering as their enclosing message. `reserved` ranges do not participate.

**Why it earns its place** — three independent reasons, which is unusual for a style rule:

1. **It is what makes positional-record generation correct.** MessagePack-CSharp matches constructor parameters to members **by ordinal over members sorted ascending by int key** — `DynamicObjectResolver.cs:1697`: `intMembers.OrderBy(x => x.Key).Select((x, i) => (Key: x.Value, Index: i))`. So a generated positional record must emit its parameters in ascending tag order regardless of declaration order. When declaration order *is* tag order, the generated C# is a line-for-line image of the `.proto`, and a reviewer can check it by eye. When it is not, the generated file silently reorders and the diff is unreadable.
2. **It makes "append a field" the only natural way to add one.** The next field number is the one after the last line. Inserting a field in the middle of the message now *requires* giving it a number greater than everything below it, which looks wrong on the page — and that visual wrongness is the point: it is exactly the moment a developer should stop and think about whether they are about to reuse a number.
3. **It makes gaps visible.** A `reserved 7;` between `6` and `8` reads as a tombstone. In an unordered message, a deleted field leaves no visual trace at all.

**Diagnostic.**

```
bets.proto(9,3): error PROTOP0201: field 'currency' has number 4, which is not greater than
  the preceding field 'status' (number 5). Field numbers must be declared in ascending order.
```

**Suppression**, following buf's mechanism exactly. `private/bufpkg/bufcheck/options_config.go:24`:

```go
const lintCommentIgnorePrefix = "buf:lint:ignore"
```

and the matching logic in `client.go:838-859` is literally `strings.HasPrefix(commentLine, prefix + " " + ruleID)`, reading **leading comments only**, resolved through `protosourcepath.GetAssociatedSourcePaths` so a comment on an enclosing element covers nested locations. Its own doc comment documents the sharp edge:

```
//	// buf:lint:ignore SERVICE_PASCAL_CASE, SERVICE_SUFFIX (only SERVICE_PASCAL_CASE is ignored)
// While the following is invalid and a nop
//	// buf:lint:ignoreSERVICE_PASCAL_CASE
```

So: **comma lists silently do not work, and a missing space silently does nothing.** Copy the mechanism (`// protop:ignore RULE_ID reason`, leading comments, one rule per line) but **not the failure mode** — reject a malformed suppression comment with a diagnostic rather than ignoring it. A suppression that silently fails to suppress is worse than no suppression.

Also copy `lint.disallow_comment_ignores: true` as a config knob, so a team can turn the escape hatch off wholesale.

### 9.5 Output formats

`--error-format` in buf accepts `text` (alias `gcc`), `json`, `msvs`, `junit`, `github-actions`, `gitlab-code-quality` (`private/bufpkg/bufanalysis/bufanalysis.go:44-88`). Three observations:

- **buf's default text format does not include the rule ID** (`file_annotation.go:94-133` emits `path:line:col:message`). That is a real usability wart: the user sees a complaint and cannot look up or suppress the rule without switching formats. **protop should always print the rule ID.**
- The JSON format is **JSONL, one object per line** (`print.go:296-306`), with `type` carrying the rule ID. Good shape; copy it.
- `buf lint` has a `config-ignore-yaml` format that emits a **ready-to-paste `ignore_only:` block**. This is the single best adoption mechanism in the whole tool: it lets a team turn on a strict rule set against a large existing schema and baseline the existing violations in one command. **Copy this.** For a tool whose selling point is strictness, the migration path is the product.

For protop specifically, the default should be the MSVS-shaped `path(line,col): error CODE: message`, because MSBuild parses it natively and Visual Studio, `dotnet build` and GitHub Actions then light up for free.

### 9.6 Baselining breaking changes

`buf breaking --against <input>` builds both sides into descriptor sets and diffs them; inputs may be `.git#branch=main`, a tarball, a directory, or a registry reference. Its five categories are `FILE` ⊃ `PACKAGE` ⊃ `WIRE_JSON` ⊃ `WIRE`, plus `CSR`.

Two things protop needs that buf does not provide:

- **`buf breaking` excludes custom options.** Every protop-specific breaking change — a changed `(protop.decimal).scale`, a `csharp_kind` flip, a removed `primary_key` component, an `on_delete` change — is invisible to it. protop must implement its own baseline diff over its own vocabulary.
- **There is no `buf:breaking:ignore`** — grep returns zero hits; breaking suppression is config-file-only. That is a defensible choice (a breaking change should be an explicit, reviewed decision recorded in a config file, not a comment someone adds in the same commit) and worth copying deliberately rather than by accident.

---

## 10. Parser scope: how big is a strict, messages-only protobuf front end?

The settled architecture makes this the load-bearing engineering question, so it is worth answering with measurements rather than intuition.

### 10.1 Two prior implementations, measured

| | `bufbuild/protocompile` (Go, Apache-2.0) | `protobuf-net.Reflection` (C#) |
|---|---|---|
| Scope | full proto2 + proto3 + editions 2023, descriptors only (no codegen) | full proto2 + proto3 + editions, plus C# **and** VB codegen |
| Lexer | `parser/lexer.go` — **793** | `TokenExtensions.cs` 735 + `Token.cs` 98 + `Peekable.cs` **55** |
| Parser | `parser/proto.y` **1,643** lines of goyacc grammar → 3,018 generated | `Parsers.cs` **4,125** (hand-written recursive descent) |
| AST → descriptor | `parser/result.go` 1,019 + `validate.go` 633 | folded into `Parsers.cs` |
| AST types | `ast/` **5,417** | — (parses straight to descriptors) |
| Name resolution | `linker/resolve.go` **835** | `Descriptor.cs` **2,000** (incl. the descriptor model) |
| Link validation | `linker/validate.go` 1,073 + `symbols.go` 636 + `descriptors.go` 1,891 | |
| **Option interpretation** | `options/` **2,948** | `CustomOptions.cs` 253 (+ option paths in `Parsers.cs`) |
| Source info | `sourceinfo/` 977 | none — comments are discarded at lex time |
| Diagnostics | `reporter/` 293 | `ErrorCode.cs` 64 |
| **Front-end total** | **≈ 27,000** non-test Go LOC | **≈ 8,000** C# LOC |
| Codegen | — | 3,321 (C# 1,298 + VB 1,012 + shared 844 + oneof 167) |

The headline: **a protoc-byte-equivalent front end is ~8,000 lines of C#, not 100,000.** protocompile's 27,000 buys three things protobuf-net does not have — a real AST (5,417 lines of it), full `SourceCodeInfo` generation with protoc's comment-attachment rules, and a diagnostics framework. protop wants the first and third of those, which is why the realistic estimate lands between the two numbers rather than at the low end.

### 10.2 What is actually hard

Three parts, and they are not the ones people expect.

**1. Name resolution is short but pedantic.** protocompile's C++-style relative-scoping algorithm — `resolve`, `fileScope`, `messageScope`, `resolveElementRelative`, `resolveElementInFile`, `matchesPkgNamespace` — is about **120 lines** inside an 835-line file. The rule is: a leading `.` means fully qualified; otherwise walk scopes innermost-to-outermost; **if the first label of a dotted name binds to a leaf, the whole reference fails rather than falling through to the next scope.** That last clause is the one naive implementations get wrong.

The remaining 700 lines are protoc's accidents, and the source comments say so plainly. `resolve.go:189`:

> Strangely, when protoc resolves extension names, it uses the *enclosing* scope instead of the message's scope. So if the message contains an extension named "i", an option cannot refer to it as simply "i" but must qualify it (at a minimum "Msg.i"). So we don't add this message's scope to our scopes slice until *after* we do options.

and `resolve.go:645`:

> In protoc, it will skip a match of the wrong type and move on to the next scope, but only if the reference is unqualified. So we mirror that behavior here.

**Read this as the real cost signal: the algorithm is short; matching protoc's accidents is what makes it long.** A strict dialect can legitimately decide not to match them — see below.

**2. Option interpretation is the single largest piece.** protocompile spends **2,948 lines** on it. `messageLiteralValue()` (`options.go:2031`) recurses through aggregate values, handles `google.protobuf.Any` expansion, repeated fields, oneof-conflict detection, and partially-qualified extension names inside literals. And the most quotable line in the whole repository, `options.go:1530`:

> ```
> // TODO: This does not *fully* implement the insane logic of protoc with regards to resolving relative references.
> ```

That is buf — the people with the most invested from-scratch protobuf front end in existence — recording that their option-literal reference resolution is knowingly incomplete against protoc.

Two-pass interpretation is unavoidable regardless of implementation: option values reference extensions that may be declared in files still being parsed, so options are collected unresolved and interpreted after linking. protobuf-net does the same thing with `ResolveOptionsImpl` → `AppendOptions(resolve: true)` then `AppendOptions(false)`.

**3. The message-literal grammar is not context-free without resolved types.** Inside `{ ... }`, a bare identifier is a field name, but a bare identifier on the *right* of a colon is an **enum value** — and you cannot tell an enum value from a malformed message without knowing the field's type. Extension fields inside a literal use `[fully.qualified.name]:` while ordinary fields use a bare name. Commas and semicolons between entries are optional. Nested messages may use `{}` or `<>`. `default` and `json_name` are pseudo-options handled specially.

### 10.3 Where a strict dialect gets to be smaller

protop is not obliged to reproduce protoc bug-for-bug, and every narrowing below removes code:

| Narrowing | What it removes |
|---|---|
| **messages, enums and options only** — `service`/`rpc`/`stream` are a diagnostic, not a production | service/method/stream productions, and their option scopes |
| **no groups** (proto2 `group` syntax) | a genuinely awkward production with its own naming rules |
| **no extensions except the protop vocabulary itself** — user `extend` blocks are a diagnostic | extension ranges, extension declarations, `ExtensionRangeOptions`, the enclosing-scope quirk above |
| **require qualified names in option literals** | the "insane logic" protocompile declined to finish |
| **one syntax level** (editions 2023, or proto3 + editions) | feature resolution across three language versions, `edition_defaults`, LEGACY handling |
| **no `import weak`, no `import public`** | transitive public-import chains through the symbol table |
| **ascending tags required** | ordering checks become a linear scan, and gap analysis is trivial |
| **reject rather than warn** everywhere protoc warns | the warning-vs-error dual paths |

Each of those is defensible on its own terms as a *profile* decision — protop narrows what it accepts and is richer in what it means — and together they plausibly take the front end from protocompile's 27,000 toward protobuf-net's 8,000 while keeping a real AST and real diagnostics.

**Realistic estimate for protop's front end: 6,000–10,000 lines of C#**, plus codegen. The MessagePack/record emitter is the small part; `CSharpCodeGenerator.cs` in protobuf-net is 1,298 lines for a much larger surface, and protop's output shape is narrower.

### 10.4 Build the differential harness on day one

Both prior implementations pin **and execute** a protoc binary in CI — both currently protoc 35.1 — and this is the single most transferable practice in the whole comparison.

- protobuf-net's `SchemaTests.CompareProtoToParser` runs a bundled protoc with `--descriptor_set_out`, then asserts **`Assert.Equal(protocHex, parserHex)`** — byte-identical serialized `FileDescriptorSet` — across **290 schemas × 2 modes ≈ 580 cases**, on every PR. Its skip list is **one file**, with the comment noting that the historical skip list *"was retired by the protoc-35.1 compat fixes"* — i.e. **the harness is what drove convergence**.
- protocompile runs ~460 table-driven accept/reject cases through both compilers and asserts the *decisions* agree, with an explicit field `expectedDiffWithProtoc` carrying **22 active entries**, each pinned to an upstream protoc bug URL, each asserted to *still* diverge so that a protoc fix breaks the build and forces a re-sync.

This is the correct reading of "protoc is not in the pipeline": **protoc is not at the user's build time; it is very much in protop's test loop.** That is the one sentence protoc-as-CI-oracle deserves, and it is worth acting on immediately rather than later, because a harness retrofitted after 5,000 lines of parser cannot tell you which of those lines were ever right.

The other transferable practice: **carry source spans on every AST node, including every scalar inside an option literal, from the first commit.** The diagnostics are the product (see the validation section), and retrofitting spans into a literal tree built without them is a miserable refactor.

### 10.5 The risk that actually materialises

Not subtle divergence — **feature lag**. Two data points:

- protobuf-net issue **#1211**: *"the latest version of `descriptor.proto` that it can parse successfully is from Protobuf 26.1"* — the managed parser fell roughly nine protobuf releases behind on language surface.
- protocompile's stable compiler is frozen at `MaxSupportedEdition = EDITION_2023` (`internal/editions/editions.go:36-45`), and buf's answer to "support edition 2024" (issues #743, #764) was *"only available in the `experimental` compiler… we don't have plans to support it in the original."* Their response to accumulated drift was a **43,026-line ground-up rewrite**, now 1.6× the size of the original.

**Mitigation: scope protop to a frozen language subset and say so explicitly in the docs.** A profile that accepts editions 2023 and nothing newer is an honest, defensible product statement. A profile that claims to accept "protobuf" and silently lags is not.

One more datum worth acting on before writing any parser: `protobuf-net.Reflection` already achieves byte-exact `FileDescriptorSet` parity with protoc 35.1 across ~580 cases, in managed C#, with an embedded WKT set. **The cheapest experiment available is pointing it at a representative corpus of the real schemas and running its own harness** — that tells you in an afternoon whether an existing managed parser already covers the surface protop needs, and turns "write a parser" into "write a validator and an emitter over someone else's descriptors" if it does. Its known limitations are real (it discards comments unconditionally at `Parsers.cs:1270`, it has open issues on `import public` with custom options — #968, open since 2022 — and it has never been tested here against editions), so this is an experiment, not a recommendation. But it is a cheap experiment with a large payoff.

---

## 11. Implementation language

**Recommendation: C#.** Under the settled architecture this is close to over-determined, and the reasoning is short.

The usual argument for Go is `google.golang.org/protobuf/compiler/protogen` — a first-party plugin SDK giving you a fully-linked descriptor graph, comments, source locations, identifier mangling, and an import-managing file writer. Its decisive feature is that it **re-links extensions it has never seen**, via `dynamicpb`: `protogen.go` (~lines 390–413) detects novel extensions, re-marshals every descriptor and re-unmarshals with a resolver that knows them (`typeRegistry.registerAllExtensionsFromFile`, then `dynamicpb.NewExtensionType(xd)` at line 1741). Nothing in C# or Rust does that. It is why `protoc-gen-go`, `protoc-gen-go-grpc` and `grpc-gateway` are all built on it.

**None of that applies here**, for two independent reasons:

1. **protoc is not in the pipeline**, so there is no `CodeGeneratorRequest` to parse and no plugin protocol to implement. `protogen`'s entire value is being the best harness for a protoc plugin.
2. **protop's option surface is closed.** It reads its own vocabulary, which it defines, embeds, and parses itself. The dynamic-extension problem — C#'s one genuine, decisive weakness in this space — never arises. (For the record, the weakness is real: `Google.Protobuf` has no `DynamicMessage`, `UnknownFieldSet` has no public enumerator, `Extension<TTarget,TValue>` requires a compile-time `FieldCodec<TValue>`, and `FieldDescriptor.CustomOptions` is marked `[Obsolete]` in favour of `GetOptions()`, which only returns known fields. If protop had to surface arbitrary third-party options, Go would win outright.)

Rust is the weakest of the three regardless. `prost` has no plugin driver and discards unknown fields entirely — `MessageOptions` in `prost-types/src/protobuf.rs` carries declared fields plus `uninterpreted_option` and nothing else. `rust-protobuf` has a real driver (`compiler_plugin::plugin_main`) but is on a 4.0-alpha line and its own `protobuf/src/ext.rs` header says: *"This is a stopgap implementation, it only allows to fetch basic singular values, and that's it… Anything similar to extension registry is not implemented yet"*, with `ExtFieldRepeated::get` literally `unimplemented!()`.

### 11.1 The decisive argument is distribution

Measured, not estimated. `Grpc.Tools` 2.83.0:

| | bytes |
|---|---|
| nupkg total | **23,904,464** (~22.8 MiB) |
| `build/_protobuf/netstandard2.0/Protobuf.MSBuild.dll` | **24,576** |
| six `protoc` binaries (win x86/x64, linux x86/x64/arm64, osx x64) | 44,321,220 uncompressed |
| six `grpc_csharp_plugin` binaries | 17,695,708 uncompressed |

**48 KB of managed task code; 99.8% of the package is native payload across six platform folders.** That is the standing cost of choosing a compiled language for a .NET build tool — and it is paid *twice*, because a protoc plugin is a separate executable, which is exactly why `grpc_csharp_plugin` ships six times alongside protoc six times.

Against that, the managed comparison, also measured from nuget.org:

| package | bytes | native binaries |
|---|---|---|
| `protobuf-net.Reflection` 3.4.21 | 502,266 | 0 |
| `protobuf-net.Core` 3.4.21 (carries `analyzers/dotnet/cs/protobuf-net.BuildTools.dll`, 1,225,216 B) | 1,289,976 | 0 |
| `protobuf-net.Protogen` 3.4.21 (`PackAsTool`) | 1,055,146 | 0 |

And the costs that do not appear in a size column: macOS codesigning and notarization for unsigned Mach-O binaries executed out of a NuGet cache, Windows SmartScreen, antivirus false positives, the executable bit not surviving NuGet extraction, and a six-way cross-compile matrix on every version bump — forever.

### 11.2 The existence proof

`protobuf-net.BuildTools` is precisely the shape protop should take: `[Generator] public sealed class ProtoFileGenerator : ISourceGenerator` (`Generators/ProtoFileGenerator.cs`), targeting `netstandard2.0`, packed as `analyzers/dotnet/cs`, embedding `google/protobuf/*.proto` as resources, taking `.proto` files as MSBuild `AdditionalFiles` with per-file knobs as item metadata (`ImportPaths`, `Package`, `OneOf`, `NullWrappers`, `ListSet`, …). **Zero native binaries, zero protoc, one managed DLL** — and it reaches every consumer of `protobuf-net.Core` by default.

### 11.3 Shipping shape

Three viable forms, in order:

1. **A Roslyn source generator** (`netstandard2.0`, `analyzers/dotnet/cs`). `.proto` files arrive as `AdditionalFiles`; generated records never hit disk; IDE feedback is live; diagnostics are real Roslyn diagnostics with real spans and real suppression. This is the best developer experience available and the smallest artifact.
2. **An MSBuild `ToolTask`** (`netstandard2.0`, or `net472`+`net8.0` multi-targeted with a `$(MSBuildRuntimeType)`-conditioned `UsingTask AssemblyFile=`, exactly as `Grpc.Tools` does). Worth it when generated files should be visible on disk and checked in.
3. **A `PackAsTool` DotnetTool** (`<PackAsTool>true</PackAsTool>`, `<ToolCommandName>praguec</ToolCommandName>`) for CLI and CI use. One caveat that bites: .NET tools are framework-dependent and **roll-forward does not cross major version boundaries**, so a `net10.0` praguec will not run on an agent that has only the .NET 8 runtime. `protobuf-net.Protogen` sets `<RollForward>LatestMajor</RollForward>` precisely for this; `dotnet tool install --allow-roll-forward` (SDK 9+) is the other half. Ship all three from one core assembly and this is a non-issue for the primary path, since a source generator loads into a compiler process that is already running.

The core parser/validator/emitter should be one `netstandard2.0` library with no MSBuild, Roslyn or CLI dependencies, so all three shells are thin.

And since the generator will be emitting C# that other people's hot paths run through: the output should honour the project's high-performance .NET conventions — `sealed` records, `readonly record struct` for the struct tier (whole-type `readonly`, so `in` parameters never take a defensive copy), no LINQ in generated code, and `decimal` construction via `new decimal(lo, mid, hi, isNegative, scale)` rather than parsing (§2.6).

---

## 12. Roadmap: views and projections

Deferred to a later phase, and recorded here for one reason: **it is a forward constraint on phase-one decisions, not a phase-one feature.** Two phase-one recommendations already exist solely because of it (no field-number density rule; `MAX_TAG_BOUND` bounds the span instead), and one architectural line is drawn by it.

### 12.1 The construct

```protop
view BetSummary of Bet { id, stake, status }
```

Three prior-art readings, all valid: a SQL view, a GraphQL fragment, a TypeScript `Pick`.

### 12.2 Settled: a view preserves the parent's tags

A view keeps each retained field's original number and simply skips the ones it drops. Everything follows from this:

- **Wire compatibility is free and by construction.** A `BetSummary` payload deserializes correctly as a `Bet` and vice versa, because both protobuf and MessagePack-CSharp tolerate the mismatch (protobuf ignores unknown fields; MessagePack skips extra array slots and leaves missing ones defaulted — verified in `DynamicObjectResolver.cs`, `default: reader.Skip()` at line 915 and `if (length <= memberIndex) goto MEMBER_ASSIGNMENT_END;`).
- **Adding a field to `Bet` is a non-event** for every view that does not keep it.
- The alternative — renumbering a view densely from 1 — reintroduces exactly the Smithy/alloy failure mode described below, at the view layer.
- Cost: views have holes in their key space and lose the 1-byte-tag advantage for numbers above 15. That is the right trade, and it is why `MAX_TAG_BOUND` bounds the span rather than requiring density.

TypeSpec independently arrived at the same answer, though by accident rather than design: `@field(n)` travels with a property through `PickProperties`/`OmitProperties`/spread, so a picked model inherits the parent's numbers. It is the right behaviour; TypeSpec simply never wrote down that it was a decision.

### 12.3 The negative result worth heeding

Smithy's proto translation (`disneystreaming/smithy-translate`, alloy's `@protoIndex`) makes `@protoIndex` **optional** with a **positional fallback** — `findFieldIndex(m).getOrElse(fieldCount + 1)`. So declaration order is the wire contract, and **adding one field to a shared mixin silently renumbers every field of every consuming message.** A wire break with no diagnostic. Worse, the mixin × `protoIndex` path has **zero test coverage in either repository** (grep for `mixin` across `smithy-translate/modules/proto/**` and alloy's proto validator, src and test: no matches).

Two rules fall out for protop: **never infer a field number positionally**, and **when a construct generates types, test the generating construct, not just the generated shape.**

### 12.4 Why views are the first genuinely justified Tier-2 syntax

The test from the struct/record section — *annotation → option, generation → syntax* — resolves cleanly here, and the asymmetry is worth stating explicitly because it is the whole reason a Tier-2 tier exists at all:

- `option (protop.csharp_kind) = STRUCT;` **annotates a message that already exists.** protobuf's option mechanism is exactly the right vehicle; stock protoc parses the file and ignores the meaning.
- `view BetSummary of Bet {...}` **brings a new type into existence.** There is no message for an option to hang on. Encoding it in Tier 1 would mean writing `BetSummary` out longhand with a `(protop.view_of)` option — at which point the option is a comment on hand-maintained duplication, and the construct has bought nothing.

So views are the first construct where new syntax pays for itself. That is also the moment `.prague` files appear, and the moment protop needs a Tier-2→Tier-1 lowering with a source map — the machinery the rest of the design deliberately avoids.

### 12.5 Design notes for when the phase arrives

- **Keep-list, not drop-list, as the idiom.** A drop-list is dangerous by default: add a field to `Bet` and it silently appears in `BetSummary`, which for a PII-redaction view is a data leak. A keep-list is additive-safe. Support both if you must; make keep the one people reach for.
- **Copy field options and doc comments verbatim.** Dropping `(protop.decimal)` would make `BetSummary.stake` a bare `sint64` that the generator maps differently from `Bet.stake` — a latent bug that type-checks.
- **Forbid renaming and re-typing.** The moment a view can rename or re-type, it is no longer a projection: it is a mapping DSL that needs runtime conversion code, not just a type. A field number is the field's identity; a view that changes what a number means has broken its own premise.
- **Views of views: allow, resolve to the root, detect cycles.** protoc will never see a cycle, so protop must.
- **Reserve nothing in the view.** Reserving the dropped numbers would block a future change from re-adding a field at its original number, and reserving-and-using are mutually exclusive.
- **Prefer `[Key(string)]` for wide-span views.** A view that keeps tags 3 and 400 emits a 400-slot array with 398 `WriteNil()` calls — and the AOT source generator emits one literal statement per gap. String keys switch to a map layout with no padding.

### 12.6 What protobuf's own ecosystem did instead

`google.protobuf.FieldMask` and AIP-157 are the sanctioned answers, and they are **runtime** answers: a mask passed as a side-channel parameter, or a `view` enum field with values like `BASIC`/`FULL`. AIP-157's own rule — *"APIs **may** add fields to a given view over time. APIs **must not** remove a field"* — is the same append-only discipline in a different clothing.

**Note what Google did not do: they did not define separate view message types.** Both patterns keep one message and one number space and vary what is populated at runtime.

That is the right call for an RPC surface where the same resource is returned at different fidelities by the same call. It is the wrong call here, for a specific reason: a runtime mask produces a nullable-everything client type with no compile-time guarantee, whereas protop's entire output is a **smaller, non-nullable C# type**. A compile-time derived message is the only one of the two that produces that. The cost — a second type to keep in sync — is exactly what `viewof` automates away.

---

## 13. Should you just use TypeSpec instead?

An honest answer, because it is the cheapest possible outcome of this whole exercise if it is yes.

**It is no, and the reasons are structural rather than matters of taste.** Evidence: `microsoft/typespec` @ `daad49a`, `@typespec/protobuf` v0.86.0.

TypeSpec is a mature, well-tooled IDL with models, decorators, namespaces, templates, a VS Code extension, a formatter, a playground, and built-in composition templates (`PickProperties<Source, Keys>`, `OmitProperties<Source, Keys>`, `OptionalProperties<Source>`, spread, `is`, `extends`). Its protobuf emitter writes `.proto` **text** from its own hand-rolled AST (`packages/protobuf/src/ast.ts`, `src/write.ts` — *"This module defines how to emit the text representation of a ProtoFile AST"*). If the goal were "a nicer front end that emits proto", TypeSpec would be a serious candidate and writing a compiler would be hard to justify.

Four findings kill it for this project:

**1. There is no way to emit a custom option — at all.** An exhaustive grep for `extension|custom option` across `packages/protobuf/{src,lib}` and the README returns **zero hits**. `ProtoFieldDeclaration` has an `options?: Partial<DefaultFieldOptions>` slot, but `DefaultFieldOptions = { packed: true; deprecated: true }` and `writeField` (`src/write.ts:179-184`) **never emits options at all** — the field is dead code. The only `option` output in the entire emitter is file-level, from `@package({options: {...}})`, restricted to string, boolean and numeric values.

Tier 1 — the protop option vocabulary — is the entire mechanism by which semantic intent reaches the C# generator. **TypeSpec structurally cannot express it.**

**2. `oneof` cannot be emitted.** `ProtoOneOfDeclaration` is defined in `ast.ts:280` and `writeOneOf` exists in `write.ts:151`, but the string `oneof` appears **zero times** in `src/transform/index.ts` — nothing ever constructs one. Unions are a hard error: `unsupported-field-type` / *"a message field's type may not be a union"*.

**3. `decimal` is a hard error.** TypeSpec *has* `decimal` and `decimal128` as built-in scalars, but the emitter's scalar table `getProtoScalarsMap` (`src/transform/index.ts:592-606`) contains only `bytes, boolean, string, int32, int64, uint32, uint64, float32, float64` plus the `Protobuf.*` fixed/sint/sfixed variants. `decimal`'s base scalar is `numeric`, which has no base, so `scalarToProto` bottoms out at `unsupported-field-type/unknown-scalar`. Given that the domain is betting and wallets, this alone is disqualifying.

**4. It is target-only: there is no `.proto` parser anywhere in the monorepo.** `git grep -l "parseProto\|protoParser" -- packages/` returns nothing. `Extern<Path, Name>` is explicitly opaque — TypeSpec emits an `import` and a bare name and never reads the referenced file. There is an OpenAPI3→TypeSpec importer (`packages/openapi3/src/cli`); there is no protobuf analogue. So existing `.proto` cannot be onboarded, migrated incrementally, or diffed against generated output.

A fifth point worth knowing, though it is an argument about correctness rather than fit: **the emitter has no duplicate-field-number check.** `@field(n)` travels with a property through spread, `is`, and the Pick/Omit templates (verified in compiler source: `checkSpreadProperty` → `cloneTypeForSymbol` → `initializeClone`, which copies `decorators: [...type.decorators]`). That is exactly right for views — a picked model inherits the parent's numbers and is wire-compatible for free — and exactly wrong for mixins, where two spreads both carrying `@field(1)` produce a silent collision that only protoc catches later, pointing at the *generated* file. Note also that inverting the usual claim: in TypeSpec, **the emitter is the semantic authority and protoc only sees the output afterwards**, so you inherit the same source-mapping problem protop would have, with less control over it.

Activity signal: `@typespec/protobuf`'s changelog shows `0.81`→`0.86` as "No changes, version bump only" except 0.82 (optional mapping) and 0.86 (a packaging fix). Maintained, not actively developed.

**The one genuinely interesting middle path:** `@typespec/protobuf`'s `ast.ts` + `write.ts` together are a clean, small (~600 LOC) `.proto` text emitter with an `options` slot already modelled and `writeOneOf` already written. If protop ever wants a `.proto` emitter — for the optional Tier-2→Tier-1 lowering, or for interop — that is the best-shaped prior art to read, and it is a much smaller delta than it looks. Worth noting also that **no official descriptor→`.proto` text emitter exists in .NET**: `protobuf-net.Reflection` is text→descriptor only (no `SchemaWriter`, no `--proto_out` in `protogen`), the only official emitter anywhere is C++ `FileDescriptor::DebugStringWithOptions()`, and the most complete is Go's third-party `protoreflect/protoprint`. If a `.proto` emitter is ever needed, it is being written from scratch in .NET regardless.

---

## 14. Suggested phasing, and where the findings say to deviate

The phasing you proposed is sound in shape:

| Phase | Content |
|---|---|
| 1 | messages, field numbers → `[Key(N)]`, plain `.proto` input, generate records |
| 2 | the custom-option vocabulary; the struct-vs-record distinction |
| 3 | views and other Tier-2 sugar |

Three things in it are harder to reverse than the phase boundary suggests, and one is cheaper to do early than late. These are the deviations the research argues for.

### 14.1 Deviation 1 — the lint engine belongs in phase 1, not phase 2

Strictness rules are **monotonically expensive to add**. Every rule added after users have files makes previously-valid files invalid; every rule present from the first release is simply "how the tool works". This is the one part of the design where being late costs real money, and it is why buf shipped `config-ignore-yaml` (§9) — a migration path is a product feature for a strictness tool, and it only exists because they added rules late.

`FIELD_TAGS_ASCENDING` specifically must be in phase 1, because phase 1 is where the generated record shape is fixed and the rule is what makes positional records safe: MessagePack-CSharp matches constructor parameters **by ordinal over key-sorted members** (§7.5), so a generated positional record's parameter order is the key order whether the author intended it or not. Adding the rule in phase 2 means either breaking phase-1 files or living with a generator that silently depends on an unenforced invariant.

Corollary: the diagnostic infrastructure — codes, severities, source spans, `--error-format` — is phase-1 infrastructure. §6 already argues spans must be carried from the first commit for a different reason; this is the second.

### 14.2 Deviation 2 — decimal cannot wait for phase 2

The domain is betting and wallets. A phase-1 tool that accepts

```proto
double stake = 3;
```

and cheerfully emits `public double Stake { get; init; }` will have that in production before phase 2 ships, and the resulting `.proto` files and payloads become the thing phase 2 has to stay compatible with. The finding in §2 is that protobuf has no decimal and never will; the whole `(protop.decimal)` mechanism exists to close that.

Two acceptable answers, in order of preference:

1. **Ship `(protop.decimal)` in phase 1.** It is one option message and one codegen branch — far smaller than the rest of the vocabulary, and it is the single option that most justifies the tool's existence.
2. **At minimum, make `double`/`float` a phase-1 *error* on any field the author has not explicitly opted into** (or an unsuppressable warning). Refusing to generate is a reversible decision; generating the wrong type is not.

This also means the phase-1/phase-2 line is not "no options in phase 1". It is "one option in phase 1".

### 14.3 Deviation 3 — decide the `[Key(N)]` offset before phase 1 ships, and write it down

Field numbers start at 1; MessagePack int keys are array indices starting at 0. `Key = tag` wastes slot 0 forever on every message; `Key = tag - 1` is dense but makes the two number spaces differ by one in every error message, every hand-written interop reader, and every debugging session.

Either is defensible. Neither is changeable after the first payload is persisted — it is a silent, total wire break with no version marker to detect it. Treat this as a phase-0 decision with a written rationale in the repo, not a phase-1 implementation detail. (§7.2 has the measurement: the cost of the wasted slot is exactly one byte of `Nil` per message, forever.)

### 14.4 Things the phasing gets right, and should not be second-guessed

- **`.proto` input in phase 1, no new syntax.** §12 (views) establishes that Tier-2 syntax only pays for itself when a construct *generates a type*, and nothing in phases 1–2 does. Shipping `.prague` early would buy nothing and commit you to a lowering pass and a source map.
- **Struct-vs-record in phase 2, not phase 1.** §8 concludes the struct tier pays off substantially in C# shape and only marginally in MessagePack, and that `csharp_kind` is immutable once published — a struct is a frozen contract with a `default(T)` hazard that has no fix. That is the right thing to ship second, once the record path is proven.
- **Views last.** They need the most machinery and they are the only feature with a settled design already (tag-preserving, §12), so they lose nothing by waiting.

### 14.5 One addition: a phase 0

Two days of work before phase 1 that the rest of the plan depends on:

1. **The protoc differential harness** (§10). Point a throwaway parser — or `protobuf-net.Reflection` — at a real `.proto` corpus and diff the resulting descriptors against `protoc --descriptor_set_out`. This answers "how big is the parser really" empirically rather than by estimate, and the harness is permanent test infrastructure either way. protoc is not in protop's pipeline; it is very much in protop's test loop.
2. **Freeze the accepted subset and write it down as a list.** §10's real risk is feature lag, not parser size: protobuf keeps shipping editions and protop will not track them. A one-page "protop accepts exactly this" document, with an explicit *"editions after 2023 are rejected with `PROTOP0001`"*, converts an open-ended maintenance liability into a closed one. It also makes the "profile of protobuf" framing true by construction rather than by aspiration.

---

## Sources

All primary. Specs and official documentation first, then source repositories with the commit or version actually read.

### Protocol Buffers — specification and official documentation

- Language guide (proto3): <https://protobuf.dev/programming-guides/proto3/> — §§ `#scalar`, `#field-labels`, `#assigning`, `#field-presence`, `#oneof`, `#unknowns`, `#updating`
- Language guide (proto2), custom options: <https://protobuf.dev/programming-guides/proto2/#customoptions>
- Field presence: <https://protobuf.dev/programming-guides/field_presence/>
- Editions: <https://protobuf.dev/editions/overview/>, <https://protobuf.dev/editions/features/>
- proto3 language specification (grammar): <https://protobuf.dev/reference/protobuf/proto3-spec/> — `#message_definition`, `#map_field`
- Well-known types reference: <https://protobuf.dev/reference/protobuf/google.protobuf/>
- ProtoJSON `int64`-as-string: <https://protobuf.dev/programming-guides/json/#int64-strings>
- Global extension-number registry: <https://github.com/protocolbuffers/protobuf/blob/main/docs/options.md>
- `descriptor.proto`: <https://github.com/protocolbuffers/protobuf/blob/main/src/google/protobuf/descriptor.proto> — `FieldOptions`, `MessageOptions`, `FeatureSet`, `UninterpretedOption`, `SourceCodeInfo`
- Decimal support issue #4406: <https://github.com/protocolbuffers/protobuf/issues/4406>; prototype PR #7847 (closed unmerged)
- `protoc` C++ implementation read for option interpretation and `DebugStringWithOptions()`: `src/google/protobuf/descriptor.cc`, `src/google/protobuf/compiler/parser.cc`

### Google API design surfaces

- `google/type/money.proto`: <https://github.com/googleapis/googleapis/blob/master/google/type/money.proto>
- `google/type/decimal.proto`: <https://github.com/googleapis/googleapis/blob/master/google/type/decimal.proto>
- `google/api/field_behavior.proto`: <https://github.com/googleapis/googleapis/blob/master/google/api/field_behavior.proto>
- `google/api/resource.proto`: <https://github.com/googleapis/googleapis/blob/master/google/api/resource.proto>
- AIP-157, partial responses / view enums: <https://google.aip.dev/157>
- `google.protobuf.FieldMask`: <https://protobuf.dev/reference/protobuf/google.protobuf/#field-mask>

### MessagePack

- MessagePack specification: <https://github.com/msgpack/msgpack/blob/master/spec.md>
- MessagePack-CSharp README, object serialization: <https://github.com/MessagePack-CSharp/MessagePack-CSharp#object-serialization>
- MessagePack-CSharp source @ `b0e5cee3e30e75004ea95e154ec3832027a46090` (v3.1.8, MIT). Files read: `src/MessagePack/Resolvers/DynamicObjectResolver.cs` (key-sorted member emit, `maxKey + 1` padding, `reader.Skip()`, `length <= memberIndex` early exit), `src/MessagePack/Resolvers/ContractlessReflectionObjectResolver.cs`, `src/MessagePack/Attributes.cs` (`MessagePackObjectAttribute`, `KeyAttribute`, `SerializationConstructorAttribute`), `src/MessagePack/Resolvers/NativeDecimalResolver.cs`, the AOT generator under `src/MessagePack.SourceGenerator/`

### .NET

- `System.Decimal`: <https://learn.microsoft.com/en-us/dotnet/api/system.decimal> (bit layout, `new decimal(lo, mid, hi, isNegative, scale)`, `"G"` vs `"R"`, parse rounding at 29 digits)
- Choosing between class and struct: <https://learn.microsoft.com/en-us/dotnet/standard/design-guidelines/choosing-between-class-and-struct> ("under 16 bytes"; "three words or less" appears in the C# language reference guidance, **not** the widely-quoted "24 bytes")
- Records: <https://learn.microsoft.com/en-us/dotnet/csharp/language-reference/builtin-types/record>
- gRPC and Protobuf in .NET: <https://learn.microsoft.com/en-us/aspnet/core/grpc/protobuf>
- MSBuild diagnostic format and `TaskLoggingHelper.LogError`: <https://learn.microsoft.com/en-us/visualstudio/msbuild/logging-in-a-multi-processor-environment>, <https://learn.microsoft.com/en-us/dotnet/api/microsoft.build.utilities.tasklogginghelper.logerror>
- Grpc.Tools 2.83.0 `.nupkg`, unpacked and measured (23,904,464 bytes total; 24,576 bytes of managed task assembly; six platform folders under `tools/`)

### Prior art — custom options as schema metadata

- protoc-gen-validate: <https://github.com/bufbuild/protoc-gen-validate/blob/main/validate/validate.proto>
- protovalidate: <https://github.com/bufbuild/protovalidate/blob/main/proto/protovalidate/buf/validate/validate.proto>; migration guide and shipped rewriter `tools/protovalidate-migrate`: <https://protovalidate.com/migration-guides/migrate-from-protoc-gen-validate/>
- protoc-gen-gorm: <https://github.com/infobloxopen/protoc-gen-gorm/blob/master/proto/options/gorm.proto>
- entproto: <https://github.com/ent/contrib/blob/master/entproto/cmd/protoc-gen-ent/options/ent/opts.proto>

### buf — lint, breaking-change detection, and the parser

- buf source @ `05bb117580e9f40e011564a3cb46ff58bfded71b`. Read: `private/bufpkg/bufcheck/buflint/` (rule catalogue and categories `STANDARD`/`DEFAULT`/`BASIC`/`MINIMAL`), `private/buf/bufcli/`, the `buf:lint:ignore` implementation (`const lintCommentIgnorePrefix = "buf:lint:ignore"`, matched with `strings.HasPrefix`; comma-separated lists silently fail to match), `--error-format` output writers, `config-ignore-yaml` migration path
- `bufbuild/protocompile` — `parser/lexer.go`, `parser/proto.y`, `parser/result.go`, `linker/resolve.go`, `linker/validate.go`, `options/options.go` (the `// TODO: this is insane logic` comment at `options.go:1530`), 22 `expectedDiffWithProtoc` test entries
- buf lint rule documentation: <https://buf.build/docs/lint/rules/>
- buf breaking rule documentation: <https://buf.build/docs/breaking/rules/> (custom options are not compared)

### .NET protobuf parsing prior art

- `protobuf-net` @ `src/protobuf-net.Reflection` — schema parser, ≈8,000 lines C#; differential tests against protoc (`Assert.Equal(protocHex, parserHex)` over ~580 schema cases); no `.proto` text emitter exists in the repository
- `protobuf-net.BuildTools` — `[Generator] ... : ISourceGenerator`, `netstandard2.0`, packaged under `analyzers/dotnet/cs`, consumes `.proto` through `AdditionalFiles`

### FlatBuffers — the struct/table model

- FlatBuffers schema documentation: <https://flatbuffers.dev/schema/> and evolution guide <https://flatbuffers.dev/evolution/>
- `google/flatbuffers` — `src/idl_parser.cpp`, and `ConformTo` in `src/idl_parser.cpp` where `value.offset` means a byte offset for a struct and a vtable slot for a table
- FlatSharp (<https://github.com/jamescourtney/FlatSharp>, Apache-2.0) — `fs_valueStruct` is opt-in, so an `fbs` struct maps to a C# **class** by default; the value tier is `[StructLayout(LayoutKind.Explicit)]` with public mutable fields and hand-written equality (`// This matches C# records`), not a `record struct`; `MemoryMarshal.Cast` fast path

### TypeSpec

- `microsoft/typespec` @ `daad49a`; `@typespec/protobuf` v0.86.0. Read: `packages/protobuf/src/ast.ts` (`ProtoFieldDeclaration.options`, `DefaultFieldOptions = { packed, deprecated }`, `ProtoOneOfDeclaration`), `packages/protobuf/src/write.ts` (`writeField` at lines 179–184 emits no options; `writeOneOf` at line 151 is never called), `packages/protobuf/src/transform/index.ts` (`getProtoScalarsMap` at 592–606; no `oneof` construction; `unsupported-field-type` diagnostics), `packages/compiler/src/core/checker.ts` (`checkSpreadProperty` → `cloneTypeForSymbol` → `initializeClone`, which copies `decorators`)
- TypeSpec built-in scalars and templates: <https://typespec.io/docs/standard-library/built-in-types/>, <https://typespec.io/docs/standard-library/built-in-utility-types/>

### Smithy

- `disneystreaming/smithy-translate` — `modules/proto/**`, `findFieldIndex(m).getOrElse(fieldCount + 1)` positional fallback; no `mixin` test coverage in either `src` or `test`
- `disneystreaming/alloy` — `@protoIndex` trait definition and its validator; no `mixin` coverage

### GraphQL

- GraphQL specification, fragments: <https://spec.graphql.org/October2021/#sec-Language.Fragments>
