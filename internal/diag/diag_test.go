package diag

import (
	"os"
	"regexp"
	"testing"

	"github.com/nanov/protobuf-to-messagepack-sharp/internal/source"
)

// Every code and the number it has shipped with, written out. codes.go opens
// with the promise that a code keeps its number forever and that a retired one
// is never reused, and until now nothing held it: the suite compared codes to
// each other, so renumbering the whole file -- or giving a new code a number an
// old one already had -- changed nothing a test could see.
//
// The numbers are literals rather than `Code.String()` of the constant beside
// them, because reading the constant would only ask it whether it agrees with
// itself. This table is the oracle; codes.go is what is under test.
var wantCodeNumbers = []struct {
	name string
	code Code
	want string
}{
	{"UnterminatedBlockComment", UnterminatedBlockComment, "PROTOP0001"},
	{"UnterminatedString", UnterminatedString, "PROTOP0002"},
	{"UnexpectedCharacter", UnexpectedCharacter, "PROTOP0003"},
	{"IntegerOutOfRange", IntegerOutOfRange, "PROTOP0004"},

	{"MissingRevision", MissingRevision, "PROTOP0100"},
	{"RevisionNotFirst", RevisionNotFirst, "PROTOP0101"},
	{"UnsupportedRevision", UnsupportedRevision, "PROTOP0102"},
	{"DuplicateRevision", DuplicateRevision, "PROTOP0103"},
	{"MissingNamespace", MissingNamespace, "PROTOP0104"},
	{"DuplicateNamespace", DuplicateNamespace, "PROTOP0105"},
	{"ImportNotInHeader", ImportNotInHeader, "PROTOP0106"},
	{"ImportNotFound", ImportNotFound, "PROTOP0107"},

	{"UnexpectedToken", UnexpectedToken, "PROTOP0200"},
	{"UnknownDeclaration", UnknownDeclaration, "PROTOP0201"},
	{"KeyNotAllowed", KeyNotAllowed, "PROTOP0202"},
	{"ViewSourceMissing", ViewSourceMissing, "PROTOP0203"},
	{"DuplicateDeclaration", DuplicateDeclaration, "PROTOP0204"},
	{"ViewSourceNotFound", ViewSourceNotFound, "PROTOP0205"},
	{"ViewSourceNotSelectable", ViewSourceNotSelectable, "PROTOP0206"},

	{"TagOutOfRange", TagOutOfRange, "PROTOP0300"},
	{"PrecisionNotAllowed", PrecisionNotAllowed, "PROTOP0301"},
	{"PrecisionRequired", PrecisionRequired, "PROTOP0302"},
	{"DuplicateTag", DuplicateTag, "PROTOP0303"},
	{"TagIsReserved", TagIsReserved, "PROTOP0304"},
	{"ReservedRangeReversed", ReservedRangeReversed, "PROTOP0305"},
	{"TypeArgumentCountWrong", TypeArgumentCountWrong, "PROTOP0306"},
	{"TypeArgumentsNotAllowed", TypeArgumentsNotAllowed, "PROTOP0307"},
	{"TagInViewMember", TagInViewMember, "PROTOP0308"},
	{"ScalarInViewMember", ScalarInViewMember, "PROTOP0309"},
	{"DuplicateMember", DuplicateMember, "PROTOP0310"},
	{"UnknownType", UnknownType, "PROTOP0311"},
	{"NoSuchMember", NoSuchMember, "PROTOP0312"},
	{"NarrowingNotAView", NarrowingNotAView, "PROTOP0313"},
	{"NarrowingSourceMismatch", NarrowingSourceMismatch, "PROTOP0314"},
	{"AliasTargetNotFound", AliasTargetNotFound, "PROTOP0315"},
	{"CompositeKeyPartNotFound", CompositeKeyPartNotFound, "PROTOP0316"},
	{"RelationTargetNotFound", RelationTargetNotFound, "PROTOP0317"},
	{"RelationTargetNotATable", RelationTargetNotATable, "PROTOP0318"},

	{"AnnotationNotAllowed", AnnotationNotAllowed, "PROTOP0400"},
	{"UnknownAnnotationPrefix", UnknownAnnotationPrefix, "PROTOP0401"},
	{"UnknownAnnotation", UnknownAnnotation, "PROTOP0402"},
	{"UnterminatedAnnotation", UnterminatedAnnotation, "PROTOP0403"},
	{"MalformedRelation", MalformedRelation, "PROTOP0404"},

	{"NotYetImplemented", NotYetImplemented, "PROTOP0900"},
}

