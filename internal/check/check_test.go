package check

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/praguedb/praguec/internal/diag"
	"github.com/praguedb/praguec/internal/source"
	"github.com/praguedb/praguec/internal/syntax"
)

// The two spec sections, under the names the program knows them by: the shared
// repository's file is what section 2's `import "prague.prague";` names, so the
// import string is the fixture's key and not an incidental file name.
//
// They are copies of the fixtures the frontend's suite parses, and they are
// copies rather than a reach into another package's testdata because testdata
// belongs to the package that uses it. Nothing holds them together except
// TestSpecFixturesAreVerbatim, which holds both of them against the spec
// itself -- which is the only thing either copy should ever be equal to.
const (
	pragueFixture   = "prague.prague"
	consumerFixture = "consumer.prague"
)

var (
	specSection1 = filepath.Join("..", "..", "docs", "spec", "protop", "shared-repository.md")
	specSection2 = filepath.Join("..", "..", "docs", "spec", "protop", "service-repository.md")
	// Section 4 lends this package no fixture either. It is the authority for
	// PROTOP0207 -- see TestSection4StatesTheBuiltinNameRuleWithoutCopyingTheSet.
	specSection4 = filepath.Join("..", "..", "docs", "spec", "protop", "declaration-keywords.md")
	// Section 5 lends this package no fixture, only rules. One of them is
	// pinned, the same way and for the same reason a fixture is -- see
	// assertSpecStates.
	specSection5 = filepath.Join("..", "..", "docs", "spec", "protop", "views.md")
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// parseSet runs the frontend over every file and fails the test if any of them
// had anything reported against it. A checking test is about the checker, and a
// fixture that does not parse would be testing the parser by accident.
func parseSet(t *testing.T, files map[string]string) (FileSet, *diag.Bag) {
	t.Helper()
	set := make(FileSet, len(files))
	var bag diag.Bag
	for path, src := range files {
		f := source.NewFile(path, src)
		set[path] = Unit{Source: f, File: syntax.Parse(f, &bag)}
	}
	if bag.Len() != 0 {
		t.Fatalf("fixtures do not parse cleanly:\n%s", renderAll(set, &bag))
	}
	return set, &bag
}

// checkSet parses and checks, returning the model and everything the checker
// had to say about it.
func checkSet(t *testing.T, files map[string]string) (FileSet, *Schema, *diag.Bag) {
	t.Helper()
	set, bag := parseSet(t, files)
	return set, Check(set, bag), bag
}

// checkClean checks and fails the test if the checker said anything at all.
func checkClean(t *testing.T, files map[string]string) *Schema {
	t.Helper()
	set, schema, bag := checkSet(t, files)
	if bag.Len() != 0 {
		t.Fatalf("unexpected diagnostics:\n%s", renderAll(set, bag))
	}
	assertGate(t, set, bag, nil)
	return schema
}

// renderAll renders a bag that may span several files, each diagnostic against
// the one it names.
func renderAll(set FileSet, bag *diag.Bag) string {
	var any *source.File
	for _, u := range set {
		any = u.Source
		break
	}
	return diag.RenderAll(any, bag)
}

// assertCodes holds a bag against the sequence of codes it should contain.
// Codes are praguec's interface and a sequence says more than a count: the
// order is which mistake was reported first, and a test that counted would pass
// on the wrong diagnostic.
func assertCodes(t *testing.T, set FileSet, bag *diag.Bag, want ...diag.Code) {
	t.Helper()
	got := make([]diag.Code, 0, bag.Len())
	for _, d := range bag.All() {
		got = append(got, d.Code)
	}
	if len(got) != len(want) {
		t.Fatalf("diagnostics:\n%s\nwant codes %v, got %v", renderAll(set, bag), want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("diagnostics:\n%s\nwant codes %v, got %v", renderAll(set, bag), want, got)
		}
	}
	assertGate(t, set, bag, want)
}

// assertGate holds the bag against what the codes it carries imply about the
// compile. bag.HasErrors is the gate the driver reads, and a diagnostic that is
// reported without reaching it is one praguec prints and then accepts anyway.
//
// The expectation is derived and never written down: the compile fails exactly
// when one of the expected codes is an error by diag's own table. So this says
// nothing about what a severity ought to be -- the day a code becomes advisory
// the expectation moves with it and no fixture here needs an edit -- and
// everything about whether the severity a code has survives the trip from
// Raise to the gate. An empty expectation derives to false, which is the clean
// case and is why the clean helpers come through here too.
//
// What it does not catch is a demotion made in diag's table and its oracle
// together, because then the derivation agrees with the demotion. The tests
// that name a severity in a literal are what hold that.
//
// What it uniquely catches, on the record, because deriving its expectation
// from diag's table makes it look like it holds nothing at all:
//
// Bag.All returns the live backing slice and Diagnostic.Severity is an
// exported field, so any stage can demote a diagnostic AFTER raising it. diag
// cannot see that -- the stage doing it is not diag -- and neither can any
// table, because nothing about the code changed. Measured: appending
// `c.bag.All()[c.bag.Len()-1].Severity = diag.Warning` to the
// AliasTargetNotFound site in check.go fails two tests, both of them here and
// both through this comparison, and nothing else in the repo.
//
// And it is the widest net there is on the severity axis: instrumented, all 45
// codes in the language reach bag.HasErrors through one of these three
// helpers. Nothing else is within reach of that. Measured against the other
// attack -- demoting a code in diag's table and its oracle together -- only 16
// of the 45 were caught anywhere outside diag at all: 8 by the handful of
// tests that write a severity out as a literal, and 8 more incidentally, by
// render goldens holding the word "error" in a line they were written to pin
// for its position.
//
// So the derived expectation is a deliberate trade and not a weakness to be
// tightened: it buys coverage of every code at the price of one attack, and
// the attack it misses is held one layer up.
func assertGate(t *testing.T, set FileSet, bag *diag.Bag, want []diag.Code) {
	t.Helper()
	fails := false
	for _, c := range want {
		if c.Severity() == diag.Error {
			fails = true
		}
	}
	if got := bag.HasErrors(); got != fails {
		t.Errorf("HasErrors = %v, want %v for %v:\n%s", got, fails, want, renderAll(set, bag))
	}
}

// --- the model, dumped -------------------------------------------------------

// dump renders what the checker promises to have resolved -- what a name
// resolved to, what a view member took from its source, what a relation points
// at and what cardinality was derived -- one line per node, so that a whole
// program is one assertion and a mismatch reads as a diff.
func dump(s *Schema) string {
	var b strings.Builder
	for _, f := range s.Files {
		fmt.Fprintf(&b, "file %s\n", f.Path)
		fmt.Fprintf(&b, "  namespace %s\n", f.Namespace)
		for _, im := range f.Imports {
			fmt.Fprintf(&b, "  import %s\n", im.Path)
		}
		for _, d := range f.Decls {
			dumpDecl(&b, d)
		}
	}
	return b.String()
}

func dumpDecl(b *strings.Builder, d *Decl) {
	fmt.Fprintf(b, "  %s %s", d.Kind, d.Name)
	if d.Of != nil {
		fmt.Fprintf(b, " of %s", d.Of.Name)
	}
	b.WriteString("\n")
	for _, m := range d.EnumMembers {
		fmt.Fprintf(b, "    %s = %d\n", m.Name, m.Number)
	}
	for _, m := range d.Members {
		fmt.Fprintf(b, "    %s\n", memberString(d, m))
	}
}

func memberString(d *Decl, m *Member) string {
	var b strings.Builder
	if d.Key == m {
		b.WriteString("key ")
	}
	fmt.Fprintf(&b, "%s %s", m.Kind, m.Name)
	if m.HasTag() {
		fmt.Fprintf(&b, " = %d", m.Tag)
	}
	if m.Type.Kind != TypeUnresolved {
		fmt.Fprintf(&b, " %s", typeString(m.Type))
	}
	if len(m.Parts) > 0 {
		parts := make([]string, len(m.Parts))
		for i, p := range m.Parts {
			parts[i] = p.Name
		}
		fmt.Fprintf(&b, " (%s)", strings.Join(parts, ", "))
	}
	if m.Aliased != nil {
		fmt.Fprintf(&b, " -> %s", m.Aliased.Name)
	}
	// Where a view member's tag and type came from, which is the one thing a
	// view's model holds that its source does not.
	if m.From != nil {
		fmt.Fprintf(&b, " from %s.%s", d.Of.Name, m.From.Name)
	}
	for _, r := range m.Relations {
		fmt.Fprintf(&b, " [references(%s) %s]", r.Target.Name, r.Cardinality)
	}
	return b.String()
}

// typeString writes a resolved type the way the source spelled it, with what it
// resolved to in front of the name: `Odd` that resolved to a record reads
// differently from `Odd` that resolved to nothing.
func typeString(t Type) string {
	var s string
	switch t.Kind {
	case TypeScalar:
		s = t.Name
	case TypeDecl:
		s = fmt.Sprintf("%s:%s", t.Decl.Kind, t.Name)
	case TypeCollection:
		args := make([]string, len(t.Args))
		for i, a := range t.Args {
			args[i] = typeString(a)
		}
		s = fmt.Sprintf("%s<%s>", t.Name, strings.Join(args, ", "))
	default:
		s = "?" + t.Name
	}
	if t.Precision != 0 {
		s += fmt.Sprintf("(%d)", t.Precision)
	}
	if t.Optional {
		s += "?"
	}
	return s
}

// --- the milestone's headline claim ------------------------------------------

// The two spec sections as one program: the shared repository's file and the
// service that imports it. Every name resolves, every view expands against the
// table it is a subset of, every tag in a view is the tag its source declares,
// and the checker has nothing to say about any of it.
func TestCheckSpecSections(t *testing.T) {
	schema := checkClean(t, map[string]string{
		pragueFixture:   readFixture(t, pragueFixture),
		consumerFixture: readFixture(t, consumerFixture),
	})

	assertDump(t, schema, `file consumer.prague
  namespace BrandDataReader.Services.Cache
  import prague.prague
  view SlimSportGame of SportGame
    key field id = 1 int64 from SportGame.event_id
    field sport_type_id = 2 int64 from SportGame.sport_type_id
    field country_id = 3 int64 from SportGame.country_id
    field league_id = 4 int64 from SportGame.league_id
    field home_team_id = 5 int64? from SportGame.home_team_id
    field venue = 7 struct:Venue? from SportGame.venue
    field sport_type = 55 record:StaticDataRef from SportGame.sport_type
  view SlimOdd of Odd
    field name = 1 string from Odd.name
    field timestamp = 5 int64 from Odd.timestamp
  view SlimBetType of BetType
    key key id = 1 (event_id, bet_type_id) from BetType.key
    field event_id = 2 int64 from BetType.event_id
    field bet_type_id = 3 int64 from BetType.id
    field name = 4 string from BetType.name
    field status = 7 enum:BetTypeStatus from BetType.status
    field status_manual = 8 enum:BetTypeStatus? from BetType.status_manual
    field status_watch_dog = 9 enum:BetTypeStatus? from BetType.status_watch_dog
    field limit_manual_single = 22 decimal(4)? from BetType.limit_manual_single
    field timestamp = 25 int64 from BetType.timestamp
    field odds = 26 list<view:SlimOdd> from BetType.odds
file prague.prague
  namespace Common.IntegratedEventApi.Cache.Prague.Models
  struct Venue
    field id = 2 int64
    field name = 3 string
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
  record StaticDataRef
    field id = 1 int64
    field name = 2 string
    field provider_source_key = 3 string?
    field provider_key = 4 int64
  record Odd
    field name = 1 string
    field odds_actual = 9 double
    field timestamp = 5 int64
  table SportGame
    key field event_id = 1 int64
    field sport_type_id = 2 int64
    field country_id = 3 int64
    field league_id = 4 int64
    field home_team_id = 5 int64?
    field away_team_id = 6 int64?
    field venue = 7 struct:Venue?
    field fixture_id = 15 int32?
    field event_status = 24 enum:EventStatus
    field event_date = 28 int64
    field provider_timestamp = 35 int64
    field live_limit_manual_single = 36 decimal(4)?
    field runners = 47 list<record:StaticDataRef>
    field sport_type = 55 record:StaticDataRef
    field country = 56 record:StaticDataRef
    field league = 57 record:StaticDataRef
    field home_team = 58 record:StaticDataRef?
    field away_team = 59 record:StaticDataRef?
  table BetType
    key key key = 1 (event_id, bet_type_id)
    field event_id = 2 int64 [references(SportGame) many-to-one]
    field id = 3 int64
    alias bet_type_id int64 -> id
    field name = 4 string
    field status = 7 enum:BetTypeStatus
    field status_manual = 8 enum:BetTypeStatus?
    field status_watch_dog = 9 enum:BetTypeStatus?
    field limit_manual_single = 22 decimal(4)?
    field timestamp = 25 int64
    field odds = 26 list<record:Odd>
`)
}

// The other half of "lifted verbatim out of the spec": these fixtures are
// copies of section 1 and section 2, not paraphrases of them. If the spec
// changes its mind about the language, this fails before the checker quietly
// goes on resolving the old one.
func TestSpecFixturesAreVerbatim(t *testing.T) {
	assertVerbatim(t, specSection1, pragueFixture)
	assertVerbatim(t, specSection2, consumerFixture)
}

// assertSpecStates is assertVerbatim for prose. A fenced block is a fixture and
// can be held byte for byte; a paragraph is written to be read, and a pin that
// failed on a rewrap would teach the next reader to delete it. Runs of
// whitespace are collapsed on both sides and nothing else is -- not the words,
// not the emphasis, not the backticks and not the numbers -- so a reflow passes
// and an inversion does not.
//
// This package carries its own copy for the same reason it carries its own
// fixtures: what a package rests on belongs to the package that rests on it,
// and the syntax suite pinning a rule would not fail when the checker's own
// test for that rule goes on asserting the opposite of what the spec says.
func assertSpecStates(t *testing.T, specPath, claim string) {
	t.Helper()
	spec, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(collapseSpace(string(spec)), collapseSpace(claim)) {
		t.Errorf("%s no longer states:\n%s", specPath, claim)
	}
}

// collapseSpace renders every run of whitespace as one space, which is the
// whole of what a prose pin is allowed to forgive.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// section5NarrowAndRename is the rule TestAViewMemberMayNarrowAndRenameAtOnce
// rests on, quoted out of section 5. It is the three facts that test asserts
// apart, said by the authority for them: the rule itself, the example, and the
// tag being the source's and neither half's. The number is in the span on
// purpose -- `26` changed to `1` reads as a spec that lets the member invent a
// number after all, and nothing else in this suite would notice.
const section5NarrowAndRename = "**A view may narrow and rename in one member.** " +
	"`SlimOdd odds as o;` is still tag 26: the leading type says what the member holds, " +
	"`as o` says what the consumer calls it, and the tag is the source's as it is on " +
	"every other view line — neither half is about the tag."

// The citation, checked. A comment may quote the spec; only a test can say the
// spec still reads that way.
func TestSection5StatesTheNarrowAndRename(t *testing.T) {
	assertSpecStates(t, specSection5, section5NarrowAndRename)
}

// --- resolution --------------------------------------------------------------

// An import names a file, and the checker is handed a set of them. It can say
// that what it was promised is not here; it cannot say where it should have
// been, and nothing about how a file is found is decided by this diagnostic.
func TestAnImportThatNamesNoFileInTheSetIsDiagnosed(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\nimport \"missing.prague\";\n",
	})
	assertCodes(t, set, bag, diag.ImportNotFound)
	assertAt(t, set, bag, 0, "a.prague:3:1", `import "missing.prague";`)
}

