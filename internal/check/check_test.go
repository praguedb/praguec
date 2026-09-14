package check

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nanov/protobuf-to-messagepack-sharp/internal/diag"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/source"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/syntax"
)

// The two spec sections, under the names the program knows them by: the shared
// repository's file is what section 2's `import "prague.protop";` names, so the
// import string is the fixture's key and not an incidental file name.
//
// They are copies of the fixtures the frontend's suite parses, and they are
// copies rather than a reach into another package's testdata because testdata
// belongs to the package that uses it. Nothing holds them together except
// TestSpecFixturesAreVerbatim, which holds both of them against the spec
// itself -- which is the only thing either copy should ever be equal to.
const (
	pragueFixture   = "prague.protop"
	consumerFixture = "consumer.protop"
)

var (
	specSection1 = filepath.Join("..", "..", "docs", "spec", "protop", "shared-repository.md")
	specSection2 = filepath.Join("..", "..", "docs", "spec", "protop", "service-repository.md")
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
// Codes are protopc's interface and a sequence says more than a count: the
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
// reported without reaching it is one protopc prints and then accepts anyway.
//
// The expectation is derived and never written down: the compile fails exactly
// when one of the expected codes is an error by diag's own table. So this says
// nothing about what a severity ought to be -- the day a code becomes advisory
// the expectation moves with it and no fixture here needs an edit -- and
// everything about whether the severity a code has survives the trip from
// Raise to the gate. An empty expectation derives to false, which is the clean
// case and is why the clean helpers come through here too.
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

	assertDump(t, schema, `file consumer.protop
  namespace BrandDataReader.Services.Cache
  import prague.protop
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
file prague.protop
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
		"a.protop": "revision 1;\nnamespace A;\nimport \"missing.protop\";\n",
	})
	assertCodes(t, set, bag, diag.ImportNotFound)
	assertAt(t, set, bag, 0, "a.protop:3:1", `import "missing.protop";`)
}

// A file sees its own declarations and those of the files it imports, and
// nothing else. Two files in one set are not one scope: `b.protop` is in the
// set, is parsed, and its table is still not visible to a file that did not ask
// for it.
func TestAFileSeesOnlyItsOwnDeclarationsAndItsImports(t *testing.T) {
	const b = "revision 1;\nnamespace B;\ntable T {\n  key int64 id = 1;\n}\n"
	set, _, bag := checkSet(t, map[string]string{
		"a.protop": "revision 1;\nnamespace A;\nstruct S {\n  T t = 1;\n}\n",
		"b.protop": b,
	})
	assertCodes(t, set, bag, diag.UnknownType)

	// The same file, importing the one that declares T, resolves.
	checkClean(t, map[string]string{
		"a.protop": "revision 1;\nnamespace A;\nimport \"b.protop\";\nstruct S {\n  T t = 1;\n}\n",
		"b.protop": b,
	})
}

// Scope is not transitive: a file that wants what its import imports writes the
// import itself. What a file can see is on the file, rather than spread across
// the graph behind it.
func TestScopeIsNotTransitive(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.protop": "revision 1;\nnamespace A;\nimport \"b.protop\";\nstruct S {\n  T t = 1;\n}\n",
		"b.protop": "revision 1;\nnamespace B;\nimport \"c.protop\";\n",
		"c.protop": "revision 1;\nnamespace C;\ntable T {\n  key int64 id = 1;\n}\n",
	})
	assertCodes(t, set, bag, diag.UnknownType)
}

func TestDuplicateDeclarationsAreDiagnosed(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.protop": "revision 1;\nnamespace A;\nstruct S {\n  int64 x = 1;\n}\nrecord S {\n  int64 y = 1;\n}\n",
	})
	assertCodes(t, set, bag, diag.DuplicateDeclaration)
}

// A name declared in an imported file is not a duplicate of one declared here.
// Shadowing is what makes a shared repository usable: a service that happens to
// name a type the way the shared schema does is not thereby broken.
func TestADeclarationMayShadowAnImportedName(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.protop": "revision 1;\nnamespace A;\nimport \"b.protop\";\nstruct S {\n  int64 x = 1;\n}\nstruct Holder {\n  S s = 1;\n}\n",
		"b.protop": "revision 1;\nnamespace B;\nstruct S {\n  int64 y = 1;\n}\n",
	})
	own := schema.File("a.protop").Decl("S")
	if got := schema.File("a.protop").Decl("Holder").Member("s").Type.Decl; got != own {
		t.Errorf("`S` resolved to the declaration in %s, want the one in a.protop", got.File.Path)
	}
}

func TestDuplicateMembersAreDiagnosed(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.protop": "revision 1;\nnamespace A;\ntable T {\n  key int64 id = 1;\n  int64 id = 2;\n}\n",
	})
	assertCodes(t, set, bag, diag.DuplicateMember)
}

// A member's name and a member's tag are two facts with two fixes, so an alias
// -- which has no tag at all -- still collides by name.
func TestAnAliasCollidesByNameWithAField(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.protop": "revision 1;\nnamespace A;\ntable T {\n  key int64 id = 1;\n  alias id = id;\n}\n",
	})
	assertCodes(t, set, bag, diag.DuplicateMember)
}

func TestAnUnknownTypeIsDiagnosed(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.protop": "revision 1;\nnamespace A;\nstruct S {\n  Missing x = 1;\n}\n",
	})
	assertCodes(t, set, bag, diag.UnknownType)
	assertAt(t, set, bag, 0, "a.protop:4:3", "Missing")
}

// A collection resolves its arguments, and nesting costs nothing: a list of a
// list is the same resolution twice.
func TestACollectionResolvesItsArguments(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.protop": "revision 1;\nnamespace A;\nrecord R {\n  int64 x = 1;\n}\nstruct S {\n  list<list<R>>? xs = 1;\n}\n",
	})
	m := schema.File("a.protop").Decl("S").Member("xs")
	if got := typeString(m.Type); got != "list<list<record:R>>?" {
		t.Errorf("type is %s, want list<list<record:R>>?", got)
	}
}

// `decimal(N)` carries its precision into the model. A precision on anything
// else is PROTOP0301 at the parser and never gets here.
func TestDecimalCarriesItsPrecision(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.protop": "revision 1;\nnamespace A;\nstruct S {\n  decimal(4)? x = 1;\n}\n",
	})
	if got := schema.File("a.protop").Decl("S").Member("x").Type.Precision; got != 4 {
		t.Errorf("precision is %d, want 4", got)
	}
}

// The caret underlines the name that is unknown and stops there. `Missing?` is
// a type written in two pieces and only one of them is the mistake: a caret
// that swallowed the `?` would tell the reader the presence marker is part of
// what protopc could not find, and the caret's width is the whole reason spans
// are carried instead of positions.
func TestAnUnknownTypeUnderlinesTheNameAndNotTheWholeTypeReference(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.protop": "revision 1;\nnamespace A;\nstruct S {\n  Missing? x = 1;\n}\n",
	})
	assertCodes(t, set, bag, diag.UnknownType)
	assertAt(t, set, bag, 0, "a.protop:4:3", "Missing")
}

// What a file declares and what a file can see are two questions, and `Decl`
// answers the first one. They differ for exactly the names an import brings,
// which is why an accessor that quietly answered the second would be right on
// every file that imports nothing -- and on none of the ones this language
// exists for.
func TestAFilesDeclarationsAreItsOwnAndNotItsImports(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.protop": "revision 1;\nnamespace A;\nimport \"b.protop\";\nstruct S {\n  T t = 1;\n}\n",
		"b.protop": "revision 1;\nnamespace B;\ntable T {\n  key int64 id = 1;\n}\n",
	})
	a, b := schema.File("a.protop"), schema.File("b.protop")
	if a.Decl("T") != nil {
		t.Error("a.protop declares `T`; it imports the file that declares it, and an import is not a declaration")
	}
	// The name is in scope all the same, and resolved to the declaration the
	// imported file owns. Scope is the wider answer, asked where a name is
	// resolved rather than here.
	if got := a.Decl("S").Member("t").Type.Decl; got != b.Decl("T") {
		t.Error("`T` did not resolve to the declaration b.protop owns")
	}
}

// An import that resolved to nothing leaves nothing behind. `Imports` is the
// list of files this one can see, and an entry for a file that was never found
// is a file every later reader of the model would walk into.
func TestAnImportThatNamesNoFileAddsNoImport(t *testing.T) {
	_, schema, bag := checkSet(t, map[string]string{
		"a.protop": "revision 1;\nnamespace A;\nimport \"missing.protop\";\n",
	})
	if !bag.HasErrors() {
		t.Fatal("the import resolved; it is supposed to name no file in the set")
	}
	if got := schema.File("a.protop").Imports; len(got) != 0 {
		t.Errorf("the file lists %d imports, want none: the only one it writes was not found", len(got))
	}
}

// --- views -------------------------------------------------------------------

func TestAViewSourceMustResolve(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.protop": "revision 1;\nnamespace A;\nview V of Missing {\n  x;\n}\n",
	})
	assertCodes(t, set, bag, diag.ViewSourceNotFound)
	assertAt(t, set, bag, 0, "a.protop:3:11", "Missing")
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
				"a.protop": "revision 1;\nnamespace A;\n" + decls + "view V of " + src + " {\n  x;\n}\n",
			})
			assertCodes(t, set, bag, diag.ViewSourceNotSelectable)
		})
	}
}

// The diagnostic the design exists to produce. The shared repo renames a field;
// every view still selecting the old name fails to compile, in the repo that
// drifted, instead of decoding to nil at runtime.
func TestAViewMemberMustNameAMemberOfItsSource(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"consumer.protop": "revision 1;\nnamespace A;\nimport \"prague.protop\";\nview V of SportGame {\n  odds;\n}\n",
		pragueFixture:     readFixture(t, pragueFixture),
	})
	assertCodes(t, set, bag, diag.NoSuchMember)
	assertAt(t, set, bag, 0, "consumer.protop:5:3", "odds")
	if msg := bag.All()[0].Message; !strings.Contains(msg, "prague.protop:") {
		t.Errorf("message does not say where `SportGame` is declared: %s", msg)
	}
}

// The severity of a checker diagnostic, end to end. A code sequence says what
// was noticed; this says what it costs, which is a different claim. bag.HasErrors
// is what cmd/protopc turns into an exit code and what every stage after the
// checker inherits, so a checker diagnostic that does not fail the compile is a
// schema protopc reports on and then generates C# from anyway.
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
	} {
		t.Run(tt.name, func(t *testing.T) {
			set, _, bag := checkSet(t, map[string]string{"a.protop": tt.src})
			if bag.Len() != 1 || bag.All()[0].Code != tt.want {
				t.Fatalf("want one %s:\n%s", tt.want, renderAll(set, bag))
			}
			if got := bag.All()[0].Severity; got != diag.Error {
				t.Errorf("%s raised as a %s, want an Error", tt.want, got)
			}
			if !bag.HasErrors() {
				t.Errorf("%s does not fail the compile; protopc would accept this schema", tt.want)
			}
		})
	}
}

// Tag, type and presence come from the source; the view line carries the name
// and an optional rename, and nothing else. `?` included: presence is not
// something a view gets to change its mind about.
func TestAViewMemberTakesTagTypeAndPresenceFromItsSource(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.protop": `revision 1;
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
	m := schema.File("a.protop").Decl("V").Member("d")
	if m.Tag != 9 || typeString(m.Type) != "decimal(4)?" {
		t.Errorf("view member is %s, want tag 9 and decimal(4)?", memberString(schema.File("a.protop").Decl("V"), m))
	}
	if m.From != schema.File("a.protop").Decl("T").Member("d") {
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
			"a.protop": "revision 1;\nnamespace A;\n" + decls + "view V of T {\n  R r;\n}\n",
		})
		assertCodes(t, set, bag, diag.NarrowingNotAView)
	})
	t.Run("a name that is nothing at all", func(t *testing.T) {
		set, _, bag := checkSet(t, map[string]string{
			"a.protop": "revision 1;\nnamespace A;\n" + decls + "view V of T {\n  SlimR r;\n}\n",
		})
		assertCodes(t, set, bag, diag.UnknownType)
	})
}

