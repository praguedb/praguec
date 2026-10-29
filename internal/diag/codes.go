package diag

import "fmt"

// Code identifies a kind of diagnostic. Codes are part of praguec's interface:
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

// Severity says what a diagnostic under this code does to the compile, and it
// is a property of the code rather than of the place that raises it.
//
// The code is the diagnostic's identity -- what a changelog cites, what an
// editor filters on, what a build script greps for, as this file opens by
// saying. A fact that stops the build where one stage reports it and is
// advisory where another does is not one fact, and a user filtering on
// PROTOP0303 has no way to say which of the two they meant. So the severity is
// written down once, beside the number: a shipped code keeps its number
// forever, and it keeps this too.
//
// It used to live at the call site, as the choice between Bag.Errorf and
// Bag.Warnf. That made a demotion one word in one file, invisible to every
// test -- and a demoted error means HasErrors reports false and praguec
// ACCEPTS a schema it exists to reject.
func (c Code) Severity() Severity { return severities[c] }

// severities names the codes that do not fail the compile. A code absent from
// it takes the zero Severity, which is Error, and the direction of that default
// is the point: a code added without a line here costs a build that stops when
// it could have carried on, which someone reports within the hour. Defaulting
// the other way would cost a schema accepted that should have been rejected,
// which nobody sees until the wire format is wrong.
//
// Every code praguec has today is an error -- nothing in protop is advisory
// yet, `map` included: the parser reports NotYetImplemented as an error, which
// is what makes the driver stop rather than hand a half-read file to a backend.
// The table is empty and is the place the first warning gets written down.
var severities = map[Code]Severity{}

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

	// ImportNotFound is an import naming no file in the set the checker was
	// handed. Which files exist and where they came from is the distribution
	// model's question, not this one's: the checker is given a set and reports
	// an import that is not in it.
	ImportNotFound Code = 107

	// ImportAmbiguous is an import string that more than one include root
	// provides. The loader searches the roots in the order the user gave them,
	// so it always has a file it *could* take -- but which root should win when
	// two answer is praguedb/praguec#48 and is unresolved,
	// and a build whose meaning depends on an unresolved rule should say so
	// rather than pick in silence. Reported, not ranked.
	ImportAmbiguous Code = 108

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

	// DuplicateDeclaration is two declarations of one name in one file. Which
	// of them a reference meant is not a question the language answers.
	DuplicateDeclaration Code = 204
	// ViewSourceNotFound is a view whose source names no declaration in scope
	// -- in its own file, or in one it imports.
	ViewSourceNotFound Code = 205
	// ViewSourceNotSelectable is a view of something that has no members to
	// select at that thing's own tags: a struct, an enum, or another view. A
	// view is a subset of a table or a record, which is section 5 in one
	// sentence.
	ViewSourceNotSelectable Code = 206
	// BuiltinTypeDeclared is a declaration whose name is one protop's own type
	// system already holds -- `record list`, `struct string`. protop has no
	// keyword token kinds, so it parses; and then nothing can ever reach it,
	// because a type written `list` or `string` is read as the built-in and the
	// declaration has no second spelling. Held apart from DuplicateDeclaration,
	// which is two declarations arguing over one name: here the name was never
	// the schema's to take, and there is no other declaration to rename.
	BuiltinTypeDeclared Code = 207

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
	// member. The leading name of `SlimQuote quotes;` asks for the member's type to
	// be narrowed to another view, and a scalar is not one -- a member that
	// keeps its source's type is written with no leading name at all.
	ScalarInViewMember Code = 309

	// DuplicateMember is two members of one declaration under one name. Held
	// apart from DuplicateTag on purpose: two members may collide by name
	// without colliding by tag -- an alias carries no tag at all -- and the two
	// facts have different fixes.
	DuplicateMember Code = 310
	// UnknownType is a type naming neither one of protop's scalars nor a
	// declaration in scope. A file sees its own declarations and those of the
	// files it imports, and nothing else, so the fix is a declaration or an
	// import.
	UnknownType Code = 311
	// NoSuchMember is a view member naming nothing its source declares. This is
	// the diagnostic the whole design exists to produce: when the shared repo
	// renames or retires a field, every view that still selects the old name
	// fails to compile, in the repo that drifted, instead of decoding to nil at
	// runtime.
	NoSuchMember Code = 312
	// NarrowingNotAView is a view member narrowed to a declaration that is not
	// a view. The leading name of `SlimQuote quotes;` asks for the member's type to
	// be narrowed, and only a view narrows one; a member that keeps its
	// source's type is written as its name alone.
	NarrowingNotAView Code = 313
	// NarrowingSourceMismatch is a narrowing to a view of some other type than
	// the member's own. It is the check the construct exists for: a narrowing
	// that points elsewhere would hand the member a set of tags belonging to a
	// different declaration.
	NarrowingSourceMismatch Code = 314
	// AliasTargetNotFound is an `alias x = y;` whose y names no other member of
	// the same declaration. An alias is a get and set passthrough and has
	// nothing to pass through to.
	AliasTargetNotFound Code = 315
	// CompositeKeyPartNotFound is a part of `key key = 1 (a, b);` naming no
	// member of the declaration the key belongs to.
	CompositeKeyPartNotFound Code = 316
	// RelationTargetNotFound is a `[references(T)]` whose T names no
	// declaration in scope.
	RelationTargetNotFound Code = 317
	// RelationTargetNotATable is a relation pointing at something that carries
	// no identity. A relation resolves against a key, and only a table has one:
	// a record or a struct is stored inside whatever holds it and there is
	// nothing to point at.
	RelationTargetNotATable Code = 318

	// AnnotationNotAllowed is an annotation written where the grammar has no
	// slot for one. An annotation follows what it describes, and there is no
	// before-slot.
	AnnotationNotAllowed Code = 400
	// UnknownAnnotationPrefix is a prefix naming a backend praguec does not
	// have. An unknown prefix is an error rather than a silent drop, because a
	// typo that vanishes from the output is worse than the rule it replaced.
	UnknownAnnotationPrefix Code = 401
	// UnknownAnnotation is an unprefixed annotation outside protop's own set,
	// which is closed and read.
	UnknownAnnotation Code = 402
	// UnterminatedAnnotation is a `[` with no matching `]`.
	UnterminatedAnnotation Code = 403

	// MalformedRelation is protop's own `references` written in some shape
	// other than `references(T)`. The parser checks an annotation's position
	// and its prefix and nothing else -- a prefixed annotation is a backend's
	// language -- so protop's own closed set is read here, where a name can be
	// looked up.
	MalformedRelation Code = 404

	// NotYetImplemented is a construct the spec defines and this milestone does
	// not read. Reporting it as its own code, rather than as a syntax error, is
	// the difference between "praguec cannot do this yet" and "you wrote this
	// wrong".
	NotYetImplemented Code = 900
)