// A file sees its own declarations and those of the files it imports, and
// nothing else. Two files in one set are not one scope: `b.prague` is in the
// set, is parsed, and its table is still not visible to a file that did not ask
// for it.
func TestAFileSeesOnlyItsOwnDeclarationsAndItsImports(t *testing.T) {
	const b = "revision 1;\nnamespace B;\ntable T {\n  key int64 id = 1;\n}\n"
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\nstruct S {\n  T t = 1;\n}\n",
		"b.prague": b,
	})
	assertCodes(t, set, bag, diag.UnknownType)

	// The same file, importing the one that declares T, resolves.
	checkClean(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\nimport \"b.prague\";\nstruct S {\n  T t = 1;\n}\n",
		"b.prague": b,
	})
}

// Scope is not transitive: a file that wants what its import imports writes the
// import itself. What a file can see is on the file, rather than spread across
// the graph behind it.
func TestScopeIsNotTransitive(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\nimport \"b.prague\";\nstruct S {\n  T t = 1;\n}\n",
		"b.prague": "revision 1;\nnamespace B;\nimport \"c.prague\";\n",
		"c.prague": "revision 1;\nnamespace C;\ntable T {\n  key int64 id = 1;\n}\n",
	})
	assertCodes(t, set, bag, diag.UnknownType)
}

// The underline is the second `S` and nothing else, which is why it is pinned
// by extent and not merely by position.
//
// What is duplicated is one word. The whole declaration is three lines of which
// two are unobjectionable, the render clamps its caret run to the first of them
// anyway, and renaming that word is the entire fix -- so an underline of the
// whole thing said that all of it was wrong and then showed only part of what
// it claimed. check.Decl carries NameSpan for this.
//
// The position inside the message is the other declaration's keyword, and
// TestAMessageQuotesADeclarationAtItsKeyword is what holds it there: where to
// point the caret and where to send a reader next are different questions, and
// this one moving is not a reason for that one to.
func TestDuplicateDeclarationsAreDiagnosed(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\nstruct S {\n  int64 x = 1;\n}\nrecord S {\n  int64 y = 1;\n}\n",
	})
	assertCodes(t, set, bag, diag.DuplicateDeclaration)
	assertAt(t, set, bag, 0, "a.prague:6:8", "S")
}

