package syntax

import (
	"strings"

	"github.com/nanov/protobuf-to-messagepack-sharp/internal/source"
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

// File is one parsed .protop source file.
//
// Revision and Namespace are separate fields rather than declarations in Decls
// because they are file properties, exactly one of each, and every later stage
// reaches for them by name. Either may be nil: the parser reports the omission
// and returns what it has, so an editor still gets an outline of a file whose
// header is missing.
type File struct {
	Revision  *Revision
	Namespace *Namespace
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

// Decl is a top-level type declaration. The interface is a marker: callers
// type-switch, because the four declaration keywords have genuinely different
// shapes and nothing useful is shared across all of them.
type Decl interface {
	isDecl()
}

// Struct is `struct Venue { ... }`: value semantics, no identity, embedded in
// whatever holds it.
type Struct struct {
	Name   Ident
	Fields []*Field
	Span   source.Span
}

func (*Struct) isDecl() {}

// Enum is `enum EventStatus { ... }`.
type Enum struct {
	Name    Ident
	Members []*EnumMember
	Span    source.Span
}

func (*Enum) isDecl() {}

// Field is one tagged member, `decimal(4)? live_limit_manual_single = 36;`.
type Field struct {
	Type TypeRef
	Name Ident
	Tag  IntLit
	Span source.Span
}

// TypeRef is a field's type as written. The name is not resolved here, so a
// scalar keyword and a reference to a declared type are the same node -- only
// the checking stage can tell them apart, and it is the stage that knows what
// is in scope.
type TypeRef struct {
	Name Ident

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
