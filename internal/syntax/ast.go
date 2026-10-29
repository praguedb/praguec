package syntax

import (
	"strings"

	"github.com/praguedb/praguec/internal/source"
)

// The AST is what the parser produces and where the frontend stops. It is a
// faithful record of what the source said -- one node per construct, spans on
// everything, no resolution and no inference. Names are not looked up, types
// are not checked against declarations, and `decimal(4)` is a name plus a
// number rather than a type. That work belongs to the checking stage that turns
// an AST into the one schema model backends see.
//
// Nodes carry their span as a field rather than behind a Pos()/End() interface.
// There is no generic tree walk in the frontend yet, and a field is what a
// parser writes with no ceremony.

// File is one parsed .prague source file.
//
// Revision and Namespace are separate fields rather than declarations in Decls
// because they are file properties, exactly one of each, and every later stage
// reaches for them by name. Either may be nil: the parser reports the omission
// and returns what it has, so an editor still gets an outline of a file whose
// header is missing.
// Imports are a field of their own for the same reason: they are a property of
// the file rather than one of its declarations, and the parser holds them to
// the header, so a reader learns what a file depends on without reading it
// entire. There may be any number of them, in source order.
type File struct {
	Revision  *Revision
	Namespace *Namespace
	Imports   []*Import
	Decls     []Decl
	Span      source.Span
}

// Ident is a word and where it was written.
type Ident struct {
	Text string
	Span source.Span
}

// IntLit is an integer literal: tags, enum values and decimal precision.
type IntLit struct {
	Value uint64
	Span  source.Span
}

// StringLit is a double-quoted literal, and there is exactly one place the
// grammar has for one: an import path.
//
// Text is the literal as written, quotes and all. Nothing in the frontend
// decodes a string -- the lexer recognises `\"` only well enough to keep it
// from closing the literal -- so stripping the quotes while leaving `\n` a
// backslash and an `n` would hand a later stage a half-cooked string it could
// not tell from a raw one. Verbatim is one rule, and the stage that resolves a
// path is the stage that gets to decide what an escape in it means.
type StringLit struct {
	Text string
	Span source.Span
}

// Revision is the dialect stamp, `revision 1;`.
type Revision struct {
	Number IntLit
	Span   source.Span
}

// Namespace is `namespace A.B.C;`, held as its parts so that a backend can
// join them with whatever separator its language uses.
type Namespace struct {
	Parts []Ident
	Span  source.Span
}

// String joins the parts with dots, the way protop writes them.
func (n *Namespace) String() string {
	parts := make([]string, len(n.Parts))
	for i, p := range n.Parts {
		parts[i] = p.Text
	}
	return strings.Join(parts, ".")
}

// Import is `import "prague.prague";`.
//
// It names a file and nothing more. Where that file is found is the
// distribution model -- git, by immutable tag, toolchain-fetched -- and every
// particular of it is open: whether the module path lives in the import line or
// in a manifest beside it, what that manifest is called, how the cache is
// keyed. The frontend parses the string and stops. No file is opened, no path
// is resolved, nothing is fetched.
type Import struct {
	Path StringLit
	Span source.Span
}

// Decl is a top-level type declaration. The interface is a marker: callers
// type-switch, because the four declaration keywords have genuinely different
// shapes and nothing useful is shared across all of them.
type Decl interface {
	isDecl()
}

// Struct is `struct Venue { ... }`: value semantics, no identity, embedded in
// whatever holds it.
type Struct struct {
	Name        Ident
	Annotations []*Annotation
	Members     []Member
	Span        source.Span
}

func (*Struct) isDecl() {}

// Record is `record StaticDataRef { ... }`: no identity either, emitted as a
// C# class rather than a C# struct.
//
// Structurally it is a struct with a different keyword, and the parser treats
// it as one. The whole of the difference is what section 4 says the keyword
// means -- and that a record cannot be a relation target, because a relation
// resolves against a key and a record has none.
type Record struct {
	Name        Ident
	Annotations []*Annotation
	Members     []Member
	Span        source.Span
}