// The check the construct exists for: a narrowing that points elsewhere would
// hand the member the tags of a declaration it has nothing to do with.
func TestANarrowingMustBeAViewOfTheMembersType(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.protop": `revision 1;
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
	assertAt(t, set, bag, 0, "a.protop:17:3", "SlimOther")
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
		"a.protop": `revision 1;
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
		"a.protop": `revision 1;
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
	if got := typeString(schema.File("a.protop").Decl("V").Member("odd").Type); got != "view:SlimOdd?" {
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
		"a.protop": `revision 1;
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
	if got := typeString(schema.File("a.protop").Decl("V").Member("odds").Type); got != "list<list<view:SlimOdd>>" {
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
		"a.protop": `revision 1;
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
	file := schema.File("a.protop")
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
				"a.protop": decls + tc.member + "}\n",
			})
			assertCodes(t, set, bag, tc.want)
		})
	}
}

// A narrowing that was rejected leaves the member's type alone. The two halves
// of PROTOP0314 are the message and the retyping that does not happen, and only
// the first of them is visible to anything that gates on HasErrors -- protopc
// does, so the driver's tests cannot see this and the CLI never would. A
// narrowing that reported the mismatch and then applied it anyway would hand
// the member a view of a declaration it has nothing to do with, which is the
// whole of what the diagnostic says must not happen.
func TestARejectedNarrowingLeavesTheMembersTypeAlone(t *testing.T) {
	set, schema, bag := checkSet(t, map[string]string{
		"a.protop": `revision 1;
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

	m := schema.File("a.protop").Decl("V").Member("o")
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
		"a.protop": `revision 1;
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
	f := schema.File("a.protop")
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
		"a.protop": `revision 1;
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
				"a.protop": "revision 1;\nnamespace A;\ntable T {\n" + tt.body + "}\n",
			})
			m := schema.File("a.protop").Decl("T").Member("other")
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
				"a.protop": "revision 1;\nnamespace A;\ntable T {\n" + tt.body + "}\n",
			})
			m := schema.File("a.protop").Decl("T").Member("k")
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
		"a.protop": "revision 1;\nnamespace A;\ntable T {\n  key int64 id = 1;\n  alias other = missing;\n}\n",
	})
	assertCodes(t, set, bag, diag.AliasTargetNotFound)
	assertAt(t, set, bag, 0, "a.protop:5:17", "missing")
}

// A composite key's parts name members of the same declaration. One of them may
// be an alias: section 1's own key is `(event_id, bet_type_id)` over a table
// where `bet_type_id` is the alias of `id`, so a check that demanded fields
// would reject the spec.
func TestACompositeKeysPartsNameMembersOfItsDeclaration(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.protop": "revision 1;\nnamespace A;\ntable T {\n  key key = 1 (a, missing);\n  int64 a = 2;\n}\n",
	})
	assertCodes(t, set, bag, diag.CompositeKeyPartNotFound)
	assertAt(t, set, bag, 0, "a.protop:4:19", "missing")

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
		"a.protop": "revision 1;\nnamespace A;\ntable T {\n  key int64 id = 1;\n  alias x = x;\n}\n",
	})
	assertCodes(t, set, bag, diag.AliasTargetNotFound)
	assertAt(t, set, bag, 0, "a.protop:5:13", "x")
}

// An alias takes its target's type only when the target is a field, because a
// field is the only member that has one to lend. An alias of an alias is legal
// and is left without a type: following the chain would be the checker deciding
// what a construct the spec says nothing about means, and taking the type
// anyway would copy whatever the target's zero value happened to be.
func TestAnAliasOfAnAliasTakesNoType(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.protop": `revision 1;
namespace A;
table T {
  key int64 id = 1;
  alias first = id;
  alias second = first;
}
`,
	})
	d := schema.File("a.protop").Decl("T")
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
		"a.protop": "revision 1;\nnamespace A;\ntable T {\n  key key = 1 (key, b);\n  int64 b = 2;\n}\n",
	})
	assertCodes(t, set, bag, diag.CompositeKeyPartNotFound)
	assertAt(t, set, bag, 0, "a.protop:4:16", "key")
}

// A message that says where a declaration is says where the declaration begins,
// keyword and all, rather than where its name happens to sit. The position is
// printed inside the message text and not at the caret, so nothing about the
// diagnostic's own span moves when this one does.
func TestAMessageQuotesADeclarationAtItsKeyword(t *testing.T) {
	t.Run("a duplicate declaration", func(t *testing.T) {
		set, _, bag := checkSet(t, map[string]string{
			"a.protop": "revision 1;\nnamespace A;\nstruct S {\n  int64 x = 1;\n}\nrecord S {\n  int64 y = 1;\n}\n",
		})
		assertCodes(t, set, bag, diag.DuplicateDeclaration)
		assertMessageMentions(t, bag, 0, "a.protop:3:1")
	})
	t.Run("a view member naming nothing its source declares", func(t *testing.T) {
		set, _, bag := checkSet(t, map[string]string{
			"a.protop": "revision 1;\nnamespace A;\ntable T {\n  key int64 id = 1;\n}\nview V of T {\n  gone;\n}\n",
		})
		assertCodes(t, set, bag, diag.NoSuchMember)
		assertMessageMentions(t, bag, 0, "a.protop:3:1")
	})
}

// Every shape that can carry an annotation carries it into the model. protopc
// does not know what a backend's annotation means and is not entitled to an
// opinion, so dropping one is not the cautious choice: it is the generated code
// silently losing an attribute, which nobody notices until the backend that
// reads it ships.
func TestAnnotationsAreCarriedOnEveryShapeThatHoldsThem(t *testing.T) {
	schema := checkClean(t, map[string]string{
		"a.protop": `revision 1;
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
	f := schema.File("a.protop")

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
	t.Run("a record is not a relation target", func(t *testing.T) {
		set, _, bag := checkSet(t, map[string]string{
			"a.protop": "revision 1;\nnamespace A;\n" + decls + "table U {\n  key int64 id = 1;\n  int64 r = 2 [references(R)];\n}\n",
		})
		assertCodes(t, set, bag, diag.RelationTargetNotATable)
	})
	t.Run("a name that is nothing at all", func(t *testing.T) {
		set, _, bag := checkSet(t, map[string]string{
			"a.protop": "revision 1;\nnamespace A;\n" + decls + "table U {\n  key int64 id = 1;\n  int64 r = 2 [references(Missing)];\n}\n",
		})
		assertCodes(t, set, bag, diag.RelationTargetNotFound)
	})
	t.Run("a relation with no target at all", func(t *testing.T) {
		set, _, bag := checkSet(t, map[string]string{
			"a.protop": "revision 1;\nnamespace A;\n" + decls + "table U {\n  key int64 id = 1;\n  int64 r = 2 [references];\n}\n",
		})
		assertCodes(t, set, bag, diag.MalformedRelation)
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
	schema := checkClean(t, map[string]string{"a.protop": src})
	f := schema.File("a.protop")

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
		"a.protop": `revision 1;
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
	rels := schema.File("a.protop").Decl("U").Member("event_id").Relations
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
		"a.protop": `revision 1;
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
	rels := schema.File("a.protop").Decl("U").Member("event_id").Relations
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
	for _, name := range []string{"e.protop", "c.protop", "a.protop", "d.protop", "b.protop"} {
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
		"a.protop": `revision 1;
namespace A;
table T {
  key int64 id = 1;
  int64 other = 2 [cs: references(Nowhere)];
}
`,
	})
	if got := schema.File("a.protop").Decl("T").Member("other").Relations; len(got) != 0 {
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
				"a.protop": "revision 1;\nnamespace A;\n" + decls +
					"table U {\n  key int64 id = 1;\n  int64 game_id = 2 " + tt.annotation + ";\n}\n",
			})
			assertCodes(t, set, bag, diag.MalformedRelation)
		})
	}
}

