package syntax

import (
	"testing"

	"github.com/praguedb/praguec/internal/diag"
	"github.com/praguedb/praguec/internal/source"
)

// lex tokenises src and fails the test if any lexical diagnostic was raised.
func lex(t *testing.T, src string) (*source.File, []Token) {
	t.Helper()
	f := source.NewFile("x.prague", src)
	var bag diag.Bag
	toks := Lex(f, &bag)
	if bag.HasErrors() {
		t.Fatalf("unexpected diagnostics:\n%s", diag.RenderAll(f, &bag))
	}
	assertGate(t, f, &bag, nil)
	return f, toks
}

func kinds(toks []Token) []Kind {
	out := make([]Kind, len(toks))
	for i, tok := range toks {
		out[i] = tok.Kind
	}
	return out
}

func assertKinds(t *testing.T, got []Kind, want ...Kind) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d tokens %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token %d is %s, want %s (all: %v)", i, got[i], want[i], got)
		}
	}
}

// The whole punctuation set lexes: `[`, `]` and `:` for annotations, `..` for
// reserved ranges, `<` and `>` for a type's arguments. A complete lexer is what
// makes growing the language a parser change alone, and this is the assertion
// that says the set is complete rather than merely sufficient for today.
func TestLexCoversTheWholePunctuationSet(t *testing.T) {
	_, toks := lex(t, "{ } ( ) [ ] < > ; , : = ? . ..")
	assertKinds(t, kinds(toks),
		KindLBrace, KindRBrace, KindLParen, KindRParen, KindLBracket, KindRBracket,
		KindLAngle, KindRAngle,
		KindSemi, KindComma, KindColon, KindAssign, KindQuestion, KindDot, KindDotDot,
		KindEOF)
}

// A collection type is a name and two angle brackets, and the words inside are
// words: `list` and `map` lex as idents like everything else, which is what
// leaves a field free to be named either.
func TestLexTypeArguments(t *testing.T) {
	_, toks := lex(t, "map<string, list<Quote>> m = 1;")
	assertKinds(t, kinds(toks),
		KindIdent, KindLAngle, KindIdent, KindComma, KindIdent, KindLAngle, KindIdent,
		KindRAngle, KindRAngle, KindIdent, KindAssign, KindInt, KindSemi, KindEOF)
}

// `>>` is two tokens and not a shift operator, which is what keeps
// `list<list<int64>>` from needing a lexer hack the way C++ did.
func TestLexDoesNotJoinClosingAngles(t *testing.T) {
	f, toks := lex(t, ">>")
	assertKinds(t, kinds(toks), KindRAngle, KindRAngle, KindEOF)
	if got := f.Slice(toks[0].Span); got != ">" {
		t.Errorf("first angle spans %q, want %q", got, ">")
	}
}

// What the two new token kinds buy beyond the type syntax. Annotation contents
// are scanned out of the token stream after the whole file is lexed, so a
// character the lexer rejects is rejected even inside an annotation: before
// `<` and `>` lexed, a C# attribute carrying a generic cost two PROTOP0003s.
func TestLexAnnotationCarryingAGeneric(t *testing.T) {
	_, toks := lex(t, "[cs: Foo<Bar>]")
	assertKinds(t, kinds(toks),
		KindLBracket, KindIdent, KindColon, KindIdent, KindLAngle, KindIdent, KindRAngle,
		KindRBracket, KindEOF)
}

// A reserved range is two integers and a `..`, not two malformed floats. This
// is why the grammar has no float literal.
func TestLexReservedRange(t *testing.T) {
	f, toks := lex(t, "reserved 8..14, 16..23;")
	assertKinds(t, kinds(toks),
		KindIdent, KindInt, KindDotDot, KindInt, KindComma, KindInt, KindDotDot, KindInt, KindSemi, KindEOF)
	if got := toks[1].Int; got != 8 {
		t.Errorf("first bound = %d, want 8", got)
	}
	if got := toks[3].Int; got != 14 {
		t.Errorf("second bound = %d, want 14", got)
	}
	if got := f.Slice(toks[2].Span); got != ".." {
		t.Errorf("range operator spans %q, want %q", got, "..")
	}
}

