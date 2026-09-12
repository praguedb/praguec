package syntax

import "github.com/nanov/protobuf-to-messagepack-sharp/internal/source"

// Kind is a token's lexical class.
//
// There are no keyword kinds, and that is the load-bearing decision in this
// package. protop spells identity as
//
//	key key = 1 (event_id, bet_type_id);
//	alias bet_type_id = id;
//
// where `key` is the construct and also the member's name, and `id` is a field
// named after a word the language uses elsewhere. A word is a keyword only by
// virtue of where it sits, so the lexer emits every word as KindIdent and the
// parser decides by position. A lexer that reserved words could not accept
// those two lines at all.
type Kind uint8

const (
	// KindEOF is the one-past-the-end token. The lexer always emits it, so the
	// parser never has to bounds-check a peek.
	KindEOF Kind = iota

	// KindIdent is any word: a declaration keyword, a type, a member name.
	KindIdent
	// KindInt is a run of decimal digits. There is no float literal in the
	// grammar -- see Lex.
	KindInt
	// KindString is a double-quoted literal, spanning its quotes.
	KindString

	KindSemi     // ;
	KindComma    // ,
	KindColon    // :
	KindAssign   // =
	KindQuestion // ?
	KindDot      // .
	KindDotDot   // ..

	KindLBrace   // {
	KindRBrace   // }
	KindLParen   // (
	KindRParen   // )
	KindLBracket // [
	KindRBracket // ]
)

// String describes the kind the way a diagnostic wants to read it: punctuation
// quoted as it is written, everything else named.
func (k Kind) String() string {
	switch k {
	case KindEOF:
		return "end of file"
	case KindIdent:
		return "an identifier"
	case KindInt:
		return "an integer"
	case KindString:
		return "a string"
	case KindSemi:
		return "`;`"
	case KindComma:
		return "`,`"
	case KindColon:
		return "`:`"
	case KindAssign:
		return "`=`"
	case KindQuestion:
		return "`?`"
	case KindDot:
		return "`.`"
	case KindDotDot:
		return "`..`"
	case KindLBrace:
		return "`{`"
	case KindRBrace:
		return "`}`"
	case KindLParen:
		return "`(`"
	case KindRParen:
		return "`)`"
	case KindLBracket:
		return "`[`"
	case KindRBracket:
		return "`]`"
	}
	return "an unknown token"
}

// Token is one lexeme: its class, where it is, and -- for KindInt -- its value,
// parsed once by the lexer that already had to scan the digits.
type Token struct {
	Kind Kind
	Span source.Span

	// Int is the literal's value, set for Kind == KindInt and zero otherwise.
	Int uint64
}