// --- the contract with the parser --------------------------------------------

// The parser's contract, inherited: a tree is best-effort and bag.HasErrors is
// what says whether it means anything. A file that lost a brace has members
// that fell out of the declarations they were written in, so every name in it
// would look undeclared and the one mistake would be buried under the noise.
func TestCheckDoesNotRunOnATreeThatDidNotParse(t *testing.T) {
	f := source.NewFile("a.protop", "revision 1;\nnamespace A;\nstruct S {\n  Missing x = 1;\n")
	var bag diag.Bag
	set := FileSet{"a.protop": {Source: f, File: syntax.Parse(f, &bag)}}
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
	f := source.NewFile("a.protop", "revision 1;\nnamespace A;\nstruct S {\n  map<string, int64> m = 1;\n}\n")
	var bag diag.Bag
	set := FileSet{"a.protop": {Source: f, File: syntax.Parse(f, &bag)}}
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
		"a.protop": "revision 1;\nnamespace A;\nstruct S {\n  int64 x = 1;\n}\n",
		"b.protop": "revision 1;\nnamespace B;\nstruct S {\n  Missing x = 1;\n}\n",
	})
	assertCodes(t, set, bag, diag.UnknownType)
	d := bag.All()[0]
	if d.File == nil || d.File.Name != "b.protop" {
		t.Fatalf("diagnostic names %v, want b.protop", d.File)
	}
	// Rendered against the wrong file, the caret would land in a.protop's text
	// at the same offset. The file the diagnostic names is what Render uses.
	if got := diag.Render(set["a.protop"].Source, d); !strings.Contains(got, "b.protop:4:3") {
		t.Errorf("rendered as:\n%s\nwant a position in b.protop", got)
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
		"a.protop": "revision 1;\nnamespace A;\nstruct S {\n  Missing x = 1;\n}\n",
		"b.protop": "revision 1;\nnamespace B;\nstruct S {\n  Absent y = 2;\n}\n",
	})
	assertCodes(t, set, bag, diag.UnknownType, diag.UnknownType)
	assertAt(t, set, bag, 0, "a.protop:4:3", "Missing")
	assertAt(t, set, bag, 1, "b.protop:4:3", "Absent")
}

