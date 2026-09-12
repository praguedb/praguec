package syntax

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nanov/protobuf-to-messagepack-sharp/internal/diag"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/source"
)

// specSubsetFixture is protop lifted verbatim out of section 1 of the spec. It
// is a file rather than a string literal so that TestSpecSubsetIsVerbatim can
// hold it against the spec byte for byte.
const specSubsetFixture = "spec_section1_subset.protop"

// specSection1 is the spec file the fixture is lifted from.
var specSection1 = filepath.Join("..", "..", "docs", "spec", "protop", "shared-repository.md")

// parse runs the whole frontend -- lexer and parser -- over src.
func parse(t *testing.T, src string) (*source.File, *File, *diag.Bag) {
	t.Helper()
	f := source.NewFile("x.protop", src)
	var bag diag.Bag
	return f, Parse(f, &bag), &bag
}

// parseClean parses src and fails the test if anything at all was reported.
func parseClean(t *testing.T, src string) (*source.File, *File) {
	t.Helper()
	f, file, bag := parse(t, src)
	if bag.Len() != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", diag.RenderAll(f, bag))
	}
	return f, file
}

// dump renders what the parser promises to get right -- names, types, tags and
// enum numbers -- one line per node, so that a whole tree can be asserted
// against a literal and a mismatch reads as a diff.
func dump(file *File) string {
	var b strings.Builder
	if file.Revision != nil {
		fmt.Fprintf(&b, "revision %d\n", file.Revision.Number.Value)
	}
	if file.Namespace != nil {
		fmt.Fprintf(&b, "namespace %s\n", file.Namespace)
	}
	for _, d := range file.Decls {
		switch d := d.(type) {
		case *Struct:
			dumpDecl(&b, "struct", d.Name, d.Annotations, d.Members)
		case *Record:
			dumpDecl(&b, "record", d.Name, d.Annotations, d.Members)
		case *Table:
			dumpDecl(&b, "table", d.Name, d.Annotations, d.Members)
		case *Enum:
			fmt.Fprintf(&b, "enum %s\n", d.Name.Text)
			for _, m := range d.Members {
				fmt.Fprintf(&b, "  %s = %d\n", m.Name.Text, m.Number.Value)
			}
		default:
			fmt.Fprintf(&b, "<unhandled declaration %T>\n", d)
		}
	}
	return b.String()
}

// dumpDecl writes one declaration and its members, which is where struct,
// record and table differ by their keyword and by nothing else.
func dumpDecl(b *strings.Builder, keyword string, name Ident, anns []*Annotation, members []Member) {
	fmt.Fprintf(b, "%s %s\n", keyword, name.Text)
	// A head's annotations get a line each; a member's trail the member, which
	// is where the source writes them both.
	for _, a := range anns {
		fmt.Fprintf(b, "  %s\n", annString(a))
	}
	for _, m := range members {
		switch m := m.(type) {
		case *Field:
			// `key` and `repeated` are both written in front of the type, and no
			// field carries both: identity is one value, not a collection.
			lead := ""
			switch {
			case m.Key:
				lead = "key "
			case m.Repeated:
				lead = "repeated "
			}
			fmt.Fprintf(b, "  %s%s %s = %d%s\n", lead, typeString(m.Type), m.Name.Text, m.Tag.Value, annSuffix(m.Annotations))
		case *CompositeKey:
			parts := make([]string, len(m.Parts))
			for i, part := range m.Parts {
				parts[i] = part.Text
			}
			fmt.Fprintf(b, "  key %s = %d (%s)%s\n", m.Name.Text, m.Tag.Value, strings.Join(parts, ", "), annSuffix(m.Annotations))
		case *Alias:
			fmt.Fprintf(b, "  alias %s = %s%s\n", m.Name.Text, m.Source.Text, annSuffix(m.Annotations))
		case *Reserved:
			fmt.Fprintf(b, "  reserved %s\n", rangeList(m.Ranges))
		default:
			fmt.Fprintf(b, "  <unhandled member %T>\n", m)
		}
	}
}

// rangeList writes a reserved list back the way the source spelled it: a range
// whose ends are equal was written as the single tag it retires.
func rangeList(ranges []ReservedRange) string {
	out := make([]string, len(ranges))
	for i, r := range ranges {
		out[i] = fmt.Sprint(r.Lo.Value)
		if r.Hi.Value != r.Lo.Value {
			out[i] += fmt.Sprintf("..%d", r.Hi.Value)
		}
	}
	return strings.Join(out, ", ")
}

