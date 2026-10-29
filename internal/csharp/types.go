package csharp

import (
	"strings"

	"github.com/praguedb/praguec/internal/check"
)

// csScalars is the mapping table's scalar half: one row per protop scalar
// keyword, and exactly as many rows as there are keywords.
//
// The set is closed here because it is closed there. `scalarTypes` in
// internal/syntax is hard-coded from a census of the spec and whether it is
// closed at all is praguedb/praguec#32, which is
// human-in-the-loop and unresolved. **Do not add a row to make a golden pass.**
// The corpus's `byte ProviderId` (3 sites) has no protop spelling and that is
// #32's question rather than a gap to fill here; so is the corpus's `DateTime`,
// which is #15. Widening this map without widening #32's map first produces a
// keyword the parser rejects, and widening both is #32 ruling itself through
// the back door.
//
// TestScalarRowsAreClosed holds this map against a hard literal list rather
// than against internal/syntax, because a test that reads the set off the map
// under test gets weaker every time the map grows -- which is the mistake #35's
// reviewer found in the frontend's suite.
var csScalars = map[string]string{
	"int64":  "long",
	"int32":  "int",
	"string": "string",
	"double": "double",
	"bool":   "bool",

	// PROVISIONAL -- #13, the scaled-decimal option.
	//
	// The corpus has 27 `decimal` members and zero `[MessagePackFormatter]`
	// attributes, so `decimal(4)?` emits `decimal?` and no attribute at all.
	// (TestCorpusCensus counts 30, and 30 is right as a *text* count: the other
	// three are the constructor parameters at
	// corpus/prague/payloads/SlimBetType.cs:25-27, inside the `ctor:` block the
	// reader skips. The census counts text and the noun here is members.)
	//
	// All 30 are `decimal?`. There is not one non-nullable `decimal` anywhere in
	// the corpus, so this row is only ever exercised with the `?` that
	// t.Optional adds, and a defect in the non-nullable spelling would have no
	// oracle at all.
	// #13's end state is a scaled representation carried by
	// `ScaledDecimalFormatter<TScale>`, and #13's own comments say that scaling
	// the corpus's money members is a wire migration rather than a recompile.
	// The oracle and the end state disagree; this milestone is measured against
	// the oracle, so the precision is carried in the model, checked by the
	// frontend, and dropped here. When #13 rules, this row is where the
	// formatter attribute appears.
	"decimal": "decimal",
}

// listCollection is protop's one implemented collection keyword. `map` parses
// in full and is then PROTOP0900 at the parser, so a schema containing one
// never reaches a backend -- which is also why there is no Dictionary row: the
// corpus has zero `Dictionary<K, V>` and could not check one.
const listCollection = "list"

// TypeName is a resolved protop type as C# spells it.
//
// Presence is the whole of the nullable axis: `T` emits `T` and `T?` emits
// `T?`, at every level, which is what makes `list<Odd>?` and `list<Odd?>`
// different text rather than the same text twice.
func TypeName(t check.Type) string {
	return typeName(t)
}

func typeName(t check.Type) string {
	var name string
	switch {
	case t.Kind == check.TypeCollection && t.Name == listCollection && len(t.Args) == 1:
		// PROVISIONAL -- #24, collection type and presence.
		//
		// The oracle contradicts itself here and no single emission passes all
		// seven of its goldens. The corpus writes `List<T>` six times, every
		// one of them in a table, and `T[]` exactly once, at
		// corpus/prague/payloads/SlimBetType.cs:89, in a view -- the member a
		// narrowing recursed into. Inferring "table emits List<T>, view emits
		// T[]" from a single array is a language ruling and #24 is where one
		// gets made; the emitter takes the majority form and the harness lists
		// SlimBetType key 25 as the one member it cannot reproduce.
		name = "List<" + typeName(t.Args[0]) + ">"
	case t.Kind == check.TypeScalar:
		if cs, ok := csScalars[t.Name]; ok {
			name = cs
		} else {
			// Unreachable while csScalars and syntax.scalarTypes hold the same
			// six keywords, which TestScalarRowsAreClosed pins. Written as the
			// protop keyword so that the day they disagree the generated C#
			// fails at the compiler rather than emitting an empty type name.
			name = t.Name
		}
	default:
		// A declared type emits its own name. A view emits the view's name,
		// which is what a narrowing is for: the member keeps its source's tag
		// and takes the narrowing view's type.
		name = t.Name
	}
	if t.Optional {
		name += "?"
	}
	return name
}

// pascal is protop's member spelling in C#'s.
//
// One rule, applied to members and to enum arms alike: split on `_`, upper-case
// the first rune of each part and lower-case the rest, concatenate. Section 1
// writes members `snake_case` and enum arms `SCREAMING_SNAKE`, and title-casing
// each part is the single rule that turns `provider_source_key` into
// `ProviderSourceKey` and `NOT_STARTED` into `NotStarted`. Two rules, chosen by
// which side of the language a name is on, would be two places to disagree.
//
// It follows that a name written in some third convention round-trips badly --
// `eventId` becomes `Eventid`. That is a consequence of the language having one
// convention rather than a defect in the conversion, and every one of the
// corpus's 339 member names is reachable from a `snake_case` spelling.
func pascal(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, part := range strings.Split(name, "_") {
		if part == "" {
			continue
		}
		b.WriteString(strings.ToUpper(part[:1]))
		b.WriteString(strings.ToLower(part[1:]))
	}
	return b.String()
}