// A declaration cannot take a name protop's own type system already holds.
//
// The language has no keyword token kinds -- that is load-bearing and is not in
// question here -- so `record list { ... }` parses like any other declaration,
// and before this rule it was then accepted in silence. The mistake is the
// declaration, so the diagnostic is at the declaration.
//
// What the rule does NOT do, written down here because ad53268's commit message
// and the first version of this comment both claimed otherwise: it does not
// replace the PROTOP0306 that #39 was reported through. A file that also writes
// the downstream `list refs = 2;` does not parse -- a bare `list` is the
// parser's arity diagnostic -- and Check returns nil on a bag that already has
// errors, so for THAT file no PROTOP0207 is reached at all. The input the
// ticket was filed about is not the input this rule reports on.
// TestADownstreamParseErrorKeepsTheBuiltinRuleFromRunning pins that, and
// whether the gate should hold for the 0200 group is a question of its own and
// not this rule's to answer by accident.
//
// It is worth having for the file that DOES parse, which is most of a schema's
// life: one under development, or one where the unnameable declaration simply
// has no use site yet. There the user is told at the declaration -- the only
// line where a fix exists, because a type written `list` is read as the
// collection and the declaration has no second spelling that reaches it.
//
// All five declaration keywords, and both closed sets. The collections are what
// #39 was reported about; a scalar has exactly the same shape, and a rule that
// covered only the generics would leave `struct string` unnameable and unsaid.
func TestADeclarationCannotTakeABuiltinTypeName(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
record list {
  int64 id = 1;
}
table map {
  key int64 id = 1;
}
struct string {
  int64 id = 1;
}
enum int64 {
  A = 1;
}
view double of T {
  id;
}
table T {
  key int64 id = 1;
}
`,
	})
	assertCodes(t, set, bag,
		diag.BuiltinTypeDeclared, diag.BuiltinTypeDeclared, diag.BuiltinTypeDeclared,
		diag.BuiltinTypeDeclared, diag.BuiltinTypeDeclared)

	// One per declaration keyword, each at its own keyword. The position is the
	// whole of the fix: it is what was wrong before, not the wording.
	for i, want := range []string{"a.prague:3:1", "a.prague:6:1", "a.prague:9:1", "a.prague:12:1", "a.prague:15:1"} {
		assertSpanStartsAt(t, bag, i, want)
	}

	// The message names which of the two sets the name was taken from, and that
	// word is derived from the sets rather than written beside the rule.
	assertMessageMentions(t, bag, 0, "protop's built-in generic type")
	assertMessageMentions(t, bag, 2, "protop's built-in scalar type")
}

// The input #39 was reported through does not reach the rule above, and that is
// a stated fact rather than an unmentioned surprise.
//
// Two unnameable declarations and a downstream `list refs = 2;`: the bare `list`
// is PROTOP0306 at the PARSER, Check returns nil when the bag already has
// errors, and so the two declarations above it are told nothing. The user is
// shown the use site and not the declarations -- which is the complaint #39 was
// filed about, and is NOT what PROTOP0207 fixed.
//
// Pinned, not repaired. Whether a parse error should suppress checker
// diagnostics is a question about the whole 0200 group, and
// TestCheckDoesNotRunOnATreeThatDidNotParse below is the deliberate contract
// that answers it today; widening that here would decide a much larger thing as
// a side effect of a rule about names. This test is what fails, loudly and with
// the reason beside it, on the day someone does decide it.
func TestADownstreamParseErrorKeepsTheBuiltinRuleFromRunning(t *testing.T) {
	f := source.NewFile("a.prague", `revision 1;
namespace A;
record list {
  int64 x = 1;
}
record string {
  int64 y = 1;
}
record Holder {
  list refs = 2;
}
`)
	var bag diag.Bag
	set := FileSet{"a.prague": {Source: f, File: syntax.Parse(f, &bag)}}
	if schema := Check(set, &bag); schema != nil {
		t.Error("Check returned a model for a file that did not parse")
	}
	// Exactly one diagnostic, and it is the parser's, at the use site. NOT the
	// two PROTOP0207s at 3:1 and 6:1 that the same file without line 10 gets --
	// the assertion is the absence, so the codes are held as a sequence and not
	// as a membership test.
	assertCodes(t, set, &bag, diag.TypeArgumentCountWrong)
	// Positioned off the source file rather than through assertSpanStartsAt: a
	// parser diagnostic carries no File, because the parser is handed one and
	// never has a second to disambiguate against.
	if got := f.Position(bag.All()[0].Span.Start).String(); got != "a.prague:10:3" {
		t.Errorf("the one diagnostic is at %s, want the use site at a.prague:10:3", got)
	}
}

// Every name in either closed set, walked rather than listed.
//
// This is what makes the rule follow the sets. The checker asks
// syntax.IsBuiltinTypeName and this asks syntax.BuiltinTypeNames; both are
// computed from `scalarTypes` and `collectionTypes` themselves, so a seventh
// scalar added to the parser is covered here with no edit at this end.
// Measured: adding one and running this file is what catches a rule that did
// not grow with it.
//
// A list written out here instead would be a second hand-maintained copy of a
// closed set, which is the failure mode this repository has produced before --
// most recently #43, where a census listed names by hand and certified the
// exact gap it was built to fail.
//
// What is IN the sets is #32's question and nothing here touches it. The claim
// is that the set is closed and that the rule reads all of it.
func TestEveryBuiltinTypeNameIsDiagnosedAtItsDeclaration(t *testing.T) {
	names := syntax.BuiltinTypeNames()
	if len(names) == 0 {
		t.Fatal("syntax.BuiltinTypeNames() is empty; the walk below would assert nothing")
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			set, _, bag := checkSet(t, map[string]string{
				"a.prague": fmt.Sprintf("revision 1;\nnamespace A;\nrecord %s {\n  int64 x = 1;\n}\n", name),
			})
			assertCodes(t, set, bag, diag.BuiltinTypeDeclared)
			assertSpanStartsAt(t, bag, 0, "a.prague:3:1")
			assertMessageMentions(t, bag, 0, "`"+name+"` already names protop's built-in ")
		})
	}
}

// section4BuiltinNames is the rule the two tests above implement, quoted out of
// section 4 -- which had nothing to say about a declaration's name at all until
// #61, and was where a reader would have looked.
//
// The span stops at the reachability clause on purpose. The rule and the reason
// for it are one claim: dropped to "a declaration may not be named with one of
// protop's built-in type names" alone it reads as an arbitrary reserved-word
// list, and a reserved-word list is a thing someone eventually writes out.
const section4BuiltinNames = "**A declaration may not be named with one of protop's built-in type names.** " +
	"The set is the language's own — its scalars and its collections — and it is referred to here " +
	"rather than transcribed, because whether the scalar set is closed at all is one of the things " +
	"this document deliberately leaves open (see the index's *Deliberately not covered*), and a copy " +
	"of it in prose is a copy that goes stale without failing anything. The rule is about reachability " +
	"and not about reserved words: a type written with such a name resolves to the built-in, so no " +
	"field, view member or relation could ever spell its way to the declaration."

// The citation for the two tests above, and the only assertion in this file
// about a document rather than about the checker. It stands here rather than
// with the other spec pins because its argument depends on standing here.
//
// Anchor or decoration, in the terms the last six of these were got wrong in.
// Two halves, and they are not the same kind of thing:
//
//   - assertSpecStates is DECORATION, and is labelled that rather than dressed
//     up. What turns it red is someone editing the sentence. It earns its place
//     for the one job the section 5 pin does -- a test resting on the spec's
//     authority should fail when the spec stops saying the thing, instead of
//     going on enforcing a rule nobody writes down any more -- and it proves
//     nothing whatever about the rule.
//
//   - the walk is the ANCHOR. What turns it red is a fact changing: section 4
//     coming to name a member of the built-in set in code voice. That happens
//     when someone transcribes the set into the prose, and it happens when
//     `scalarTypes` or `collectionTypes` grows to include a word section 4
//     already writes that way. Neither needs anyone to touch this file, which
//     is the whole of why it goes through syntax.BuiltinTypeNames() and not
//     through a list written here -- a list written here would be the fourth
//     copy of the set, in the file complaining about the third.
//
// The rule's own truth is not re-asserted. It is asserted directly above, over
// the same syntax.BuiltinTypeNames(), so the day the rule narrows to the
// collections the person repairing those two tests is already reading this one.
//
// Where the walk reads the code form, and where it stops. The backticked walk
// reads the code form, the bare backticked name, because that is how this
// document writes a type name. Section 4's `list<T>` is a usage of a collection
// rather than a naming of the set and passes for that reason.
//
// It is not enough on its own, and #61 said so out loud: a transcription in
// running prose with no backticks passes it, and so -- found by mutation, not
// predicted -- does one in a fenced block, which is the more natural way to
// write a set down. The argument for leaving it there was that no scan of the
// plain words is possible, because section 4 says "a struct holding a string is
// allowed" about the English noun and any such scan would fail on it.
//
// That argument confuses a mention with a transcription. A transcription is a
// list, and a list is dense: it puts most of the set in one place. Section 4
// today names one built-in per paragraph at most -- `string` the English noun
// in the struct paragraph, `list` in the record one -- and nothing it could
// reasonably come to say needs three of them in a single paragraph except a
// copy of the set. So the plain-word walk counts distinct built-ins per
// paragraph with the backticks and fence markers stripped, and fails at three.
// Two names together is a comparison. Three is an enumeration.
//
// The threshold is the one number here that is a judgement rather than a fact,
// and it is deliberately loose: the mutations it exists to catch put all eight
// names in one paragraph, so it has five names of margin, and the room it
// leaves is room for prose that discusses a pair of built-ins -- which section
// 4 is entitled to do and has no way to do if the threshold is two.
const section4Transcription = 3

// section4Paragraphs is the document split on blank lines, with backticks and
// fence markers stripped. Stripping them is the point: the backticked walk
// already has the code form, and this one exists for the forms it cannot see.
func section4Paragraphs(spec string) []string {
	plain := strings.ReplaceAll(spec, "`", "")
	var out []string
	for _, p := range strings.Split(plain, "\n\n") {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

// namesWholeWord reports whether name occurs in text as a whole word -- so
// `string` is found in "holding a string" and not in "forbids strings", and
// `list` is found in "list<T>" because `<` ends a word.
func namesWholeWord(text, name string) bool {
	for i := 0; ; {
		j := strings.Index(text[i:], name)
		if j < 0 {
			return false
		}
		j += i
		before := j == 0 || !isWordByte(text[j-1])
		after := j+len(name) == len(text) || !isWordByte(text[j+len(name)])
		if before && after {
			return true
		}
		i = j + 1
	}
}

func isWordByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}

func TestSection4StatesTheBuiltinNameRuleWithoutCopyingTheSet(t *testing.T) {
	assertSpecStates(t, specSection4, section4BuiltinNames)

	spec, err := os.ReadFile(specSection4)
	if err != nil {
		t.Fatal(err)
	}
	names := syntax.BuiltinTypeNames()
	if len(names) == 0 {
		t.Fatal("syntax.BuiltinTypeNames() is empty; the walks below would assert nothing")
	}
	for _, name := range names {
		if strings.Contains(string(spec), "`"+name+"`") {
			t.Errorf("%s writes `%s`: the rule is stated by reference to the built-in set, "+
				"and a copy of the set in prose is the copy #32 would leave stale", specSection4, name)
		}
	}
	for _, para := range section4Paragraphs(string(spec)) {
		var found []string
		for _, name := range names {
			if namesWholeWord(para, name) {
				found = append(found, name)
			}
		}
		if len(found) >= section4Transcription {
			t.Errorf("%s names %d built-ins in one paragraph (%s): that is the set transcribed, "+
				"whatever it is dressed in, and it is the copy #32 would leave stale. "+
				"The paragraph is:\n\n%s", specSection4, len(found), strings.Join(found, ", "),
				strings.TrimSpace(para))
		}
	}
}

// Only the two closed sets. A name that merely resembles one of protop's is an
// ordinary declaration: a rule that could not tell the difference would have
// taken the user's whole name space rather than the eight words the language
// actually owns.
func TestANameThatMerelyResemblesABuiltinIsFine(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
record List {
  int64 x = 1;
}
record lists {
  int64 x = 1;
}
record Int64 {
  int64 x = 1;
}
struct my_string {
  int64 x = 1;
}
`,
	})
	for _, name := range []string{"List", "lists", "Int64", "my_string"} {
		if schema.File("a.prague").Decl(name) == nil {
			t.Errorf("`%s` is not in the model; it names nothing protop has", name)
		}
	}
}

// A MEMBER may take a built-in type name. The rule is confined to declarations,
// and this is what holds it there.
//
// `int64 string = 1;` is legal and has to stay legal: a member name is looked
// up in the declaration that carries it and never in the type system, so
// nothing about it competes with the scalar. Only a DECLARATION's name is what
// a type gets written as, and so only a declaration's name can be taken.
//
// Held by a fixture because it was held by nothing else. Measured: moving the
// raise out of declareOne and into addMember, guarded by
// syntax.IsBuiltinTypeName on the member's name, leaves every OTHER test in the
// repository green and examples/prague/*.prague clean -- no member in the
// corpus happens to be named like a built-in, so neither the suite nor the
// corpus knew the boundary was there. This test is the only thing that fails.
//
// The field names are read from syntax.BuiltinTypeNames rather than written
// out, for the same reason the positive walk does: a seventh scalar added to
// the parser is then also proved still legal on a member, with no edit here.
// The alias and the view rename are hand-written, because a second loop over
// the same names would only collide with the fields -- but all three member
// forms funnel into one addMember, and a fixture of fields alone would leave
// two of its three callers unpinned.
func TestAMemberMayTakeABuiltinTypeName(t *testing.T) {
	names := syntax.BuiltinTypeNames()
	if len(names) == 0 {
		t.Fatal("syntax.BuiltinTypeNames() is empty; the fixture below would carry no member")
	}
	var b strings.Builder
	b.WriteString("revision 1;\nnamespace A;\ntable T {\n  key int64 id = 1;\n")
	for i, name := range names {
		fmt.Fprintf(&b, "  int64 %s = %d;\n", name, i+2)
	}
	b.WriteString("}\ntable U {\n  key int64 id = 1;\n  alias list = id;\n  alias string = id;\n}\nview V of T {\n  id as double;\n}\n")

	schema := checkClean(t, map[string]string{"a.prague": b.String()})

	// Each one arrived under the name it was written with. checkClean says
	// nothing was reported; this says the members are in the model rather than
	// silently dropped, which is the other way the rule could have eaten them.
	f := schema.File("a.prague")
	for _, name := range names {
		if f.Decl("T").Member(name) == nil {
			t.Errorf("`T` has no member named `%s`; a member name is not a type name", name)
		}
	}
	for _, name := range []string{"list", "string"} {
		if f.Decl("U").Member(name) == nil {
			t.Errorf("`U` has no alias named `%s`", name)
		}
	}
	if f.Decl("V").Member("double") == nil {
		t.Error("`V` has no member named `double`; a view rename may land on a built-in name too")
	}
}

// The offending declaration is still registered, and that is a decision rather
// than an omission.
//
// The name belongs to the language and not to this file, but the DECLARATION is
// well formed: it has members, tags and a key, and every other rule in the
// checker still has something true to say about it. Dropping it would turn one
// mistake into a second wave of them, because a view's source, a relation's
// target and a narrowing are looked up in the file's scope rather than in the
// type system -- all three would then also report the declaration missing, on
// lines the user got right.
//
// DuplicateDeclaration drops its declaration for a reason that does not reach
// here: two declarations cannot both hold one symbol-table slot. Nothing is
// contending for a slot in this case -- `string` is not in byName at all until
// this declaration puts it there.
func TestADeclarationWithABuiltinNameIsStillRegistered(t *testing.T) {
	set, schema, bag := checkSet(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
record string {
  int64 id = 1;
}
view V of string {
  id;
}
`,
	})
	// One diagnostic, about the declaration. Dropping the declaration instead
	// would add a PROTOP0205 about the view, which is a line the user got right.
	assertCodes(t, set, bag, diag.BuiltinTypeDeclared)

	d := schema.File("a.prague").Decl("string")
	if d == nil {
		t.Fatal("the declaration is not in the model; the rule reports a name, it does not delete a declaration")
	}
	if got := schema.File("a.prague").Decl("V").Of; got != d {
		t.Error("the view did not resolve against the declaration it names")
	}
}

