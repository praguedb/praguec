# Language prototype: shapes, and what the Protobuf constraint costs

Two spellings of the same content, written so they can be read side by side.
Both model the same four corpus types.

| | |
|---|---|
| [`a-strict-protobuf/prague.proto`](a-strict-protobuf/prague.proto) | protop as a strict profile. **Verified: compiles under `protoc` 36.1.** |
| [`b-native/prague.protop`](b-native/prague.protop) | protop as its own language. **Verified: `protoc` fails at line 12.** |

The immediate motivation was the **table / record / struct** distinction, which
Protobuf has no way to spell — it has exactly one declaration form, `message`.
Writing both out turned up six things, only two of which are about shapes.

---

## 1. The shape axis is cheap in both spellings

One `option (protop.shape) = SHAPE_STRUCT;` line per type in A, one keyword in
B. This is *not* where the cost is, and it is not on its own an argument for
leaving Protobuf. Worth saying plainly, because it was the thing that prompted
the question.

Semantics that fall out, unchanged by spelling:

- **struct** → C# `struct`. Value payload, embedded, no identity.
- **record** → C# `sealed partial class`. Payload, no identity.
- **table** → C# `sealed partial class` + the `B2Tech.Feed.Cache` attributes, with
  one field marked as the identity.

## 2. Presence costs 145 annotations

Editions 2023 defaults `field_presence` to **EXPLICIT** — nullable. The corpus
is the other way round:

```
333 members across 18 corpus types
188 non-nullable  (56%)
145 nullable      (44%)
```

So a protop file flips the default at file scope (`option
features.field_presence = IMPLICIT;` — features inherit, custom options do not,
which is the only reason this works at all) and then **145 members opt back in
one at a time**, each carrying `[features.field_presence = EXPLICIT]`.

In spelling B the same information is one character: `int64? home_team_id = 5;`.

This is the real cost, and it lands on the axis the whole project is about —
the thesis is *field presence is the nullable/NOT NULL axis*, and in spelling A
that axis is the noisiest thing in the file.

## 3. protop cannot demand explicitness uniformly

Measured:

```
h.proto:4:19: Message fields can't specify implicit presence.
```

A message-typed field may not carry `IMPLICIT`, and `EXPLICIT` is its only legal
value, so writing it is redundant noise. A protop rule of the form *"every field
states its presence"* — the obvious strict-profile rule — is **not
implementable** in spelling A: there is a whole class of field where protoc
rejects one value and the other is vacuous.

File-scope `IMPLICIT` inherits silently *past* message fields (rc=0, no
diagnostic), so the behaviour is correct. It just cannot be written down.

## 4. `FIRST_TAG_IS_ONE` rejects two corpus types — independent of spelling

With `Key = tag - 1`, the rule was meant to eliminate nil padding. Measuring
every corpus type's lowest key:

```
Venue      first key = 1  →  first tag = 2
BOVenue    first key = 1  →  first tag = 2
everything else            first key = 0  →  first tag = 1
```

**The only two types that break the rule are the two structs.** Renumbering them
to tag 1 moves every member one slot and breaks the wire. So one of these has to
give: the rule gains an exception, the rule becomes a warning protop cannot
have (all diagnostics are errors), or two corpus types are inexpressible — and
*"a decision that cannot express the corpus is not done."*

This has nothing to do with A vs B. It is a conflict between two already-settled
decisions that only became visible by writing a file out.

## 5. Projections are the one thing spelling A genuinely cannot express

A **projection** carries a subset of another type's fields *at the same tags*.
In spelling A, `SlimSportGame` is simply an unrelated `message` that happens to
reuse numbers; nothing in the file says what it is a projection *of*, and
nothing checks the alignment. That check is why
[the cross-message key-alignment check](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/17)
exists as a ticket at all, and why `tools/keymap.py` found **7** divergences in
the corpus rather than the 1 that was known.

In spelling B it is a declaration:

```protop
projection SlimSportGame of SportGame {
  event_id;
  sport_type_id;
  venue;
  sport_type;
}
```

Alignment is then true *by construction* — the tags are not restated, so they
cannot disagree, and drift stops being a class of bug. This is the only finding
where B does not merely save typing but changes what is checkable.

## 6. Projection padding is structural, and neither spelling fixes it

```
SlimOdd        14 members → 28 slots   (50% nil)
SlimBetType    12 members → 26 slots   (54% nil)
SlimSportGame  41 members → 59 slots   (31% nil)
```

A projection must keep the parent's tags, so it inherits the parent's *maximum*
tag, so it pays padding proportional to what it dropped. That is a MessagePack
dense-array fact, not a language fact.

---

## What leaving Protobuf would actually cost

Spelling B is not free, and the bill is larger than the syntax:

1. **The oracle dies.** The differential harness is descriptor equality against
   `protoc --descriptor_set_out`. `protoc` cannot read spelling B, so there is
   nothing to differentiate against, and protopc becomes a compiler checked only
   by its own tests.
2. **[The versioning contract](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/27)
   is void in its carrier half.** The whole reason the dialect stamp had to be a
   custom option — rather than a `revision 1;` statement — is that a keyword
   makes the file unparseable by `protoc`. In spelling B, `revision 1;` is just
   a statement, and the mandatory `import "protop/options.proto"` disappears
   along with [ticket #28](https://github.com/nanov/protobuf-to-messagepack-sharp/issues/28).
3. **`CONTEXT.md`'s definition of protop changes** — *"protop adds no syntax that
   `protoc` would refuse"* is the current glossary entry, and it stops being true.
4. **Every Protobuf tool stops working**: editors, formatters, linters,
   `buf`, and anyone who wanted to generate a Go or Java reader from the same file.

## Option C, which neither specimen is

**Write B, lower to A.** protopc parses the native spelling and emits the
strict-Protobuf form as a byproduct; the harness runs `protoc` on *that*. The
oracle survives, the versioning carrier survives, the ergonomics are B's, and
projections stay checkable because the lowering is where the check happens.

What it costs: the lowering is a second emitter to write and to keep honest, and
a round-trip property to test (*lower, compile with protoc, compare descriptors*)
that does not exist today. It also means two files exist for every one the
author wrote, which has to be explained to anyone reading a stack trace.

This is the option worth discussing, and it is not on the map in any form.
