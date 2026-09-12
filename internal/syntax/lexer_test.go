package syntax

import (
	"testing"

	"github.com/nanov/protobuf-to-messagepack-sharp/internal/diag"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/source"
)

// lex tokenises src and fails the test if any lexical diagnostic was raised.
func lex(t *testing.T, src string) (*source.File, []Token) {
	t.Helper()
	f := source.NewFile("x.protop", src)
	var bag diag.Bag
	toks := Lex(f, &bag)
	if bag.HasErrors() {
		t.Fatalf("unexpected diagnostics:\n%s", diag.RenderAll(f, &bag))
	}
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

// The whole punctuation set lexes, including the three pieces the parser cannot
// read yet: `[`, `]` and `:` for annotations, `..` for reserved ranges. A
// complete lexer is what makes growing the language a parser change alone.
func TestLexCoversTheWholePunctuationSet(t *testing.T) {
	_, toks := lex(t, "{ } ( ) [ ] ; , : = ? . ..")
	assertKinds(t, kinds(toks),
		KindLBrace, KindRBrace, KindLParen, KindRParen, KindLBracket, KindRBracket,
		KindSemi, KindComma, KindColon, KindAssign, KindQuestion, KindDot, KindDotDot,
		KindEOF)
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
		"key key = 1 (event_id, bet_type_id);",
		"alias bet_type_id = id;",
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
	f, toks := lex(t, `import "prague.protop";`)
	assertKinds(t, kinds(toks), KindIdent, KindString, KindSemi, KindEOF)
	if got := f.Slice(toks[1].Span); got != `"prague.protop"` {
		t.Errorf("string spans %q, want %q", got, `"prague.protop"`)
	}
}

func TestLexDiagnostics(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantCode diag.Code
		wantLine int
		wantCol  int
	}{
		{"unterminated block comment", "revision 1;\n/* and then nothing\n", diag.UnterminatedBlockComment, 2, 1},
		{"unterminated string at a newline", "import \"prague.protop\n;", diag.UnterminatedString, 1, 8},
		{"unterminated string at end of file", "import \"prague", diag.UnterminatedString, 1, 8},
		{"unexpected character", "revision @ 1;", diag.UnexpectedCharacter, 1, 10},
		{"integer out of range", "tag = 99999999999999999999;", diag.IntegerOutOfRange, 1, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := source.NewFile("x.protop", tt.src)
			var bag diag.Bag
			Lex(f, &bag)

			if bag.Len() != 1 {
				t.Fatalf("got %d diagnostics, want 1:\n%s", bag.Len(), diag.RenderAll(f, &bag))
			}
			d := bag.All()[0]
			if d.Code != tt.wantCode {
				t.Errorf("code = %s, want %s", d.Code, tt.wantCode)
			}
			if pos := f.Position(d.Span.Start); pos.Line != tt.wantLine || pos.Column != tt.wantCol {
				t.Errorf("reported at %d:%d, want %d:%d", pos.Line, pos.Column, tt.wantLine, tt.wantCol)
			}
		})
	}
}

// One stray byte must not hide every mistake after it: the lexer reports and
// keeps scanning.
func TestLexRecoversFromAnUnexpectedCharacter(t *testing.T) {
	f := source.NewFile("x.protop", "revision # 1;")
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
