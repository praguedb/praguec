# Is `LEGACY_REQUIRED` accepted on scalar fields?

Resolves [issue #2](https://github.com/praguedb/praguec/issues/2). Corrects and extends
`docs/research/proto-to-messagepack-csharp.md` §1.3, which only ever exercised message-typed fields.

**Short answer: yes.** `features.field_presence = LEGACY_REQUIRED` is legal on every singular field type —
`int64`, `string`, `bool`, `bytes`, `double`, enum, and message alike. There is **no type restriction at all**
in `protoc`'s validation. The restrictions are *cardinality and scope* restrictions, not type restrictions:
repeated, map, oneof-member and extension fields are rejected, and the value cannot be set as a file- or
message-level default.

---

## 0. Method and toolchain

All `[protoc]` results below were produced by running the real compiler on `.proto` files authored for this
ticket:

```
$ protoc --version
libprotoc 36.1          # homebrew, /opt/homebrew/bin/protoc
```

Descriptors were inspected with
`protoc --descriptor_set_out=x.pb x.proto` followed by
`protoc --decode=google.protobuf.FileDescriptorSet -I /opt/homebrew/include google/protobuf/descriptor.proto < x.pb`.

Runtime behaviour was checked with:

* **C++** — `protoc --encode` / `--decode`, which drive the C++ runtime.
* **Go** — `protoc-gen-go v1.36.12` + `google.golang.org/protobuf v1.36.12`, executed.
* **C#** — `protoc --csharp_out`, generated code inspected; runtime source read at tag `v36.1`.
* **Java** — `protoc --java_out`, generated code inspected. **No JRE on this machine**, so the Java claims
  below are read off generated source, not executed. Marked where relevant.

Source citations are against `protocolbuffers/protobuf` tag **v36.1**
(`https://github.com/protocolbuffers/protobuf/blob/v36.1/...`). `descriptor.proto` line numbers are from the
locally installed copy at `/opt/homebrew/include/google/protobuf/descriptor.proto`, which matches that tag.

---

## 1. Legality matrix

Input file (edition 2023), all seven fields in one message:

```proto
edition = "2023";
package t;

message M {
  int64  i = 1 [features.field_presence = LEGACY_REQUIRED];
  string s = 2 [features.field_presence = LEGACY_REQUIRED];
  bool   b = 3 [features.field_presence = LEGACY_REQUIRED];
  bytes  y = 4 [features.field_presence = LEGACY_REQUIRED];
  double d = 5 [features.field_presence = LEGACY_REQUIRED];
  E      e = 6 [features.field_presence = LEGACY_REQUIRED];
  Sub    m = 7 [features.field_presence = LEGACY_REQUIRED];
}
enum E { E_UNKNOWN = 0; E_A = 1; }
message Sub { int32 x = 1; }
```

`[protoc 36.1]` — compiles clean, exit 0, no diagnostics.

| Construct | Result | Exact diagnostic |
|---|---|---|
| `int64` singular | **accepted** | — |
| `string` singular | **accepted** | — |
| `bool` singular | **accepted** | — |
| `bytes` singular | **accepted** | — |
| `double` singular | **accepted** | — |
| enum singular | **accepted** | — |
| message singular | **accepted** | — |
| singular + `[default = 7]` | **accepted** | — (an explicit default and `LEGACY_REQUIRED` coexist) |
| `repeated int32` | **rejected** | `rep.proto:4:18: Repeated fields can't specify field presence.` |
| `map<string, int32>` | **rejected** | `map.proto:4:22: Repeated fields can't specify field presence.`<br>`map.proto:4:3: map_entry should not be set explicitly. Use map<KeyType, ValueType> instead.` |
| oneof member | **rejected** | `oneof.proto:5:11: Oneof fields can't specify field presence.` |
| extension field | **rejected** | `ext.proto:7:9: Extensions can't be required.` |
| file-scope `option features.field_presence = LEGACY_REQUIRED;` | **rejected** | `filescope.proto:1:1: Required presence can't be specified by default.` |
| message-scope `option features.field_presence = LEGACY_REQUIRED;` | **rejected** | ``msgscope.proto: Option google.protobuf.FeatureSet.field_presence cannot be set on an entity of type `message`.`` |
| `required int32 a = 1;` (the keyword) | **rejected** | `kw.proto:4:3: Label "required" is not supported in editions, use features.field_presence = LEGACY_REQUIRED.` |
| edition 2024, `string` singular | **accepted** | — (not 2023-only) |

Note the map diagnostic: the *generated* `MapEntry` synthetic message inherits the field's features and trips
the repeated check, which is why the error points at column 22 (the field name) and is phrased "Repeated".

### Why there is no type restriction — the validation source

`ValidateFieldFeatures` is the whole of the per-field check
([descriptor.cc:8111–8181](https://github.com/protocolbuffers/protobuf/blob/v36.1/src/google/protobuf/descriptor.cc#L8111-L8181)):

```cpp
  if (field->is_extension() && field->is_required()) {
    AddError(..., "Extensions can't be required.");                   // :8151-8153
  }
  ...
  if (field->proto_features_->has_field_presence()) {
    if (field->containing_oneof() != nullptr) {
      AddError(..., "Oneof fields can't specify field presence.");    // :8166-8169
    } else if (field->is_repeated()) {
      AddError(..., "Repeated fields can't specify field presence."); // :8170-8172
    } else if (field->is_extension() && ... != FeatureSet::LEGACY_REQUIRED) {
      AddError(..., "Extensions can't specify field presence.");      // :8176-8179
    } else if (field->message_type() != nullptr &&
               field->proto_features_->field_presence() == FeatureSet::IMPLICIT) {
      AddError(..., "Message fields can't specify implicit presence.");
    }
  }
```

The only branch that inspects the field's *type* is the last one, and it restricts `IMPLICIT` on message
fields — the mirror image of the question asked. Nothing anywhere restricts `LEGACY_REQUIRED` by type.

File scope is separately rejected in `ValidateFileFeatures`
([descriptor.cc:8092–8109](https://github.com/protocolbuffers/protobuf/blob/v36.1/src/google/protobuf/descriptor.cc#L8092-L8109)):

```cpp
  if (file->features().field_presence() == FeatureSet::LEGACY_REQUIRED) {
    AddError(file->name(), proto, ..., "Required presence can't be specified by default.");
  }
```

Message scope is rejected one level earlier, by generic option-target checking, because the feature's
declared targets do not include messages — `descriptor.proto:1067–1077`:

```proto
  optional FieldPresence field_presence = 1 [
    retention = RETENTION_RUNTIME,
    targets = TARGET_TYPE_FIELD,
    targets = TARGET_TYPE_FILE,
    feature_support = { edition_introduced: EDITION_2023, },
    edition_defaults = { edition: EDITION_LEGACY, value: "EXPLICIT" },
    edition_defaults = { edition: EDITION_PROTO3, value: "IMPLICIT" },
    edition_defaults = { edition: EDITION_2023, value: "EXPLICIT" }
  ];
```

The keyword form is caught in the parser, not the validator —
[parser.cc:2492–2494](https://github.com/protocolbuffers/protobuf/blob/v36.1/src/google/protobuf/compiler/parser.cc#L2492-L2494) —
with a second belt-and-braces check in `ValidateFieldFeatures` for descriptors built dynamically rather than
parsed (`"Required label is not allowed under editions. Use the feature field_presence = LEGACY_REQUIRED to
control this behavior."`, descriptor.cc:8120–8124).

**Answer to issue question 3: per-field only.** The *feature* targets file and field; the *value*
`LEGACY_REQUIRED` is legal only at field scope. There is no inherited-default form of it, at any scope.

---

## 2. What the descriptor actually records

`--descriptor_set_out` for the seven-field message above, decoded (abridged — every field is identical in
shape):

```
    field {
      name: "s"
      number: 2
      label: LABEL_OPTIONAL          <-- NOT LABEL_REQUIRED
      type: TYPE_STRING
      options {
        features {
          field_presence: LEGACY_REQUIRED
        }
      }
      json_name: "s"
    }
```

**The descriptor records `LEGACY_REQUIRED` explicitly and leaves `label = LABEL_OPTIONAL`.** It does *not*
fold into `LABEL_REQUIRED`. Compare a proto2 file, which does the opposite:

```
    field { name: "id" number: 1 label: LABEL_REQUIRED type: TYPE_INT64 json_name: "id" }
```

So the two representations are **disjoint on the wire of the descriptor itself**, and a consumer must handle
both:

| Source | `label` | `options.features.field_presence` |
|---|---|---|
| proto2 `required int64 id = 1;` | `LABEL_REQUIRED` | absent |
| editions `int64 id = 1 [features.field_presence = LEGACY_REQUIRED];` | `LABEL_OPTIONAL` | `LEGACY_REQUIRED` |

### The in-memory descriptor *does* fold it — a trap

`protoc` rewrites the in-memory `label_` to `LABEL_REQUIRED` during the build
([descriptor.cc:5222–5229](https://github.com/protocolbuffers/protobuf/blob/v36.1/src/google/protobuf/descriptor.cc#L5222-L5229)):

```cpp
void internal::DescriptorBuilder::PostProcessFieldFeatures(
    FieldDescriptor& field, const FieldDescriptorProto& proto) {
  // TODO This can be replace by a runtime check in `is_required`
  // once the `label` getter is hidden.
  if (field.features().field_presence() == FeatureSet::LEGACY_REQUIRED &&
      field.label_ == FieldDescriptor::LABEL_OPTIONAL) {
    field.label_ = FieldDescriptor::LABEL_REQUIRED;
  }
```

…and then undoes it on the way back out, in `FieldDescriptor::CopyTo`
([descriptor.cc:3130–3138](https://github.com/protocolbuffers/protobuf/blob/v36.1/src/google/protobuf/descriptor.cc#L3130-L3138)):

```cpp
  if (is_required() && !IsLegacyEdition(file()->edition())) {
    // Editions files have no required keyword, and we only set this label
    // during descriptor build.
    proto->set_label(LABEL_OPTIONAL);
  } else {
    proto->set_label(label_);
  }
```

So **reflection sees `LABEL_REQUIRED`, serialized descriptors see `LABEL_OPTIONAL`.** Anything that
round-trips through a `FileDescriptorSet` sees the feature; anything that walks a live `FieldDescriptor` sees
the label. Both are canonical; they simply disagree.

The canonical predicate is the feature, not the label —
[descriptor.cc:4099–4101](https://github.com/protocolbuffers/protobuf/blob/v36.1/src/google/protobuf/descriptor.cc#L4099-L4101):

```cpp
bool FieldDescriptor::is_required() const {
  return features().field_presence() == FeatureSet::LEGACY_REQUIRED;
}
```

proto2 `required` is normalised *into* the feature at load time, by
`InferLegacyProtoFeatures` ([descriptor.cc:5137–5139](https://github.com/protocolbuffers/protobuf/blob/v36.1/src/google/protobuf/descriptor.cc#L5137-L5139)):
`if (proto.label() == LABEL_REQUIRED) features.set_field_presence(FeatureSet::LEGACY_REQUIRED);`.
That is the direction of travel: **`LEGACY_REQUIRED` is the model; `required` is the legacy encoding of it.**

### Features in the descriptor are *as-written*, never resolved

An important secondary finding for anyone writing their own front end. Given:

```proto
edition = "2023";
option features.field_presence = IMPLICIT;
message M { int32 a = 1; int32 b = 2 [features.field_presence = EXPLICIT]; }
```

the emitted descriptor carries `options { features { field_presence: IMPLICIT } }` on the *file* and
`field_presence: EXPLICIT` on field `b` — and **field `a` carries no `options` at all.** Feature inheritance
(defaults → file → message → field) is *not* materialised into `--descriptor_set_out`. Every consumer
re-runs resolution itself. `praguec` will have to implement that resolution chain; it cannot read effective
presence off a descriptor field-by-field.

---

## 3. Wire and runtime behaviour

Schema used (`edition = "2023"`):

```proto
message Req {
  int64  id   = 1 [features.field_presence = LEGACY_REQUIRED];
  string name = 2;
}
```

### Serialization shape

A `LEGACY_REQUIRED` scalar is a hasbit field, exactly like `EXPLICIT`. Setting it to the zero value still
emits it:

```
$ printf 'id: 0\n' | protoc --encode=w.Req wire.proto | xxd
00000000: 0800     # field 1, varint 0 — present on the wire
```

This matches protobuf.dev's definition of the feature: *"The field is required for parsing and
serialization. Any explicitly-set value is serialized onto the wire (even if it is the same as the default
value)."* (<https://protobuf.dev/editions/features/#field_presence>). The wire encoding is
**indistinguishable from `EXPLICIT`** — `LEGACY_REQUIRED` adds a validation rule, not a wire format.

### C++ (via `protoc` itself)

```
$ printf 'name: "x"\n' | protoc --encode=w.Req wire.proto > missing.bin
warning:  Input message is missing required fields:  id
$ xxd missing.bin
00000000: 1201 78

$ protoc --decode=w.Req wire.proto < missing.bin
warning:  Input message is missing required fields:  id
name: "x"
```

Decoding the same bytes against a schema without the feature produces no warning. So the C++ descriptor
layer treats an editions `LEGACY_REQUIRED` scalar as a genuine required field. (The `protoc` CLI uses
partial serialize/parse and downgrades to a warning; `MessageLite::ParseFromString` /
`SerializeToString` return `false` in the same situation. The library-level behaviour was **not executed
here** — inferred from `is_required()` feeding the standard `IsInitialized` machinery.)

### Go — hard error, both directions, transitively

`protoc-gen-go` emits a pointer field with the legacy `req` struct tag:

```go
type Req struct {
	Id   *int64  `protobuf:"varint,1,req,name=id" json:"id,omitempty"`
	Name *string `protobuf:"bytes,2,opt,name=name" json:"name,omitempty"`
}
```

Executed:

```
marshal bytes=120178 err=proto: required field w.Req.id not set
unmarshal err=proto: required field w.Req.id not set  parsed_name="x"  id_set=false
```

Note that `proto.Marshal` **returns bytes *and* an error** — the bytes are the partial encoding. Callers
that ignore the error silently produce invalid messages.

The failure is **transitive**, which is the old proto2 hazard intact. With
`Inner{ int64 id = 1 [LEGACY_REQUIRED]; }` and `Outer{ Inner inner = 1; string note = 2; }`:

```
outer marshal bytes=0a0012026869 err=proto: required field n.Inner.id not set
outer unmarshal err=proto: required field n.Inner.id not set note="hi"
outer-without-inner marshal bytes=12026869 err=<nil>
```

An unset required field three levels down poisons the whole parent. Omitting the sub-message entirely is
fine; including an incomplete one is not.

### Java — enforced at `build()`; parse path not executed

`protoc --java_out` emits the classic required machinery (`out_java/w/Wire.java`):

```java
    public final boolean isInitialized() {
      if (!hasId()) { return false; }      // :465
      return true;
    }
    public w.Wire.Req build() {
      w.Wire.Req result = buildPartial();
      if (!result.isInitialized()) {
        throw newUninitializedMessageException(result);   // :411
      }
      return result;
    }
```

So Java *does* enforce it, and throws on `build()`. **Unverified:** no JRE was available on this machine, so
the parse-side behaviour (`parseFrom` raising `InvalidProtocolBufferException` via `AbstractParser`'s
initialization check) is asserted from the generated code's shape and the known `AbstractParser` contract,
not observed.

### C# — completely ignored by codegen; opt-in only at runtime

This is the finding that matters most to this project.

`protoc --csharp_out` output for `Req` with `LEGACY_REQUIRED` on `id` is **byte-for-byte identical** to the
output for the same file without the feature, apart from the file name and the embedded descriptor blob:

```
$ diff out_cs/Wire.cs out_cs_opt/Opt.cs
3c3   <   source: wire.proto        >   source: opt.proto
...   (class names, and the base64 descriptor)
```

The generated property is an ordinary hasbit scalar:

```csharp
public const int IdFieldNumber = 1;
private readonly static long IdDefaultValue = 0L;
public long Id {
  get { if ((_hasBits0 & 1) != 0) { return id_; } else { return IdDefaultValue; } }
  set { _hasBits0 |= 1; id_ = value; }
}
public bool HasId { get { return (_hasBits0 & 1) != 0; } }
public void ClearId() { _hasBits0 &= ~1; }
```

`grep -i initialized out_cs/Wire.cs` → **no matches**. The C# generator contains no handling for
`is_required` at all (only occurrence of the word "IsInitialized" in
`src/google/protobuf/compiler/csharp/csharp_message.cc` is a comment about extension sets, at line 142).

The C# *runtime* does know about the feature, but only through an opt-in reflective extension method:

* [`Reflection/FieldDescriptor.cs:240`](https://github.com/protocolbuffers/protobuf/blob/v36.1/csharp/src/Google.Protobuf/Reflection/FieldDescriptor.cs#L240) —
  `public bool IsRequired => Features.FieldPresence == FeatureSet.Types.FieldPresence.LegacyRequired;`
  (so `IsRequired` is correctly driven by the *feature*, editions and proto2 alike).
* [`MessageExtensions.cs:201`](https://github.com/protocolbuffers/protobuf/blob/v36.1/csharp/src/Google.Protobuf/MessageExtensions.cs#L201) —
  `public static bool IsInitialized(this IMessage message)`, which walks `Descriptor.Fields` and returns
  false when `f.IsRequired && !f.Accessor.HasValue(message)`.

But nothing calls it. `MessageParser.cs` contains zero references to `IsInitialized`; neither does generated
code. **In C#, a missing `LEGACY_REQUIRED` field parses and serializes silently, on any field type.**

### Summary table for issue question 4

| Runtime | Missing required on serialize | Missing required on parse | Transitive through sub-messages |
|---|---|---|---|
| C++ (`protoc` CLI) | warns, emits partial bytes | warns, yields partial message | yes (verified for Go; C++ same machinery, **unverified**) |
| Go | **error** + partial bytes returned | **error** | **yes, verified** |
| Java | throws at `Builder.build()` | throws (**unverified** — no JRE) | yes, via generated `isInitialized()` recursion |
| **C#** | **nothing** | **nothing** | n/a — never checked |

Same schema, same descriptor, four different answers. That divergence is the headline.

---

## 4. Does the ecosystem want you to use it?

* protobuf.dev's own style guidance is unambiguous, and predates editions:
  > **"Don't Add a Required Field.** Never add a required field, instead add `// required` to document the
  > API contract. Required fields are considered harmful by so many they were removed from proto3
  > completely."
  > — <https://protobuf.dev/best-practices/dos-donts/>
* `descriptor.proto:266–269` calls the label legacy in its own comment: *"The required label is only allowed
  in `google.protobuf`. In proto3 and Editions it's explicitly prohibited. In Editions, the `field_presence`
  feature can be used to get this behavior."*
* The enum value is literally named `LEGACY_`. It carries **no** `feature_support.edition_deprecated`
  annotation in v36.1 or on `main`, however — so it is not formally deprecated or scheduled for removal, and
  it still compiles under `edition = "2024"` (verified). Migration guidance actively instructs its use when
  upgrading proto2: *"Remove all instances of the `required` label and add the
  `features.field_presence=LEGACY_REQUIRED` option at the field level."*
  (<https://protobuf.dev/editions/features/#field_presence>)
* `protobuf.dev/programming-guides/field_presence/` — the presence app note — **does not mention
  `LEGACY_REQUIRED` at all.** There is no official document describing its hazards under editions; the
  reader is expected to know the proto2 `required` folklore.

The classic reasons `required` was removed still hold verbatim here, because the semantics are identical:
you cannot remove a required field without breaking every old reader; a producer that adds one breaks old
producers' payloads for new readers; and an incomplete sub-message renders its entire ancestry unparseable
(demonstrated in §3).

---

## 5. What this means for protop

1. **The `LEGACY_REQUIRED` → non-null + guard branch of the presence mapping is sound, and applies to every
   singular field type.** The untested assumption behind `proto-to-messagepack-csharp.md` §1.3 holds and then
   some: scalars, strings, bytes, enums and messages all accept it. The three-way mapping
   (`EXPLICIT` → nullable, `IMPLICIT` → non-null zero-defaulted, `LEGACY_REQUIRED` → non-null + guard) is
   well-founded for the full type surface.

2. **Read the feature, never the label.** protop's front end must key off
   `field.options.features.field_presence`, and — if it ever ingests a `FileDescriptorSet` produced by
   `protoc` — must *also* accept `label == LABEL_REQUIRED` as the proto2 spelling of the same thing. The two
   never co-occur. Do not write a check that only looks at `label`; editions descriptors say
   `LABEL_OPTIONAL` for required fields.

3. **protop must implement feature resolution itself.** `--descriptor_set_out` stores as-written features
   only; inherited defaults are not materialised. Since protop parses `.proto` directly this was going to be
   true anyway, but it also means the differential-test oracle cannot be used to *check* resolved presence
   by descriptor diffing — only by comparing generated behaviour.

4. **A strict profile has a clean, defensible reason to allow `LEGACY_REQUIRED` where protoc does, and a
   clean reason to restrict it further.** The grammar-level restrictions come free from protoc (no repeated,
   no map, no oneof member, no extension, no file/message default) — protop should mirror those diagnostics
   rather than invent its own. Beyond that, the live question is whether protop should permit it *at all*:

   * **For:** it is the only first-class way protobuf expresses NOT NULL, it is exactly the thesis of the
     project, and — critically — **the reference C# generator ignores it entirely**, so protop enforcing it
     is a pure, visible win over `protoc --csharp_out` rather than a divergence.
   * **Against:** the wire semantics are proto2 `required` semantics, including transitive parse failure,
     and `LEGACY_REQUIRED` cannot be removed from a published schema without breaking readers in Go, Java and
     C++ — while *no* C# reader would ever have noticed it was there.

   The asymmetry in that last sentence is the real risk and it is new information. A protop schema whose
   only consumers are protop-generated C#/MessagePack types is safe: protop owns the guard and can place it
   at construction time rather than at parse time, where it is a compile-time non-nullable parameter instead
   of a runtime failure. The moment the same `.proto` is also compiled by stock `protoc` for a Go or Java
   consumer, a field protop treats as "non-null, checked at construction" becomes "unparseable if absent,
   transitively" for that consumer — a far harsher failure mode, arrived at without the author asking for it.

5. **Concrete recommendation for the frozen subset (§14 open question 4).** Permit `LEGACY_REQUIRED` on
   singular fields of any type, reject everything protoc rejects with the same diagnostics, and add one
   protop-specific lint: **warn (not error) when `LEGACY_REQUIRED` is used, naming the cross-language
   consequence.** Do not adopt `IMPLICIT` as a substitute for it — implicit presence cannot distinguish
   "absent" from "zero" and so cannot express NOT NULL on a scalar at all; `LEGACY_REQUIRED` genuinely has no
   replacement in the language.

6. **Differential testing note.** Because `protoc --csharp_out` emits identical code with and without the
   feature, `protoc` is *useless* as an oracle for the presence axis in C#. Presence differentials must be
   taken against **Go or Java** gencode, or against descriptor contents, not against the C# backend.

---

## 6. Explicitly unverified

* Java runtime parse-side rejection (no JRE available). Generated `isInitialized()` and the throwing
  `build()` were read directly; the `parseFrom` path is inferred.
* C++ library-level (`ParseFromString` returning `false`) as distinct from the `protoc` CLI's warn-and-continue
  behaviour.
* JSON mapping of a missing `LEGACY_REQUIRED` field in any runtime.
* Behaviour of `LEGACY_REQUIRED` combined with `features.message_encoding = DELIMITED`.
* Whether `buf`, `protovalidate` or other third-party tooling recognises the feature.