func (*Record) isDecl() {}

// Table is `table SportGame [cs: ...] { ... }`: the one declaration that
// carries identity -- a `key` -- and so the only one a relation can name.
type Table struct {
	Name        Ident
	Annotations []*Annotation
	Members     []Member
	Span        source.Span
}

func (*Table) isDecl() {}

// View is `view SlimSportGame of SportGame { ... }`: a field subset of one
// source, at that source's tags. Tag, type and presence come from the source,
// and the view line carries the name and an optional rename.
//
// Source is the name as written and is not resolved -- whether `SportGame` is a
// table, a record, or nothing at all is a question about a whole program, and
// the view inherits the source's topic rather than restating it, so there is no
// second place for the two to disagree.
type View struct {
	Name        Ident
	Source      Ident
	Annotations []*Annotation
	Members     []*ViewMember
	Span        source.Span
}

func (*View) isDecl() {}

// ViewMember is one line of a view body, in all three of its spellings:
//
//	sport_type_id;      a plain selection
//	event_id as id;     the same, renamed
//	SlimOdd odds;       narrowed to another view
//
// It is not a Member. A view body and a struct body have no line in common --
// a view may never write a tag and a struct has nothing to select from -- so
// one type that could hold either would exist only to be type-switched back
// apart by everything that reads it.
type ViewMember struct {
	// Narrow is the leading name of `SlimOdd odds;`, nil when the member takes
	// the source's type unchanged. It asks for the member's type to be narrowed
	// to another view; that the narrowing is a view, and a view of the right
	// thing, is checked where the two declarations are both in scope.
	Narrow *Ident

	// Name is the member selected out of the source -- `odds`, not `SlimOdd`.
	Name Ident

	// As is the rename of `event_id as id;`, nil when the member keeps its
	// name. `as` is an ident in the position after the name, not a keyword:
	// `key as id;` selects the member named `key`, which is exactly why the
	// lexer reserves no words.
	As *Ident

	Annotations []*Annotation
	Span        source.Span
}

// Enum is `enum EventStatus { ... }`.
type Enum struct {
	Name    Ident
	Members []*EnumMember
	Span    source.Span
}

func (*Enum) isDecl() {}

// Member is one line of a struct, record or table body. Like Decl it is a
// marker: the member shapes have little in common beyond appearing between the
// same braces, and every caller type-switches.
//
// One ordered slice rather than a slice per shape, because source order is what
// a backend emits and what a diagnostic quotes -- `reserved` above the key is
// how section 1 is written, and the tree says so.
type Member interface {
	isMember()
}

// Field is one tagged member, `decimal(4)? live_limit_manual_single = 36;`.
// The fields are declared in the order the source writes them.
type Field struct {
	// Key marks `key int64 event_id = 1;`, a field that is also the
	// declaration's identity. A simple key is a field and nothing more; only
	// the composite spelling needs a node of its own.
	Key bool

	Type        TypeRef
	Name        Ident
	Tag         IntLit
	Annotations []*Annotation
	Span        source.Span
}

func (*Field) isMember() {}

// CompositeKey is `key key = 1 (event_id, bet_type_id);` -- a name over fields
// that keep their own ordinals, borrowed from Cap'n Proto's group. Cap'n
// Proto's group consumes no ordinal; this one does, because the corpus's wire
// format already says so, and writing the tag is what makes the duplication
// visible and the collision checkable.
//
// Parts are the names as written. Matching them against the fields beside them
// needs the whole declaration read and is the checking stage's job.
type CompositeKey struct {
	Name        Ident
	Tag         IntLit
	Parts       []Ident
	Annotations []*Annotation
	Span        source.Span
}

func (*CompositeKey) isMember() {}

// Alias is `alias bet_type_id = id;`: get and set passthrough, no wire slot and
// no tag. It exists so a table's API matches the view that renames the field.
type Alias struct {
	Name        Ident
	Source      Ident
	Annotations []*Annotation
	Span        source.Span
}