// annString writes an annotation back the way the source spelled it, which for
// its contents is verbatim.
func annString(a *Annotation) string {
	if a.Prefix != nil {
		return fmt.Sprintf("[%s: %s]", a.Prefix.Text, a.Text)
	}
	return fmt.Sprintf("[%s]", a.Text)
}

func annSuffix(anns []*Annotation) string {
	var b strings.Builder
	for _, a := range anns {
		fmt.Fprintf(&b, " %s", annString(a))
	}
	return b.String()
}

// typeString writes a TypeRef back the way the source spelled it.
func typeString(t TypeRef) string {
	s := t.Name.Text
	if t.Precision != nil {
		s += fmt.Sprintf("(%d)", t.Precision.Value)
	}
	if t.Optional {
		s += "?"
	}
	return s
}

func assertDump(t *testing.T, file *File, want string) {
	t.Helper()
	if got := dump(file); got != want {
		t.Errorf("tree:\n%s\nwant:\n%s", got, want)
	}
}

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The milestone's headline claim: the subset of the language this parser reads
// parses straight out of the spec, with nothing to say about it.
func TestParseSpecSection1Subset(t *testing.T) {
	src := readFixture(t, specSubsetFixture)
	f, file := parseClean(t, src)

	assertDump(t, file, `revision 1
namespace Common.IntegratedEventApi.Cache.Prague.Models
struct Venue
  int64 id = 2
  string name = 3
enum EventStatus
  UNKNOWN = 0
  NOT_STARTED = 1
  IN_PROGRESS = 2
  FINISHED = 3
enum BetTypeStatus
  UNKNOWN = 0
  OPEN = 1
  SUSPENDED = 2
  CLOSED = 3
`)

	// A declaration's span covers the whole declaration, opening keyword to
	// closing brace. Later stages quote source by span, so slicing one back has
	// to return what was written.
	venue, ok := file.Decls[0].(*Struct)
	if !ok {
		t.Fatalf("first declaration is %T, want *Struct", file.Decls[0])
	}
	const wantVenue = "struct Venue {\n  int64  id   = 2;\n  string name = 3;\n}"
	if got := f.Slice(venue.Span); got != wantVenue {
		t.Errorf("Venue spans:\n%s\nwant:\n%s", got, wantVenue)
	}
}

// What keeps "parses verbatim out of the spec" honest: the fixture is not a
// paraphrase of section 1, it is a copy of it. If the spec changes its mind
// about the language, this fails before the parser silently keeps reading the
// old one.
func TestSpecSubsetIsVerbatim(t *testing.T) {
	fixture := readFixture(t, specSubsetFixture)
	spec, err := os.ReadFile(specSection1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(spec), fixture) {
		t.Errorf("testdata/%s no longer appears verbatim in %s", specSubsetFixture, specSection1)
	}
}

// Presence and precision, which section 1's Venue does not exercise: `?` binds
// to the type, and `decimal` carries its scale.
func TestParsePresenceAndPrecision(t *testing.T) {
	_, file := parseClean(t, `revision 1;
namespace A;
struct S {
  int64? x = 1;
  decimal(4)? y = 2;
}
`)
	assertDump(t, file, `revision 1
namespace A
struct S
  int64? x = 1
  decimal(4)? y = 2
`)

	s := file.Decls[0].(*Struct)
	if x := s.Members[0].(*Field).Type; !x.Optional || x.Precision != nil {
		t.Errorf("int64? is %+v, want Optional and no Precision", x)
	}
	y := s.Members[1].(*Field).Type
	if !y.Optional {
		t.Errorf("decimal(4)? is not Optional")
	}
	if y.Precision == nil {
		t.Fatalf("decimal(4)? carries no Precision")
	}
	if y.Precision.Value != 4 {
		t.Errorf("precision = %d, want 4", y.Precision.Value)
	}
}