// Two declarations of one built-in name raise the built-in rule twice and the
// duplicate rule once, in that order.
//
// Both, because they are two facts with two fixes -- the same reason
// DuplicateMember is held apart from DuplicateTag. Renaming one of the pair
// answers PROTOP0204 and leaves the other one unnameable; renaming both is what
// PROTOP0207 asks for, and a user shown only the duplicate would think the
// first rename had finished the job.
//
// Twice, because each declaration independently takes a name the schema was
// never entitled to. The order follows from where the rule sits: the built-in
// check runs before the name is looked up in the symbol table, so the second
// declaration is told what is wrong with it before it is told what it collides
// with.
//
// The two diagnostics about the second declaration land in different columns,
// and that is the point rather than an accident. PROTOP0207 is about the
// declaration -- this whole thing is unreachable, because nothing can spell its
// way to it -- so it underlines the whole thing. PROTOP0204 is about the name
// alone, because renaming that one word is the entire fix.
//
// Which is why all three are held by EXTENT and not by where they start. A
// start alone would let 0207 collapse to a caret and let 0204 grow back to the
// declaration, and either way the sentence above would still read as true while
// the thing it describes had gone. The extents ARE the decision; the columns
// are only how it shows.
func TestTwoDeclarationsOfOneBuiltinNameRaiseBoth(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
record list {
  int64 a = 1;
}
record list {
  int64 b = 1;
}
`,
	})
	assertCodes(t, set, bag,
		diag.BuiltinTypeDeclared, diag.BuiltinTypeDeclared, diag.DuplicateDeclaration)
	const whole = "record list {\n  int64 a = 1;\n}"
	assertAt(t, set, bag, 0, "a.prague:3:1", whole)
	assertAt(t, set, bag, 1, "a.prague:6:1", strings.Replace(whole, "a = 1", "b = 1", 1))
	assertAt(t, set, bag, 2, "a.prague:6:8", "list")
}

// PROTOP0204's underline is the name for all five declaration keywords.
//
// One row per keyword because the span comes from five separate assignments --
// check.declareOne type-switches over the five AST types and each arm builds
// its own Decl -- so covering one covers one. A suite whose every duplicate
// fixture happened to be `struct S` then `record S` held two of the five and
// read as though it held them all.
//
// A dropped assignment does not widen the underline back to the declaration,
// which would at least be recognisable. It leaves a zero span, and a zero span
// is position 0: the diagnostic reports at line 1, column 1, and sends the
// reader to the `revision` at the top of a file whose fault is forty lines
// down. That is the failure this pins against, and it is worse than the one
// the span fix was for.
//
// The first declaration is a `struct` in every row so that only the second
// keyword varies, and the column each row expects is the width of its own
// keyword plus a space -- which is the assertion, since a span taken from the
// declaration rather than the name would sit in column 1 for all five and a
// zero span would sit at 1:1 for all five.
func TestADuplicateUnderlinesTheNameForEveryDeclarationKeyword(t *testing.T) {
	const preamble = `revision 1;
namespace A;
table Src {
  key int64 id = 1;
}
struct S {
  int64 x = 1;
}
`
	tests := []struct {
		kind   string
		second string
		at     string
	}{
		{"struct", "struct S {\n  int64 y = 1;\n}\n", "a.prague:9:8"},
		{"record", "record S {\n  int64 y = 1;\n}\n", "a.prague:9:8"},
		{"table", "table S {\n  key int64 id = 1;\n}\n", "a.prague:9:7"},
		{"view", "view S of Src {\n  id;\n}\n", "a.prague:9:6"},
		{"enum", "enum S {\n  A = 0;\n}\n", "a.prague:9:6"},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			set, _, bag := checkSet(t, map[string]string{"a.prague": preamble + tt.second})
			assertCodes(t, set, bag, diag.DuplicateDeclaration)
			assertAt(t, set, bag, 0, tt.at, "S")
		})
	}
}

// A name declared in an imported file is not a duplicate of one declared here.
// Shadowing is what makes a shared repository usable: a service that happens to
// name a type the way the shared schema does is not thereby broken.
func TestADeclarationMayShadowAnImportedName(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\nimport \"b.prague\";\nstruct S {\n  int64 x = 1;\n}\nstruct Holder {\n  S s = 1;\n}\n",
		"b.prague": "revision 1;\nnamespace B;\nstruct S {\n  int64 y = 1;\n}\n",
	})
	own := schema.File("a.prague").Decl("S")
	if got := schema.File("a.prague").Decl("Holder").Member("s").Type.Decl; got != own {
		t.Errorf("`S` resolved to the declaration in %s, want the one in a.prague", got.File.Path)
	}
}

// The underline is the whole of the second member and not its name alone,
// which is deliberate and is why it is written out: the member is what is
// duplicated, the first one is named by position inside the message, and a
// member is one line so the caret does not run away down the file. That is the
// difference from PROTOP0204 above, whose span is three lines.
func TestDuplicateMembersAreDiagnosed(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\ntable T {\n  key int64 id = 1;\n  int64 id = 2;\n}\n",
	})
	assertCodes(t, set, bag, diag.DuplicateMember)
	assertAt(t, set, bag, 0, "a.prague:5:3", "int64 id = 2;")
}

// A member's name and a member's tag are two facts with two fixes, so an alias
// -- which has no tag at all -- still collides by name.
func TestAnAliasCollidesByNameWithAField(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\ntable T {\n  key int64 id = 1;\n  alias id = id;\n}\n",
	})
	assertCodes(t, set, bag, diag.DuplicateMember)
	// The alias, whole: an alias carries no tag, so the member is all there is
	// to point at and the underline says which of the two `id`s is the second.
	assertAt(t, set, bag, 0, "a.prague:5:3", "alias id = id;")
}

func TestAnUnknownTypeIsDiagnosed(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\nstruct S {\n  Missing x = 1;\n}\n",
	})
	assertCodes(t, set, bag, diag.UnknownType)
	assertAt(t, set, bag, 0, "a.prague:4:3", "Missing")
}

// A collection resolves its arguments, and nesting costs nothing: a list of a
// list is the same resolution twice.
func TestACollectionResolvesItsArguments(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\nrecord R {\n  int64 x = 1;\n}\nstruct S {\n  list<list<R>>? xs = 1;\n}\n",
	})
	m := schema.File("a.prague").Decl("S").Member("xs")
	if got := typeString(m.Type); got != "list<list<record:R>>?" {
		t.Errorf("type is %s, want list<list<record:R>>?", got)
	}
}

// `decimal(N)` carries its precision into the model. A precision on anything
// else is PROTOP0301 at the parser and never gets here.
func TestDecimalCarriesItsPrecision(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\nstruct S {\n  decimal(4)? x = 1;\n}\n",
	})
	if got := schema.File("a.prague").Decl("S").Member("x").Type.Precision; got != 4 {
		t.Errorf("precision is %d, want 4", got)
	}
}

// The caret underlines the name that is unknown and stops there. `Missing?` is
// a type written in two pieces and only one of them is the mistake: a caret
// that swallowed the `?` would tell the reader the presence marker is part of
// what praguec could not find, and the caret's width is the whole reason spans
// are carried instead of positions.
func TestAnUnknownTypeUnderlinesTheNameAndNotTheWholeTypeReference(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\nstruct S {\n  Missing? x = 1;\n}\n",
	})
	assertCodes(t, set, bag, diag.UnknownType)
	assertAt(t, set, bag, 0, "a.prague:4:3", "Missing")
}

// What a file declares and what a file can see are two questions, and `Decl`
// answers the first one. They differ for exactly the names an import brings,
// which is why an accessor that quietly answered the second would be right on
// every file that imports nothing -- and on none of the ones this language
// exists for.
func TestAFilesDeclarationsAreItsOwnAndNotItsImports(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\nimport \"b.prague\";\nstruct S {\n  T t = 1;\n}\n",
		"b.prague": "revision 1;\nnamespace B;\ntable T {\n  key int64 id = 1;\n}\n",
	})
	a, b := schema.File("a.prague"), schema.File("b.prague")
	if a.Decl("T") != nil {
		t.Error("a.prague declares `T`; it imports the file that declares it, and an import is not a declaration")
	}
	// The name is in scope all the same, and resolved to the declaration the
	// imported file owns. Scope is the wider answer, asked where a name is
	// resolved rather than here.
	if got := a.Decl("S").Member("t").Type.Decl; got != b.Decl("T") {
		t.Error("`T` did not resolve to the declaration b.prague owns")
	}
}

// An import that resolved to nothing leaves nothing behind. `Imports` is the
// list of files this one can see, and an entry for a file that was never found
// is a file every later reader of the model would walk into.
func TestAnImportThatNamesNoFileAddsNoImport(t *testing.T) {
	_, schema, bag := checkSet(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\nimport \"missing.prague\";\n",
	})
	if !bag.HasErrors() {
		t.Fatal("the import resolved; it is supposed to name no file in the set")
	}
	if got := schema.File("a.prague").Imports; len(got) != 0 {
		t.Errorf("the file lists %d imports, want none: the only one it writes was not found", len(got))
	}
}

// --- views -------------------------------------------------------------------

func TestAViewSourceMustResolve(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\nview V of Missing {\n  x;\n}\n",
	})
	assertCodes(t, set, bag, diag.ViewSourceNotFound)
	assertAt(t, set, bag, 0, "a.prague:3:11", "Missing")
}

// A view's source is a table or a record. Section 5 in one sentence, and three
// ways to get it wrong.
func TestAViewSourceIsATableOrARecord(t *testing.T) {
	const decls = `struct S {
  int64 x = 1;
}
enum E {
  A = 0;
}
record R {
  int64 x = 1;
}
view OfR of R {
  x;
}
`
	for _, src := range []string{"S", "E", "OfR"} {
		t.Run(src, func(t *testing.T) {
			set, _, bag := checkSet(t, map[string]string{
				"a.prague": "revision 1;\nnamespace A;\n" + decls + "view V of " + src + " {\n  x;\n}\n",
			})
			assertCodes(t, set, bag, diag.ViewSourceNotSelectable)
			// The source name alone, in all three arms. The message says what
			// that name is -- a struct, an enum, a view -- so an underline
			// under the whole `view V of S {` would point at two names and
			// mean one of them.
			assertAt(t, set, bag, 0, "a.prague:15:11", src)
		})
	}
}

// The diagnostic the design exists to produce. The shared repo renames a field;
// every view still selecting the old name fails to compile, in the repo that
// drifted, instead of decoding to nil at runtime.
func TestAViewMemberMustNameAMemberOfItsSource(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"consumer.prague": "revision 1;\nnamespace A;\nimport \"prague.prague\";\nview V of SportGame {\n  odds;\n}\n",
		pragueFixture:     readFixture(t, pragueFixture),
	})
	assertCodes(t, set, bag, diag.NoSuchMember)
	assertAt(t, set, bag, 0, "consumer.prague:5:3", "odds")
	if msg := bag.All()[0].Message; !strings.Contains(msg, "prague.prague:") {
		t.Errorf("message does not say where `SportGame` is declared: %s", msg)
	}
}

// The severity of a checker diagnostic, end to end. A code sequence says what
// was noticed; this says what it costs, which is a different claim. bag.HasErrors
// is what cmd/praguec turns into an exit code and what every stage after the
// checker inherits, so a checker diagnostic that does not fail the compile is a
// schema praguec reports on and then generates C# from anyway.
//
// The wanted severity is written here as a literal, and that is the point --
// assertGate cannot make this claim, because assertGate derives what it expects
// from diag's own table and so agrees with a demotion made in that table. These
// three codes are pinned twice, by two tests that cannot both be satisfied by
// the same edit: diag's table says what the code means, and this says the
// meaning survives the trip from Raise to the gate.
//
// PROTOP0312 leads because codes.go calls it the diagnostic the whole design
// exists to produce. It was demotable in two edits -- the table and its oracle
// -- with all seven packages staying green, which is the hole this closes.
func TestACheckerDiagnosticFailsTheCompile(t *testing.T) {
	for _, tt := range []struct {
		name string
		src  string
		want diag.Code
	}{
		{
			name: "a view member the source does not declare",
			src: `revision 1;
namespace A;
table T {
  key int64 id = 1;
}
view V of T {
  missing;
}
`,
			want: diag.NoSuchMember,
		},
		{
			name: "a field of a type nothing declares",
			src: `revision 1;
namespace A;
struct S {
  Nope n = 1;
}
`,
			want: diag.UnknownType,
		},
		{
			name: "one name declared twice",
			src: `revision 1;
namespace A;
struct S {
  int64 x = 1;
}
record S {
  int64 y = 1;
}
`,
			want: diag.DuplicateDeclaration,
		},
		{
			name: "a declaration under a built-in type name",
			src: `revision 1;
namespace A;
record list {
  int64 x = 1;
}
`,
			want: diag.BuiltinTypeDeclared,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			set, _, bag := checkSet(t, map[string]string{"a.prague": tt.src})
			if bag.Len() != 1 || bag.All()[0].Code != tt.want {
				t.Fatalf("want one %s:\n%s", tt.want, renderAll(set, bag))
			}
			if got := bag.All()[0].Severity; got != diag.Error {
				t.Errorf("%s raised as a %s, want an Error", tt.want, got)
			}
			if !bag.HasErrors() {
				t.Errorf("%s does not fail the compile; praguec would accept this schema", tt.want)
			}
		})
	}
}

// Tag, type and presence come from the source; the view line carries the name
// and an optional rename, and nothing else. `?` included: presence is not
// something a view gets to change its mind about.
func TestAViewMemberTakesTagTypeAndPresenceFromItsSource(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
table T {
  key int64 id = 1;
  decimal(4)? d = 9;
}
view V of T {
  d;
}
`,
	})
	m := schema.File("a.prague").Decl("V").Member("d")
	if m.Tag != 9 || typeString(m.Type) != "decimal(4)?" {
		t.Errorf("view member is %s, want tag 9 and decimal(4)?", memberString(schema.File("a.prague").Decl("V"), m))
	}
	if m.From != schema.File("a.prague").Decl("T").Member("d") {
		t.Error("view member does not point at the source member it took its tag from")
	}
}

// `event_id as id;` renames in the view and nowhere else. The alias on the
// source table is what keeps the two APIs matching, and it is a separate
// declaration -- the checker does not synthesize one.
func TestARenameIsTheViewsAlone(t *testing.T) {
	schema := checkClean(t, map[string]string{
		pragueFixture:   readFixture(t, pragueFixture),
		consumerFixture: readFixture(t, consumerFixture),
	})
	view := schema.File(consumerFixture).Decl("SlimSportGame")
	table := schema.File(pragueFixture).Decl("SportGame")

	if view.Member("id") == nil || view.Member("event_id") != nil {
		t.Error("the view knows the member by its new name and not by its old one")
	}
	if table.Member("event_id") == nil || table.Member("id") != nil {
		t.Error("the rename reached the source table; a view renames in the view and nowhere else")
	}
	// BetType's alias is written in section 1, by hand, and there is exactly
	// one of it. A rename that synthesized an alias would put a second member
	// on the table nobody wrote.
	bet := schema.File(pragueFixture).Decl("BetType")
	if got := len(bet.Members); got != 11 {
		t.Errorf("BetType has %d members, want the 11 section 1 writes", got)
	}
}

// A narrowing asks for another view. A scalar in that position is PROTOP0309 at
// the parser -- the one case a single file can answer -- and everything else is
// answered here.
func TestANarrowingMustNameAView(t *testing.T) {
	const decls = `record R {
  int64 x = 1;
}
table T {
  key int64 id = 1;
  R r = 2;
}
`
	t.Run("a declaration that is not a view", func(t *testing.T) {
		set, _, bag := checkSet(t, map[string]string{
			"a.prague": "revision 1;\nnamespace A;\n" + decls + "view V of T {\n  R r;\n}\n",
		})
		assertCodes(t, set, bag, diag.NarrowingNotAView)
		// The narrowing name and not the member: `R r;` is wrong in its first
		// word only, and the message says to delete that word.
		assertAt(t, set, bag, 0, "a.prague:11:3", "R")
	})
	t.Run("a name that is nothing at all", func(t *testing.T) {
		set, _, bag := checkSet(t, map[string]string{
			"a.prague": "revision 1;\nnamespace A;\n" + decls + "view V of T {\n  SlimR r;\n}\n",
		})
		assertCodes(t, set, bag, diag.UnknownType)
	})
}