// Diagnostics come out in pass order, then file order, then source order. The
// view's mistake is written below the relation's and is reported above it,
// because expanding the views is a pass that finishes before any relation is
// resolved. Source order is the obvious order and is the wrong one here, which
// is what makes this worth pinning: a compiler whose output moved when a pass
// was reordered has no order to diff against.
func TestDiagnosticsComeOutInPassOrderBeforeSourceOrder(t *testing.T) {
	set, _, bag := checkSet(t, map[string]string{
		"a.protop": `revision 1;
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
// "B.protop"` opens `b.protop` on a case-insensitive filesystem and fails on
// Linux; nothing cleans, so `./b.protop` and `b.protop` are two keys. Both are
// language questions -- protoc rejects a non-canonical import string outright,
// buf cleans it silently -- and neither is answered here. This test pins what
// the function does so that answering them is a deliberate edit with a failing
// test under it.
func TestImportPathStripsTheQuotesAndNothingElse(t *testing.T) {
	for _, tc := range []struct{ lit, want string }{
		{`"b.protop"`, "b.protop"},
		{`"./b.protop"`, "./b.protop"},
		{`"sub/../b.protop"`, "sub/../b.protop"},
		{`"B.Protop"`, "B.Protop"},
		{`"Sub/B.protop"`, "Sub/B.protop"},
		{`"sub\b.protop"`, `sub\b.protop`},
		{`""`, ""},
	} {
		if got := ImportPath(syntax.StringLit{Text: tc.lit}); got != tc.want {
			t.Errorf("ImportPath(%s) = %q, want %q", tc.lit, got, tc.want)
		}
	}
}