// Every diagnostic the parser raises, pinned by code sequence and by where it
// points. Codes are protopc's interface -- an editor filters on them and a
// changelog cites them -- so the sequence is asserted exactly.
//
// Lexical diagnostics come first in the bag whatever their position, because
// Parse tokenises the whole file before it reads a token of it.
func TestParseDiagnostics(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantCodes []diag.Code
		wantLine  int
		wantCol   int
	}{
		{
			name:      "missing revision",
			src:       "namespace A;\nstruct S { int64 a = 1; }\n",
			wantCodes: []diag.Code{diag.MissingRevision},
			wantLine:  1, wantCol: 1,
		},
		{
			name:      "missing namespace",
			src:       "revision 1;\nstruct S { int64 a = 1; }\n",
			wantCodes: []diag.Code{diag.MissingNamespace},
			wantLine:  1, wantCol: 1,
		},
		{
			// Both are checked after the whole file, so a header-less file still
			// reports everything else wrong with it -- here, nothing.
			name:      "missing both, reported at the head of the file",
			src:       "struct S { int64 a = 1; }\n",
			wantCodes: []diag.Code{diag.MissingRevision, diag.MissingNamespace},
			wantLine:  1, wantCol: 1,
		},
		{
			name: "tag 0",
			src: `revision 1;
namespace A;
struct S {
  int64 id = 0;
}
`,
			wantCodes: []diag.Code{diag.TagOutOfRange},
			wantLine:  4, wantCol: 14,
		},
		{
			name: "precision on a type that takes none",
			src: `revision 1;
namespace A;
struct S {
  int64(4) x = 1;
}
`,
			wantCodes: []diag.Code{diag.PrecisionNotAllowed},
			wantLine:  4, wantCol: 9, // the precision itself, not the type
		},
		{
			name: "decimal without its precision",
			src: `revision 1;
namespace A;
struct S {
  decimal x = 1;
}
`,
			wantCodes: []diag.Code{diag.PrecisionRequired},
			wantLine:  4, wantCol: 3,
		},
		{
			name:      "unterminated block comment",
			src:       "revision 1;\nnamespace A;\n/* and then nothing\n",
			wantCodes: []diag.Code{diag.UnterminatedBlockComment},
			wantLine:  3, wantCol: 1,
		},
		{
			// A construct the spec defines and this milestone does not read is
			// the compiler's fault, not the user's, and says so.
			name: "view declaration",
			src: `revision 1;
namespace A;
view SlimVenue of Venue {
  name;
}
`,
			wantCodes: []diag.Code{diag.NotYetImplemented},
			wantLine:  3, wantCol: 1,
		},
		{
			name:      "unsupported revision",
			src:       "revision 2;\nnamespace A;\n",
			wantCodes: []diag.Code{diag.UnsupportedRevision},
			wantLine:  1, wantCol: 10,
		},
		{
			name:      "revision after another declaration",
			src:       "namespace A;\nrevision 1;\n",
			wantCodes: []diag.Code{diag.RevisionNotFirst},
			wantLine:  2, wantCol: 1,
		},
		{
			name:      "duplicate revision",
			src:       "revision 1;\nrevision 1;\nnamespace A;\n",
			wantCodes: []diag.Code{diag.DuplicateRevision},
			wantLine:  2, wantCol: 1,
		},
		{
			name:      "duplicate namespace",
			src:       "revision 1;\nnamespace A;\nnamespace B;\n",
			wantCodes: []diag.Code{diag.DuplicateNamespace},
			wantLine:  3, wantCol: 1,
		},
		{
			name:      "unknown declaration",
			src:       "revision 1;\nnamespace A;\nwidget W {\n}\n",
			wantCodes: []diag.Code{diag.UnknownDeclaration},
			wantLine:  3, wantCol: 1,
		},
		{
			// Section 3 has one position and no before-slot. The annotation is
			// skipped and the declaration behind it still parses, so the
			// mistake costs one diagnostic rather than a lost declaration.
			name: "annotation before a declaration head",
			src: `revision 1;
namespace A;
[cs: DataCacheTopic("t")]
struct S {
  int64 a = 1;
}
`,
			wantCodes: []diag.Code{diag.AnnotationNotAllowed},
			wantLine:  3, wantCol: 1,
		},
		{
			name: "annotation before a tag",
			src: `revision 1;
namespace A;
struct S {
  int64 a [cs: X] = 1;
}
`,
			wantCodes: []diag.Code{diag.AnnotationNotAllowed},
			wantLine:  4, wantCol: 11,
		},
		{
			// A backend protopc does not have. An unknown prefix is an error
			// rather than a silent drop: a typo that vanishes from the output is
			// worse than the rule this replaced.
			name: "unknown annotation prefix",
			src: `revision 1;
namespace A;
struct S {
  int64 a = 1 [go: json:"a"];
}
`,
			wantCodes: []diag.Code{diag.UnknownAnnotationPrefix},
			wantLine:  4, wantCol: 16,
		},
		{
			// protop's own set is closed, so an unprefixed name outside it is a
			// typo rather than passthrough -- passthrough carries a prefix.
			name: "unknown unprefixed annotation",
			src: `revision 1;
namespace A;
struct S {
  int64 a = 1 [refrences(T)];
}
`,
			wantCodes: []diag.Code{diag.UnknownAnnotation},
			wantLine:  4, wantCol: 16,
		},
		{
			name: "unterminated annotation",
			src: `revision 1;
namespace A;
struct S {
  int64 a = 1 [cs: Foo;
}
`,
			wantCodes: []diag.Code{diag.UnterminatedAnnotation},
			wantLine:  4, wantCol: 15,
		},
		{
			// Unterminated only if the scan were a search for the next `]`: the
			// one inside the string literal belongs to the literal, so the
			// annotation closes at the bracket after it and this file is clean
			// apart from the tag it is here to carry.
			name: "a `]` inside a string does not end the annotation",
			src: `revision 1;
namespace A;
struct S {
  int64 a = 0 [cs: Foo("]")];
}
`,
			wantCodes: []diag.Code{diag.TagOutOfRange},
			wantLine:  4, wantCol: 13,
		},
		{
			// A range's `..` promises an upper bound, and a member that fails
			// mid-way is recovered from at the `;` like any other -- the next
			// member is still read.
			name: `reserved range with no upper bound`,
			src: `revision 1;
namespace A;
table T {
  reserved 8..;
  int64 a = 1;
}
`,
			wantCodes: []diag.Code{diag.UnexpectedToken},
			wantLine:  4, wantCol: 15,
		},
		{
			// An alias is two names and an `=`; without the second name there is
			// nothing to pass through to.
			name: `alias with no source`,
			src: `revision 1;
namespace A;
table T {
  alias bet_type_id = ;
}
`,
			wantCodes: []diag.Code{diag.UnexpectedToken},
			wantLine:  4, wantCol: 23,
		},
		{
			// A composite key spends an ordinal exactly as a field does, so the
			// bound that makes tag 0 name no slot is the same bound.
			name: `composite key with tag 0`,
			src: `revision 1;
namespace A;
table T {
  key key = 0 (a, b);
}
`,
			wantCodes: []diag.Code{diag.TagOutOfRange},
			wantLine:  4, wantCol: 13,
		},
		{
			// A precision belongs to `decimal` alone, and a declared type is
			// told it so in the same words a scalar is: neither is resolved
			// here, and the fix is the same.
			name: "precision on a declared type",
			src: `revision 1;
namespace A;
struct S {
  Venue(4) v = 1;
}
`,
			wantCodes: []diag.Code{diag.PrecisionNotAllowed},
			wantLine:  4, wantCol: 9,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, _, bag := parse(t, tt.src)

			var got []diag.Code
			for _, d := range bag.All() {
				got = append(got, d.Code)
			}
			if !slices.Equal(got, tt.wantCodes) {
				t.Fatalf("codes = %v, want %v:\n%s", got, tt.wantCodes, diag.RenderAll(f, bag))
			}
			if pos := f.Position(bag.All()[0].Span.Start); pos.Line != tt.wantLine || pos.Column != tt.wantCol {
				t.Errorf("first diagnostic at %d:%d, want %d:%d", pos.Line, pos.Column, tt.wantLine, tt.wantCol)
			}
		})
	}
}