// The check the construct exists for: a narrowing that points elsewhere would
// hand the member the tags of a declaration it has nothing to do with.
func TestANarrowingMustBeAViewOfTheMembersType(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
record R {
  int64 x = 1;
}
record Other {
  int64 y = 1;
}
view SlimOther of Other {
  y;
}
table T {
  key int64 id = 1;
  R r = 2;
}
view V of T {
  SlimOther r;
}
`,
	})
	assertCodes(t, set, bag, diag.NarrowingSourceMismatch)
	assertAt(t, set, bag, 0, "a.prague:17:3", "SlimOther")
}

// A narrowing reaches through a collection to what it holds: section 5 gives
// `SlimOdd odds;` tag 26, and tag 26 in section 1 is `list<Odd> odds`.
func TestANarrowingReachesThroughACollection(t *testing.T) {
	schema := checkClean(t, map[string]string{
		pragueFixture:   readFixture(t, pragueFixture),
		consumerFixture: readFixture(t, consumerFixture),
	})
	m := schema.File(consumerFixture).Decl("SlimBetType").Member("odds")
	if got := typeString(m.Type); got != "list<view:SlimOdd>" {
		t.Errorf("narrowed type is %s, want list<view:SlimOdd>", got)
	}
}

// A view of a view is not written -- PROTOP0206 says so -- but a narrowing to a
// view of a view is, and the tags stay the original's either way. `SlimBetType`
// holds `SlimOdd` at BetType's tag 26, and `SlimOdd`'s own members are at
// `Odd`'s tags 1 and 5. Neither view invented a number.
func TestANarrowingToAViewKeepsTheOriginalsTags(t *testing.T) {
	schema := checkClean(t, map[string]string{
		pragueFixture:   readFixture(t, pragueFixture),
		consumerFixture: readFixture(t, consumerFixture),
	})
	consumer := schema.File(consumerFixture)
	if got := consumer.Decl("SlimBetType").Member("odds").Tag; got != 26 {
		t.Errorf("`SlimOdd odds;` is tag %d, want BetType's 26", got)
	}
	slim := consumer.Decl("SlimOdd")
	if got, want := slim.Member("name").Tag, uint64(1); got != want {
		t.Errorf("SlimOdd.name is tag %d, want Odd's %d", got, want)
	}
	if got, want := slim.Member("timestamp").Tag, uint64(5); got != want {
		t.Errorf("SlimOdd.timestamp is tag %d, want Odd's %d", got, want)
	}
}

// A view cannot write a tag, so the only way for one to hold a tag twice is to
// select the same member twice under two names. It collides like any other.
func TestTwoViewMembersCannotSelectOneMember(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
table T {
  key int64 id = 1;
  int64 x = 2;
}
view V of T {
  x as a;
  x as b;
}
`,
	})
	assertCodes(t, set, bag, diag.DuplicateTag)
}

// A view's identity is its source's, section 4 -- under whatever name the view
// gives the member.
func TestAViewInheritsItsSourcesIdentity(t *testing.T) {
	schema := checkClean(t, map[string]string{
		pragueFixture:   readFixture(t, pragueFixture),
		consumerFixture: readFixture(t, consumerFixture),
	})
	view := schema.File(consumerFixture).Decl("SlimSportGame")
	if view.Key == nil || view.Key.Name != "id" {
		t.Errorf("view key is %v, want the member `id` that selects SportGame's key", view.Key)
	}
}

// A narrowing narrows the type and leaves the presence alone: an optional
// member stays optional, and `Odd? odd;` narrowed to `SlimOdd` is a `SlimOdd?`.
// Presence is a promise about whether the slot can be absent on the wire, and a
// view is not entitled to make that promise stronger than the source's.
func TestANarrowingKeepsTheMembersPresence(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
record Odd {
  int64 id = 1;
}
view SlimOdd of Odd {
  id;
}
table T {
  key int64 id = 1;
  Odd? odd = 2;
}
view V of T {
  SlimOdd odd;
}
`,
	})
	if got := typeString(schema.File("a.prague").Decl("V").Member("odd").Type); got != "view:SlimOdd?" {
		t.Errorf("narrowed type is %s, want view:SlimOdd?: the source member is optional and a narrowing does not make it required", got)
	}
}

// A narrowing reaches the element however deeply a collection holds it. What is
// narrowed is what is finally stored; the collections around it are structure,
// and one of them is as much structure as two. Stopping at the outermost
// argument would compare a `list` against the declaration the view is a view of
// and reject code the language allows.
func TestANarrowingReachesThroughNestedCollections(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
record Odd {
  int64 id = 1;
}
view SlimOdd of Odd {
  id;
}
table T {
  key int64 id = 1;
  list<list<Odd>> odds = 2;
}
view V of T {
  SlimOdd odds;
}
`,
	})
	if got := typeString(schema.File("a.prague").Decl("V").Member("odds").Type); got != "list<list<view:SlimOdd>>" {
		t.Errorf("narrowed type is %s, want list<list<view:SlimOdd>>", got)
	}
}

// Narrowing and renaming in one member, which section 5 writes as `SlimOdd
// odds as o;`. The three facts are asserted apart because they come from three
// different places -- the name from the view line's rename, the type from the
// narrowing, the tag from the source and from neither half -- and a test that
// compared one rendered member would pass on any two of the three.
//
// The parser's own test for this asserts the dump, so it holds the shape that
// survived parsing and says nothing about the model a backend reads. This is
// the model.
func TestAViewMemberMayNarrowAndRenameAtOnce(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
record Inner {
  int64 id = 1;
}
view SlimInner of Inner {
  id;
}
table Outer {
  key int64 id = 1;
  Inner odds = 7;
}
view SlimOuter of Outer {
  SlimInner odds as o;
}
`,
	})
	file := schema.File("a.prague")
	view := file.Decl("SlimOuter")

	// The name is the rename's, and it is the member's only name: a view that
	// still answered to `odds` would have renamed nothing, it would have
	// aliased.
	m := view.Member("o")
	if m == nil {
		t.Fatalf("the view has no member `o`:\n%s", dump(schema))
	}
	if view.Member("odds") != nil {
		t.Error("the view still knows the member by its source's name `odds`; a rename replaces the name rather than adding one")
	}

	// The type is the narrowing's. `Inner` is what the source lends and
	// `SlimInner` is what the leading type asks for.
	if got := typeString(m.Type); got != "view:SlimInner" {
		t.Errorf("`SlimInner odds as o;` is %s, want view:SlimInner -- the leading type is what the member holds", got)
	}

	// And the tag is the source's, untouched by either half. 7 is neither the
	// member's position in the view nor a tag of `SlimInner`, so a tag read off
	// the wrong thing does not land on it by luck.
	if m.Tag != 7 {
		t.Errorf("`SlimInner odds as o;` is tag %d, want Outer's 7: a view invents no tags, and narrowing and renaming are not tags", m.Tag)
	}
	if m.From != file.Decl("Outer").Member("odds") {
		t.Error("the member does not point at the source member `odds` it took its tag from")
	}
}

// Every diagnostic about a view member, with a rename on the member that
// earns it. Section 5 rules `SlimOdd odds as o;` in, and the risk a new
// spelling brings is not that it fails -- it is that the checks the old
// spellings answer to stop being asked when the new one is present. A rename
// is a branch above all five of these, and each one of them would go silent
// under a guard as small as `mw.view.As != nil` with the rest of the suite
// green.
//
// The fixture is one set of declarations with the view's body substituted, so
// that what differs between the cases is the member and nothing else.
func TestAViewMemberDiagnosticSurvivesARename(t *testing.T) {
	const decls = `revision 1;
namespace A;
record Inner {
  int64 id = 1;
}
view SlimInner of Inner {
  id;
}
record Other {
  int64 y = 1;
}
view SlimOther of Other {
  y;
}
record Plain {
  int64 z = 1;
}
table Outer {
  key int64 id = 1;
  Inner odds = 7;
  int64 other = 8;
  Plain p = 9;
}
view V of Outer {
`
	for _, tc := range []struct {
		name   string
		member string
		want   diag.Code
	}{
		// Two members under one name, one of them arriving at that name by
		// narrowing and renaming at once.
		{"a duplicate name", "  SlimInner odds as o;\n  other as o;\n", diag.DuplicateMember},
		// A narrowing to a name that is nothing at all.
		{"a narrowing to nothing", "  Nope odds as o;\n", diag.UnknownType},
		// The diagnostic the design exists to produce, on a renamed member:
		// the rename is what the consumer calls it, and calling it something
		// does not make the source declare it.
		{"a member the source does not declare", "  missing as o;\n", diag.NoSuchMember},
		// A narrowing to a declaration that is not a view.
		{"a narrowing to a record", "  Plain p as x;\n", diag.NarrowingNotAView},
		// A narrowing to a view of something else entirely, which is the
		// mistake the construct exists to catch.
		{"a narrowing to a view of another type", "  SlimOther odds as o;\n", diag.NarrowingSourceMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The case is only the case its name claims while the rename is
			// on the member. Strip it and every one of these becomes a
			// diagnostic the un-renamed spellings already answer for, tested
			// twice under two names -- and the guard this test exists to
			// catch goes back to surviving with the suite green.
			if !strings.Contains(tc.member, " as ") {
				t.Fatalf("the member %q carries no rename, so the case no longer asks what the test is named for", tc.member)
			}
			set, _, bag := checkSet(t, map[string]string{
				"a.prague": decls + tc.member + "}\n",
			})
			assertCodes(t, set, bag, tc.want)
		})
	}
}

// A narrowing that was rejected leaves the member's type alone. The two halves
// of PROTOP0314 are the message and the retyping that does not happen, and only
// the first of them is visible to anything that gates on HasErrors -- praguec
// does, so the driver's tests cannot see this and the CLI never would. A
// narrowing that reported the mismatch and then applied it anyway would hand
// the member a view of a declaration it has nothing to do with, which is the
// whole of what the diagnostic says must not happen.
func TestARejectedNarrowingLeavesTheMembersTypeAlone(t *testing.T) {
	set, schema, bag := checkSet(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
record Inner {
  int64 id = 1;
}
record Other {
  int64 y = 1;
}
view SlimOther of Other {
  y;
}
table Outer {
  key int64 id = 1;
  Inner odds = 7;
}
view V of Outer {
  SlimOther odds as o;
}
`,
	})
	assertCodes(t, set, bag, diag.NarrowingSourceMismatch)

	m := schema.File("a.prague").Decl("V").Member("o")
	if m == nil {
		t.Fatalf("the view has no member `o`:\n%s", dump(schema))
	}
	if got := typeString(m.Type); got != "record:Inner" {
		t.Errorf("the member is %s after a rejected narrowing, want the record:Inner its source lends it", got)
	}
}

// A view member that selects an alias keeps what the alias passes through to.
// An alias is nothing but that passthrough -- it has no wire slot and no value
// of its own -- so a view member that selects one and forgot its target is a
// property with nothing behind it, and following `From` into the source to find
// out is a hop every reader of the model would have to know to take.
func TestAViewMemberThatSelectsAnAliasKeepsWhatItPassesThroughTo(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
table T {
  key int64 id = 1;
  alias other = id;
}
view V of T {
  other;
}
`,
	})
	f := schema.File("a.prague")
	m := f.Decl("V").Member("other")
	if m.Kind != MemberAlias {
		t.Fatalf("the view member is a %s, want the alias kind it selects", m.Kind)
	}
	if m.Aliased != f.Decl("T").Member("id") {
		t.Error("the view's alias does not pass through to the source's `id`; an alias that forgot its target passes through to nothing")
	}
}

// Two view members may both hold tag 0, because an alias holds no wire slot and
// two of them collide over nothing. A tag is a position in the MessagePack
// array and 0 is not one, so a duplicate-tag check that counted zeros would
// report a collision on the one thing a view is free to select twice.
func TestTwoUntaggedViewMembersDoNotCollide(t *testing.T) {
	checkClean(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
table T {
  key int64 id = 1;
  alias a = id;
  alias b = id;
}
view V of T {
  a;
  b;
}
`,
	})
}

// An alias takes its target's type by value, so the order the two are written
// in must not matter. Section 1 writes the alias below its target and the
// corpus writes it above -- corpus/prague/payloads/BPBetTypeCacheItem.cs opens
// with `[IgnoreMember] public long BetTypeId` and declares `Id` twenty lines
// later -- and a single source-order resolution pass gives the second of those
// the zero Type, which reaches the backend as `public  BetTypeId` with no type
// at all. That compiles nowhere and is diagnosed nothing, which is the worst
// kind of bug for a compiler to have.
func TestAnAliasResolvesItsTypeWhereverItsTargetIsWritten(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"below", "  int64 id = 1;\n  alias other = id;\n"},
		{"above", "  alias other = id;\n  int64 id = 1;\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			schema := checkClean(t, map[string]string{
				"a.prague": "revision 1;\nnamespace A;\ntable T {\n" + tt.body + "}\n",
			})
			m := schema.File("a.prague").Decl("T").Member("other")
			if got := typeString(m.Type); got != "int64" {
				t.Errorf("the alias has type %q, want int64", got)
			}
		})
	}
}

