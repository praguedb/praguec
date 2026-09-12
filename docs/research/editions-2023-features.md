# The Editions 2023 feature surface `protopc` must resolve

Primary-source research for issue #4. Every claim cites either a file and line in real source, an official doc page, or a reproducible `protoc` invocation. Compiler output is marked `[protoc 36.1]` and every such result in this document was produced by running the command shown — the reproduction script is in §10.

**Environment.** `protoc` 36.1 (Homebrew `protobuf@36`). `descriptor.proto` line numbers refer to `/opt/homebrew/include/google/protobuf/descriptor.proto` as shipped by that release, which is byte-identical to `src/google/protobuf/descriptor.proto` at tag `v36.1`. Implementation line numbers refer to `src/google/protobuf/descriptor.cc` and `src/google/protobuf/feature_resolver.cc` at tag `v36.1`.

One caveat on the version: protoc 36.1 knows editions 2023, 2024 and 2026. This is *useful* — it lets us see exactly which features exist but are locked at 2023 — but it also means the `descriptor.proto` read here is newer than the one that shipped with edition 2023. Where that matters it is called out. Nothing in the 2023 *surface* has changed; the additions are all gated by `edition_introduced` (§2.3).

---

## Bottom line

**1. The edition 2023 feature surface is exactly six features, and all six are settable by a protop author.** `field_presence`, `enum_type`, `repeated_field_encoding`, `utf8_validation`, `message_encoding`, `json_format`. There are three further fields in `FeatureSet` — `enforce_naming_style`, `default_symbol_visibility`, `enforce_proto_limits` — that exist in the message but are **hard-locked at edition 2023**: protoc rejects any attempt to set them with an explicit error, and their 2023 values are fixed constants. That is not a style guideline; it is the compiler (§2.3). protopc must therefore also reject them, with the same reasoning, or it diverges from the oracle on a *diagnostic*.

**2. The resolution algorithm is four lines and has no surprises except two.** `merged = editionDefaults; merged.MergeFrom(parentMerged); merged.MergeFrom(thisNodesExplicitFeatures)` — a proto3 `MergeFrom` over singular enum fields, i.e. last-set-wins per field (`feature_resolver.cc:523-533`). The two surprises: **a `oneof` is the parent of its fields**, not the message; and **an extension's parent is its declaring scope, not the extendee** (`descriptor.cc:1243-1256`). A naive file→message→field walk gets both wrong.

