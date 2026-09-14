package diag

import (
	"os"
	"regexp"
	"strings"
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
	{"ImportAmbiguous", ImportAmbiguous, "PROTOP0108"},

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

// Every code and the severity it carries. Severity is what a diagnostic DOES:
// an error is what HasErrors reports and what stops the driver before it hands
// a half-read file to a backend, so a code demoted to a warning is protopc
// accepting a schema it exists to reject. It used to be unpinnable -- the
// severity was an argument at each of the 56 call sites, chosen by which method
// name was typed, and nothing about a Code could be asked what it was.
//
// Every code is an error today. That is not a reason to leave the table out:
// the table is what makes a demotion a line of this file rather than a silent
// change of behaviour, and "all of them" is the claim being made.
var wantCodeSeverities = []struct {
	name string
	code Code
	want Severity
}{
	{"UnterminatedBlockComment", UnterminatedBlockComment, Error},
	{"UnterminatedString", UnterminatedString, Error},
	{"UnexpectedCharacter", UnexpectedCharacter, Error},
	{"IntegerOutOfRange", IntegerOutOfRange, Error},

	{"MissingRevision", MissingRevision, Error},
	{"RevisionNotFirst", RevisionNotFirst, Error},
	{"UnsupportedRevision", UnsupportedRevision, Error},
	{"DuplicateRevision", DuplicateRevision, Error},
	{"MissingNamespace", MissingNamespace, Error},
	{"DuplicateNamespace", DuplicateNamespace, Error},
	{"ImportNotInHeader", ImportNotInHeader, Error},
	{"ImportNotFound", ImportNotFound, Error},
	{"ImportAmbiguous", ImportAmbiguous, Error},

	{"UnexpectedToken", UnexpectedToken, Error},
	{"UnknownDeclaration", UnknownDeclaration, Error},
	{"KeyNotAllowed", KeyNotAllowed, Error},
	{"ViewSourceMissing", ViewSourceMissing, Error},
	{"DuplicateDeclaration", DuplicateDeclaration, Error},
	{"ViewSourceNotFound", ViewSourceNotFound, Error},
	{"ViewSourceNotSelectable", ViewSourceNotSelectable, Error},

	{"TagOutOfRange", TagOutOfRange, Error},
	{"PrecisionNotAllowed", PrecisionNotAllowed, Error},
	{"PrecisionRequired", PrecisionRequired, Error},
	{"DuplicateTag", DuplicateTag, Error},
	{"TagIsReserved", TagIsReserved, Error},
	{"ReservedRangeReversed", ReservedRangeReversed, Error},
	{"TypeArgumentCountWrong", TypeArgumentCountWrong, Error},
	{"TypeArgumentsNotAllowed", TypeArgumentsNotAllowed, Error},
	{"TagInViewMember", TagInViewMember, Error},
	{"ScalarInViewMember", ScalarInViewMember, Error},
	{"DuplicateMember", DuplicateMember, Error},
	{"UnknownType", UnknownType, Error},
	{"NoSuchMember", NoSuchMember, Error},
	{"NarrowingNotAView", NarrowingNotAView, Error},
	{"NarrowingSourceMismatch", NarrowingSourceMismatch, Error},
	{"AliasTargetNotFound", AliasTargetNotFound, Error},
	{"CompositeKeyPartNotFound", CompositeKeyPartNotFound, Error},
	{"RelationTargetNotFound", RelationTargetNotFound, Error},
	{"RelationTargetNotATable", RelationTargetNotATable, Error},

	{"AnnotationNotAllowed", AnnotationNotAllowed, Error},
	{"UnknownAnnotationPrefix", UnknownAnnotationPrefix, Error},
	{"UnknownAnnotation", UnknownAnnotation, Error},
	{"UnterminatedAnnotation", UnterminatedAnnotation, Error},
	{"MalformedRelation", MalformedRelation, Error},

	{"NotYetImplemented", NotYetImplemented, Error},
}

// A code keeps its severity the way it keeps its number, and for the same
// reason: both are what the code MEANS to whoever reads protopc's output, and
// neither is the business of the stage that happens to raise it.
//
// The collision check is the whole of what holds the table's CODE column, and
// it needs to be, because the want column cannot hold it. Over in the number
// table each row carries a literal -- "PROTOP0304" -- so pointing a row's code
// at the wrong constant is caught by the row itself. Here every row's
// answer is the same word, Error, so the want column carries no identity at
// all: swap a row's code for another code and the row still passes. Two edits
// then hide a demotion completely -- register TagIsReserved as a Warning in
// codes.go, and point its row at TagOutOfRange -- and protopc goes on to
// GENERATE C# for a field reusing a retired tag, printing a warning and
// exiting 0. Measured across the language, 29 of the 45 codes survived that.
//
// What kills all 29 is one map. TestEveryCodeIsInTheSeverityTable pins the
// table complete by NAME against codes.go, so 45 rows name 45 distinct codes;
// any row aliased to another row's constant necessarily makes some code value
// appear twice, and some other code value not appear at all. The completeness
// being pinned elsewhere is what makes counting duplicates sufficient here.
func TestEveryCodeKeepsItsSeverity(t *testing.T) {
	byCode := make(map[Code]string, len(wantCodeSeverities))
	for _, c := range wantCodeSeverities {
		if got := c.code.Severity(); got != c.want {
			t.Errorf("%s is a %s, want a %s", c.name, got, c.want)
		}
		if prev, taken := byCode[c.code]; taken {
			t.Errorf("the rows for %s and %s both hold %s, so one code in the language has no row saying whether it fails the compile", prev, c.name, c.code)
		}
		byCode[c.code] = c.name
	}
}

// Nothing in protop is advisory, said once about the whole language rather than
// once per code.
//
// This is the third place a severity is written down and the three are not one
// fact repeated. severities says what a code does; wantCodeSeverities is the
// oracle that pins each code by name; and this pins the SHAPE of the set. The
// first two can be edited together -- and a demotion made in both agrees with
// itself, passes TestEveryCodeKeepsItsSeverity, and passes every derived gate
// assertion in check, load and syntax, because those derive what they expect
// from this package and so follow a demotion wherever it goes. Measured across
// all 45 codes, such an edit went unnoticed for 29 of them. This is what
// notices.
//
// It reads the code column of the oracle and ignores its want column on
// purpose: the table is held complete by TestEveryCodeIsInTheSeverityTable, so
// iterating it is iterating the language, and the expectation here is a literal
// that no edit to either of the other two places can move.
//
// It is the coarsest of the three because the fact is coarse: protop has no
// warning. `map` is not one -- the parser raises NotYetImplemented as an error,
// which is what stops the driver instead of handing a half-read file to a
// backend. The day protop grows an advisory diagnostic, this test is where that
// decision gets argued rather than absorbed, which is the right price for it.
func TestNoCodeIsAdvisoryYet(t *testing.T) {
	for _, c := range wantCodeSeverities {
		if got := c.code.Severity(); got != Error {
			t.Errorf("%s is a %s; protop has no advisory diagnostic, and a code that does not fail the compile is one protopc prints and then accepts the schema anyway", c.name, got)
		}
	}
}

// The table is only an oracle if it is complete, so the names are read back out
// of codes.go, exactly as TestEveryCodeIsInTheTable does for the numbers. A
// code added without a line here fails, which is the point at which someone has
// to say out loud whether the new diagnostic stops the build.
//
// The severities stay literals in the table. Reading them from the map in
// codes.go would ask the implementation whether it agrees with itself, and
// since an unlisted code takes the zero value it would agree about every code
// nobody has thought about.
func TestEveryCodeIsInTheSeverityTable(t *testing.T) {
	src, err := os.ReadFile("codes.go")
	if err != nil {
		t.Fatal(err)
	}
	listed := make(map[string]bool, len(wantCodeSeverities))
	for _, c := range wantCodeSeverities {
		listed[c.name] = true
	}
	declared := regexp.MustCompile(`(?m)^\t(\w+) Code = \d+$`).FindAllStringSubmatch(string(src), -1)
	if len(declared) != len(wantCodeSeverities) {
		t.Errorf("codes.go declares %d codes, the severity table lists %d", len(declared), len(wantCodeSeverities))
	}
	for _, m := range declared {
		if !listed[m[1]] {
			t.Errorf("%s is declared in codes.go and is in no test that says whether it fails the compile", m[1])
		}
	}
}

// Raise reads the severity off the code rather than holding one of its own,
// which is the whole of the change and is invisible while every code is an
// error: Raise could pass Error as a constant and the table above would agree
// with it about all 45. So one code is registered as a warning for the length
// of this test and the answer is read back through both raising paths.
//
// The registry being a map, and being in this package, is what makes that
// possible -- and is the reason the severity table lives in codes.go rather
// than in a switch inside Raise.
func TestRaiseTakesTheSeverityFromTheCode(t *testing.T) {
	severities[NotYetImplemented] = Warning
	t.Cleanup(func() { delete(severities, NotYetImplemented) })

	f := source.NewFile("x.protop", "revision 1;\n")
	var b Bag
	b.Raise(NotYetImplemented, source.SpanAt(0), "on the bag")
	b.In(f).Raise(NotYetImplemented, source.SpanAt(0), "on the sink")

	for _, d := range b.All() {
		if d.Severity != Warning {
			t.Errorf("%q raised as a %s; Raise is not reading the code's severity", d.Message, d.Severity)
		}
	}
	if b.HasErrors() {
		t.Error("a bag of warnings reports HasErrors")
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
	b.Raise(TagOutOfRange, source.Span{Start: 40, End: 41}, "field tag must be greater than 0")

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
	b.Raise(UnterminatedBlockComment, source.Span{Start: 0, End: source.Pos(len(f.Text))}, "unterminated block comment")

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
	b.Raise(MissingRevision, source.SpanAt(0), "a protop file must open with `revision 1;`")

	want := "x.protop:1:1: error PROTOP0100: a protop file must open with `revision 1;`"
	if got := Render(f, b.All()[0]); got != want {
		t.Errorf("Render = %q, want %q", got, want)
	}
}

// The severity is rendered, and a warning says so. No code in the language is
// a warning today -- `map` included: NotYetImplemented is an error, which is
// what makes Check bail on a file holding one -- so Severity.String could
// return "error" for both and every golden raised through Raise would still
// pass. The unexported add is how this test raises one anyway: the warning path
// has to keep working for the first code that wants it, and a path with no test
// on it is a path that is already broken and does not know. The word is the
// only thing in the line that tells a reader whether the build stopped, and a
// warning that prints as an error is a build someone goes looking for a reason
// to have failed.
func TestRenderSaysWhichSeverityItIs(t *testing.T) {
	f := source.NewFile("x.protop", "map<string, int64> m = 1;\n")
	var b Bag
	b.add(Warning, NotYetImplemented, source.Span{Start: 0, End: 3}, "`map` is not read yet")

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
	b.add(Warning, NotYetImplemented, source.SpanAt(0), "first")
	if b.HasErrors() {
		t.Error("a warning must not fail the compile")
	}
	b.Raise(TagOutOfRange, source.SpanAt(1), "second")
	if !b.HasErrors() || b.Len() != 2 {
		t.Fatalf("Len=%d HasErrors=%v, want 2 and true", b.Len(), b.HasErrors())
	}
	if all := b.All(); all[0].Message != "first" || all[1].Message != "second" {
		t.Errorf("diagnostics out of order: %q, %q", all[0].Message, all[1].Message)
	}
}

// HasErrors looks at every diagnostic, not at one of them.
//
// TestBagCollects builds its bag warning-then-error and so pins one half: a
// HasErrors rewritten to read only the FIRST diagnostic fails there. The other
// half was unheld -- one rewritten to read only the LAST passed the whole
// suite, because no bag anywhere in it held an error followed by a warning.
//
// That is harmless while every code is an error and stops being harmless on
// exactly the day this severity axis was built for. Recovery means a bag
// collects several diagnostics per file and the driver fills one bag across
// all of them, so the first advisory code protop ships will land after an
// error constantly -- and a gate reading the last diagnostic would report a
// clean compile for a schema it had just printed an error about.
//
// The bag is built through the unexported add, which is why add is unexported
// and in-package rather than gone: no code in the language is a warning, so
// there is no way to raise one through the exported path and no way to write
// this test without it.
func TestHasErrorsFindsAnErrorAnywhereInTheBag(t *testing.T) {
	for _, tc := range []struct {
		name string
		sevs []Severity
		want bool
	}{
		{"nothing at all", nil, false},
		{"a warning alone", []Severity{Warning}, false},
		{"an error alone", []Severity{Error}, true},
		{"a warning then an error", []Severity{Warning, Error}, true},
		{"an error then a warning", []Severity{Error, Warning}, true},
		{"an error between warnings", []Severity{Warning, Error, Warning}, true},
		{"warnings only", []Severity{Warning, Warning, Warning}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var b Bag
			for i, sev := range tc.sevs {
				b.add(sev, NotYetImplemented, source.SpanAt(source.Pos(i)), "%d", i)
			}
			if got := b.HasErrors(); got != tc.want {
				t.Errorf("HasErrors = %v over %v, want %v; the gate reads the whole bag", got, tc.sevs, tc.want)
			}
		})
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
	b.In(consumer).Raise(ImportNotFound, source.Span{Start: 7, End: 20}, "no file %q in the set", "nope.protop")

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
	b.Raise(TagOutOfRange, source.SpanAt(0), "field tag must be greater than 0")
	if b.All()[0].File != nil {
		t.Error("a diagnostic raised on the bag itself must not name a file")
	}
}

// The MSVS form is the one shape MSBuild's canonical error parser reads, and
// the literal below is what that parser expects, character for character.
//
// The authority is dotnet/msbuild, src/Shared/CanonicalError.cs, and its
// OriginCategoryCodeTextExpressionPattern: `Origin : Subcategory Category Code
// : Text`, in that order, with the location one of `(line)`, `(line,col)`,
// `(line,col,line,col)` or `(line,col-col)`. **The comma between line and
// column is load-bearing** -- the `(line-col)` in Microsoft's own documentation
// is a typo with no regex behind it, and a build that got it wrong would log
// protopc's errors as plain messages and pass.
//
// Nothing generates this literal and no flag can rewrite it. It is written out
// because the format is a contract with a program protopc does not own, and the
// only way to hold a contract like that is to write down the other side of it.
func TestRenderMSVSIsTheShapeMSBuildParses(t *testing.T) {
	f := source.NewFile("prague.protop", "revision 1;\nstruct S {\n  int64 id = 0;\n}\n")

	var b Bag
	b.Raise(TagOutOfRange, source.Span{Start: 36, End: 37}, "field tag must be greater than 0")

	got := RenderAs(MSVS, f, b.All()[0])
	want := "prague.protop(3,14): error PROTOP0300: field tag must be greater than 0"
	if got != want {
		t.Errorf("RenderAs(MSVS) = %q\nwant %q", got, want)
	}

	// The same diagnostic in the same place, in the default form, so that the
	// two are held apart by something other than which function was called.
	if gcc := RenderAs(GCC, f, b.All()[0]); !strings.HasPrefix(gcc, "prague.protop:3:14: error PROTOP0300: ") {
		t.Errorf("RenderAs(GCC) = %q, want the colon form", gcc)
	}
}

// A diagnostic about no particular file takes the tool name as its origin, with
// a space before the colon, which is the spelling CanonicalError accepts for a
// message about no particular place. protopc raises none today -- every
// diagnostic carries a span on purpose -- so this pins the spelling before the
// first one exists rather than after a build has silently swallowed it.
func TestRenderMSVSOfADiagnosticWithNoFile(t *testing.T) {
	var b Bag
	b.Raise(ImportNotFound, source.SpanAt(0), "no file named %q was found", "gone.protop")

	got := RenderAs(MSVS, nil, b.All()[0])
	want := `protopc : error PROTOP0107: no file named "gone.protop" was found`
	if got != want {
		t.Errorf("RenderAs(MSVS) = %q\nwant %q", got, want)
	}
}

// The caret form has no positionless spelling of its own -- there is no line to
// quote and no column to point at -- so it is the headline under the same
// origin. Asserted because the alternative, until now, was a nil dereference.
func TestRenderGCCOfADiagnosticWithNoFile(t *testing.T) {
	var b Bag
	b.Raise(ImportNotFound, source.SpanAt(0), "no file named %q was found", "gone.protop")

	got := Render(nil, b.All()[0])
	want := `protopc: error PROTOP0107: no file named "gone.protop" was found`
	if got != want {
		t.Errorf("Render = %q\nwant %q", got, want)
	}
}

// MSBuild's error parser is line-based, so a message that wrapped would have
// its tail read as an unrelated line of build output. protopc's messages are
// long enough for this not to be hypothetical.
func TestRenderMSVSFlattensAMultiLineMessage(t *testing.T) {
	f := source.NewFile("a.protop", "revision 1;\n")

	var b Bag
	b.Raise(UnknownType, source.SpanAt(0), "one\ntwo\r\nthree\rfour")

	got := RenderAs(MSVS, f, b.All()[0])
	want := "a.protop(1,1): error PROTOP0311: one two three four"
	if got != want {
		t.Errorf("RenderAs(MSVS) = %q\nwant %q", got, want)
	}
}

// A warning renders as a warning in both forms. MSBuild branches on the word:
// the same line with `error` fails a build that the `warning` spelling does not.
func TestRenderMSVSSaysWhichSeverityItIs(t *testing.T) {
	f := source.NewFile("a.protop", "revision 1;\n")

	var b Bag
	b.add(Warning, NotYetImplemented, source.SpanAt(0), "not yet")

	got := RenderAs(MSVS, f, b.All()[0])
	want := "a.protop(1,1): warning PROTOP0900: not yet"
	if got != want {
		t.Errorf("RenderAs(MSVS) = %q\nwant %q", got, want)
	}
}

// The MSVS renderer honours the file a diagnostic names, the way the GCC one
// does. Rendering a cross-file diagnostic against the wrong file is the bug
// Diagnostic.File exists to prevent, and it has to be prevented in both forms.
func TestRenderMSVSHonoursTheFileTheDiagnosticNames(t *testing.T) {
	prague := source.NewFile("prague.protop", "revision 1;\n")
	consumer := source.NewFile("consumer.protop", "import \"nope.protop\";\n")

	var b Bag
	b.In(consumer).Raise(ImportNotFound, source.Span{Start: 7, End: 20}, "no file %q in the set", "nope.protop")

	got := RenderAs(MSVS, prague, b.All()[0])
	want := "consumer.protop(1,8): error PROTOP0107: no file \"nope.protop\" in the set"
	if got != want {
		t.Errorf("RenderAs(MSVS) = %q\nwant %q", got, want)
	}
}

// The two names --error-format accepts, and the fact that it accepts no others.
// The strings are buf's, which is where a user's fingers come from.
func TestParseFormat(t *testing.T) {
	for _, tc := range []struct {
		name string
		want Format
		ok   bool
	}{
		{"gcc", GCC, true},
		{"msvs", MSVS, true},
		{"text", GCC, false},
		{"json", GCC, false},
		{"github-actions", GCC, false},
		{"", GCC, false},
		{"GCC", GCC, false},
	} {
		got, ok := ParseFormat(tc.name)
		if got != tc.want || ok != tc.ok {
			t.Errorf("ParseFormat(%q) = %v, %v; want %v, %v", tc.name, got, ok, tc.want, tc.ok)
		}
	}
}

// The spellings round-trip, because --error-format's value and the name in a
// message about an unknown one are the same word.
func TestFormatString(t *testing.T) {
	for _, name := range []string{"gcc", "msvs"} {
		f, ok := ParseFormat(name)
		if !ok {
			t.Fatalf("ParseFormat(%q) rejected a name it must accept", name)
		}
		if f.String() != name {
			t.Errorf("Format(%q).String() = %q", name, f.String())
		}
	}
}

// The default format is GCC. It is asserted against the zero value rather than
// against a constant, because what makes it the default is that it is what a
// Format nobody set renders as.
//
// It is provisional: docs/research/proto-to-messagepack-csharp.md:924 and :1292
// both say the MSVS form should be the default. The contradiction is filed, not
// resolved, and this test is what a resolution would have to come and change.
func TestTheDefaultFormatIsTerminalFirst(t *testing.T) {
	var zero Format
	if zero != GCC {
		t.Fatalf("the zero Format is %v, want gcc", zero)
	}
	f := source.NewFile("a.protop", "revision 1;\n")
	var b Bag
	b.Raise(UnknownType, source.SpanAt(0), "x")
	if RenderAs(zero, f, b.All()[0]) != Render(f, b.All()[0]) {
		t.Error("the zero Format does not render as the caret form")
	}
}

// A stage that reports one file at a time gets its file stamped by the driver
// that owns the several. Without this, a parse error in the fourth file of a
// compile renders its snippet against the first -- which is not a wrong
// position, it is a position in the wrong file, and it points at real source.
func TestWithinStampsTheFileOnWhatItAdds(t *testing.T) {
	first := source.NewFile("first.protop", "revision 1;\n")
	fourth := source.NewFile("fourth.protop", "revision 1;\n")

	var b Bag
	b.Raise(MissingNamespace, source.SpanAt(0), "before")
	b.Within(fourth, func(bag *Bag) {
		bag.Raise(MissingNamespace, source.SpanAt(0), "inside")
	})
	b.Raise(MissingNamespace, source.SpanAt(0), "after")

	got := []*source.File{b.All()[0].File, b.All()[1].File, b.All()[2].File}
	want := []*source.File{nil, fourth, nil}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("diagnostic %d names %v, want %v", i, got[i], want[i])
		}
	}
	if b.All()[1].File == first {
		t.Error("the wrong file was stamped")
	}
}

