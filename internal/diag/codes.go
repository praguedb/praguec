package diag

import "fmt"

// Code identifies a kind of diagnostic. Codes are part of protopc's interface:
// they are what a changelog cites, what an editor filters on, and what a build
// script greps for. So a code that has shipped keeps its number forever, and a
// retired one is never reused for a different fact.
//
// Numbers are grouped by what the diagnostic is about, with room to grow inside
// each group:
//
//	0001-0099  lexical -- what the lexer alone can see
//	0100-0199  file structure -- revision, namespace
//	0200-0299  declarations
//	0300-0399  fields, types and tags
//	0400-0499  annotations
//	0900-0999  recognised by the spec, not yet implemented by this compiler
//
// The 0900 group is temporary by construction: every code in it is a promise
// that the construct is real, and each one is deleted rather than redefined
// when its milestone lands.
type Code int

// String renders the code in the stable PROTOP%04d form that appears in
// messages and in documentation.
func (c Code) String() string { return fmt.Sprintf("PROTOP%04d", int(c)) }

const (
	// UnterminatedBlockComment is a /* that reaches the end of the file.
	UnterminatedBlockComment Code = 1
	// UnterminatedString is a string literal closed by a newline or by the end
	// of the file rather than by a quote.
	UnterminatedString Code = 2
	// UnexpectedCharacter is a rune that begins no token in the language.
	UnexpectedCharacter Code = 3
	// IntegerOutOfRange is an integer literal too large to hold.
	IntegerOutOfRange Code = 4

	// MissingRevision is a file that does not stamp its dialect.
	MissingRevision Code = 100
	// RevisionNotFirst is a revision stamp that something precedes.
	RevisionNotFirst Code = 101
	// UnsupportedRevision is a dialect this compiler does not implement.
	UnsupportedRevision Code = 102
	// DuplicateRevision is a second revision stamp in one file.
	DuplicateRevision Code = 103
	// MissingNamespace is a file that declares no namespace.
	MissingNamespace Code = 104
	// DuplicateNamespace is a second namespace declaration in one file.
	DuplicateNamespace Code = 105
	// ImportNotInHeader is an import written below a declaration. What a file
	// depends on is a property of the whole file, not of the lines after it, so
	// the imports are grouped under the header where a reader finds them
	// without reading the file entire.
	ImportNotInHeader Code = 106

	// UnexpectedToken is the generic shape error: the parser wanted one thing
	// and found another.
	UnexpectedToken Code = 200
	// UnknownDeclaration is a top-level word that names no declaration keyword.
	UnknownDeclaration Code = 201
	// KeyNotAllowed is a `key` on a declaration that has no identity. Only a
	// table has one, and only a table can be the target of a relation.
	KeyNotAllowed Code = 202
	// ViewSourceMissing is a view that names nothing to be a view of, either
	// because the `of` is absent or because nothing follows it. A view is a
	// subset of one source and carries none of its own tags, so a view without
	// a source describes nothing.
	ViewSourceMissing Code = 203

	// TagOutOfRange is a field tag outside the legal range. A tag is 1-based
	// because a MessagePack key is tag - 1, so tag 0 names no slot.
	TagOutOfRange Code = 300
	// PrecisionNotAllowed is a (N) on a type that takes no precision.
	PrecisionNotAllowed Code = 301
	// PrecisionRequired is a decimal written without its precision.
	PrecisionRequired Code = 302
	// DuplicateTag is one tag on two members of the same declaration. A tag is a
	// position in a MessagePack array, and two members cannot hold one position.
	DuplicateTag Code = 303
	// TagIsReserved is a tag the declaration's own `reserved` list has retired.
	// Catching the reuse is the whole point of writing the reservation down: a
	// reader built against the old schema would decode the new field into the
	// old one and be quietly wrong.
	TagIsReserved Code = 304
	// ReservedRangeReversed is a range whose upper bound is below its lower one,
	// which retires nothing. `14..8` is `8..14` written backwards.
	ReservedRangeReversed Code = 305
	// TypeArgumentCountWrong is a collection given the wrong number of type
	// arguments -- `list<A, B>`, or a bare `list` with none. A collection's
	// arity is fixed by what it is: a list has one element type and a map has a
	// key and a value.
	TypeArgumentCountWrong Code = 306
	// TypeArgumentsNotAllowed is a `<...>` on a type that takes none. Only a
	// collection is generic; `int64<string>` names no type protop has.
	TypeArgumentsNotAllowed Code = 307
	// TagInViewMember is a `= N` on a view member. A view is pinned to its
	// source's tags and only the source extends the tag space: a tag written
	// here is never populated by the producer, so it would deserialize to nil
	// forever while squatting a slot the source might later claim.
	TagInViewMember Code = 308
	// ScalarInViewMember is a scalar in the narrowing position of a view
	// member. The leading name of `SlimOdd odds;` asks for the member's type to
	// be narrowed to another view, and a scalar is not one -- a member that
	// keeps its source's type is written with no leading name at all.
	ScalarInViewMember Code = 309

	// AnnotationNotAllowed is an annotation written where the grammar has no
	// slot for one. An annotation follows what it describes, and there is no
	// before-slot.
	AnnotationNotAllowed Code = 400
	// UnknownAnnotationPrefix is a prefix naming a backend protopc does not
	// have. An unknown prefix is an error rather than a silent drop, because a
	// typo that vanishes from the output is worse than the rule it replaced.
	UnknownAnnotationPrefix Code = 401
	// UnknownAnnotation is an unprefixed annotation outside protop's own set,
	// which is closed and read.
	UnknownAnnotation Code = 402
	// UnterminatedAnnotation is a `[` with no matching `]`.
	UnterminatedAnnotation Code = 403

	// NotYetImplemented is a construct the spec defines and this milestone does
	// not read. Reporting it as its own code, rather than as a syntax error, is
	// the difference between "protopc cannot do this yet" and "you wrote this
	// wrong".
	NotYetImplemented Code = 900
)
