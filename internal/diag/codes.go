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

	// UnexpectedToken is the generic shape error: the parser wanted one thing
	// and found another.
	UnexpectedToken Code = 200
	// UnknownDeclaration is a top-level word that names no declaration keyword.
	UnknownDeclaration Code = 201

	// TagOutOfRange is a field tag outside the legal range. A tag is 1-based
	// because a MessagePack key is tag - 1, so tag 0 names no slot.
	TagOutOfRange Code = 300
	// PrecisionNotAllowed is a (N) on a type that takes no precision.
	PrecisionNotAllowed Code = 301
	// PrecisionRequired is a decimal written without its precision.
	PrecisionRequired Code = 302

	// NotYetImplemented is a construct the spec defines and this milestone does
	// not read. Reporting it as its own code, rather than as a syntax error, is
	// the difference between "protopc cannot do this yet" and "you wrote this
	// wrong".
	NotYetImplemented Code = 900
)