// A tag is 1-based because a MessagePack key is tag - 1. An enum member's
// number is a value on the wire and not a position, so the same 0 that is an
// error on a field is idiomatic here -- it is how section 1 spells UNKNOWN.
func TestZeroIsATagErrorAndALegalEnumNumber(t *testing.T) {
	_, file := parseClean(t, "revision 1;\nnamespace A;\nenum E {\n  UNKNOWN = 0;\n}\n")
	assertDump(t, file, "revision 1\nnamespace A\nenum E\n  UNKNOWN = 0\n")

	_, _, bag := parse(t, "revision 1;\nnamespace A;\nstruct S {\n  int64 id = 0;\n}\n")
	if bag.Len() != 1 || bag.All()[0].Code != diag.TagOutOfRange {
		t.Fatalf("tag 0 in a struct raised %d diagnostics, want one TagOutOfRange", bag.Len())
	}
}

// The caret is the reason spans are carried rather than positions, so one
// diagnostic is pinned as the user reads it -- code, location and the width of
// the underline under the token that is wrong.
func TestRenderPointsAtTheOffendingToken(t *testing.T) {
	f, _, bag := parse(t, "revision 1;\nnamespace A;\nstruct S {\n  int64 id = 0;\n}\n")
	if bag.Len() != 1 {
		t.Fatalf("got %d diagnostics, want 1:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	want := "x.protop:4:14: error PROTOP0300: field tag must be greater than 0; " +
		"a MessagePack key is tag - 1, so tag 0 names no slot\n" +
		"  |\n" +
		"4 |   int64 id = 0;\n" +
		"  |              ^"
	if got := diag.Render(f, bag.All()[0]); got != want {
		t.Errorf("rendered:\n%s\nwant:\n%s", got, want)
	}
}

// On an error path the parser still appends what it managed to read: a field
// whose tag failed lands in the tree with the zero tag. That is safe only
// because bag.HasErrors gates everything downstream, and a partial tree is what
// lets an editor outline a file that is still being typed.
func TestParseKeepsAPartialFieldWhenItsTagFails(t *testing.T) {
	f, file, bag := parse(t, "revision 1;\nnamespace A;\nstruct S {\n  int64 id = ;\n}\n")
	if !bag.HasErrors() {
		t.Fatal("a field with no tag was accepted")
	}
	if bag.Len() != 1 || bag.All()[0].Code != diag.UnexpectedToken {
		t.Fatalf("got %d diagnostics, want one UnexpectedToken:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	assertDump(t, file, "revision 1\nnamespace A\nstruct S\n  int64 id = 0\n")
}

// A missing `;` is reported where it is and not recovered from: the token that
// is not a semicolon is almost always the start of the next member, and
// skipping to the next `;` would report one mistake and swallow a member that
// was perfectly good.
func TestParseReportsAMissingSemicolonInPlace(t *testing.T) {
	f, file, bag := parse(t, `revision 1;
namespace A;
struct S {
  int64 a = 1
  int64 b = 2;
}
`)
	if bag.Len() != 1 || bag.All()[0].Code != diag.UnexpectedToken {
		t.Fatalf("got %d diagnostics, want one UnexpectedToken:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	// Reported at the token that should have been a `;`: the next member's type.
	if pos := f.Position(bag.All()[0].Span.Start); pos.Line != 5 || pos.Column != 3 {
		t.Errorf("reported at %d:%d, want 5:3", pos.Line, pos.Column)
	}
	assertDump(t, file, "revision 1\nnamespace A\nstruct S\n  int64 a = 1\n  int64 b = 2\n")
}

// Recovery inside a body leaves the closing `}` for the loop that reads the
// body, so a member the parser gave up on ends that body and nothing after it.
func TestRecoveryInsideABodyLeavesTheClosingBrace(t *testing.T) {
	f, file, bag := parse(t, `revision 1;
namespace A;
struct S {
  int64 a
}
enum E {
  UNKNOWN = 0;
}
`)
	if bag.Len() != 1 || bag.All()[0].Code != diag.UnexpectedToken {
		t.Fatalf("got %d diagnostics, want one UnexpectedToken:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	// Had recovery swallowed the `}`, the struct body would have run on into the
	// enum and every line of it would be a second mistake.
	assertDump(t, file, `revision 1
namespace A
struct S
  int64 a = 0
enum E
  UNKNOWN = 0
`)
}

// Recovery at the top level consumes the balanced `}` of the declaration it is
// skipping, so one unreadable declaration costs exactly one diagnostic and the
// next declaration parses.
func TestRecoveryAtTopLevelConsumesTheDeclarationItSkips(t *testing.T) {
	f, file, bag := parse(t, `revision 1;
namespace A;
view SlimVenue of Venue {
  name;
}
struct S {
  int64 a = 1;
}
`)
	if bag.Len() != 1 || bag.All()[0].Code != diag.NotYetImplemented {
		t.Fatalf("got %d diagnostics, want one NotYetImplemented:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	assertDump(t, file, "revision 1\nnamespace A\nstruct S\n  int64 a = 1\n")
}

// `record` is `struct` with a different keyword: the parser reads one shape and
// section 4 gives the two different meanings. A field may name a declared type,
// and `repeated` is a modifier on the field rather than part of its type.
//
// Nothing here is resolved -- `Venue` names no declaration in this file and the
// parser has nothing to say about that, because whether a name is in scope is a
// question only the stage that knows the scope can ask.
func TestParseRecordNamedTypesAndRepeated(t *testing.T) {
	_, file := parseClean(t, `revision 1;
namespace A;
record StaticDataRef {
  int64  id   = 1;
  string name = 2;
}
struct S {
  StaticDataRef  sport_type = 1;
  StaticDataRef? home_team  = 2;
  repeated StaticDataRef runners = 3;
  Venue? venue = 4;
}
`)
	assertDump(t, file, `revision 1
namespace A
record StaticDataRef
  int64 id = 1
  string name = 2
struct S
  StaticDataRef sport_type = 1
  StaticDataRef? home_team = 2
  repeated StaticDataRef runners = 3
  Venue? venue = 4
`)

	if _, ok := file.Decls[0].(*Record); !ok {
		t.Errorf("first declaration is %T, want *Record", file.Decls[0])
	}
	s := file.Decls[1].(*Struct)
	// `repeated` is not part of the type: the element type is what a later
	// stage resolves, and the collection is a property of the field.
	runners := s.Members[2].(*Field)
	if !runners.Repeated || runners.Type.Name.Text != "StaticDataRef" {
		t.Errorf("runners is %+v, want Repeated of StaticDataRef", runners)
	}
	if s.Members[0].(*Field).Repeated {
		t.Error("sport_type is Repeated")
	}
}

// Whether protop's scalar set is closed at all is unresolved -- #32 -- and the
// parser no longer branches on it: a name outside the set is a reference to a
// declared type. What keeps the census honest is the fixture. Every type
// section 1 names is one of the six or a declaration in the same file, so a
// seventh scalar appearing in the spec fails here rather than parsing quietly
// as a name that will resolve to nothing.
func TestSpecSubsetNamesNoTypeItDoesNotDeclare(t *testing.T) {
	_, file := parseClean(t, readFixture(t, specSubsetFixture))

	declared := map[string]bool{}
	for _, d := range file.Decls {
		switch d := d.(type) {
		case *Struct:
			declared[d.Name.Text] = true
		case *Record:
			declared[d.Name.Text] = true
		case *Table:
			declared[d.Name.Text] = true
		case *Enum:
			declared[d.Name.Text] = true
		}
	}
	for _, d := range file.Decls {
		for _, m := range declMembers(d) {
			f, ok := m.(*Field)
			if !ok {
				continue
			}
			if name := f.Type.Name.Text; !scalarTypes[name] && !declared[name] {
				t.Errorf("field %s names %s, which is neither a scalar nor declared in the fixture", f.Name.Text, name)
			}
		}
	}
}

// declMembers is the body of whichever declaration carries one.
func declMembers(d Decl) []Member {
	switch d := d.(type) {
	case *Struct:
		return d.Members
	case *Record:
		return d.Members
	case *Table:
		return d.Members
	}
	return nil
}

// Annotations, in the three positions section 1 uses them: after a declaration
// head, after a tag, and stacked by juxtaposition. Contents are carried
// verbatim and in source order -- the two on `timestamp` are not sorted and the
// two identical ones on `x` are both kept, because protopc does not know what
// either would mean to the backend that reads them.
func TestParseAnnotations(t *testing.T) {
	_, file := parseClean(t, `revision 1;
namespace A;
record R
  [cs: DataCache(CacheClassName = nameof(BPSportGameCache))]
  [cs: DataCacheContract(typeof(SportGameCacheContract), Role = ContractRole.Producer)]
{
  int64 event_id  = 1 [references(SportGame)];
  int64 timestamp = 2 [cs: DataCacheFromTimestamp] [cs: DataCacheIgnoreEquality];
  int64 x         = 3 [cs: A] [cs: A];
}
`)
	assertDump(t, file, `revision 1
namespace A
record R
  [cs: DataCache(CacheClassName = nameof(BPSportGameCache))]
  [cs: DataCacheContract(typeof(SportGameCacheContract), Role = ContractRole.Producer)]
  int64 event_id = 1 [references(SportGame)]
  int64 timestamp = 2 [cs: DataCacheFromTimestamp] [cs: DataCacheIgnoreEquality]
  int64 x = 3 [cs: A] [cs: A]
`)

	// A prefix is a name the parser keeps; protop's own annotations carry none,
	// and that is the difference between text that is checked and text that is
	// copied out untouched.
	r := file.Decls[0].(*Record)
	if p := r.Annotations[0].Prefix; p == nil || p.Text != "cs" {
		t.Errorf("head annotation prefix is %v, want cs", p)
	}
	if p := r.Members[0].(*Field).Annotations[0].Prefix; p != nil {
		t.Errorf("`references` carries prefix %q, want none", p.Text)
	}
}

// The case that says the scan is not a search for the next `]`. The bracket
// inside the string literal is part of that literal's token, so it cannot close
// the annotation -- which is what tokenising the whole file before parsing any
// of it buys.
func TestAnnotationTextKeepsBracketsInsideStringsAndNesting(t *testing.T) {
	_, file := parseClean(t, `revision 1;
namespace A;
struct S {
  int64 a = 1 [cs: Foo("]")];
  int64 b = 2 [cs: Bar(new[] { 1 })] [cs: Baz];
}
`)
	s := file.Decls[0].(*Struct)

	a := s.Members[0].(*Field).Annotations
	if len(a) != 1 {
		t.Fatalf("`[cs: Foo(\"]\")]` read as %d annotations, want 1: %s", len(a), annSuffix(a))
	}
	if got := a[0].Text; got != `Foo("]")` {
		t.Errorf("text = %s, want %s", got, `Foo("]")`)
	}

	// A `]` that closes a `[` inside the annotation does not close the
	// annotation either: the scan counts brackets.
	b := s.Members[1].(*Field).Annotations
	if len(b) != 2 {
		t.Fatalf("nested brackets read as %d annotations, want 2: %s", len(b), annSuffix(b))
	}
	if got := b[0].Text; got != "Bar(new[] { 1 })" {
		t.Errorf("text = %q, want %q", got, "Bar(new[] { 1 })")
	}
}

// A table is the one declaration with identity, and the simple spelling of it
// is a field with `key` in front: one member, not two, so the tag it carries is
// the tag of the field that is the key.
func TestParseTableWithASimpleKey(t *testing.T) {
	_, file := parseClean(t, `revision 1;
namespace A;
table SportGame
  [cs: DataCacheTopic(TopicNames.BrandFromConfigs.SportGame)]
{
  reserved 8..14, 16..23, 25;

  key int64 event_id = 1;

  int64? home_team_id = 5;
  repeated StaticDataRef runners = 47;
}
`)
	assertDump(t, file, `revision 1
namespace A
table SportGame
  [cs: DataCacheTopic(TopicNames.BrandFromConfigs.SportGame)]
  reserved 8..14, 16..23, 25
  key int64 event_id = 1
  int64? home_team_id = 5
  repeated StaticDataRef runners = 47
`)

	tbl, ok := file.Decls[0].(*Table)
	if !ok {
		t.Fatalf("declaration is %T, want *Table", file.Decls[0])
	}
	// The key is a field and nothing else: it has a type and spends an ordinal,
	// and only the composite spelling needs a node of its own.
	key, ok := tbl.Members[1].(*Field)
	if !ok {
		t.Fatalf("the key is %T, want *Field", tbl.Members[1])
	}
	if !key.Key || key.Type.Name.Text != "int64" || key.Tag.Value != 1 {
		t.Errorf("key is %+v, want Key int64 with tag 1", key)
	}
	if tbl.Members[2].(*Field).Key {
		t.Error("home_team_id is a Key")
	}
	// `reserved` is a member and holds its place: section 1 writes it above the
	// key, and source order is what a backend emits and a diagnostic quotes.
	if _, ok := tbl.Members[0].(*Reserved); !ok {
		t.Errorf("first member is %T, want *Reserved", tbl.Members[0])
	}
}

// `reserved 8..14, 25;` retires both ends of a range and the single tag beside
// it. Which of the two spellings was written is not a difference downstream, so
// a single is the range that begins and ends with it.
func TestParseReservedRangesAreInclusive(t *testing.T) {
	_, file := parseClean(t, "revision 1;\nnamespace A;\ntable T {\n  reserved 8..14, 25;\n}\n")

	r := file.Decls[0].(*Table).Members[0].(*Reserved)
	if len(r.Ranges) != 2 {
		t.Fatalf("read %d ranges, want 2", len(r.Ranges))
	}
	for _, tt := range []struct {
		tag  uint64
		want bool
	}{{7, false}, {8, true}, {11, true}, {14, true}, {15, false}, {24, false}, {25, true}, {26, false}} {
		got := r.Ranges[0].Contains(tt.tag) || r.Ranges[1].Contains(tt.tag)
		if got != tt.want {
			t.Errorf("tag %d reserved = %v, want %v", tt.tag, got, tt.want)
		}
	}
}

// The member the whole no-keyword-token-kinds decision was made for: `key key =
// 1 (event_id, bet_type_id);` is a member named `key`, of a kind named `key`,
// over fields that keep their own ordinals. Nothing here is a reserved word, and
// the two spellings of identity are told apart by what follows rather than by
// what the lexer called it.
func TestParseCompositeKeyAndAlias(t *testing.T) {
	_, file := parseClean(t, `revision 1;
namespace A;
table BetType {
  key key = 1 (event_id, bet_type_id);

  int64 event_id = 2 [references(BetType)];
  int64 id       = 3;

  alias bet_type_id = id;
}
`)
	assertDump(t, file, `revision 1
namespace A
table BetType
  key key = 1 (event_id, bet_type_id)
  int64 event_id = 2 [references(BetType)]
  int64 id = 3
  alias bet_type_id = id
`)

	tbl := file.Decls[0].(*Table)
	k, ok := tbl.Members[0].(*CompositeKey)
	if !ok {
		t.Fatalf("the key is %T, want *CompositeKey", tbl.Members[0])
	}
	// The composite key spends an ordinal of its own -- the one thing that
	// separates it from Cap'n Proto's group -- and writing it is what makes the
	// duplication visible.
	if k.Name.Text != "key" || k.Tag.Value != 1 {
		t.Errorf("key is %s = %d, want key = 1", k.Name.Text, k.Tag.Value)
	}
	if len(k.Parts) != 2 || k.Parts[0].Text != "event_id" || k.Parts[1].Text != "bet_type_id" {
		t.Errorf("parts are %+v, want event_id then bet_type_id", k.Parts)
	}

	// An alias names no wire slot, so it has no tag to get wrong: it is a second
	// name for a field that already has one.
	a, ok := tbl.Members[3].(*Alias)
	if !ok {
		t.Fatalf("the alias is %T, want *Alias", tbl.Members[3])
	}
	if a.Name.Text != "bet_type_id" || a.Source.Text != "id" {
		t.Errorf("alias is %s = %s, want bet_type_id = id", a.Name.Text, a.Source.Text)
	}
}

// What the lookahead costs and what it buys. `key` in leading position opens
// either spelling, and the word after it does not settle which: the simple
// spelling may name its field `key` too. What settles it is the `=`, two tokens
// in for the composite spelling and further on for the simple one.
func TestKeyIsAWordAndNotAKeyword(t *testing.T) {
	_, file := parseClean(t, `revision 1;
namespace A;
table T {
  key int64 key = 1;
  int64 alias = 2;
  int64 reserved = 3;
}
`)
	assertDump(t, file, `revision 1
namespace A
table T
  key int64 key = 1
  int64 alias = 2
  int64 reserved = 3
`)

	tbl := file.Decls[0].(*Table)
	k, ok := tbl.Members[0].(*Field)
	if !ok {
		t.Fatalf("`key int64 key = 1;` read as %T, want *Field", tbl.Members[0])
	}
	if !k.Key || k.Name.Text != "key" {
		t.Errorf("member is %+v, want a Key field named key", k)
	}
}