// A composite key's parts are the members themselves rather than copies of
// them, so a part named before its own field is declared still sees the type
// once the field has one. That is a different mechanism from the alias above
// and is pinned separately, because "both orders work" is true of each for its
// own reason.
func TestACompositeKeyPartResolvesWhereverItsFieldIsWritten(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"below", "  int64 a = 2;\n  int64 b = 3;\n  key k = 1 (a, b);\n"},
		{"above", "  key k = 1 (a, b);\n  int64 a = 2;\n  int64 b = 3;\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			schema := checkClean(t, map[string]string{
				"a.prague": "revision 1;\nnamespace A;\ntable T {\n" + tt.body + "}\n",
			})
			m := schema.File("a.prague").Decl("T").Member("k")
			if len(m.Parts) != 2 {
				t.Fatalf("the key has %d parts, want 2", len(m.Parts))
			}
			for _, p := range m.Parts {
				if got := typeString(p.Type); got != "int64" {
					t.Errorf("key part %s has type %q, want int64", p.Name, got)
				}
			}
		})
	}
}

// --- declarations ------------------------------------------------------------

// An alias is a get and set passthrough and needs something to pass through to.
func TestAnAliasNamesAMemberOfItsOwnDeclaration(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\ntable T {\n  key int64 id = 1;\n  alias other = missing;\n}\n",
	})
	assertCodes(t, set, bag, diag.AliasTargetNotFound)
	assertAt(t, set, bag, 0, "a.prague:5:17", "missing")
}

// A composite key's parts name members of the same declaration. One of them may
// be an alias: section 1's own key is `(event_id, bet_type_id)` over a table
// where `bet_type_id` is the alias of `id`, so a check that demanded fields
// would reject the spec.
func TestACompositeKeysPartsNameMembersOfItsDeclaration(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\ntable T {\n  key key = 1 (a, missing);\n  int64 a = 2;\n}\n",
	})
	assertCodes(t, set, bag, diag.CompositeKeyPartNotFound)
	assertAt(t, set, bag, 0, "a.prague:4:19", "missing")

	schema := checkClean(t, map[string]string{
		pragueFixture: readFixture(t, pragueFixture),
	})
	key := schema.File(pragueFixture).Decl("BetType").Key
	if len(key.Parts) != 2 || key.Parts[1].Kind != MemberAlias {
		t.Errorf("BetType's key parts are %s, want its second part to be the alias", memberString(schema.File(pragueFixture).Decl("BetType"), key))
	}
}

// An alias cannot pass through to itself. Accepting one would put a member in
// the model pointing at the member it is, and every reader that follows an
// alias to the field behind it follows this one forever.
func TestAnAliasCannotNameItself(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\ntable T {\n  key int64 id = 1;\n  alias x = x;\n}\n",
	})
	assertCodes(t, set, bag, diag.AliasTargetNotFound)
	assertAt(t, set, bag, 0, "a.prague:5:13", "x")
}

// An alias takes its target's type only when the target is a field, because a
// field is the only member that has one to lend. An alias of an alias is legal
// and is left without a type: following the chain would be the checker deciding
// what a construct the spec says nothing about means, and taking the type
// anyway would copy whatever the target's zero value happened to be.
func TestAnAliasOfAnAliasTakesNoType(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
table T {
  key int64 id = 1;
  alias first = id;
  alias second = first;
}
`,
	})
	d := schema.File("a.prague").Decl("T")
	if got := typeString(d.Member("first").Type); got != "int64" {
		t.Errorf("the alias of a field is %s, want the int64 the field lends it", got)
	}
	if got := typeString(d.Member("second").Type); got != "?" {
		t.Errorf("the alias of an alias is %s, want it left unresolved: only a field lends a type", got)
	}
	if d.Member("second").Aliased != d.Member("first") {
		t.Error("the alias of an alias does not point at the alias it names")
	}
}

// A composite key's part cannot be the key itself. The parts are what the
// identity is made of and the key is what they make, so a key naming itself is
// an identity derived from its own derivation -- and cardinality, which
// compares a foreign key against the parts, would be reading off that cycle.
func TestACompositeKeyCannotNameItselfAsAPart(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\ntable T {\n  key key = 1 (key, b);\n  int64 b = 2;\n}\n",
	})
	assertCodes(t, set, bag, diag.CompositeKeyPartNotFound)
	assertAt(t, set, bag, 0, "a.prague:4:16", "key")
}

// A message that says where a declaration is says where the declaration begins,
// keyword and all, rather than where its name happens to sit. The position is
// printed inside the message text and not at the caret, so nothing about the
// diagnostic's own span moves when this one does.
func TestAMessageQuotesADeclarationAtItsKeyword(t *testing.T) {
	t.Run("a duplicate declaration", func(t *testing.T) {
		set, _, bag := checkSet(t, map[string]string{
			"a.prague": "revision 1;\nnamespace A;\nstruct S {\n  int64 x = 1;\n}\nrecord S {\n  int64 y = 1;\n}\n",
		})
		assertCodes(t, set, bag, diag.DuplicateDeclaration)
		assertMessageMentions(t, bag, 0, "a.prague:3:1")
	})
	t.Run("a view member naming nothing its source declares", func(t *testing.T) {
		set, _, bag := checkSet(t, map[string]string{
			"a.prague": "revision 1;\nnamespace A;\ntable T {\n  key int64 id = 1;\n}\nview V of T {\n  gone;\n}\n",
		})
		assertCodes(t, set, bag, diag.NoSuchMember)
		assertMessageMentions(t, bag, 0, "a.prague:3:1")
	})
}

// The messages that say more than their code number does, one row per code.
//
// #40's census split all 45 codes by the ticket's own rule: a message that only
// names a token is well covered by the code, and a message that carries an
// enumeration, a worked example, or a position to go and look at is not. These
// fourteen are this package's half of the load-bearing set, and replacing any
// of their messages with the literal "unexpected token" failed nothing here.
//
// The rows are their own fixtures rather than a reach into the tests above,
// because a test named for what the checker CONCLUDES should stay free to
// change its fixture, and a golden that moves with it pins nothing. What is
// asserted here is the sentence; what is asserted there is the conclusion.
//
// wantMessage is the whole sentence and wantMessageHas is the parts of it that
// must not drift; a row sets one or the other. The split follows the census:
// a sentence that is entirely teaching is pinned entire, and one built around
// an interpolated position or example is pinned at the interpolation, so that
// a fixture edit above does not turn into a golden edit here.
func TestTheCheckersMessagesSayWhatTheCodeCannot(t *testing.T) {
	tests := []struct {
		name           string
		files          map[string]string
		wantCode       diag.Code
		wantMessage    string
		wantMessageHas []string
	}{
		{
			name: "an import naming no file in the set",
			files: map[string]string{
				"a.prague": "revision 1;\nnamespace A;\nimport \"gone.prague\";\nstruct S {\n  int64 x = 1;\n}\n",
			},
			wantCode: diag.ImportNotFound,
			wantMessageHas: []string{
				"no file named \"gone.prague\" was given to the compiler",
				"where that file comes from is the build's question, not the schema's",
			},
		},
		{
			name: "two declarations of one name",
			files: map[string]string{
				"a.prague": "revision 1;\nnamespace A;\nstruct S {\n  int64 x = 1;\n}\nrecord S {\n  int64 y = 1;\n}\n",
			},
			wantCode: diag.DuplicateDeclaration,
			wantMessageHas: []string{
				"a.prague:3:1",
				"which of the two a reference means is not a question protop answers",
			},
		},
		{
			name: "a declaration under a built-in type name",
			files: map[string]string{
				"a.prague": "revision 1;\nnamespace A;\nrecord list {\n  int64 x = 1;\n}\n",
			},
			wantCode:    diag.BuiltinTypeDeclared,
			wantMessage: "`list` already names protop's built-in generic type; a type written `list` is read as the built-in, so nothing can ever spell its way to this declaration",
		},
		{
			name: "a view of something that is not there",
			files: map[string]string{
				"a.prague": "revision 1;\nnamespace A;\nview V of Gone {\n  x;\n}\n",
			},
			wantCode:    diag.ViewSourceNotFound,
			wantMessage: "unknown view source `Gone`; a file sees its own declarations and those of the files it imports, and nothing else",
		},
		{
			name: "a view of something with no tags to select",
			files: map[string]string{
				"a.prague": "revision 1;\nnamespace A;\nstruct S {\n  int64 x = 1;\n}\nview V of S {\n  x;\n}\n",
			},
			wantCode:    diag.ViewSourceNotSelectable,
			wantMessage: "`S` is a struct, and a view is a subset of a table or a record; there are no tags here for a view to be pinned to",
		},
		{
			name: "one source member selected twice",
			files: map[string]string{
				"a.prague": "revision 1;\nnamespace A;\ntable T {\n  key int64 id = 1;\n  int64 x = 2;\n}\nview V of T {\n  x as a;\n  x as b;\n}\n",
			},
			wantCode: diag.DuplicateTag,
			wantMessageHas: []string{
				"tag 2 is already used at a.prague:8:3",
				"`a` and `b` select the same member of `T`",
				"a tag is a position in the MessagePack array that two members cannot hold",
			},
		},
		{
			name: "a type naming nothing in scope",
			files: map[string]string{
				"a.prague": "revision 1;\nnamespace A;\nstruct S {\n  Gone x = 1;\n}\n",
			},
			wantCode:    diag.UnknownType,
			wantMessage: "unknown type `Gone`; a file sees its own declarations and those of the files it imports, and nothing else",
		},
		{
			name: "a view member naming nothing its source declares",
			files: map[string]string{
				"a.prague": "revision 1;\nnamespace A;\ntable T {\n  key int64 id = 1;\n}\nview V of T {\n  gone;\n}\n",
			},
			wantCode: diag.NoSuchMember,
			wantMessageHas: []string{
				"declared at a.prague:3:1",
				"a rename or a removal there is a compile error here rather than a field that decodes to nil",
			},
		},
		{
			name: "a narrowing to something that is not a view",
			files: map[string]string{
				"a.prague": "revision 1;\nnamespace A;\nrecord R {\n  int64 x = 1;\n}\ntable T {\n  key int64 id = 1;\n  R r = 2;\n}\nview V of T {\n  R r;\n}\n",
			},
			wantCode:    diag.NarrowingNotAView,
			wantMessage: "`R` is a record, and a view member narrows to another view; a member that keeps the type its source gives it is written as its name alone",
		},
		{
			name: "a narrowing to a view of something else",
			files: map[string]string{
				"a.prague": "revision 1;\nnamespace A;\nrecord R {\n  int64 x = 1;\n}\nrecord Q {\n  int64 y = 1;\n}\ntable T {\n  key int64 id = 1;\n  R r = 2;\n}\nview VQ of Q {\n  y;\n}\nview V of T {\n  VQ r;\n}\n",
			},
			wantCode:    diag.NarrowingSourceMismatch,
			wantMessage: "`VQ` is a view of `Q` and `r` holds `R`; a narrowing pins the member to a subset of what it already holds, and a view of anything else carries a different declaration's tags",
		},
		{
			name: "an alias with nothing to pass through to",
			files: map[string]string{
				"a.prague": "revision 1;\nnamespace A;\ntable T {\n  key int64 id = 1;\n  alias other = gone;\n}\n",
			},
			wantCode:    diag.AliasTargetNotFound,
			wantMessage: "`gone` names no other member of `T`; an alias is a get and set passthrough and needs something to pass through to",
		},
		{
			name: "a composite key over a member that is not there",
			files: map[string]string{
				"a.prague": "revision 1;\nnamespace A;\ntable T {\n  key key = 1 (gone);\n  int64 x = 2;\n}\n",
			},
			wantCode:    diag.CompositeKeyPartNotFound,
			wantMessage: "`gone` names no member of `T`; a composite key is a name over members of the declaration it belongs to",
		},
		{
			name: "a relation pointing at nothing in scope",
			files: map[string]string{
				"a.prague": "revision 1;\nnamespace A;\ntable T {\n  key int64 id = 1;\n  int64 other_id = 2 [references(Gone)];\n}\n",
			},
			wantCode:    diag.RelationTargetNotFound,
			wantMessage: "unknown relation target `Gone`; a file sees its own declarations and those of the files it imports, and nothing else",
		},
		{
			name: "a relation pointing at something with no identity",
			files: map[string]string{
				"a.prague": "revision 1;\nnamespace A;\nstruct S {\n  int64 x = 1;\n}\ntable T {\n  key int64 id = 1;\n  int64 s_id = 2 [references(S)];\n}\n",
			},
			wantCode:    diag.RelationTargetNotATable,
			wantMessage: "`S` is a struct and cannot be the target of a relation; a relation resolves against a key, and only a `table` has one",
		},
		{
			name: "a relation written in some other shape",
			files: map[string]string{
				"a.prague": "revision 1;\nnamespace A;\ntable T {\n  key int64 id = 1;\n  int64 x = 2 [references];\n}\n",
			},
			wantCode: diag.MalformedRelation,
			wantMessageHas: []string{
				"`references` names the table a member points at",
				"as in `[references(SportGame)]`",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set, _, bag := checkSet(t, tt.files)
			assertCodes(t, set, bag, tt.wantCode)
			msg := bag.All()[0].Message
			if tt.wantMessage != "" && len(tt.wantMessageHas) > 0 {
				t.Errorf("row pins the whole message AND parts of it; pick one")
			}
			if tt.wantMessage != "" && msg != tt.wantMessage {
				t.Errorf("%s reads\n\t%q\nwant\n\t%q", tt.wantCode, msg, tt.wantMessage)
			}
			for _, want := range tt.wantMessageHas {
				if !strings.Contains(msg, want) {
					t.Errorf("%s reads\n\t%q\nwant it to contain\n\t%q", tt.wantCode, msg, want)
				}
			}
		})
	}

	// The table holds every code on the list and the list holds every code in
	// the table. Without this the table is the span column before it had to
	// answer for itself: a code drops out of it in a refactor and nothing
	// anywhere says a message stopped being held.
	inTable := map[diag.Code]bool{}
	for _, tt := range tests {
		if inTable[tt.wantCode] {
			t.Errorf("%s has two rows; one code, one row, or the list below stops meaning anything", tt.wantCode)
		}
		inTable[tt.wantCode] = true
	}
	for _, c := range loadBearingMessages {
		if !inTable[c] {
			t.Errorf("%s carries a load-bearing message and has no row", c)
		}
	}
	for c := range inTable {
		if !slices.Contains(loadBearingMessages, c) {
			t.Errorf("%s has a row and is not on the list; say why it is load-bearing or take the row out", c)
		}
	}
}

// loadBearingMessages is the codes raised in this package whose message text is
// worth more than the code number -- #40's census, its checker half.
//
// PROTOP0107 is on it for the wording raised HERE, which is not the loader's.
// The loader says which include roots it searched; this one says the file was
// never given to the compiler at all, because the checker is handed a set and
// has no roots to name. The site is dead from the driver's point of view --
// load raises 0107 first and Check returns early on HasErrors -- and it is
// tested rather than deleted, so its sentence is held like any other.
//
// PROTOP0303 is on it for the same reason: the parser's wording is about two
// members of a declaration, and this one is about two view members selecting
// one source member, which is the only way a view can collide.
var loadBearingMessages = []diag.Code{
	diag.ImportNotFound,
	diag.DuplicateDeclaration,
	diag.BuiltinTypeDeclared,
	diag.ViewSourceNotFound,
	diag.ViewSourceNotSelectable,
	diag.DuplicateTag,
	diag.UnknownType,
	diag.NoSuchMember,
	diag.NarrowingNotAView,
	diag.NarrowingSourceMismatch,
	diag.AliasTargetNotFound,
	diag.CompositeKeyPartNotFound,
	diag.RelationTargetNotFound,
	diag.RelationTargetNotATable,
	diag.MalformedRelation,
}

// Every shape that can carry an annotation carries it into the model. praguec
// does not know what a backend's annotation means and is not entitled to an
// opinion, so dropping one is not the cautious choice: it is the generated code
// silently losing an attribute, which nobody notices until the backend that
// reads it ships.
func TestAnnotationsAreCarriedOnEveryShapeThatHoldsThem(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
struct S [cs: OnStruct] {
  int64 x = 1;
}
record R [cs: OnRecord] {
  int64 x = 1;
}
table T [cs: OnTable] {
  key key = 1 (part) [cs: OnKey];
  int64 part = 2 [cs: OnField];
  alias other = part [cs: OnAlias];
}
view V of T [cs: OnView] {
  part [cs: OnViewMember];
}
`,
	})
	f := schema.File("a.prague")

	decls := []struct{ decl, want string }{
		{"S", "OnStruct"},
		{"R", "OnRecord"},
		{"T", "OnTable"},
		{"V", "OnView"},
	}
	for _, tt := range decls {
		t.Run(tt.decl, func(t *testing.T) {
			assertAnnotation(t, f.Decl(tt.decl).Annotations, tt.want)
		})
	}

	members := []struct{ decl, member, want string }{
		{"T", "key", "OnKey"},
		{"T", "part", "OnField"},
		{"T", "other", "OnAlias"},
		{"V", "part", "OnViewMember"},
	}
	for _, tt := range members {
		t.Run(tt.decl+"."+tt.member, func(t *testing.T) {
			assertAnnotation(t, f.Decl(tt.decl).Member(tt.member).Annotations, tt.want)
		})
	}
}