func (*Alias) isMember() {}

// Reserved is `reserved 8..14, 16..23, 25;`, the mechanism that keeps cross-repo
// skew safe: a tag is retired where it was declared, and a removal without a
// reservation breaks every view at compile time.
type Reserved struct {
	Ranges []ReservedRange
	Span   source.Span
}

func (*Reserved) isMember() {}

// ReservedRange is one entry of a `reserved` list, inclusive at both ends. A
// single tag is a range whose ends are equal: nothing downstream cares which of
// the two spellings was used, and the span still says which was written.
type ReservedRange struct {
	Lo   IntLit
	Hi   IntLit
	Span source.Span
}

// Contains reports whether the range retires tag.
func (r ReservedRange) Contains(tag uint64) bool {
	return r.Lo.Value <= tag && tag <= r.Hi.Value
}

// Annotation is one bracket: `[cs: DataCacheTopic("...")]` or
// `[references(SportGame)]`. An annotation follows what it describes; there is
// no second position and no before-slot, and annotations stack by juxtaposition
// with no comma, because their contents already contain commas.
//
// Text is raw, and that is the one place praguec carries something it does not
// understand. An annotation aimed at a backend is written in that backend's
// language, so the check that matters happens when the generated code compiles;
// praguec checks position and prefix and nothing else. Annotations are held in
// source order and are neither sorted nor deduplicated.
type Annotation struct {
	// Prefix names the target backend -- the `cs` of `[cs: X]` -- and is nil
	// when the annotation is protop's own closed set, which is read and checked.
	Prefix *Ident

	// Text is everything after the prefix, verbatim, from the first character
	// of the contents to the last. Empty when the brackets hold nothing.
	Text string

	// TextSpan is where Text was written, so that a stage which reads inside
	// the contents can report against what it found there rather than against
	// the whole bracket. The contents are raw text to the parser and stay that
	// way -- protop's own `references(T)` is read by the checker, and without
	// this the checker could say `T` is unknown while underlining a bracket
	// that is otherwise fine.
	//
	// An offset into Text is TextSpan.Start plus that offset, because Text is
	// the slice of exactly this span. That is one line of parseAnnotation
	// holding it rather than anything structural -- nothing here would notice a
	// TextSpan that kept the right start and took its end from the bracket --
	// so it is pinned as an invariant by
	// TestAnAnnotationsTextSpanIsWhereItsTextWasWritten, which is where to look
	// before changing either.
	TextSpan source.Span

	// Span is the whole bracket, `[` to `]`. It is what a diagnostic about the
	// annotation AS A WHOLE underlines -- one written where the grammar has no
	// slot for it, or one whose contents are not the shape protop's own
	// annotation takes.
	Span source.Span
}

// TypeRef is a field's type as written. The name is not resolved here, so a
// scalar keyword and a reference to a declared type are the same node -- only
// the checking stage can tell them apart, and it is the stage that knows what
// is in scope.
type TypeRef struct {
	Name Ident

	// Args are the type's arguments: `list<Odd>` has one, `map<string, Odd>`
	// two, and everything else none. Repetition is a type here rather than a
	// label on the field, which is what lets presence compose -- `list<Odd>?`
	// is Optional on the outer ref and `list<Odd?>` on the inner one, a
	// difference `repeated Odd?` had no way to write. Nesting needs no further
	// machinery: `list<list<int64>>` is this field twice.
	Args []TypeRef

	// Precision is the N of `decimal(N)`, nil when the type carries none.
	Precision *IntLit

	// Optional is the trailing `?`. Presence is the whole point of protop, and
	// in the AST it is this one bool.
	Optional bool

	Span source.Span
}

// EnumMember is `NOT_STARTED = 1;`.
//
// Number, not Tag: an enum member's number is a value on the wire, not a
// position in a MessagePack array, so 0 is legal here and is not a tag.
type EnumMember struct {
	Name   Ident
	Number IntLit
	Span   source.Span
}
