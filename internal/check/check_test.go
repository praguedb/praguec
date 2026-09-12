package check

import (
	"fmt"
	"os"
	"path/filepath"
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