// --- relations ---------------------------------------------------------------

// `references(T)` names a table. A record has no identity, so there is nothing
// for a relation to resolve against -- section 4's one line, and the reason
// `list<T>` needs no second keyword to tell composition from reference.
func TestARelationNamesATable(t *testing.T) {
	const decls = `record R {
  int64 x = 1;
}
table T {
  key int64 id = 1;
}
`
	// Each arm underlines what its own diagnostic is about, and the three do not
	// agree -- which is the reason all three are pinned by extent.
	//
	// PROTOP0317 and PROTOP0318 are about the NAME. `[references(R)]` is a
	// well-formed relation naming the wrong sort of thing, and the fix is to
	// write a different word between the parentheses, so the caret goes under
	// that word. PROTOP0404 is about the annotation: `[references]` names
	// nothing at all, there is no word to point at, and what is malformed is
	// the shape of the whole bracket. That one underlining the whole thing is
	// correct and is pinned here so it stays that way.
	t.Run("a record is not a relation target", func(t *testing.T) {
		set, _, bag := checkSet(t, map[string]string{
			"a.prague": "revision 1;\nnamespace A;\n" + decls + "table U {\n  key int64 id = 1;\n  int64 r = 2 [references(R)];\n}\n",
		})
		assertCodes(t, set, bag, diag.RelationTargetNotATable)
		assertAt(t, set, bag, 0, "a.prague:11:27", "R")
	})
	t.Run("a name that is nothing at all", func(t *testing.T) {
		set, _, bag := checkSet(t, map[string]string{
			"a.prague": "revision 1;\nnamespace A;\n" + decls + "table U {\n  key int64 id = 1;\n  int64 r = 2 [references(Missing)];\n}\n",
		})
		assertCodes(t, set, bag, diag.RelationTargetNotFound)
		assertAt(t, set, bag, 0, "a.prague:11:27", "Missing")
	})
	t.Run("a name written with room around it", func(t *testing.T) {
		// The span is computed by walking the annotation's text, so the caret
		// has to survive whitespace the writer is entitled to. Without this,
		// an offset measured from the `(` rather than from the name would pass
		// every other arm here.
		set, _, bag := checkSet(t, map[string]string{
			"a.prague": "revision 1;\nnamespace A;\n" + decls + "table U {\n  key int64 id = 1;\n  int64 r = 2 [ references ( Missing ) ];\n}\n",
		})
		assertCodes(t, set, bag, diag.RelationTargetNotFound)
		assertAt(t, set, bag, 0, "a.prague:11:30", "Missing")
	})
	t.Run("a relation with no target at all", func(t *testing.T) {
		set, _, bag := checkSet(t, map[string]string{
			"a.prague": "revision 1;\nnamespace A;\n" + decls + "table U {\n  key int64 id = 1;\n  int64 r = 2 [references];\n}\n",
		})
		assertCodes(t, set, bag, diag.MalformedRelation)
		assertAt(t, set, bag, 0, "a.prague:11:15", "[references]")
	})
}

// Section 6's table, and the three rows of it checked against the corpus types
// the section itself checks: BetType's FK is part of a composite, LiveObject's
// FK is the whole key, and SportGame.league_id is in no key at all.
func TestCardinalityIsDerivedFromTheKey(t *testing.T) {
	const src = `revision 1;
namespace A;
table SportGame {
  key int64 event_id = 1;
  int64 league_id = 4 [references(League)];
}
table League {
  key int64 league_id = 1;
}
table BetType {
  key key = 1 (event_id, bet_type_id);
  int64 event_id = 2 [references(SportGame)];
  int64 bet_type_id = 3;
}
table LiveObject {
  key int64 event_id = 1 [references(SportGame)];
}
record Embedded {
  int64 event_id = 1 [references(SportGame)];
}
`
	schema := checkClean(t, map[string]string{"a.prague": src})
	f := schema.File("a.prague")

	tests := []struct {
		decl, member string
		want         Cardinality
	}{
		{"BetType", "event_id", ManyToOne},    // part of a composite
		{"LiveObject", "event_id", OneToOne},  // the whole key
		{"SportGame", "league_id", ManyToOne}, // in no key
		{"Embedded", "event_id", ManyToOne},   // no key at all to be part of
	}
	for _, tt := range tests {
		t.Run(tt.decl+"."+tt.member, func(t *testing.T) {
			rels := f.Decl(tt.decl).Member(tt.member).Relations
			if len(rels) != 1 {
				t.Fatalf("%d relations, want 1", len(rels))
			}
			if rels[0].Cardinality != tt.want {
				t.Errorf("cardinality is %s, want %s", rels[0].Cardinality, tt.want)
			}
		})
	}
}

// A composite key of one part is the whole key spelled the long way, and the
// key enforces the one as much as a simple key would.
func TestACompositeKeyOfOnePartIsAWholeKey(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
table T {
  key int64 id = 1;
}
table U {
  key key = 1 (event_id);
  int64 event_id = 2 [references(T)];
}
`,
	})
	rels := schema.File("a.prague").Decl("U").Member("event_id").Relations
	if len(rels) != 1 || rels[0].Cardinality != OneToOne {
		t.Errorf("cardinality is %v, want one-to-one", rels)
	}
}

// A key part may be an alias, which is how section 1 writes its own: the part
// is `bet_type_id` and the field is `id`. Deriving cardinality against the
// alias rather than against what it passes through to would call a whole key
// something else, so the rule follows it.
func TestAKeyPartThatIsAnAliasIsTheMemberItPassesThroughTo(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
table T {
  key int64 id = 1;
}
table U {
  key key = 1 (event);
  int64 event_id = 2 [references(T)];
  alias event = event_id;
}
`,
	})
	rels := schema.File("a.prague").Decl("U").Member("event_id").Relations
	if len(rels) != 1 || rels[0].Cardinality != OneToOne {
		t.Errorf("cardinality is %v, want one-to-one: the one part of the key is an alias of this member", rels)
	}
}

// A view inherits the relations of the members it lists; section 6 says so, and
// says that is why no rule is needed forbidding a view from declaring its own.
// The relation is on the member the view selected, which is what `From` is for.
func TestAViewInheritsItsSourcesRelations(t *testing.T) {
	schema := checkClean(t, map[string]string{
		pragueFixture:   readFixture(t, pragueFixture),
		consumerFixture: readFixture(t, consumerFixture),
	})
	m := schema.File(consumerFixture).Decl("SlimBetType").Member("event_id")
	if len(m.Relations) != 0 {
		t.Errorf("the view declared %d relations of its own, want 0", len(m.Relations))
	}
	if len(m.From.Relations) != 1 || m.From.Relations[0].Target.Name != "SportGame" {
		t.Errorf("the member it selects carries %v, want one relation to SportGame", m.From.Relations)
	}
}

// The checker is handed a map, and a map has no order. A compiler whose output
// moved between runs could not be diffed in a build log, so the files come out
// sorted by the import string they are known by. Checked over and over because
// the thing being ruled out is Go's randomized map iteration: one run agreeing
// with sorted order proves nothing.
func TestFilesComeOutInAStableOrder(t *testing.T) {
	files := map[string]string{}
	for _, name := range []string{"e.prague", "c.prague", "a.prague", "d.prague", "b.prague"} {
		files[name] = "revision 1;\nnamespace N;\n"
	}
	set, _ := parseSet(t, files)
	for i := 0; i < 20; i++ {
		var bag diag.Bag
		var got []string
		for _, f := range Check(set, &bag).Files {
			got = append(got, f.Path)
		}
		if !slices.IsSorted(got) {
			t.Fatalf("files came out %v, want them sorted", got)
		}
	}
}