// A code keeps its number, and a number names one fact. Both halves matter: a
// renumbering breaks every changelog entry and editor filter that cites the old
// one, and a collision is worse still, because two different mistakes would
// render as the same code and no message could tell a user which had happened.
func TestEveryCodeKeepsItsNumber(t *testing.T) {
	byNumber := make(map[string]string, len(wantCodeNumbers))
	for _, c := range wantCodeNumbers {
		if got := c.code.String(); got != c.want {
			t.Errorf("%s is %s, want %s; a code that has shipped keeps its number", c.name, got, c.want)
		}
		if prev, taken := byNumber[c.code.String()]; taken {
			t.Errorf("%s and %s are both %s; a number names one fact", prev, c.name, c.code)
		}
		byNumber[c.code.String()] = c.name
	}
}

// The table above is only an oracle if it is complete, so the names -- and only
// the names -- are read back out of codes.go. A code added without a line here
// fails, which is the point at which someone has to decide what group it is in
// and what number is free in it. The numbers stay literals in the table; taking
// those from the source too would be the tautology this test exists to avoid.
func TestEveryCodeIsInTheTable(t *testing.T) {
	src, err := os.ReadFile("codes.go")
	if err != nil {
		t.Fatal(err)
	}
	listed := make(map[string]bool, len(wantCodeNumbers))
	for _, c := range wantCodeNumbers {
		listed[c.name] = true
	}
	declared := regexp.MustCompile(`(?m)^\t(\w+) Code = \d+$`).FindAllStringSubmatch(string(src), -1)
	if len(declared) != len(wantCodeNumbers) {
		t.Errorf("codes.go declares %d codes, the table lists %d", len(declared), len(wantCodeNumbers))
	}
	for _, m := range declared {
		if !listed[m[1]] {
			t.Errorf("%s is declared in codes.go and is in no test that says what number it has", m[1])
		}
	}
}

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

// The severity is rendered, and a warning says so. Nothing in the repo raises
// one today -- NotYetImplemented is the only Warnf caller and no test renders it
// -- so Severity.String could return "error" for both and every golden would
// still pass. The word is the only thing in the line that tells a reader whether
// the build stopped, and a warning that prints as an error is a build someone
// goes looking for a reason to have failed.
func TestRenderSaysWhichSeverityItIs(t *testing.T) {
	f := source.NewFile("x.protop", "map<string, int64> m = 1;\n")
	var b Bag
	b.Warnf(NotYetImplemented, source.Span{Start: 0, End: 3}, "`map` is not read yet")

	want := "x.protop:1:1: warning PROTOP0900: `map` is not read yet\n" +
		"  |\n" +
		"1 | map<string, int64> m = 1;\n" +
		"  | ^^^"
	if got := Render(f, b.All()[0]); got != want {
		t.Errorf("Render =\n%s\n\nwant\n%s", got, want)
	}
	if Warning.String() == Error.String() {
		t.Errorf("both severities render as %q", Error)
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

// A span is a pair of byte offsets and says nothing about which file it indexes
// into, so a stage that sees several files at once -- the checking stage is the
// first -- reports through a sink that stamps the file on. Render then prefers
// the diagnostic's own file over whatever the caller had to hand, which is what
// makes one bag over a whole program renderable at all.
func TestADiagnosticCanNameTheFileItsSpanIsIn(t *testing.T) {
	prague := source.NewFile("prague.protop", "revision 1;\n")
	consumer := source.NewFile("consumer.protop", "import \"nope.protop\";\n")

	var b Bag
	b.In(consumer).Errorf(ImportNotFound, source.Span{Start: 7, End: 20}, "no file %q in the set", "nope.protop")

	got := Render(prague, b.All()[0])
	want := "consumer.protop:1:8: error PROTOP0107: no file \"nope.protop\" in the set\n" +
		"  |\n" +
		"1 | import \"nope.protop\";\n" +
		"  |        ^^^^^^^^^^^^^"
	if got != want {
		t.Errorf("Render =\n%s\n\nwant\n%s", got, want)
	}
	if b.All()[0].File != consumer {
		t.Error("the sink did not stamp its file on the diagnostic")
	}
}

// A stage that sees one file at a time leaves the file off, and the frontend is
// such a stage: the caller handed Parse a file and knows which one it got back.
func TestADiagnosticRaisedOnTheBagNamesNoFile(t *testing.T) {
	var b Bag
	b.Errorf(TagOutOfRange, source.SpanAt(0), "field tag must be greater than 0")
	if b.All()[0].File != nil {
		t.Error("a diagnostic raised on the bag itself must not name a file")
	}
}