**3. The decisive finding for the differential harness: `--descriptor_set_out` contains *no resolved features at all*.** It is a faithful round-trip of what the author wrote, at the scope they wrote it, and nothing else. protobuf.dev states it outright — *"The resulting descriptor protos should be a faithful representation of the original proto files, and should contain **unresolved features** inside of the options"* (<https://protobuf.dev/editions/implementation/>) — and the implementation confirms it: `ResolveFeaturesImpl` *strips* `options.features` into a side table (`descriptor.cc:5168-5172`) and `RestoreFeaturesToOptions` puts back only `proto_features_`, the as-written set (`descriptor.cc:1198-1203`). Verified empirically (§6). **Consequence: the harness cannot verify protopc's feature resolution against protoc at all.** It can only verify that protopc *echoes* the same explicit features. Resolution correctness needs its own oracle — see §9.4, which proposes one that works.

**4. protoc erases the two post-processing rewrites on the way out, so the descriptor lies about `label` and `type` relative to the in-memory descriptor.** In the `DescriptorPool`, `LEGACY_REQUIRED` rewrites `label_` to `LABEL_REQUIRED` and `DELIMITED` rewrites `type_` to `TYPE_GROUP` (`descriptor.cc:5222-5242`). But `FieldDescriptor::CopyTo` explicitly undoes both for editions files (`descriptor.cc:3130-3147`), so the serialized descriptor shows `LABEL_OPTIONAL` and `TYPE_MESSAGE`. protopc must emit the *undone* form. This is a trap that looks like a bug in your emitter for a full afternoon.

**5. Custom features and custom options are different mechanisms, and protop's 50000–50099 range can only be the latter.** `FeatureSet` extension numbers are restricted to `1000 to 9994`, `9995 to 9999`, and `10000` (`descriptor.proto:1250-1291`). 50000 is rejected outright: `"google.protobuf.FeatureSet" does not declare 50000 as an extension number` `[protoc 36.1]`. So `(protop.*)` at 50000+ are extensions of `FieldOptions`/`MessageOptions`/etc., which **do not inherit** — no file→message→field cascade, no `edition_defaults`, no `feature_support` lifetimes. If protop wants inheritable metadata it has to implement that cascade itself; it cannot borrow the feature machinery (§7).

**6. Exactly one feature touches presence, and no feature touches field numbering.** `field_presence` is the entire presence axis and maps directly onto the C# nullability thesis (§8.1). Nothing — not `DELIMITED`, not `PACKED`, not `LEGACY_REQUIRED` — changes a field's number, so `[Key(N)] = tag - 1` is feature-invariant (§8.2). The one adjacent hazard is that `IMPLICIT` makes a scalar field indistinguishable from default-valued, which collides with MessagePack-CSharp's inability to distinguish `default(T)` from a written zero — the same hazard already documented for the struct tier.

**7. Recommended profile, for the human to decide:** accept and honour `field_presence` and `enum_type`; accept `utf8_validation`; **reject** `message_encoding = DELIMITED`, `repeated_field_encoding = EXPANDED`, `json_format = LEGACY_BEST_EFFORT`, and `field_presence = LEGACY_REQUIRED` at file scope. Reasoning per feature in §9.1. Two of these rejections (`DELIMITED`, `EXPANDED`) are wire-format knobs with no MessagePack analogue — honouring them would be meaningless and rejecting them costs nothing.

---

## 1. The `FeatureSet` message: the complete field inventory

`FeatureSet` is declared at `descriptor.proto:1060-1291`. Nine singular fields, numbers 1–9, plus a reserved 999 and three extension ranges.

| # | Field | Type | `edition_introduced` | Available in 2023? |
|---|---|---|---|---|
| 1 | `field_presence` | `FieldPresence` | `EDITION_2023` | **yes** |
| 2 | `enum_type` | `EnumType` | `EDITION_2023` | **yes** |
| 3 | `repeated_field_encoding` | `RepeatedFieldEncoding` | `EDITION_2023` | **yes** |
| 4 | `utf8_validation` | `Utf8Validation` | `EDITION_2023` | **yes** |
| 5 | `message_encoding` | `MessageEncoding` | `EDITION_2023` | **yes** |
| 6 | `json_format` | `JsonFormat` | `EDITION_2023` | **yes** |
| 7 | `enforce_naming_style` | `EnforceNamingStyle` | `EDITION_2024` | no — locked |
| 8 | `default_symbol_visibility` | `VisibilityFeature.DefaultSymbolVisibility` | `EDITION_2024` | no — locked |
| 9 | `enforce_proto_limits` | `ProtoLimitsFeature.EnforceProtoLimits` | `EDITION_2026` | no — locked |

Source: `descriptor.proto:1067-1246`. The `feature_support { edition_introduced: ... }` annotation on each field is the authority; §2.3 shows what "locked" means in practice.

**There is no tenth field and no hidden surface.** `FeatureSet` reserves 999 and then goes straight to extension ranges (`descriptor.proto:1248-1291`). Everything else that looks like a feature — `(pb.cpp).string_type`, `(pb.java).large_enum`, `(pb.go).api_level` — is an *extension* of `FeatureSet` owned by a language runtime, not part of the language surface (§7.1).

### 1.1 Enum values, verbatim

```
FieldPresence:           FIELD_PRESENCE_UNKNOWN=0  EXPLICIT=1  IMPLICIT=2  LEGACY_REQUIRED=3
EnumType:                ENUM_TYPE_UNKNOWN=0       OPEN=1      CLOSED=2
RepeatedFieldEncoding:   REPEATED_FIELD_ENCODING_UNKNOWN=0  PACKED=1  EXPANDED=2
Utf8Validation:          UTF8_VALIDATION_UNKNOWN=0  VERIFY=2  NONE=3   (1 is reserved)
MessageEncoding:         MESSAGE_ENCODING_UNKNOWN=0  LENGTH_PREFIXED=1  DELIMITED=2
JsonFormat:              JSON_FORMAT_UNKNOWN=0     ALLOW=1     LEGACY_BEST_EFFORT=2
```

`descriptor.proto:1061-1158`. Note `Utf8Validation` skips 1 — `reserved 1;` at `descriptor.proto:1115`. A hand-rolled parser that assumes contiguous enum numbering will assign the wrong wire value to `NONE`.

Every `*_UNKNOWN = 0` value is **not a legal resolved value**. `ValidateMergedFeatures` rejects any feature that resolves to `_UNKNOWN` with *"Feature field `X` must resolve to a known value, found X_UNKNOWN"* (`feature_resolver.cc:285-300`). protopc's resolver should assert the same invariant at the end of resolution; it can only trip if the defaults table is wrong.

### 1.2 Where `features` lives on each options message

protopc has to write these field numbers into its descriptor emitter. They are not uniform:

| Options message | `features` field number | `descriptor.proto` line |
|---|---|---|
| `FileOptions` | 50 | 572 |
| `MessageOptions` | 12 | 669 |
| `FieldOptions` | 21 | 817 |
| `OneofOptions` | 1 | 864 |
| `EnumOptions` | 7 | 903 |
| `EnumValueOptions` | 2 | 927 |
| `ServiceOptions` | 34 | 958 |
| `MethodOptions` | 35 | 1010 |
| `ExtensionRangeOptions` | 50 | 207 |

---

## 2. The edition 2023 defaults, from the defaults mechanism

### 2.1 The authoritative dump

protoc has an undocumented-in-`--help` flag, `--edition_defaults_out`, which emits a `FeatureSetDefaults` message. It is the mechanism, not prose. protobuf.dev documents it: *"Alternatively, you can invoke protoc directly (outside of Bazel) to generate this data: `protoc --edition_defaults_out=defaults.binpb`"* (<https://protobuf.dev/editions/implementation/>).

```
protoc --edition_defaults_out=/tmp/ed.bin \
       --proto_path=/opt/homebrew/include google/protobuf/descriptor.proto
protoc --decode=google.protobuf.FeatureSetDefaults \
       --proto_path=/opt/homebrew/include google/protobuf/descriptor.proto < /tmp/ed.bin
```

The `EDITION_2023` entry, verbatim `[protoc 36.1]`:

```
defaults {
  edition: EDITION_2023
  overridable_features {
    field_presence: EXPLICIT
    enum_type: OPEN
    repeated_field_encoding: PACKED
    utf8_validation: VERIFY
    message_encoding: LENGTH_PREFIXED
    json_format: ALLOW
  }
  fixed_features {
    enforce_naming_style: STYLE_LEGACY
    default_symbol_visibility: EXPORT_ALL
    enforce_proto_limits: LEGACY_NO_EXPLICIT_LIMITS
  }
}
```

**This single block is the answer to "what are the 2023 defaults".** It also answers, for free, the question of which features are locked: the split between `overridable_features` and `fixed_features` *is* that distinction, computed mechanically from `edition_introduced`/`edition_removed` in `FillDefaults` (`feature_resolver.cc:237-283`).

### 2.2 The reference table

| Feature | 2023 default | Overridable at 2023? | Settable on | Emitted to descriptor when set? |
|---|---|---|---|---|
| `field_presence` | `EXPLICIT` | yes | file, field | yes, verbatim |
| `enum_type` | `OPEN` | yes | file, enum | yes, verbatim |
| `repeated_field_encoding` | `PACKED` | yes | file, field | yes, verbatim |
| `utf8_validation` | `VERIFY` | yes | file, field | yes, verbatim |
| `message_encoding` | `LENGTH_PREFIXED` | yes | file, field | yes, verbatim |
| `json_format` | `ALLOW` | yes | file, message, enum | yes, verbatim |
| `enforce_naming_style` | `STYLE_LEGACY` | **no** | — | n/a (cannot be set) |
| `default_symbol_visibility` | `EXPORT_ALL` | **no** | — | n/a (cannot be set) |
| `enforce_proto_limits` | `LEGACY_NO_EXPLICIT_LIMITS` | **no** | — | n/a (cannot be set) |

Defaults column: the dump in §2.1. "Settable on" column: the `targets` annotations at `descriptor.proto:1067-1158`, confirmed empirically in §5.1. "Emitted" column: §6.

Cross-check against the prose doc at <https://protobuf.dev/editions/features/> — it agrees on every row, and also agrees on the locked features' 2023 values (`STYLE_LEGACY`, `EXPORT_ALL`, `LEGACY_NO_EXPLICIT_LIMITS`). That is a genuine two-source confirmation, since the doc is written by hand and the dump is generated.

### 2.3 What "locked" means, and why protopc must reproduce it

Not a warning. A hard error `[protoc 36.1]`:

```
edition = "2023";
option features.enforce_naming_style = STYLE_LEGACY;
→ google.protobuf.FeatureSet.enforce_naming_style wasn't introduced until
  edition 2024 and can't be used in edition 2023

edition = "2023";
option features.default_symbol_visibility = EXPORT_ALL;
→ google.protobuf.FeatureSet.default_symbol_visibility wasn't introduced until
  edition 2024 and can't be used in edition 2023
```

Note that both of these attempts set the feature to *exactly its locked 2023 value* and are still rejected. The check is on the act of setting, not on the value. Source: `feature_resolver.cc:326-341`, `ValidateFeatureSupport`.

This matters to protop even though protop's dialect is frozen at 2023 and its authors will never write these lines: a protopc that silently accepts `features.enforce_naming_style` is accepting a file protoc rejects, which is a dialect divergence in the permissive direction. Those are the expensive ones.

### 2.4 How the defaults themselves are derived — and the trap in the encoding

The defaults in §2.1 are not written anywhere as a table. They are computed from `edition_defaults` annotations on each `FeatureSet` field, and the encoding is sparse in a way that is easy to misread. From `descriptor.proto:1084-1093`:

```proto
optional EnumType enum_type = 2 [
  ...
  edition_defaults = { edition: EDITION_LEGACY, value: "CLOSED" },
  edition_defaults = { edition: EDITION_PROTO3, value: "OPEN" }
];
```

There is **no `EDITION_2023` entry**, yet the 2023 default is `OPEN`. The rule is "closest matching edition at or before" (`FillDefaults`, `feature_resolver.cc:265-276`: sort by edition, `upper_bound`, take `prev`). The `Edition` enum is deliberately ordered by value: `EDITION_LEGACY = 900`, `EDITION_PROTO2 = 998`, `EDITION_PROTO3 = 999`, `EDITION_2023 = 1000` (`descriptor.proto:45-82`). So 2023 falls through to the `PROTO3` entry.

Five of the six live features resolve this way at 2023:

| Feature | Entry that actually supplies the 2023 value |
|---|---|
| `field_presence` | explicit `EDITION_2023` entry → `EXPLICIT` (`descriptor.proto:1076`) |
| `enum_type` | falls through to `EDITION_PROTO3` → `OPEN` (`:1092`) |
| `repeated_field_encoding` | falls through to `EDITION_PROTO3` → `PACKED` (`:1108`) |
| `utf8_validation` | falls through to `EDITION_PROTO3` → `VERIFY` (`:1125`) |
| `message_encoding` | falls through all the way to `EDITION_LEGACY` → `LENGTH_PREFIXED` (`:1140`) |
| `json_format` | falls through to `EDITION_PROTO3` → `ALLOW` (`:1157`) |

`field_presence` is the only one needing an explicit 2023 entry, because it is the only one where edition 2023 differs from proto3 (`EXPLICIT` vs `IMPLICIT`) — which is, in one line, the whole point of editions for this project.

**Implementation guidance.** protopc should not reimplement the `edition_defaults` fallthrough. It is frozen at one edition; it should hard-code the nine resolved constants from §2.1, and carry the dump command in a test that asserts they still match. That converts a subtle algorithm into a one-line regression test.

---

## 3. The resolution algorithm

### 3.1 The merge

`feature_resolver.cc:523-533`, in full:

```cpp
absl::StatusOr<FeatureSet> FeatureResolver::MergeFeatures(
    const FeatureSet& merged_parent, const FeatureSet& unmerged_child) const {
  FeatureSet merged = defaults_;
  merged.MergeFrom(merged_parent);
  merged.MergeFrom(unmerged_child);
  RETURN_IF_ERROR(ValidateMergedFeatures(merged));
  return merged;
}
```

Three facts make this simple:

- `defaults_` is the per-edition `FeatureSet`, built as `fixed_features` then `MergeFrom(overridable_features)` (`feature_resolver.cc:632-633`). For 2023 that is exactly the nine values in §2.1.
- `merged_parent` is the parent's **already fully resolved** set. Resolution is a single top-down pass; each node merges onto its parent's result, never re-walking ancestors.
- `unmerged_child` is only this node's **explicitly written** features. Every field of `FeatureSet` is a singular (optional) enum, so `MergeFrom` copies a field only if it is *set* on the source. Unset fields leave the accumulator alone.

So the effective precedence, per feature, is: **nearest enclosing scope that explicitly set it wins; if none did, the edition default.** Setting a feature to the same value it already had is a no-op semantically but is still recorded (§6.3).

### 3.2 The parent chain

`descriptor.cc:1221-1275`. This is the part worth copying verbatim into protopc, because two of the edges are counter-intuitive.

| Node | Parent for feature resolution | Line |
|---|---|---|
| file | **none** — the empty `FeatureSet` | 1221-1223 |
| top-level message | its file | 1226-1228 |
| nested message | its **containing message** | 1229-1230 |
| oneof | its containing message | 1233-1236 |
| extension range | its containing message | 1238-1241 |
| field in a oneof | **the oneof** | 1244-1246 |
| top-level extension | **its declaring file** | 1248-1250 |
| nested extension | **its declaring (`extend`-enclosing) message** | 1251-1252 |
| ordinary field | its containing message | 1254-1256 |
| top-level enum | its file | 1259-1260 |
| nested enum | its containing message | 1261-1262 |
| enum value | its enum | 1265-1267 |
| service | its file | 1269-1271 |
| method | its service | 1273-1275 |

**A file's parent is the empty set, not anything from its imports.** `GetParentFeatures(const FileDescriptor*)` returns `FeatureSet::default_instance()` (`descriptor.cc:1221-1223`). Features never cross a file boundary. An imported message keeps the features of *its* file. For protop this means a `.protop` file is a self-contained feature universe, which is a good property and should be stated in the spec rather than left to be discovered.

**An extension's parent is where it is declared, not what it extends** (`descriptor.cc:1247-1253`). protobuf.dev flags this as *"a bit surprising"* (<https://protobuf.dev/editions/implementation/>). If protop ever allows `extend`, this is the rule.

**A oneof sits between a message and its fields** (`descriptor.cc:1244-1246`). Even though almost no feature can be *set* on a oneof (§5.1 — every one of the six is rejected on `TARGET_TYPE_ONEOF`), the oneof is still a real link in the chain. Today it is a pure pass-through; a naive implementation that skips it is accidentally correct and will break if that ever changes. Model it properly.

### 3.3 The file node is special-cased

`ResolveFeatures(const FileDescriptorProto&, ...)` calls `ResolveFeaturesImpl` with `force_merge = true` (`descriptor.cc:5211-5218`), with the comment *"File descriptors always need their own merged feature set, even without any explicit features."* Every other node takes a fast path: if it set nothing, it aliases its parent's resolved set rather than allocating a copy (`descriptor.cc:5185-5189`). That is a memory optimisation, not a semantic difference — but it means the file node is the one place where "resolved = defaults" must be materialised.

### 3.4 Implementable pseudocode

```
DEFAULTS_2023 = {                      # §2.1, hard-coded
  field_presence: EXPLICIT, enum_type: OPEN,
  repeated_field_encoding: PACKED, utf8_validation: VERIFY,
  message_encoding: LENGTH_PREFIXED, json_format: ALLOW,
  enforce_naming_style: STYLE_LEGACY,
  default_symbol_visibility: EXPORT_ALL,
  enforce_proto_limits: LEGACY_NO_EXPLICIT_LIMITS,
}

resolve(node, parentResolved):
    explicit = node.options.features          # only what the author wrote
    rejectIfAnyLockedFeatureSet(explicit)     # §2.3
    rejectIfTargetNotAllowed(node, explicit)  # §5.1
    merged = parentResolved.copy()
    for each feature f set in explicit:
        merged[f] = explicit[f]
    assert no field of merged is *_UNKNOWN    # feature_resolver.cc:285-300
    node.resolved = merged
    for child in featureChildrenOf(node):     # §3.2 parent table, inverted
        resolve(child, merged)

resolve(file, DEFAULTS_2023)
```

Two things this pseudocode gets right that a first draft usually does not: the recursion carries `merged`, never the defaults, so the chain is genuinely transitive; and `explicit` is read from the node's own options *before* those options are rewritten for output (§6.1).

### 3.5 Ordering within a file

Feature resolution requires descriptors to be resolved parent-before-child, which falls out of the recursion above. But protopc also needs `message_encoding` and `field_presence` validation to know a field's *type*, which requires name resolution to have completed (`PostProcessFieldFeatures` does the `DELIMITED` check via `LookupSymbol`, `descriptor.cc:5236-5239`). So the phase order is: parse → build symbol table → resolve names → resolve features → validate features. Resolving features before names will work for five of the six and fail on `message_encoding`.

---

## 4. Feature lifetimes

`FieldOptions.FeatureSupport` (`descriptor.proto:820-843`) carries `edition_introduced`, `edition_deprecated`, `deprecation_warning`, `edition_removed`, `removal_error`. At edition 2023 the only one that fires is `edition_introduced` (§2.3); nothing is deprecated or removed at 2023.

There is a second lifetime check protopc should know exists even if it will never trigger for a frozen 2023 dialect: `ValidateFeatureLifetimes` also validates **enum values** of a feature, not just the feature field (`feature_resolver.cc:615-625`). So a feature could gain a new enum value in a later edition and setting *that value* would be rejected earlier, even though the feature itself is available. No 2023 feature has per-value lifetimes today — **unverified** whether any is planned.

---

## 5. Scope validity: what can be set where

Two independent layers of validation, and protopc needs both.

### 5.1 Layer 1 — `targets`, the entity-kind check

Each feature carries `targets = TARGET_TYPE_*` annotations (`descriptor.proto:1067-1158`; the enum is at `:792-803`). Setting a feature on an entity kind not in its `targets` is an error naming the kind. Verified exhaustively `[protoc 36.1]`:

| Attempted | Result |
|---|---|
| `field_presence` on a **message** | `Option google.protobuf.FeatureSet.field_presence cannot be set on an entity of type 'message'.` |
| `field_presence` on a **oneof** | `... cannot be set on an entity of type 'oneof'.` |
| `enum_type` on an **enum value** | `... cannot be set on an entity of type 'enum entry'.` |
| `json_format` on a **field** | `... cannot be set on an entity of type 'field'.` |
| `json_format` on a **oneof** | `... cannot be set on an entity of type 'oneof'.` |
| `json_format` on a **service** | `... cannot be set on an entity of type 'service'.` |
| `json_format` on an **extension range** | `... cannot be set on an entity of type 'extension range'.` |
| `enum_type` on an **enum** | accepted |
| `json_format` on file / message / enum | accepted |

The resulting matrix for the six live features — this is the table protopc's validator encodes:

| | file | msg | field | oneof | enum | enum val | ext range | service | method |
|---|---|---|---|---|---|---|---|---|---|
| `field_presence` | ✓ | | ✓ | | | | | | |
| `enum_type` | ✓ | | | | ✓ | | | | |
| `repeated_field_encoding` | ✓ | | ✓ | | | | | | |
| `utf8_validation` | ✓ | | ✓ | | | | | | |
| `message_encoding` | ✓ | | ✓ | | | | | | |
| `json_format` | ✓ | ✓ | | | ✓ | | | | |

**No feature in edition 2023 is settable on a oneof, enum value, extension range, service or method.** Those five scopes exist in the inheritance chain (§3.2) but are, at 2023, pure conduits. The `enforce_naming_style` feature is the one that targets all nine — and it is locked at 2023.

### 5.2 Layer 2 — semantic checks on the field's shape

Separate from `targets`, `DescriptorBuilder::ValidateOptions(const FieldDescriptor*, ...)` applies checks against the field's actual type and label (`descriptor.cc:8164-8210`). Every one of these fires on `proto_features_` — the *explicitly set* features — not on the resolved value. That distinction is load-bearing and is the subject of §5.3.

Verified `[protoc 36.1]`:

| Attempted | Error |
|---|---|
| `field_presence` on a field inside a oneof | `Oneof fields can't specify field presence.` |
| `field_presence` on a `repeated` field | `Repeated fields can't specify field presence.` |
| `field_presence = IMPLICIT` on a message-typed field | `Message fields can't specify implicit presence.` |
| `field_presence` on an extension (unless `LEGACY_REQUIRED`) | `Extensions can't specify field presence.` (`descriptor.cc:8176-8184`) |
| `repeated_field_encoding` on a non-repeated field | `Only repeated fields can specify repeated field encoding.` |
| `repeated_field_encoding = PACKED` on `repeated string` | `Only repeated primitive fields can specify PACKED repeated field encoding.` |
| `utf8_validation` on an `int32` field | `Only string fields can specify utf8 validation.` |
| `message_encoding` on a scalar field | `Only message fields can specify message encoding.` |
| `message_encoding = DELIMITED` on a `map<string, M>` field | `Only message fields can specify message encoding.` |

Plus one enum-level check, `descriptor.cc:8218-8224`: an **open** enum's first declared value must be zero — `The first enum value must be zero for open enums.` A **closed** enum has no such requirement (verified: `enum E { option features.enum_type = CLOSED; E_A = 1; }` compiles). Since 2023's default is `OPEN`, the practical effect is that the zero-first rule applies by default and is lifted only by an explicit `CLOSED`.

### 5.3 The asymmetry worth writing down

These checks run on **explicitly set** features only, which produces a deliberate asymmetry: a file-level `option features.utf8_validation = NONE;` is legal even in a file full of `int32` fields, because each `int32` field merely *inherits* it and never *sets* it. Only a field that writes `[features.utf8_validation = NONE]` on an `int32` is rejected. Confirmed both in source (`descriptor.cc:8164`, all branches guarded by `field->proto_features_->has_*()`) and empirically — the §6 test file sets `features.utf8_validation = NONE` at file scope over a message containing `int32` fields and compiles clean.

protopc must reproduce this asymmetry exactly. A validator that checks the *resolved* value against the field's type would reject files protoc accepts. This is the single most likely place for a hand-rolled implementation to be *stricter than the oracle by accident*.

Generated map entry fields get an explicit bypass — `descriptor.cc:8155-8162`, *"Skip validation of explicit features on generated map fields. These will be blindly propagated from the original map field."*

### 5.4 Syntax the editions grammar removes

Not features, but the errors protopc must produce for the same inputs, and they point at features `[protoc 36.1]`:

```
optional int32 a = 1;
→ Label "optional" is not supported in editions. By default, all singular
  fields have presence unless features.field_presence is set.

required int32 a = 1;
→ Label "required" is not supported in editions, use
  features.field_presence = LEGACY_REQUIRED.

group G = 1 { ... }
→ Group syntax is no longer supported in editions. To get group behavior you
  can specify features.message_encoding = DELIMITED on a message field.
```

`repeated` survives. `optional`, `required` and `group` do not.

---

## 6. Descriptor observability — the decisive finding

### 6.1 The mechanism

`ResolveFeaturesImpl` removes features from the options proto as it resolves them (`descriptor.cc:5166-5172`):

```cpp
if (options->has_features()) {
  // Remove the features from the child's options proto to avoid leaking
  // internal details.
  descriptor->proto_features_ =
      tables_->InternFeatureSet(std::move(*options->mutable_features()));
  options->clear_features();
}
```

The resolved set goes into a *separate* member, `merged_features_` (`descriptor.cc:5200`). Serialization back to proto calls `RestoreFeaturesToOptions(proto_features_, proto)` (`descriptor.cc:2768, 3031, 3098, 3190, 3202, 3227, 3237, 3250, 3276`) — nine call sites, one per descriptor kind — and that function writes back `proto_features_`, the **as-written** set, and only when it is non-empty (`descriptor.cc:1198-1203`):

```cpp
template <typename ProtoT>
void RestoreFeaturesToOptions(const FeatureSet* features, ProtoT* proto) {
  if (features != &FeatureSet::default_instance()) {
    *proto->mutable_options()->mutable_features() = *features;
  }
}
```

`merged_features_` is never serialized. It is reachable only through `InternalFeatureHelper`, an internal C++ API.

This matches the spec: *"The resulting descriptor protos should be a faithful representation of the original proto files, and should contain unresolved features inside of the options"* (<https://protobuf.dev/editions/implementation/>).

### 6.2 Verified empirically

Compiling a file that sets three features at file scope and six at field scope, then decoding the `--descriptor_set_out` `[protoc 36.1]`, the output contains exactly the nine settings that were written and nothing else. Every field that set nothing carries **no `options` at all** — not an empty `features {}`, not a materialised default. From the §10 test file:

```
field { name: "a"  number: 1  label: LABEL_OPTIONAL  type: TYPE_INT32  json_name: "a" }
field { name: "b"  number: 2  label: LABEL_OPTIONAL  type: TYPE_INT32
        options { features { field_presence: LEGACY_REQUIRED } }  json_name: "b" }
...
options { features { field_presence: IMPLICIT  enum_type: CLOSED  utf8_validation: NONE } }
syntax: "editions"
edition: EDITION_2023
```

A nested message and a nested enum under a file that set `json_format: LEGACY_BEST_EFFORT` emit **no options whatsoever** — the inherited value is nowhere in the descriptor.

### 6.3 A redundant explicit setting is preserved verbatim

Setting a feature to a value it already inherits is *not* elided:

```proto
edition = "2023";
option features.field_presence = EXPLICIT;      // == the 2023 default
message M {
  int32 a = 1 [features.field_presence = EXPLICIT];       // == inherited
  repeated int32 r = 2 [features.repeated_field_encoding = PACKED];  // == default
}
```

produces `options { features { field_presence: EXPLICIT } }` on the file *and* on field `a`, and `repeated_field_encoding: PACKED` on `r` `[protoc 36.1]`. The descriptor is a transcript, not a minimisation.

**For the harness this is good news of a narrow kind:** it means the comparison is well-defined and normalisation does not need to decide whether to elide redundant settings. It also means protopc must *not* helpfully optimise them away.

### 6.4 The `label` / `type` round-trip trap

In the `DescriptorPool`, `PostProcessFieldFeatures` rewrites two things (`descriptor.cc:5222-5242`):

```cpp
if (field.features().field_presence() == FeatureSet::LEGACY_REQUIRED &&
    field.label_ == FieldDescriptor::LABEL_OPTIONAL) {
  field.label_ = FieldDescriptor::LABEL_REQUIRED;
}
if (field.type_ == FieldDescriptor::TYPE_MESSAGE && ... &&
    field.features().message_encoding() == FeatureSet::DELIMITED) {
  ... field.type_ = FieldDescriptor::TYPE_GROUP;
}
```

Note these read `field.features()` — the **resolved** set. So a field that inherits `LEGACY_REQUIRED` from its file is required in memory.

But `FieldDescriptor::CopyTo` undoes both, specifically for editions files (`descriptor.cc:3130-3147`):

```cpp
if (is_required() && !IsLegacyEdition(file()->edition())) {
  // Editions files have no required keyword, and we only set this label
  // during descriptor build.
  proto->set_label(LABEL_OPTIONAL);
} else { ... }
if (type() == TYPE_GROUP && !IsLegacyEdition(file()->edition())) {
  proto->set_type(TYPE_MESSAGE);
} else { ... }
```

Verified `[protoc 36.1]` — a `LEGACY_REQUIRED` field emits `label: LABEL_OPTIONAL`, and a `DELIMITED` message field emits `type: TYPE_MESSAGE`:

```
field { name: "a" number: 1 label: LABEL_OPTIONAL type: TYPE_INT32
        options { features { field_presence: LEGACY_REQUIRED } } json_name: "a" }
field { name: "d" number: 2 label: LABEL_OPTIONAL type: TYPE_MESSAGE type_name: ".M"
        options { features { message_encoding: DELIMITED } } json_name: "d" }
```

So the descriptor's `label` and `type` are the *pre-feature* values. protopc, emitting straight from its own AST, gets this right by doing nothing — which is exactly why it is worth writing down: the bug appears only if someone "helpfully" applies the feature to the emitted label.

**Contrast with legacy syntax.** For a `syntax = "proto2"` file, `IsLegacyEdition` is true (`descriptor.cc:1277-1279`: `edition < EDITION_2023`), so the rewrite is *not* undone: a `required` field emits `LABEL_REQUIRED` and a `group` emits `TYPE_GROUP` `[protoc 36.1]`. And the features that protoc *inferred* from the legacy syntax (`InferLegacyProtoFeatures`, `descriptor.cc:5124-5148`, which sets `LEGACY_REQUIRED` for `required`, `DELIMITED` for groups, and a `repeated_field_encoding` from `[packed]`) are **not emitted either** — they go into `base_features`, never into `proto_features_`. A proto2 descriptor from protoc 36.1 contains zero `features` blocks. Not protop's problem directly, but it is the reason the differential harness cannot use proto2 fixtures to learn anything about features.

### 6.5 Option retention also affects the descriptor

Orthogonal to features but decisive for protop's own `(protop.*)` options: `--descriptor_set_out` **strips `RETENTION_SOURCE` options by default**. Verified `[protoc 36.1]` with three custom options — `(protop.key) = 50000` (default retention), `(protop.note) = 50001` (`retention = RETENTION_SOURCE`), `(protop.packed_map) = 50002` (default):

```
options { 50000: 7 }          # 50001 absent
options { 50002: 1 }
```

With `--retain_options`, 50001 reappears. protoc documents the flag: *"When using --descriptor_set_out, do not strip any options from the FileDescriptorProto."*

**Harness consequence:** the differential harness must fix a retention policy and use it on both sides. If protop declares any option with `RETENTION_SOURCE` and the harness does not pass `--retain_options`, protopc must strip it too, or every fixture using that option diverges. The safer default is to pass `--retain_options` and compare everything.

---

## 7. Custom features vs custom options

These are **two different mechanisms** and the distinction decides where protop's 50000–50099 range can live.

### 7.1 Custom features: extensions of `FeatureSet`

Declared as `extend google.protobuf.FeatureSet { ... }`. They participate fully in resolution — inheritance, `edition_defaults`, `feature_support` lifetimes — and `FeatureResolver::CompileDefaults` folds them into the `FeatureSetDefaults` message (`feature_resolver.cc:409-479`). This is how `(pb.cpp)`, `(pb.java)`, `(pb.go)`, `(pb.csharp)` work.

**The extension numbers are a closed, curated set.** `descriptor.proto:1250-1291`:

```proto
extensions 1000 to 9994 [ declaration = { number: 1000, full_name: ".pb.cpp", ... },
                          declaration = { number: 1001, full_name: ".pb.java", ... },
                          declaration = { number: 1002, full_name: ".pb.go", ... },
                          declaration = { number: 1003, full_name: ".pb.python", ... },
                          declaration = { number: 1004, full_name: ".pb.csharp", ... },
                          declaration = { number: 1100, full_name: ".imp.impress_feature_set", ... },
                          declaration = { number: 9989, full_name: ".pb.java_mutable", ... },
                          declaration = { number: 9990, full_name: ".pb.proto1", ... } ];
extensions 9995 to 9999;  // For internal testing
extensions 10000;         // for https://github.com/bufbuild/protobuf-es
```

Verified `[protoc 36.1]`:

| Extension number on `FeatureSet` | Result |
|---|---|
| 1005 (in range, not declared) | accepted |
| 9995 (internal testing range) | accepted |
| 10000 (protobuf-es) | accepted |
| 10001 | `"google.protobuf.FeatureSet" does not declare 10001 as an extension number.` |
| **50000** | `"google.protobuf.FeatureSet" does not declare 50000 as an extension number.` |

(1005 is accepted because `ExtensionRangeOptions.verification` defaults to `UNVERIFIED` — `descriptor.proto:219-220` — so the `declaration` list is advisory rather than enforced within the range. The *range bound* is what is enforced, and 50000 is outside every range.)

**So protop cannot define custom features at 50000–50099.** Full stop. If protop ever genuinely wanted an inheriting feature it would need a number in 1000–9994, which means asking the protobuf team for a registration — the `declaration` list is a public registry — and inheriting a whole second axis of `edition_defaults`/`feature_support` obligations. Not worth it.

### 7.2 Custom options: extensions of the `*Options` messages

Declared as `extend google.protobuf.FieldOptions { ... }` etc. `FieldOptions` has `extensions 1000 to max` (`descriptor.proto:853`). Verified: `int32 key = 50000;` on `FieldOptions`, `bool packed_map = 50002;` on `MessageOptions`, both accepted and both round-tripped through `--descriptor_set_out` `[protoc 36.1]`.

**This is the mechanism protop's 50000–50099 range uses**, and the differences from custom features are the whole point:

| | custom feature | custom option |
|---|---|---|
| Extends | `google.protobuf.FeatureSet` | `FileOptions`, `MessageOptions`, `FieldOptions`, … |
| Number range | 1000–9994 / 9995–9999 / 10000 only | 1000 to max (50000–50099 fine) |
| Registration | public `declaration` registry | none |
| **Inherits file→message→field** | **yes, automatically** | **no** |
| Has an edition default | yes, via `edition_defaults` | no — absent means absent |
| Lifetime gating | yes, via `feature_support` | no |
| In `--descriptor_set_out` | unresolved, as written | as written (retention permitting, §6.5) |

**The consequence protop has to plan for:** there is no free cascade. If `(protop.decimal) { scale: 4 }` should be settable at message scope and inherited by fields, protopc implements that cascade itself. Since protopc already implements the feature cascade (§3.4), the pragmatic move is to write *one* generic inheritance walker parameterised by a "which parent" function, and run both the nine built-in features and protop's own options through it. The parent chain in §3.2 is the right chain for both.

### 7.3 Nothing in `pb.csharp` is available at 2023

Worth recording since the project's output is C#: `CSharpFeatures` has exactly one field, `nullable_reference_types`, with `edition_introduced: EDITION_2026` (`c_sharp_features.proto:26-36`). At edition 2023 it is locked at `false`. So the official C# feature extension offers protop nothing, and protop's nullability mapping is entirely its own.

---

## 8. Interactions with presence and field numbering

### 8.1 Presence: `field_presence` is the whole axis

Resolved `field_presence` has three values and each has a distinct C# consequence:

| Resolved value | Wire / semantics | Natural C# mapping |
|---|---|---|
| `EXPLICIT` (2023 default) | hasbit; can distinguish unset from default-valued | nullable — `int?`, `string?` |
| `IMPLICIT` | no presence; default-valued is indistinguishable from unset; not serialized when default | non-nullable with a default — `int`, `string` |
| `LEGACY_REQUIRED` | required for parsing *and* serialization | non-nullable, and a parse error if absent |

Values and semantics from `descriptor.proto:1061-1066` and <https://protobuf.dev/editions/features/>.

Four consequences for protop, in descending order of importance:

**(a) Edition 2023's default is `EXPLICIT`, and that is the project's thesis working as advertised.** Under proto3 the default was `IMPLICIT` (`descriptor.proto:1075`) and the only escape was the `optional` keyword's synthetic-oneof trick. Under 2023, every singular field has presence unless someone opts out. The default C# mapping is therefore nullable, and non-nullability is the annotated case — which is the right way round for a schema language.

**(b) `IMPLICIT` is not available on message-typed fields.** `Message fields can't specify implicit presence.` (`descriptor.cc:8185-8190`) — message fields always have presence. So a message-typed C# property is always nullable, with no author override. That simplifies the mapping and should be stated as a rule rather than derived per-field.

**(c) A field in a `oneof` cannot specify presence and always has it.** `Oneof fields can't specify field presence.` (§5.2). And `repeated` fields cannot specify it either. So the presence axis applies to exactly one category: singular, non-oneof, non-extension fields.

**(d) The `IMPLICIT` + MessagePack hazard.** With `IMPLICIT`, a zero is not serialized on the protobuf wire. MessagePack-CSharp's int-key layout writes a dense array padded to `maxKey + 1`, and on read it cannot distinguish a written zero from a `default(T)` slot. So an `IMPLICIT` field is the one case where the two formats agree by accident — both collapse "unset" and "zero" — and an `EXPLICIT` field is the case where MessagePack needs a nullable to carry the distinction that protobuf's hasbit carries. This is the same hazard already documented for the struct tier (`default(T)` is all-zero and indistinguishable), arriving from a second direction. If protop honours `IMPLICIT`, the generated C# must use a non-nullable property and the two formats stay aligned; if protop honours `EXPLICIT` (the default), it must use a nullable. Getting this backwards is silent data corruption, not a compile error.

### 8.2 Field numbering: no feature touches it

**No feature in edition 2023 changes a field's number.** Checked against each:

- `message_encoding = DELIMITED` changes the wire *type* (group encoding, `SGROUP`/`EGROUP` instead of `LEN`) but the field number in the tag is unchanged.
- `repeated_field_encoding = EXPANDED` emits one tag per element instead of one `LEN` record — same field number on every tag.
- `field_presence = IMPLICIT` changes whether a field is emitted at all, never its number.
- `enum_type`, `utf8_validation`, `json_format` are value-domain and format concerns with no tag involvement.

So `[Key(N)] = tag - 1` is **feature-invariant**, and the open question in the earlier research doc (`Key = tag` vs `Key = tag - 1`) is not affected by anything here. The mapping depends only on `FieldDescriptorProto.number`, which no feature can reach.

The one *indirect* interaction is §8.1(d): features do not change which key a field gets, but `field_presence` changes what a default-valued slot at that key *means*.

---

## 9. What this means for protop

### 9.1 Accept / reject, per feature

The decision is the human's. This is the evidence and a recommendation.

**`field_presence` — accept and honour. Restrict `LEGACY_REQUIRED`.**
It is the project's core axis (§8.1) and the reason to be on editions at all. `EXPLICIT` and `IMPLICIT` both map cleanly. `LEGACY_REQUIRED` is the interesting one: it gives protop a real `NOT NULL` with parse-time enforcement, which is genuinely valuable for a schema language — but it is a permanent, unremovable wire contract (removing a required field breaks every old reader), and protobuf's own history is that `required` was the single worst mistake in proto2. Recommendation: **allow `LEGACY_REQUIRED` at field scope only, never at file scope.** A file-level `LEGACY_REQUIRED` makes every singular field in the file required by inheritance, which is a foot-gun of a size no one intends. protoc permits it; a strict profile should not.

**`enum_type` — accept and honour.**
`OPEN` vs `CLOSED` is a real schema decision with a real C# consequence: a closed enum can reject out-of-range values at parse time, an open one stores them. It costs protopc almost nothing (the only extra rule is the zero-first check for open enums, §5.2) and it is exactly the kind of constraint a schema language should be able to express. MessagePack has no unknown-field set, so a `CLOSED` enum's "store out-of-range in unknowns" semantics have no MessagePack analogue — which means protop's `CLOSED` should be interpreted as **reject at parse**, and that difference must be documented rather than silently inherited.

**`utf8_validation` — accept, low stakes.**
`VERIFY` (the 2023 default) versus `NONE`. MessagePack's `str` type is nominally UTF-8 and MessagePack-CSharp does not validate by default — **unverified**, worth checking before relying on it. Honouring `NONE` means "treat as bytes". The cost of accepting is one flag on the emitted property; the cost of rejecting is an author who genuinely has legacy non-UTF-8 string data and no way to say so. Mild recommendation: accept.

**`repeated_field_encoding` — reject.**
Pure protobuf wire concern. In MessagePack a repeated field is an array; there is no packed-vs-expanded distinction to honour. Accepting it means accepting a knob that provably does nothing, which is worse than rejecting it. The only argument for accepting is wire-compatibility with an existing protobuf consumer that requires `EXPANDED` — and if that consumer exists, it is reading protobuf, not MessagePack, so it is outside protop's remit. Reject, with a diagnostic that says why.

**`message_encoding` — reject `DELIMITED`.**
Same reasoning, more strongly. `DELIMITED` is group encoding — the resurrection of a syntax protobuf deleted. It exists in editions purely as a migration path for Google-internal proto1 groups. MessagePack has no groups. And protopc would have to carry the `TYPE_GROUP`/`TYPE_MESSAGE` round-trip complication (§6.4) purely to support a feature it cannot honour. Reject `DELIMITED`; `LENGTH_PREFIXED` is the default and needs no syntax to obtain, so in practice this means rejecting the feature entirely.

**`json_format` — reject.**
`LEGACY_BEST_EFFORT` exists to accommodate proto2 files whose field names or enum values do not round-trip through protobuf JSON. protop emits MessagePack and C#; it has no JSON mapping to weaken. Accepting the feature would be accepting an annotation with no referent. Reject.

**`enforce_naming_style`, `default_symbol_visibility`, `enforce_proto_limits` — reject, and match protoc's wording.**
Not a profile decision — protoc rejects them at edition 2023 already (§2.3). protopc must too, or it is more permissive than the oracle. Worth reproducing protoc's message closely, since the differential harness may compare diagnostics.

**Summary:** accept 3 of 6 (`field_presence` with a restriction, `enum_type`, `utf8_validation`), reject 3 of 6 as wire/format knobs with no MessagePack referent, reject 3 of 3 locked features as protoc does.

A useful framing for the spec: the three accepted features are all **schema semantics** (does this field have a value; is this enum's domain closed; is this string text). The three rejected ones are all **protobuf wire encoding** (how bytes are laid out). protop is a schema language that happens to have a protobuf-shaped syntax, so that line is the natural place to cut, and it is defensible in one sentence to a user who asks why.

### 9.2 What protopc must implement regardless of the profile

Even rejecting half the features, protopc still needs the full resolver, because rejection is a *diagnostic* and diagnostics need resolved context:

1. The nine 2023 constants, hard-coded, with a test asserting they match `--edition_defaults_out` (§2.4).
2. The parent chain of §3.2, including the oneof link and the extension rule.
3. The merge of §3.1 — trivial once the chain is right.
4. `targets` validation (§5.1) and shape validation (§5.2), the latter **only on explicitly set features** (§5.3).
5. Rejection of `edition` values other than `2023`, and of `syntax = "proto2"` / `"proto3"`.
6. A descriptor emitter that writes back only explicitly-set features, at the scope written, undoing nothing (§6.1, §6.4).

Items 4 and 6 are the ones most likely to be skipped in a first pass and most likely to fail the harness.

### 9.3 The harness consequence, stated plainly

The differential harness compares normalised `--descriptor_set_out` from both compilers. Because protoc emits unresolved features (§6), the harness verifies:

- ✅ that protopc **parses** feature syntax identically (right scope, right value, right enum number);
- ✅ that protopc **rejects** the same files protoc rejects, if diagnostics are compared;
- ✅ that protopc emits the same `label` / `type` for `LEGACY_REQUIRED` / `DELIMITED` fields (§6.4) — a real, non-obvious check that the harness gets for free;
- ❌ **nothing at all about whether protopc resolved a single feature correctly.**

A protopc whose resolver inherits `json_format` from the wrong parent, or whose 2023 default for `enum_type` is `CLOSED` instead of `OPEN`, produces a **byte-identical descriptor** to a correct one. The harness is blind to the entire subject of this document.

### 9.4 A second oracle that closes the gap

The gap is closeable, and cheaply, because protoc *will* tell you its resolved values — just not through `--descriptor_set_out`. Three options, in increasing order of cost:

**(a) `--edition_defaults_out` as a defaults-table oracle.** One command, already working (§2.1), asserts the nine constants. Catches a wrong default. Does not catch a wrong parent chain. **Do this on day one** — it is a single test and it pins the most-likely-wrong thing.

**(b) A generated-code differential.** protoc's own C# backend consumes resolved features. A field resolved `IMPLICIT` versus `EXPLICIT` generates observably different `Google.Protobuf` C# — `HasX` properties, `ClearX` methods. Comparing `protoc --csharp_out` against protopc's *understanding* of presence, per field, is an indirect but real check on the resolver. Cost: a fixture harness that greps generated C#, and a mapping that is itself a guess. Medium confidence, medium cost.

**(c) A resolved-feature probe.** A tiny `protoc` plugin — or, since the pipeline is C#, a fixture generated once by a throwaway C++/Python program against the C++ `DescriptorPool` — that walks every descriptor and dumps `InternalFeatureHelper::GetFeatures(d)` as a flat `path → resolved FeatureSet` table. protopc emits the same table. Compare. This is the only option that tests the resolver directly and completely, including the oneof and extension parent edges. Cost: one small non-C# tool, run once per fixture-set change, output checked in. **Unverified** whether `InternalFeatureHelper` is reachable from Python or Go bindings without C++ — worth ten minutes before committing to the design, since it collapses the cost if so.

Recommendation: (a) immediately, (c) when the resolver ships. Skip (b) — it tests the resolver through two layers of guesswork.

---

## 10. Reproduction

Everything marked `[protoc 36.1]` came from these. `PB=/opt/homebrew/include`.

**Edition defaults (§2.1):**
```sh
protoc --edition_defaults_out=/tmp/ed.bin --proto_path=$PB google/protobuf/descriptor.proto
protoc --decode=google.protobuf.FeatureSetDefaults --proto_path=$PB \
       google/protobuf/descriptor.proto < /tmp/ed.bin
```
Narrow to one edition with `--edition_defaults_minimum=2023 --edition_defaults_maximum=2023`. Add `google/protobuf/{cpp,java,go,c_sharp}_features.proto` to the inputs to see the language extensions folded in.

**Descriptor observability (§6):**
```sh
protoc --descriptor_set_out=/tmp/t.desc --proto_path=. --proto_path=$PB t.proto
protoc --decode=google.protobuf.FileDescriptorSet --proto_path=$PB \
       google/protobuf/descriptor.proto < /tmp/t.desc
```

with `t.proto`:

```proto
edition = "2023";
package t;

option features.field_presence = IMPLICIT;
option features.enum_type = CLOSED;
option features.utf8_validation = NONE;

message Plain { int32 a = 1; string b = 2; repeated int32 r = 3; }

message Override {
  option features.json_format = LEGACY_BEST_EFFORT;
  int32 a = 1;
  int32 b = 2 [features.field_presence = LEGACY_REQUIRED];
  int32 c = 8 [features.field_presence = EXPLICIT];
  repeated int32 packed_default = 3;
  repeated int32 expanded = 4 [features.repeated_field_encoding = EXPANDED];
  string s = 5 [features.utf8_validation = VERIFY];
  Plain nested = 6 [features.message_encoding = DELIMITED];
  oneof o { int32 x = 7; }
  enum InnerE { IE_A = 1; }
}

enum E { E_A = 1; }
enum OpenE { option features.enum_type = OPEN; OE_A = 0; }
service S { rpc Do(Plain) returns (Plain); }
```

**Validation matrix (§5):** compile each snippet in §5.1/§5.2 with `--descriptor_set_out=/dev/null` and read stderr.

**Sources.**
- `descriptor.proto` — <https://github.com/protocolbuffers/protobuf/blob/v36.1/src/google/protobuf/descriptor.proto>
- `descriptor.cc` — <https://github.com/protocolbuffers/protobuf/blob/v36.1/src/google/protobuf/descriptor.cc>
- `feature_resolver.cc` — <https://github.com/protocolbuffers/protobuf/blob/v36.1/src/google/protobuf/feature_resolver.cc>
- `c_sharp_features.proto` — <https://github.com/protocolbuffers/protobuf/blob/v36.1/src/google/protobuf/c_sharp_features.proto>
- Editions overview — <https://protobuf.dev/editions/overview/>
- Feature settings — <https://protobuf.dev/editions/features/>
- Implementing editions support — <https://protobuf.dev/editions/implementation/>

**Unverified claims in this document**, collected: (1) whether any 2023 feature is planned to gain per-value lifetime gating (§4); (2) whether MessagePack-CSharp validates UTF-8 on `str` reads (§9.1); (3) whether `InternalFeatureHelper`-equivalent resolved-feature access is reachable from the Python or Go protobuf bindings without writing C++ (§9.4c).
