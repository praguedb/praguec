# 3. Annotations — one position, one form

An annotation follows what it describes. There is no second position and no before-slot.

```protop
table SportGame [cs: DataCacheTopic("...")] { }      // after the declaration head
int64 timestamp = 25 [cs: DataCacheFromTimestamp];   // after the tag
alias bet_type_id = id [cs: JsonIgnore];             // after the alias
```

Legal on a table, record, struct, view, field, view member, or alias. Same rule everywhere.

**One bracket, one annotation. Stack by juxtaposition.** No commas between annotations, because annotation contents already contain commas — `DataCacheContract(typeof(X), Role = Y)` — and a separator that competes with them buys nothing. Whitespace between brackets is insignificant, so a head carrying four annotations wraps freely across lines. protopc emits them in source order and never sorts or deduplicates.

**A prefix names the target language. No prefix means protop's own.**

| Form | Meaning |
|---|---|
| `[references(SportGame)]` | protop's own closed set. Read, checked, unknown is an error. |
| `[cs: DataCacheTopic("...")]` | Verbatim C# attribute text. Copied out, never inspected. |
| `[go: json:"sportGame"]` | Verbatim Go tag text, when a Go backend exists. |

The legal prefix set is exactly the backends protopc has. An unknown prefix is an error rather than a silent drop, because a typo that vanishes from the output is worse than the lexical rule this replaced. Adding `go:` means adding a backend, not just a keyword.

Everything after the prefix is raw text up to the matching close bracket, with nesting counted and string literals respected. This is the one place protopc parses something it does not understand, so the grammar states it explicitly.

### What this costs

protop does not validate annotation contents. Protobuf does — custom options are typed values declared by extending the descriptor messages, with globally coordinated extension numbers. protop checks position and prefix only.

That trade is right here and would not be right for Protobuf. An annotation targeting a code generator is written in that generator's language and emitted verbatim; the check that matters happens when the generated code compiles. Protobuf cannot make that trade because anyone may generate anything from a `.proto`, which is also why spelling A has to smuggle C# attributes as string literals that nothing validates.

### Why one position and not Protobuf's two

Protobuf puts field options in a bracket after the tag and type options in a body statement — `option x = y;` — which is two syntaxes for the same kind of fact, chosen by whether it describes a type or a field. The reason is parser economy: `message Foo [x] {` needs lookahead past the bracket to find the brace, and a body statement does not.

protopc is recursive-descent, where that lookahead is free. Adopting the split would mean inheriting a consequence without its cause.

The rule works here because the scope is narrow. protop targets one pipeline with known backends, which is what makes opaque namespaced passthrough safe. Protobuf's heavier machinery is not a mistake; it is what a cross-ecosystem format needs.