// A prefixed annotation is a backend's language and protop does not read it,
// whatever word it happens to open with. `references` is a word protop has
// spent, not a word it has taken away: a C# annotation that spells it means
// whatever the C# backend says it means, and a checker that resolved it would
// report a protop diagnostic about a sentence written in another language.
func TestAPrefixedAnnotationIsNotProtopsRelation(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
table T {
  key int64 id = 1;
  int64 other = 2 [cs: references(Nowhere)];
}
`,
	})
	if got := schema.File("a.prague").Decl("T").Member("other").Relations; len(got) != 0 {
		t.Errorf("the member carries %d relations, want none: a prefixed annotation is not protop's to interpret", len(got))
	}
}

// `references(T)` is one name inside a matched pair of parentheses, and
// anything else is malformed rather than nearly right. The annotation's
// contents are raw text with no span inside them, so a reading that salvaged
// part of a broken one would report an unknown name the user never typed,
// underlined at a bracket that looks fine.
func TestARelationsTargetIsOneNameInsideMatchedParentheses(t *testing.T) {
	const decls = `table SportGame {
  key int64 event_id = 1;
}
`
	// Both spellings are a name away from resolving, which is the point: the
	// first loses its last character to the missing `)` and the second keeps
	// only its first word, and either one would resolve to something.
	tests := []struct{ name, annotation string }{
		{"a closing parenthesis that is not there", "[references(SportGame]"},
		{"two names where one goes", "[references(SportGame Odd)]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			set, _, bag := checkSet(t, map[string]string{
				"a.prague": "revision 1;\nnamespace A;\n" + decls +
					"table U {\n  key int64 id = 1;\n  int64 game_id = 2 " + tt.annotation + ";\n}\n",
			})
			assertCodes(t, set, bag, diag.MalformedRelation)
			// The whole annotation, which is the one shape of it that is
			// right: nothing inside a malformed relation is a name praguec
			// read, so there is nothing narrower to point at that the user
			// would recognise.
			assertAt(t, set, bag, 0, "a.prague:8:21", tt.annotation)
		})
	}
}

// --- the contract with the parser --------------------------------------------

// The parser's contract, inherited: a tree is best-effort and bag.HasErrors is
// what says whether it means anything. A file that lost a brace has members
// that fell out of the declarations they were written in, so every name in it
// would look undeclared and the one mistake would be buried under the noise.
func TestCheckDoesNotRunOnATreeThatDidNotParse(t *testing.T) {
	f := source.NewFile("a.prague", "revision 1;\nnamespace A;\nstruct S {\n  Missing x = 1;\n")
	var bag diag.Bag
	set := FileSet{"a.prague": {Source: f, File: syntax.Parse(f, &bag)}}
	before := bag.Len()
	if before == 0 {
		t.Fatal("the fixture parses; it is supposed not to")
	}
	if schema := Check(set, &bag); schema != nil {
		t.Error("Check returned a model for a file that did not parse")
	}
	if bag.Len() != before {
		t.Errorf("Check added %d diagnostics to a bag that already had errors", bag.Len()-before)
	}
}

// `map` is PROTOP0900 at the parser and stays there. It is not the checker's to
// lift, and the contract above is what keeps a file holding one from ever
// reaching a stage that would have to decide what a map's key may be.
func TestAMapIsStillNotYetImplemented(t *testing.T) {
	f := source.NewFile("a.prague", "revision 1;\nnamespace A;\nstruct S {\n  map<string, int64> m = 1;\n}\n")
	var bag diag.Bag
	set := FileSet{"a.prague": {Source: f, File: syntax.Parse(f, &bag)}}
	if got := bag.All(); len(got) != 1 || got[0].Code != diag.NotYetImplemented {
		t.Fatalf("the parser said %v, want one PROTOP0900", got)
	}
	if Check(set, &bag) != nil {
		t.Error("Check returned a model for a file the parser has not finished reading")
	}
}

// A diagnostic raised across a set of files says which of them it is about. A
// span is a byte offset and nothing more, so without this every position in
// every file but one would be rendered against the wrong text.
func TestADiagnosticNamesTheFileItIsAbout(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\nstruct S {\n  int64 x = 1;\n}\n",
		"b.prague": "revision 1;\nnamespace B;\nstruct S {\n  Missing x = 1;\n}\n",
	})
	assertCodes(t, set, bag, diag.UnknownType)
	d := bag.All()[0]
	if d.File == nil || d.File.Name != "b.prague" {
		t.Fatalf("diagnostic names %v, want b.prague", d.File)
	}
	// Rendered against the wrong file, the caret would land in a.prague's text
	// at the same offset. The file the diagnostic names is what Render uses.
	if got := diag.Render(set["a.prague"].Source, d); !strings.Contains(got, "b.prague:4:3") {
		t.Errorf("rendered as:\n%s\nwant a position in b.prague", got)
	}
}

// Every diagnostic in a set names its own file, not the file the last one was
// raised against. One diagnostic cannot tell the difference -- it is both the
// first and the last -- and a run with one mistake in it is not the run this
// matters for. With two, a stamp that landed on the wrong diagnostic renders
// the second file's caret into the first file's text and leaves the second
// naming no file at all.
func TestEveryDiagnosticInASetNamesItsOwnFile(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": "revision 1;\nnamespace A;\nstruct S {\n  Missing x = 1;\n}\n",
		"b.prague": "revision 1;\nnamespace B;\nstruct S {\n  Absent y = 2;\n}\n",
	})
	assertCodes(t, set, bag, diag.UnknownType, diag.UnknownType)
	assertAt(t, set, bag, 0, "a.prague:4:3", "Missing")
	assertAt(t, set, bag, 1, "b.prague:4:3", "Absent")
}

// Diagnostics come out in pass order, then file order, then source order. The
// view's mistake is written below the relation's and is reported above it,
// because expanding the views is a pass that finishes before any relation is
// resolved. Source order is the obvious order and is the wrong one here, which
// is what makes this worth pinning: a compiler whose output moved when a pass
// was reordered has no order to diff against.
func TestDiagnosticsComeOutInPassOrderBeforeSourceOrder(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.prague": `revision 1;
namespace A;
table T {
  key int64 id = 1;
  int64 other = 2 [references(Nowhere)];
}
view V of T {
  gone;
}
`,
	})
	// Which diagnostic is which comes off the message rather than off the span,
	// so that this test says nothing about the file a diagnostic names and
	// fails for one reason only.
	assertCodes(t, set, bag, diag.NoSuchMember, diag.RelationTargetNotFound)
	assertMessageMentions(t, bag, 0, "`gone`")
	assertMessageMentions(t, bag, 1, "`Nowhere`")
}

// --- helpers -----------------------------------------------------------------

func assertDump(t *testing.T, s *Schema, want string) {
	t.Helper()
	if got := dump(s); got != want {
		t.Errorf("model:\n%s\nwant:\n%s", got, want)
	}
}

// assertAt holds a diagnostic's span against the position it should report and
// the text it should underline. Positions come from f.Position and spans from
// f.Slice: a hand-computed byte offset would be a second implementation of the
// thing under test.
func assertAt(t *testing.T, set FileSet, bag *diag.Bag, i int, wantPos, wantText string) {
	t.Helper()
	d := bag.All()[i]
	if d.File == nil {
		t.Fatalf("diagnostic %d names no file", i)
	}
	if got := d.File.Position(d.Span.Start).String(); got != wantPos {
		t.Errorf("diagnostic %d is at %s, want %s", i, got, wantPos)
	}
	if got := d.File.Slice(d.Span); got != wantText {
		t.Errorf("diagnostic %d underlines %q, want %q", i, got, wantText)
	}
}

// assertSpanStartsAt is assertAt with the extent left out: it holds where a
// diagnostic begins and says nothing about how far it runs.
//
// For a diagnostic whose span is a whole declaration, WHERE it points is the
// fact worth pinning -- #39 was a rule reported at a use site downstream of the
// declaration that was wrong -- and the extent is then three lines of which the
// render shows one, so a golden for it would pin more of the rendering rule
// than of the diagnostic. Use assertAt where the extent is the point, as
// PROTOP0204 now does: its underline is one word, and that is the fact.
func assertSpanStartsAt(t *testing.T, bag *diag.Bag, i int, want string) {
	t.Helper()
	d := bag.All()[i]
	if d.File == nil {
		t.Fatalf("diagnostic %d names no file", i)
	}
	if got := d.File.Position(d.Span.Start).String(); got != want {
		t.Errorf("diagnostic %d is at %s, want %s", i, got, want)
	}
}

// assertMessageMentions holds a diagnostic's message text against something it
// has to name. What a message says is not the diagnostic's own span, so assertAt
// cannot see it and a position printed inside one can move with nothing noticing.
func assertMessageMentions(t *testing.T, bag *diag.Bag, i int, want string) {
	t.Helper()
	if msg := bag.All()[i].Message; !strings.Contains(msg, want) {
		t.Errorf("diagnostic %d reads:\n%s\nwant it to name %s", i, msg, want)
	}
}

// assertAnnotation holds what a declaration or a member carries against the one
// annotation it was written with. The text is the whole of what a backend
// reads, so the assertion is that the text arrived and not merely that
// something did.
func assertAnnotation(t *testing.T, anns []*syntax.Annotation, want string) {
	t.Helper()
	if len(anns) != 1 {
		t.Fatalf("%d annotations carried, want the one that was written", len(anns))
	}
	if got := anns[0].Text; got != want {
		t.Errorf("annotation reads %q, want %q", got, want)
	}
}

// assertVerbatim holds a fixture against the spec it was lifted from, twice
// over: containment says it appears there, and equality with a whole fenced
// block says it is the block ENTIRE. Containment alone cannot see a truncation,
// because every prefix of a block is contained in the file too.
func assertVerbatim(t *testing.T, specPath, fixtureName string) {
	t.Helper()
	fixture := readFixture(t, fixtureName)
	spec, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(spec), fixture) {
		t.Errorf("testdata/%s no longer appears verbatim in %s", fixtureName, specPath)
		return
	}
	for _, block := range protopBlocks(t, specPath) {
		if block == fixture {
			return
		}
	}
	t.Errorf("testdata/%s appears in %s but is no whole ```protop block of it; "+
		"a fixture that is part of a block is a paraphrase by omission", fixtureName, specPath)
}

// protopBlocks returns the contents of every ```protop fenced block in a spec
// file, in order, each including its trailing newline.
func protopBlocks(t *testing.T, specPath string) []string {
	t.Helper()
	spec, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	var blocks []string
	var cur strings.Builder
	inBlock := false
	for _, line := range strings.SplitAfter(string(spec), "\n") {
		switch {
		case !inBlock && strings.TrimRight(line, "\n") == "```protop":
			inBlock = true
		case inBlock && strings.TrimRight(line, "\n") == "```":
			blocks = append(blocks, cur.String())
			cur.Reset()
			inBlock = false
		case inBlock:
			cur.WriteString(line)
		}
	}
	if inBlock {
		t.Fatalf("%s has an unclosed ```protop block", specPath)
	}
	if len(blocks) == 0 {
		t.Fatalf("%s has no ```protop blocks; the fixtures are lifted from nothing", specPath)
	}
	return blocks
}

// ImportPath strips the quotes and does nothing else, which is the whole of
// what it promises and the reason it is exported: the stage that builds a
// FileSet keys it with this and the stage that reads one looks names up with
// this, and they are two packages. One normalisation that is no normalisation,
// in one place -- two spellings of "the literal's contents" would put a file in
// the map under a name nothing can find it by.
//
// What is *not* here is deliberate. Nothing case-folds, so `import
// "B.prague"` opens `b.prague` on a case-insensitive filesystem and fails on
// Linux; nothing cleans, so `./b.prague` and `b.prague` are two keys. Both are
// language questions -- protoc rejects a non-canonical import string outright,
// buf cleans it silently -- and neither is answered here. This test pins what
// the function does so that answering them is a deliberate edit with a failing
// test under it.
func TestImportPathStripsTheQuotesAndNothingElse(t *testing.T) {
	for _, tc := range []struct{ lit, want string }{
		{`"b.prague"`, "b.prague"},
		{`"./b.prague"`, "./b.prague"},
		{`"sub/../b.prague"`, "sub/../b.prague"},
		{`"B.Protop"`, "B.Protop"},
		{`"Sub/B.prague"`, "Sub/B.prague"},
		{`"sub\b.prague"`, `sub\b.prague`},
		{`""`, ""},
	} {
		if got := ImportPath(syntax.StringLit{Text: tc.lit}); got != tc.want {
			t.Errorf("ImportPath(%s) = %q, want %q", tc.lit, got, tc.want)
		}
	}
}