// The load-bearing property: there are no keyword token kinds. Both of these
// lines are unparseable by a lexer that reserves words, and both are protop.
func TestLexEmitsEveryWordAsAnIdent(t *testing.T) {
	for _, src := range []string{
		"key key = 1 (event_id, ticket_type_id);",
		"alias ticket_type_id = id;",
		"struct struct = 1;",
	} {
		_, toks := lex(t, src)
		for i, tok := range toks {
			if tok.Kind == KindEOF {
				continue
			}
			switch tok.Kind {
			case KindIdent, KindInt, KindAssign, KindSemi, KindComma, KindLParen, KindRParen:
			default:
				t.Errorf("%q: token %d is %s, want a word or punctuation", src, i, tok.Kind)
			}
		}
	}
}

func TestLexSkipsComments(t *testing.T) {
	_, toks := lex(t, "// leading\nrevision /* inline */ 1; /* trailing\nspanning lines */ ;")
	assertKinds(t, kinds(toks), KindIdent, KindInt, KindSemi, KindSemi, KindEOF)
}

func TestLexStringLiteralSpansItsQuotes(t *testing.T) {
	f, toks := lex(t, `import "prague.prague";`)
	assertKinds(t, kinds(toks), KindIdent, KindString, KindSemi, KindEOF)
	if got := f.Slice(toks[1].Span); got != `"prague.prague"` {
		t.Errorf("string spans %q, want %q", got, `"prague.prague"`)
	}
}

// Every diagnostic the lexer raises, pinned by code, by where it starts and by
// what it underlines.
//
// wantSpan is the third of those and was the missing one: a line and a column
// say where a caret goes and nothing said how far it runs, so the underline
// under `99999999999999999999` could shrink to the first `9` with the suite
// green. Three of these five codes -- 0002, 0003 and 0004 -- had no width
// assertion anywhere in the repo.
//
// It is a plain string and not the parser table's *string because a lexical
// span cannot be empty: the lexer raises about bytes it has already consumed,
// so there is always at least the one it choked on. An unterminated block
// comment is deliberately the narrowest of them -- the two opening bytes rather
// than the rest of the file, which is what keeps the caret off every line below
// it.
func TestLexDiagnostics(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantCode diag.Code
		wantLine int
		wantCol  int
		wantSpan string
	}{
		{"unterminated block comment", "revision 1;\n/* and then nothing\n", diag.UnterminatedBlockComment, 2, 1, "/*"},
		{"unterminated string at a newline", "import \"prague.prague\n;", diag.UnterminatedString, 1, 8, "\"prague.prague"},
		{"unterminated string at end of file", "import \"prague", diag.UnterminatedString, 1, 8, "\"prague"},
		{"unexpected character", "revision @ 1;", diag.UnexpectedCharacter, 1, 10, "@"},
		{"integer out of range", "tag = 99999999999999999999;", diag.IntegerOutOfRange, 1, 7, "99999999999999999999"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := source.NewFile("x.prague", tt.src)
			var bag diag.Bag
			Lex(f, &bag)

			if bag.Len() != 1 {
				t.Fatalf("got %d diagnostics, want 1:\n%s", bag.Len(), diag.RenderAll(f, &bag))
			}
			d := bag.All()[0]
			if d.Code != tt.wantCode {
				t.Errorf("code = %s, want %s", d.Code, tt.wantCode)
			}
			assertGate(t, f, &bag, []diag.Code{tt.wantCode})
			if pos := f.Position(d.Span.Start); pos.Line != tt.wantLine || pos.Column != tt.wantCol {
				t.Errorf("reported at %d:%d, want %d:%d", pos.Line, pos.Column, tt.wantLine, tt.wantCol)
			}
			if got := f.Slice(d.Span); got != tt.wantSpan {
				t.Errorf("underlines %q, want %q", got, tt.wantSpan)
			}
		})
	}
}

// One stray byte must not hide every mistake after it: the lexer reports and
// keeps scanning.
func TestLexRecoversFromAnUnexpectedCharacter(t *testing.T) {
	f := source.NewFile("x.prague", "revision # 1;")
	var bag diag.Bag
	toks := Lex(f, &bag)
	if bag.Len() != 1 {
		t.Fatalf("got %d diagnostics, want 1", bag.Len())
	}
	assertKinds(t, kinds(toks), KindIdent, KindInt, KindSemi, KindEOF)
}

func TestLexAlwaysEndsWithEOF(t *testing.T) {
	for _, src := range []string{"", "   ", "// just a comment", "revision 1;"} {
		_, toks := lex(t, src)
		if len(toks) == 0 || toks[len(toks)-1].Kind != KindEOF {
			t.Errorf("%q: token stream does not end with EOF: %v", src, kinds(toks))
		}
	}
}
