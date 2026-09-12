package diag

import (
	"testing"

	"github.com/nanov/protobuf-to-messagepack-sharp/internal/source"
)

func TestCodeString(t *testing.T) {
	if got := UnterminatedBlockComment.String(); got != "PROTOP0001" {
		t.Errorf("got %q, want PROTOP0001", got)
	}
	if got := NotYetImplemented.String(); got != "PROTOP0900" {
		t.Errorf("got %q, want PROTOP0900", got)
	}
}

func TestRenderUnderlinesTheSpan(t *testing.T) {
	f := source.NewFile("prague.protop", "revision 1;\nstruct Venue {\n  int64 id = 0;\n}\n")
	var b Bag
	b.Errorf(TagOutOfRange, source.Span{Start: 40, End: 41}, "field tag must be greater than 0")

	want := "prague.protop:3:14: error PROTOP0300: field tag must be greater than 0\n" +
		"  |\n" +
		"3 |   int64 id = 0;\n" +
		"  |              ^"
	if got := Render(f, b.All()[0]); got != want {
		t.Errorf("Render =\n%s\n\nwant\n%s", got, want)
	}
}

func TestRenderClampsAMultiLineSpanToItsFirstLine(t *testing.T) {
	f := source.NewFile("x.protop", "/* forever\nand ever\n")
	var b Bag
	b.Errorf(UnterminatedBlockComment, source.Span{Start: 0, End: source.Pos(len(f.Text))}, "unterminated block comment")

	want := "x.protop:1:1: error PROTOP0001: unterminated block comment\n" +
		"  |\n" +
		"1 | /* forever\n" +
		"  | ^^^^^^^^^^"
	if got := Render(f, b.All()[0]); got != want {
		t.Errorf("Render =\n%s\n\nwant\n%s", got, want)
	}
}

func TestRenderOfAnEmptyLineIsTheHeadlineAlone(t *testing.T) {
	f := source.NewFile("x.protop", "")
	var b Bag
	b.Errorf(MissingRevision, source.SpanAt(0), "a protop file must open with `revision 1;`")

	want := "x.protop:1:1: error PROTOP0100: a protop file must open with `revision 1;`"
	if got := Render(f, b.All()[0]); got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}

// Diagnostics are collected, never thrown: a bag holds every one in the order
// it was raised, and severity is what decides whether the compile failed.
func TestBagCollects(t *testing.T) {
	var b Bag
	if b.HasErrors() || b.Len() != 0 {
		t.Fatalf("a fresh bag reports Len=%d HasErrors=%v", b.Len(), b.HasErrors())
	}
	b.Warnf(NotYetImplemented, source.SpanAt(0), "first")
	if b.HasErrors() {
		t.Error("a warning must not fail the compile")
	}
	b.Errorf(TagOutOfRange, source.SpanAt(1), "second")
	if !b.HasErrors() || b.Len() != 2 {
		t.Fatalf("Len=%d HasErrors=%v, want 2 and true", b.Len(), b.HasErrors())
	}
	if all := b.All(); all[0].Message != "first" || all[1].Message != "second" {
		t.Errorf("diagnostics out of order: %q, %q", all[0].Message, all[1].Message)
	}
}