// Within stamps every diagnostic the stage added, not the first one.
//
// A stage that sees one file at a time is a parser, and a parser that finds one
// mistake finds several: recovery is what the frontend does instead of
// stopping. A stamp that reached only as far as the first would leave the rest
// rendering their snippets against whichever file the printer was handed --
// which is real source at a real position in the wrong file, the hardest kind
// of wrong output to disbelieve.
func TestWithinStampsEveryDiagnosticTheStageAdded(t *testing.T) {
	first := source.NewFile("first.protop", "revision 1;\n")
	fourth := source.NewFile("fourth.protop", "revision 1;\n")

	var b Bag
	b.Within(first, func(bag *Bag) {
		bag.Raise(MissingNamespace, source.SpanAt(0), "one")
	})
	b.Within(fourth, func(bag *Bag) {
		for _, msg := range []string{"one", "two", "three", "four"} {
			bag.Raise(MissingNamespace, source.SpanAt(0), "%s", msg)
		}
	})

	if b.Len() != 5 {
		t.Fatalf("%d diagnostics, want 5", b.Len())
	}
	for i, d := range b.All()[1:] {
		if d.File != fourth {
			t.Errorf("diagnostic %d of the second file names %v, want fourth.protop", i, d.File)
		}
	}
}

// Within does not overwrite a file a diagnostic already names. A stage that
// knows which file it is talking about -- the checker, reporting a view in one
// file against its source in another -- is more right than the driver is.
func TestWithinLeavesAnAlreadyNamedFileAlone(t *testing.T) {
	outer := source.NewFile("outer.protop", "revision 1;\n")
	inner := source.NewFile("inner.protop", "revision 1;\n")

	var b Bag
	b.Within(outer, func(bag *Bag) {
		bag.In(inner).Raise(ViewSourceNotFound, source.SpanAt(0), "elsewhere")
	})
	if b.All()[0].File != inner {
		t.Errorf("Within overwrote the file the stage named")
	}
}
