package syntax

import (
	"strconv"
	"unicode/utf8"

	"github.com/nanov/protobuf-to-messagepack-sharp/internal/diag"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/source"
)

// Lex tokenises a whole file, appending any lexical diagnostics to bag. The
// returned slice always ends with an KindEOF token.
//
// The lexer covers protop's entire punctuation set, including what the parser
// cannot read yet: `[`, `]` and `:` belong to annotations, `..` to reserved
// ranges, `<` and `>` to a type's arguments. Tokenising is cheap, and a
// complete lexer means growing the language is a parser change alone.
//
// `<` and `>` are also what make an annotation carrying a generic -- `[cs:
// Foo<Bar>]` -- scan as text rather than as two unexpected characters:
// annotation contents are read out of the token stream after the whole file has
// been lexed, so a character the lexer rejects is rejected even inside them.
//
// Integer literals have no decimal point, which is what makes `8..14` lex as
// 8, `..`, 14 rather than as two malformed floats. Nothing in the grammar is a
// float: tags, enum values, reserved ranges and decimal precision are all
// integers, and the money type carries its scale as `decimal(4)`.
//
// Lexical errors never stop the scan. An unexpected character is reported and
// skipped, so one stray byte does not hide every mistake after it.
func Lex(f *source.File, bag *diag.Bag) []Token {
	l := lexer{f: f, src: f.Text, bag: bag}
	// One token per ~4 bytes of source is a deliberate over-estimate: the slice
	// is short-lived and one allocation beats a dozen regrowths.
	l.toks = make([]Token, 0, len(f.Text)/4+1)
	l.run()
	return l.toks
}

type lexer struct {
	f    *source.File
	src  string
	bag  *diag.Bag
	toks []Token
	i    int // byte offset of the next unread character
}

func (l *lexer) run() {
	for {
		l.skipSpaceAndComments()
		if l.i >= len(l.src) {
			l.emit(KindEOF, l.i, l.i)
			return
		}

		start := l.i
		c := l.src[start]
		switch {
		case isIdentStart(c):
			l.i++
			for l.i < len(l.src) && isIdentContinue(l.src[l.i]) {
				l.i++
			}
			l.emit(KindIdent, start, l.i)

		case isDigit(c):
			for l.i < len(l.src) && isDigit(l.src[l.i]) {
				l.i++
			}
			l.emitInt(start, l.i)

		case c == '"':
			l.lexString(start)

		default:
			if kind, width := l.punctuation(start); width > 0 {
				l.i += width
				l.emit(kind, start, l.i)
				continue
			}
			_, width := utf8.DecodeRuneInString(l.src[start:])
			l.i += width
			l.bag.Raise(diag.UnexpectedCharacter, source.Span{Start: source.Pos(start), End: source.Pos(l.i)},
				"unexpected character %q", l.src[start:l.i])
		}
	}
}

// punctuation matches the operator at offset i, returning its kind and byte
// width, or width 0 when nothing matches. `..` is tested before `.` -- the only
// place in the language where one operator prefixes another.
func (l *lexer) punctuation(i int) (Kind, int) {
	if l.src[i] == '.' {
		if i+1 < len(l.src) && l.src[i+1] == '.' {
			return KindDotDot, 2
		}
		return KindDot, 1
	}
	switch l.src[i] {
	case ';':
		return KindSemi, 1
	case ',':
		return KindComma, 1
	case ':':
		return KindColon, 1
	case '=':
		return KindAssign, 1
	case '?':
		return KindQuestion, 1
	case '{':
		return KindLBrace, 1
	case '}':
		return KindRBrace, 1
	case '(':
		return KindLParen, 1
	case ')':
		return KindRParen, 1
	case '[':
		return KindLBracket, 1
	case ']':
		return KindRBracket, 1
	case '<':
		return KindLAngle, 1
	case '>':
		return KindRAngle, 1
	}
	return 0, 0
}

// lexString scans a double-quoted literal. Escapes are recognised only well
// enough to keep a `\"` from closing the string; nothing decodes the contents
// yet, because the only consumers -- import paths and annotation text -- are
// both carried through verbatim.
func (l *lexer) lexString(start int) {
	l.i++ // opening quote
	for l.i < len(l.src) {
		switch l.src[l.i] {
		case '\\':
			l.i++
			if l.i < len(l.src) && l.src[l.i] != '\n' {
				l.i++
			}
		case '"':
			l.i++
			l.emit(KindString, start, l.i)
			return
		case '\n':
			// A string never spans a line: reporting it here, rather than at
			// the end of the file, points at the line that forgot the quote.
			l.unterminatedString(start)
			return
		default:
			l.i++
		}
	}
	l.unterminatedString(start)
}

func (l *lexer) unterminatedString(start int) {
	l.bag.Raise(diag.UnterminatedString, source.Span{Start: source.Pos(start), End: source.Pos(l.i)},
		"unterminated string literal")
	l.emit(KindString, start, l.i)
}

func (l *lexer) skipSpaceAndComments() {
	for l.i < len(l.src) {
		switch c := l.src[l.i]; {
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			l.i++
		case c == '/' && l.i+1 < len(l.src) && l.src[l.i+1] == '/':
			for l.i < len(l.src) && l.src[l.i] != '\n' {
				l.i++
			}
		case c == '/' && l.i+1 < len(l.src) && l.src[l.i+1] == '*':
			l.skipBlockComment()
		default:
			return
		}
	}
}

// skipBlockComment consumes a /* ... */ comment. Block comments do not nest,
// following Protobuf and C: a `/*` inside one is ordinary comment text.
func (l *lexer) skipBlockComment() {
	start := l.i
	l.i += 2
	for l.i+1 < len(l.src) {
		if l.src[l.i] == '*' && l.src[l.i+1] == '/' {
			l.i += 2
			return
		}
		l.i++
	}
	l.i = len(l.src)
	// The span is the opening delimiter alone. Underlining the rest of the file
	// would be accurate and useless; the `/*` is what has to be found.
	l.bag.Raise(diag.UnterminatedBlockComment, source.Span{Start: source.Pos(start), End: source.Pos(start + 2)},
		"unterminated block comment")
}

func (l *lexer) emit(kind Kind, start, end int) {
	l.toks = append(l.toks, Token{Kind: kind, Span: source.Span{Start: source.Pos(start), End: source.Pos(end)}})
}

func (l *lexer) emitInt(start, end int) {
	span := source.Span{Start: source.Pos(start), End: source.Pos(end)}
	v, err := strconv.ParseUint(l.src[start:end], 10, 64)
	if err != nil {
		// Out of range is the only way ParseUint fails here: the scan admitted
		// digits and nothing else. Emitting the token anyway keeps the parser
		// on its feet, and the bag already says the file is bad.
		l.bag.Raise(diag.IntegerOutOfRange, span, "integer literal %s does not fit in 64 bits", l.src[start:end])
	}
	l.toks = append(l.toks, Token{Kind: KindInt, Span: span, Int: v})
}

func isIdentStart(c byte) bool {
	return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

func isIdentContinue(c byte) bool { return isIdentStart(c) || isDigit(c) }

func isDigit(c byte) bool { return '0' <= c && c <= '9' }
