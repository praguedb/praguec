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

// The two fenced blocks of section 2, lifted the same way: the consuming
// service's whole file, and the table such a service may declare of its own.
const (
	specSection2Fixture      = "spec_section2.protop"
	specSection2TableFixture = "spec_section2_table.protop"
)

// specSection1 and specSection2 are the spec files the fixtures are lifted from.
var (
	specSection1 = filepath.Join("..", "..", "docs", "spec", "protop", "shared-repository.md")
	specSection2 = filepath.Join("..", "..", "docs", "spec", "protop", "service-repository.md")
	// Section 5 lends this file no fixture. It lends it a rule, in prose, and
	// a rule is pinned the same way a fixture is -- see assertSpecStates.
	specSection5 = filepath.Join("..", "..", "docs", "spec", "protop", "views.md")
)

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
	assertGate(t, f, bag, nil)
	return f, file
}

// assertGate holds the bag against what the codes it carries imply about the
// compile. bag.HasErrors is the gate the driver reads and every later stage
// inherits, so a diagnostic that is reported without reaching it is one protopc
// prints and then accepts anyway.
//
// The expectation is derived and never written down: the compile fails exactly
// when one of the expected codes is an error by diag's own table. So this says
// nothing about what a severity ought to be -- the day a code becomes advisory
// the expectation moves with it and no fixture here needs an edit -- and
// everything about whether the severity a code has survives the trip from Raise
// to the gate. An empty expectation derives to false, which is the clean case
// and is why the clean helpers come through here too.
//
// What it does not catch is a demotion made in diag's table and its oracle
// together, because then the derivation agrees with the demotion. That is what
// TestAParserDiagnosticFailsTheCompile is for, and why the two are not the same
// test written twice.
//
// What it uniquely catches, on the record, because deriving its expectation
// from diag's table makes it look like it holds nothing at all:
//
// Bag.All returns the live backing slice and Diagnostic.Severity is an
// exported field, so any stage can demote a diagnostic AFTER raising it --
// diag's own tests cannot see that, because the stage doing it is not diag.
// Measured: appending `p.bag.All()[p.bag.Len()-1].Severity = diag.Warning` to
// the TagIsReserved site in parser.go leaves every other test in the repo
// green, and deleting the three HasErrors comparisons in these helpers makes
// it green everywhere. This comparison is the only thing in the repo that
// fails.
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
func assertGate(t *testing.T, f *source.File, bag *diag.Bag, want []diag.Code) {
	t.Helper()
	fails := false
	for _, c := range want {
		if c.Severity() == diag.Error {
			fails = true
		}
	}
	if got := bag.HasErrors(); got != fails {
		t.Errorf("HasErrors = %v, want %v for %v:\n%s", got, fails, want, diag.RenderAll(f, bag))
	}
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
	for _, im := range file.Imports {
		fmt.Fprintf(&b, "import %s\n", im.Path.Text)
	}
	for _, d := range file.Decls {
		switch d := d.(type) {
		case *Struct:
			dumpDecl(&b, "struct", d.Name, d.Annotations, d.Members)
		case *Record:
			dumpDecl(&b, "record", d.Name, d.Annotations, d.Members)
		case *Table:
			dumpDecl(&b, "table", d.Name, d.Annotations, d.Members)
		case *View:
			fmt.Fprintf(&b, "view %s of %s\n", d.Name.Text, d.Source.Text)
			for _, a := range d.Annotations {
				fmt.Fprintf(&b, "  %s\n", annString(a))
			}
			for _, m := range d.Members {
				fmt.Fprintf(&b, "  %s\n", viewMemberString(m))
			}
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
			// `key` is the one word written in front of the type. Repetition is
			// not: it is part of the type, and typeString writes it there.
			lead := ""
			if m.Key {
				lead = "key "
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

// viewMemberString writes a view member back the way the source spelled it: the
// narrowing in front of the name, the rename after it. A dump that wrote the
// three spellings the same way would not be able to tell `SlimOdd odds` from a
// selection of a member called `SlimOdd` renamed to `odds`.
func viewMemberString(m *ViewMember) string {
	s := m.Name.Text
	if m.Narrow != nil {
		s = m.Narrow.Text + " " + s
	}
	if m.As != nil {
		s += " as " + m.As.Text
	}
	return s + annSuffix(m.Annotations)
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

// typeString writes a TypeRef back the way the source spelled it, arguments
// included -- which is what makes `list<Odd>?` and `list<Odd?>` two different
// lines of a dump rather than one.
func typeString(t TypeRef) string {
	s := t.Name.Text
	if len(t.Args) > 0 {
		args := make([]string, len(t.Args))
		for i, a := range t.Args {
			args[i] = typeString(a)
		}
		s += "<" + strings.Join(args, ", ") + ">"
	}
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

// The milestone's headline claim, and the reason the fixture is now section 1's
// fenced block entire rather than the half of it M1 could read: every line of
// the spec's shared repository parses, and the parser has nothing to say about
// any of it.
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
record StaticDataRef
  int64 id = 1
  string name = 2
record Odd
  string name = 1
  double odds_actual = 9
  int64 timestamp = 5
table SportGame
  [cs: DataCache(CacheClassName = nameof(BPSportGameCache))]
  [cs: DataCacheTopic(TopicNames.BrandFromConfigs.SportGame)]
  [cs: DataCacheContract(typeof(SportGameCacheContract), Role = ContractRole.Producer)]
  reserved 8..14, 16..23, 25..27, 29..34, 37..46, 48..54
  key int64 event_id = 1
  int64 sport_type_id = 2
  int64 country_id = 3
  int64 league_id = 4
  int64? home_team_id = 5
  int64? away_team_id = 6
  Venue? venue = 7
  int32? fixture_id = 15
  EventStatus event_status = 24
  int64 event_date = 28
  int64 provider_timestamp = 35
  decimal(4)? live_limit_manual_single = 36
  list<StaticDataRef> runners = 47
  StaticDataRef sport_type = 55
  StaticDataRef country = 56
  StaticDataRef league = 57
  StaticDataRef? home_team = 58
  StaticDataRef? away_team = 59
table BetType
  [cs: DataCache(CacheClassName = nameof(BPBetTypeCache))]
  [cs: DataCacheTopic(TopicNames.BrandFromConfigs.BetType)]
  [cs: DataCacheContract(typeof(BetTypeCacheContract), Role = ContractRole.Producer)]
  reserved 5, 6, 10..21, 23, 24
  key key = 1 (event_id, bet_type_id)
  int64 event_id = 2 [references(SportGame)]
  int64 id = 3
  alias bet_type_id = id
  string name = 4
  BetTypeStatus status = 7
  BetTypeStatus? status_manual = 8
  BetTypeStatus? status_watch_dog = 9
  decimal(4)? limit_manual_single = 22
  int64 timestamp = 25 [cs: DataCacheFromTimestamp] [cs: DataCacheIgnoreEquality]
  list<Odd> odds = 26
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

// assertVerbatim holds a fixture against the spec it was lifted from, twice
// over. Containment is the claim that the fixture appears in that file and
// nowhere else has to be consulted to read it; equality with a whole fenced
// block is the claim that it is the block ENTIRE. Containment alone cannot see
// a truncation -- every prefix of a block is contained in the file too, so a
// fixture that lost its last declarations would go on passing while the test
// named after wholeness watched it happen.
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

// assertSpecStates is assertVerbatim for prose. A fenced block is a fixture
// and can be held byte for byte; a paragraph is written to be read, and a pin
// that failed on a rewrap would teach the next reader to delete it. Runs of
// whitespace are collapsed on both sides and nothing else is -- not the words,
// not the emphasis, not the backticks and not the numbers -- so a reflow
// passes and an inversion does not.
//
// A test whose comment cites a rule and whose behaviour rests on it is resting
// on an authority nothing checks. The spec can be edited to say the opposite
// with every package green, and the citation then reads as license for the
// behaviour the spec now forbids.
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

// What keeps "parses verbatim out of the spec" honest: the fixture is not a
// paraphrase of section 1, it is a copy of it. If the spec changes its mind
// about the language, this fails before the parser silently keeps reading the
// old one.
func TestSpecSubsetIsVerbatim(t *testing.T) {
	assertVerbatim(t, specSection1, specSubsetFixture)
}

// The consuming side of the language, and with it the last of the grammar: a
// service that imports the shared schema and declares only what it reads. Every
// line of section 2's first block parses, and the parser has nothing to say
// about any of it.
//
// Nothing here is resolved and that is the milestone's boundary, not an
// omission. `of SportGame` finds no SportGame, `sport_type_id;` is not held
// against the source's members, `SlimOdd odds;` is not checked to be a view of
// whatever type `odds` has, and `import "prague.protop";` opens no file.
func TestParseSpecSection2(t *testing.T) {
	src := readFixture(t, specSection2Fixture)
	f, file := parseClean(t, src)

	assertDump(t, file, `revision 1
namespace BrandDataReader.Services.Cache
import "prague.protop"
view SlimSportGame of SportGame
  event_id as id
  sport_type_id
  country_id
  league_id
  home_team_id
  venue
  sport_type
view SlimOdd of Odd
  name
  timestamp
view SlimBetType of BetType
  key as id
  event_id
  id as bet_type_id
  name
  status
  status_manual
  status_watch_dog
  limit_manual_single
  timestamp
  SlimOdd odds
`)

	// `key as id;` is the line the whole no-keyword-token-kinds decision is for.
	// `key` is the composite identity section 1 declares, and here it is a name
	// being selected -- an ident like any other, needing nothing of its own.
	bet, ok := file.Decls[2].(*View)
	if !ok {
		t.Fatalf("third declaration is %T, want *View", file.Decls[2])
	}
	if m := bet.Members[0]; m.Name.Text != "key" || m.As == nil || m.As.Text != "id" || m.Narrow != nil {
		t.Errorf("`key as id;` read as %+v, want the member `key` renamed to `id`", m)
	}
	// A narrowing is two idents and nothing else: no `= tag`, which is what
	// separates it from a field, and one more ident than a plain selection.
	if m := bet.Members[9]; m.Narrow == nil || m.Narrow.Text != "SlimOdd" || m.Name.Text != "odds" || m.As != nil {
		t.Errorf("`SlimOdd odds;` read as %+v, want `odds` narrowed to `SlimOdd`", m)
	}

	// A declaration's span covers the whole declaration, and a view's opens at
	// its keyword and closes at its brace like every other.
	const wantSlimOdd = "view SlimOdd of Odd {\n  name;\n  timestamp;\n}"
	if got := f.Slice(file.Decls[1].(*View).Span); got != wantSlimOdd {
		t.Errorf("SlimOdd spans:\n%s\nwant:\n%s", got, wantSlimOdd)
	}
}

// Section 2's second block: a service may declare tables of its own, for caches
// it produces and nobody else reads. M2 already read every construct in it, so
// this is the cheapest possible regression test for that claim -- and it is
// worth having precisely because nothing had to be written to make it pass.
//
// The block is a declaration rather than a file, so the header is prepended
// here instead of being copied into the fixture: the fixture has to stay
// byte-for-byte what the spec fences, and the spec fences no header around it.
func TestParseSpecSection2Table(t *testing.T) {
	_, file := parseClean(t, "revision 1;\nnamespace A;\n"+readFixture(t, specSection2TableFixture))

	assertDump(t, file, `revision 1
namespace A
table BetTypeBrandSettings
  [cs: DataCache]
  key int64 bet_type_id = 1
  bool is_enabled = 2
`)
}

// The other half of "parses verbatim out of the spec", for section 2's two
// blocks. The fixtures are copies of the spec, not paraphrases of it, and if
// the spec changes its mind about the language this fails before the parser
// quietly keeps reading the old one.
func TestSpecSection2IsVerbatim(t *testing.T) {
	for _, name := range []string{specSection2Fixture, specSection2TableFixture} {
		assertVerbatim(t, specSection2, name)
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

// underlines boxes the text a row expects its first diagnostic to underline, so
// that a row which says nothing about the span and a row which says the span is
// empty are two different rows.
//
// They used to be one. The column was a plain string read as
// `if tt.wantSpan != ""`, which spells "not filled in" and "underlines nothing"
// the same way -- and an empty underline is not a degenerate case here, it is
// what EVERY end-of-file diagnostic has. `expected `;`, found end of file`
// points at the zero-width position one past the last byte, so f.Slice returns
// "" and Render prints no snippet line at all. Under the old column that row
// could not be written: asserting "" asserted nothing, and the whole class of
// diagnostic was silently unassertable while looking like a column somebody had
// merely not got to yet.
//
// A pointer rather than a `wantNoSpan bool` because the two facts are one fact
// -- there is an expectation, or there is not -- and a bool beside a string lets
// a row set both and mean neither.
func underlines(s string) *string { return &s }

// Every diagnostic the parser raises, pinned by code sequence and by where it
// points. Codes are protopc's interface -- an editor filters on them and a
// changelog cites them -- so the sequence is asserted exactly.
//
// wantSpan is the text the first diagnostic must underline, written out rather
// than computed. A line and a column pin only where an underline STARTS, so a
// message whose span was shortened to a single caret -- or lengthened to the
// rest of the line -- still passed. The span is the parser's whole contribution
// to what a user sees, and a test that reads it back out of the source is the
// only thing that says how wide it is.
//
// Seven rows pin nothing, and say why in spanUnpinned. All seven are
// nanov/protobuf-to-messagepack-sharp#57: PROTOP0100 and PROTOP0104 underline
// each other's header line, and PROTOP0400 underlines the `[` rather than the
// annotation its message says to move. Pinning a wrong underline is worse than
// leaving it open -- it turns a defect into a golden that #57 then has to argue
// with -- so those spans are left to #57 and the row records the debt instead
// of hiding it. Three rows that were already pinned that way stay pinned, each
// with the citation beside it.
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
		wantSpan  *string
		// spanUnpinned is why this row asserts no underline, and a row must
		// give one or the other. Nothing used to demand either, so the column
		// was 14 rows of 52 and the gap read as an oversight rather than as a
		// decision -- which is how a class of diagnostic stays unpinned for a
		// milestone at a time. The reason is prose because the only reasons
		// that survive review are the ones that name an issue.
		spanUnpinned string
		// wantMessage is the whole sentence, for a message whose whole sentence
		// is the teaching, and wantMessageHas is the parts that must not drift,
		// for one built around an interpolated enumeration or a worked example.
		// A row sets one or the other and never both -- the split is
		// nanov/protobuf-to-messagepack-sharp#40's census, which also names the
		// 14 codes that say only what their code name says and earn no golden
		// at all. Those rows set neither, deliberately, and
		// loadBearingMessages below is what keeps that a decision rather than
		// an omission.
		wantMessage    string
		wantMessageHas []string
	}{
		{
			name:      "missing revision",
			src:       "namespace A;\nstruct S { int64 a = 1; }\n",
			wantCodes: []diag.Code{diag.MissingRevision},
			wantLine:  1, wantCol: 1,
			spanUnpinned: "#57: headSpan is toks[0], so this underlines `namespace` -- the other header line",
		},
		{
			name:      "missing namespace",
			src:       "revision 1;\nstruct S { int64 a = 1; }\n",
			wantCodes: []diag.Code{diag.MissingNamespace},
			wantLine:  1, wantCol: 1,
			spanUnpinned: "#57: headSpan is toks[0], so this underlines `revision` -- the other header line",
		},
		{
			// Both are checked after the whole file, so a header-less file still
			// reports everything else wrong with it -- here, nothing.
			name:      "missing both, reported at the head of the file",
			src:       "struct S { int64 a = 1; }\n",
			wantCodes: []diag.Code{diag.MissingRevision, diag.MissingNamespace},
			wantLine:  1, wantCol: 1,
			spanUnpinned: "#57: headSpan is toks[0], here the `struct` neither diagnostic is about",
		},
		{
			name: "tag 0",
			src: `revision 1;
namespace A;
struct S {
  int64 id = 0;
}
`,
			wantCodes:   []diag.Code{diag.TagOutOfRange},
			wantMessage: "field tag must be greater than 0; a MessagePack key is tag - 1, so tag 0 names no slot",
			wantLine:    4, wantCol: 14,
			wantSpan: underlines("0"),
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
			wantMessageHas: []string{
				"only allowed on `decimal`",
				"not on `int64`",
			},
			wantLine: 4, wantCol: 9, // the precision itself, not the type
			wantSpan: underlines("4"),
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
			wantMessageHas: []string{
				"`decimal` requires a precision",
				"as in `decimal(4)`",
			},
			wantLine: 4, wantCol: 3,
			wantSpan: underlines("decimal"),
		},
		{
			name:      "unterminated block comment",
			src:       "revision 1;\nnamespace A;\n/* and then nothing\n",
			wantCodes: []diag.Code{diag.UnterminatedBlockComment},
			wantLine:  3, wantCol: 1,
			wantSpan: underlines("/*"),
		},
		{
			name:      "unsupported revision",
			src:       "revision 2;\nnamespace A;\n",
			wantCodes: []diag.Code{diag.UnsupportedRevision},
			wantMessageHas: []string{
				"unknown revision 2",
				"this compiler implements revision 1",
			},
			wantLine: 1, wantCol: 10,
			wantSpan: underlines("2"),
		},
		{
			// Named for what it is: the namespace is a header line and not a
			// declaration, so this covers one arm of the condition and the case
			// below covers the other. They were one case with a name that
			// claimed both, and the declaration arm could be deleted outright
			// with the suite green.
			name:      "revision after the namespace",
			src:       "namespace A;\nrevision 1;\n",
			wantCodes: []diag.Code{diag.RevisionNotFirst},
			wantLine:  2, wantCol: 1,
			wantSpan: underlines("revision 1;"),
		},
		{
			// The arm that was dead to the suite. `revision` stamps the dialect
			// the rest of the file is read in, so a declaration above it has
			// already been read in a dialect the file had not yet named.
			//
			// The declaration is the ONLY thing above the revision here --
			// namespace and imports come after -- so this case turns on the
			// `len(file.Decls) > 0` arm and on nothing else. With a namespace
			// above it too, the arm could be deleted outright and the case
			// would go on passing on the namespace's account.
			name: "revision after a declaration",
			src: `struct S {
  int64 a = 1;
}
revision 1;
namespace A;
`,
			wantCodes: []diag.Code{diag.RevisionNotFirst},
			wantLine:  4, wantCol: 1,
			wantSpan: underlines("revision 1;"),
		},
		{
			name:      "duplicate revision",
			src:       "revision 1;\nrevision 1;\nnamespace A;\n",
			wantCodes: []diag.Code{diag.DuplicateRevision},
			wantLine:  2, wantCol: 1,
			wantSpan: underlines("revision 1;"),
		},
		{
			name:      "duplicate namespace",
			src:       "revision 1;\nnamespace A;\nnamespace B;\n",
			wantCodes: []diag.Code{diag.DuplicateNamespace},
			wantLine:  3, wantCol: 1,
			wantSpan: underlines("namespace B;"),
		},
		{
			// The duplicate is below a declaration rather than beside the
			// header, which is where a second one actually gets written --
			// someone appends to the wrong file, or a merge lands two halves
			// together. Both of these checks read whether the header line has
			// been seen and nothing about where the second one is, and every
			// other case keeps the two adjacent, so a check narrowed to the
			// header would report nothing here and the second namespace would
			// be silently ignored.
			name: "duplicate namespace below a declaration",
			src: `revision 1;
namespace A;
view V of T {
  a;
}
namespace B;
`,
			wantCodes: []diag.Code{diag.DuplicateNamespace},
			wantLine:  6, wantCol: 1,
			wantSpan: underlines("namespace B;"),
		},
		{
			// And the same for `revision`, where narrowing the check does not
			// silence the diagnostic but DOWNGRADES it: a second stamp below a
			// declaration is a duplicate first and a misplacement second, and
			// reporting only that it is not first says the wrong thing about a
			// file that already has one.
			name: "duplicate revision below a declaration",
			src: `revision 1;
namespace A;
view V of T {
  a;
}
revision 1;
`,
			wantCodes: []diag.Code{diag.DuplicateRevision},
			wantLine:  6, wantCol: 1,
			wantSpan: underlines("revision 1;"),
		},
		{
			name:      "unknown declaration",
			src:       "revision 1;\nnamespace A;\nwidget W {\n}\n",
			wantCodes: []diag.Code{diag.UnknownDeclaration},
			wantMessageHas: []string{
				"unknown declaration `widget`",
				"`struct`, `enum`, `record`, `table`, `view` or `import`",
			},
			wantLine: 3, wantCol: 1,
			wantSpan: underlines("widget"),
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
			wantCodes:   []diag.Code{diag.AnnotationNotAllowed},
			wantMessage: "an annotation follows what it describes and there is no before-slot; write this one after the declaration's name",
			wantLine:    3, wantCol: 1,
			spanUnpinned: "#57: PROTOP0400 underlines the `[` alone, not the annotation it says to move",
		},
		{
			name: "annotation before a tag",
			src: `revision 1;
namespace A;
struct S {
  int64 a [cs: X] = 1;
}
`,
			wantCodes:   []diag.Code{diag.AnnotationNotAllowed},
			wantMessage: "an annotation follows what it describes and there is no before-slot; write this one after the member's tag",
			wantLine:    4, wantCol: 11,
			spanUnpinned: "#57: PROTOP0400 underlines the `[` alone, not the annotation it says to move",
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
			wantMessageHas: []string{
				"unknown annotation prefix `go`",
				"protopc's backends are `cs`",
				"writing a backend rather than a keyword",
			},
			wantLine: 4, wantCol: 16,
			wantSpan: underlines("go"),
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
			wantMessageHas: []string{
				"unknown annotation `refrences`",
				"protop's own annotations are `references`",
				"as in `[cs: refrences]`",
			},
			wantLine: 4, wantCol: 16,
			wantSpan: underlines("refrences"),
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
			wantSpan: underlines("["),
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
			wantSpan: underlines("0"),
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
			wantSpan: underlines(";"),
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
			wantSpan: underlines(";"),
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
			wantSpan: underlines("0"),
		},
		{
			// Two members cannot hold one position in the array. The second one
			// is reported, and the message points back at the first.
			name: `tag used twice`,
			src: `revision 1;
namespace A;
struct S {
  int64 a = 1;
  int64 b = 1;
}
`,
			wantCodes:   []diag.Code{diag.DuplicateTag},
			wantMessage: "tag 1 is already used at x.protop:4:13; a tag is a position in the MessagePack array and two members cannot hold one position",
			wantLine:    5, wantCol: 13,
			wantSpan: underlines("1"),
		},
		{
			// The reservation is checked against the whole declaration however it
			// is ordered, so a `reserved` written below the field it retires
			// still catches it.
			name: `tag retired by a reserved range written below it`,
			src: `revision 1;
namespace A;
table T {
  int64 a = 10;
  reserved 8..14;
}
`,
			wantCodes:   []diag.Code{diag.TagIsReserved},
			wantMessage: "tag 10 is reserved at x.protop:5:12; a retired tag is never reused, because a reader built against the old schema would decode this field into the old one",
			wantLine:    4, wantCol: 13,
			wantSpan: underlines("10"),
		},
		{
			// A range that ends before it begins retires nothing, so it is a
			// reservation that silently does not hold.
			name: `reversed reserved range`,
			src: `revision 1;
namespace A;
table T {
  reserved 14..8;
}
`,
			wantCodes: []diag.Code{diag.ReservedRangeReversed},
			wantLine:  4, wantCol: 12,
			wantSpan: underlines("14..8"),
		},
		{
			// Identity is what a table has and the value declarations do not: a
			// relation resolves against a key, and nothing can point at a struct.
			name: `key on a struct`,
			src: `revision 1;
namespace A;
struct S {
  key int64 id = 1;
}
`,
			wantCodes:   []diag.Code{diag.KeyNotAllowed},
			wantMessage: "only a `table` carries identity; a `struct` cannot declare a `key`",
			wantLine:    4, wantCol: 3,
			wantSpan: underlines("key"),
		},
		{
			// The composite spelling is held to the same rule, and is told so
			// before its own contents are read.
			name: `composite key on a record`,
			src: `revision 1;
namespace A;
record R {
  key key = 1 (a, b);
  int64 a = 2;
  int64 b = 3;
}
`,
			wantCodes:   []diag.Code{diag.KeyNotAllowed},
			wantMessage: "only a `table` carries identity; a `record` cannot declare a `key`",
			wantLine:    4, wantCol: 3,
			wantSpan: underlines("key"),
		},
		{
			// A composite key spends an ordinal, so it collides with a field
			// that spends the same one -- which is the reason the spelling
			// writes the ordinal down instead of inferring it.
			name: `composite key colliding with a field`,
			src: `revision 1;
namespace A;
table T {
  key key = 1 (a, b);
  int64 a = 1;
  int64 b = 2;
}
`,
			wantCodes: []diag.Code{diag.DuplicateTag},
			wantLine:  5, wantCol: 13,
			wantSpan: underlines("1"),
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
			wantMessageHas: []string{
				"only allowed on `decimal`",
				"not on `Venue`",
			},
			wantLine: 4, wantCol: 9,
			wantSpan: underlines("4"),
		},
		{
			// Section 3 gives an annotation one position, after what it
			// describes. Before a member is the mirror of before a declaration
			// head and costs the same one diagnostic.
			name: "annotation before a member",
			src: `revision 1;
namespace A;
struct S {
  [cs: X] int64 a = 1;
}
`,
			wantCodes:   []diag.Code{diag.AnnotationNotAllowed},
			wantMessage: "an annotation follows what it describes and there is no before-slot; write this one after the member's tag",
			wantLine:    4, wantCol: 3,
			spanUnpinned: "#57: PROTOP0400 underlines the `[` alone, not the annotation it says to move",
		},
		{
			// The position that would have to mean "annotate the type" if any
			// position did. None does: an annotation describes the member.
			name: "annotation between a type and a name",
			src: `revision 1;
namespace A;
struct S {
  int64 [cs: Y] b = 2;
}
`,
			wantCodes: []diag.Code{diag.AnnotationNotAllowed},
			wantLine:  4, wantCol: 9,
			spanUnpinned: "#57: PROTOP0400 underlines the `[` alone, not the annotation it says to move",
		},
		{
			// The backend set is closed, and closed means closed against every
			// prefix rather than against the one another test happens to use.
			name: "unknown annotation prefix that is not `go`",
			src: `revision 1;
namespace A;
struct S {
  int64 a = 1 [kt: Serializable];
}
`,
			wantCodes: []diag.Code{diag.UnknownAnnotationPrefix},
			wantMessageHas: []string{
				"unknown annotation prefix `kt`",
				"protopc's backends are `cs`",
			},
			wantLine: 4, wantCol: 16,
			wantSpan: underlines("kt"),
		},
		{
			// A list has one element type. Two is not a list of pairs, it is a
			// type protop does not have.
			name: "list given two type arguments",
			src: `revision 1;
namespace A;
struct S {
  list<Odd, Venue> odds = 1;
}
`,
			wantCodes: []diag.Code{diag.TypeArgumentCountWrong},
			wantMessageHas: []string{
				"`list` takes 1 type argument and was given 2",
				"as in `list<Odd>`",
			},
			wantLine: 4, wantCol: 3,
			wantSpan: underlines("list<Odd, Venue>"),
		},
		{
			// A bare `list` in type position is not a declared type named
			// `list`: the collections are a closed set and the name is taken
			// there, so what is missing is the element type.
			name: "list with no type argument",
			src: `revision 1;
namespace A;
struct S {
  list odds = 1;
}
`,
			wantCodes: []diag.Code{diag.TypeArgumentCountWrong},
			wantMessageHas: []string{
				"`list` takes 1 type argument and was given 0",
				"as in `list<Odd>`",
			},
			wantLine: 4, wantCol: 3,
			wantSpan: underlines("list"),
		},
		{
			// Only a collection is generic. The diagnostic underlines the whole
			// type because what is wrong is the combination.
			name: "type arguments on a type that takes none",
			src: `revision 1;
namespace A;
struct S {
  int64<string> a = 1;
}
`,
			wantCodes: []diag.Code{diag.TypeArgumentsNotAllowed},
			wantMessageHas: []string{
				"`int64` takes no type arguments",
				"protop's generic types are `list`, `map`",
			},
			wantLine: 4, wantCol: 3,
			wantSpan: underlines("int64<string>"),
		},
		{
			// `map` is reserved, not implemented: it parses in full -- the
			// arguments are read and checked -- and is then reported against as
			// a compiler that is behind, not a file that is wrong.
			name: "map is parsed and not implemented",
			src: `revision 1;
namespace A;
struct S {
  map<string, Odd> odds_by_name = 1;
}
`,
			wantCodes: []diag.Code{diag.NotYetImplemented},
			wantLine:  4, wantCol: 3,
			wantSpan: underlines("map"),
		},
		{
			// Both facts are independently true and both are told: the arity is
			// wrong, and the construct would not have been read anyway.
			name: "map with the wrong arity is told both things",
			src: `revision 1;
namespace A;
struct S {
  map<string> m = 1;
}
`,
			wantCodes: []diag.Code{diag.TypeArgumentCountWrong, diag.NotYetImplemented},
			wantMessageHas: []string{
				"`map` takes 2 type arguments and was given 1",
				"as in `map<string, Odd>`",
			},
			wantLine: 4, wantCol: 3,
			wantSpan: underlines("map<string>"),
		},
		{
			// The `<` promises a type argument, and a member that fails mid-way
			// is recovered from at the `;` like any other.
			name: "type argument list with no closing angle",
			src: `revision 1;
namespace A;
struct S {
  list<Odd odds = 1;
}
`,
			wantCodes: []diag.Code{diag.UnexpectedToken},
			wantLine:  4, wantCol: 12,
			wantSpan: underlines("odds"),
		},
		{
			// A member that repeats a tag is told that and nothing else. The
			// reservation has already been reported against the member that
			// holds the tag first, and saying it twice would describe one
			// retired tag as two problems.
			name: "a tag both reserved and already used is told once per member",
			src: `revision 1;
namespace A;
table T {
  int64 b = 10;
  int64 c = 10;
  reserved 8..14;
}
`,
			wantCodes: []diag.Code{diag.TagIsReserved, diag.DuplicateTag},
			wantLine:  4, wantCol: 13,
			wantSpan: underlines("10"),
		},
		{
			// The best message in the milestone, and the reason it is its own
			// code. A tag here is not a token in the wrong place: it is a
			// misunderstanding of what a view is, so the message says what a
			// view is instead of what the parser expected.
			//
			// The `=` underline is #57's last entry, and the one #57 does not
			// call a defect: the equals sign is where the mistake starts, and
			// the message names the member anyway. It stays pinned here and in
			// TestRenderOfTheDiagnosticsThisMilestoneAdds because #57 asks for
			// the choice to be written down rather than inferred from two
			// tests -- this is the writing down. If #57 rules it a defect the
			// two pins move together.
			name: "a tag on a view member",
			src: `revision 1;
namespace A;
view V of T {
  sport_type_id = 5;
}
`,
			wantCodes:   []diag.Code{diag.TagInViewMember},
			wantMessage: "a view may not write a tag: `sport_type_id` is pinned to the tag its source gives it, and only the source extends the tag space; a tag added here is never written by the producer, so it would decode to nil forever while squatting a slot the source may later claim",
			wantLine:    4, wantCol: 17,
			wantSpan: underlines("="),
		},
		{
			// A view is a subset of one source. Without the `of` it is a subset
			// of nothing, and it carries no tags of its own to fall back on.
			//
			// The `{` underline is #57's: the caret lands on the token AFTER
			// the gap where the source belongs. Already checked in, so it is
			// left pinned rather than loosened -- #57 moves it, and this pin is
			// what will say so.
			name: "a view with no `of`",
			src: `revision 1;
namespace A;
view V {
  name;
}
`,
			wantCodes:   []diag.Code{diag.ViewSourceMissing},
			wantMessage: "a view is a subset of one source and declares no members of its own; name that source, as in `view V of SportGame { ... }`",
			wantLine:    3, wantCol: 8,
			wantSpan: underlines("{"),
		},
		{
			// The word is `of` and not merely some word: a view head is a name,
			// `of`, and a source, and a parser that took whatever ident it
			// found in the middle would read `view V from T` as a view of `T`
			// with the `from` silently thrown away.
			//
			// PROTOP0203's underline is #57's; this arm is the one where it
			// lands on something the user did type.
			name: "a view whose source is not introduced by `of`",
			src: `revision 1;
namespace A;
view V from T {
  name;
}
`,
			wantCodes: []diag.Code{diag.ViewSourceMissing},
			wantLine:  3, wantCol: 8,
			wantSpan: underlines("from"),
		},
		{
			// The same mistake said the other way, and so the same code: the
			// `of` is there and names nothing.
			//
			// `{` again, and #57's again -- see the `view with no of` row.
			name: "a view whose `of` names nothing",
			src: `revision 1;
namespace A;
view V of {
  name;
}
`,
			wantCodes: []diag.Code{diag.ViewSourceMissing},
			wantLine:  3, wantCol: 11,
			wantSpan: underlines("{"),
		},
		{
			// The leading name of a view member narrows it to another view. A
			// scalar is not one, and it is the case the parser can rule on
			// alone: the scalars are closed and nothing declares a view over
			// one of those names.
			name: "a scalar in a view member's narrowing position",
			src: `revision 1;
namespace A;
view V of T {
  int64 x;
}
`,
			wantCodes:   []diag.Code{diag.ScalarInViewMember},
			wantMessage: "`int64` is a scalar and a view member narrows to another view; a member keeps the type its source gives it when it is written as its name alone",
			wantLine:    4, wantCol: 3,
			wantSpan: underlines("int64"),
		},
		{
			// A field written into a view body is two mistakes, and both are
			// independently true: the leading name is not a view, and the tag
			// is not the view's to write.
			name: "a field written into a view body is told both things",
			src: `revision 1;
namespace A;
view V of T {
  int64 x = 1;
}
`,
			wantCodes: []diag.Code{diag.ScalarInViewMember, diag.TagInViewMember},
			wantLine:  4, wantCol: 3,
			wantSpan: underlines("int64"),
		},
		{
			// Section 3 gives an annotation one position, after what it
			// describes, and a view body is not an exception to it.
			//
			// This is the `wantSpan: "["` #57 calls the trap. PROTOP0400 is
			// raised with the span of the bracket TOKEN while the accepted
			// path in the same file uses the whole-bracket span that the
			// rejected path computes and discards, so a message reading "write
			// this one after ..." underlines one character of the seven it
			// means. It is left pinned because it was already checked in and
			// #57 is what moves it -- but it is not precedent: the four other
			// PROTOP0400 rows in this table are deliberately unpinned, which is
			// what spanUnpinned records.
			name: "annotation before a view member",
			src: `revision 1;
namespace A;
view V of T {
  [cs: X] name;
}
`,
			wantCodes:   []diag.Code{diag.AnnotationNotAllowed},
			wantMessage: "an annotation follows what it describes and there is no before-slot; write this one after the member's name",
			wantLine:    4, wantCol: 3,
			wantSpan: underlines("["),
		},
		{
			// Imports are grouped under the header. What a file depends on is a
			// property of the whole file, and nothing in protop is scoped by
			// where it is written, so an import below a declaration would come
			// into force above it anyway.
			name: "an import below a declaration",
			src: `revision 1;
namespace A;
struct S {
  int64 a = 1;
}
import "prague.protop";
`,
			wantCodes:   []diag.Code{diag.ImportNotInHeader},
			wantMessage: "an `import` belongs with the file's other imports, above the first declaration; what a file depends on is a property of the whole file and not of the lines below it",
			wantLine:    6, wantCol: 1,
			wantSpan: underlines(`import "prague.protop";`),
		},
		{
			// An import names a file with a string literal. A bare word is not
			// one, and the path is never a name the parser resolves.
			name: "an import with no string literal",
			src: `revision 1;
namespace A;
import prague;
`,
			wantCodes: []diag.Code{diag.UnexpectedToken},
			wantLine:  3, wantCol: 8,
			wantSpan: underlines("prague"),
		},
		{
			// `revision` is first in the file, and an import is something it
			// has to be first of -- otherwise a file could stamp its dialect
			// after depending on another one.
			name:      "revision after an import",
			src:       "import \"prague.protop\";\nrevision 1;\nnamespace A;\n",
			wantCodes: []diag.Code{diag.RevisionNotFirst},
			wantLine:  2, wantCol: 1,
			wantSpan: underlines("revision 1;"),
		},
		{
			// Overlapping reservations are one reservation as far as a tag is
			// concerned: the tag is retired once however many ranges cover it.
			name: "overlapping reserved ranges retire a tag once",
			src: `revision 1;
namespace A;
table T {
  reserved 8..14, 10..20;
  int64 a = 12;
}
`,
			wantCodes: []diag.Code{diag.TagIsReserved},
			wantLine:  5, wantCol: 13,
			wantSpan: underlines("12"),
		},
		{
			// The end of the file is a legitimately empty underline, and the
			// reason the column is a pointer. The `}` that is missing is at no
			// token, so the span is the zero-width position one past the last
			// byte: the headline names the line and the column and Render emits
			// no snippet, because there is no source left to point at.
			//
			// Written as a row rather than left to the renderer's own tests
			// because the parser is what chooses this span. An EOF diagnostic
			// raised at the last token instead -- at the `;`, say -- would read
			// as a complaint about a line that is perfectly fine.
			name: "a declaration the file ends inside",
			src: `revision 1;
namespace A;
struct S {
  int64 a = 1;
`,
			wantCodes: []diag.Code{diag.UnexpectedToken},
			wantLine:  5, wantCol: 1,
			wantSpan: underlines(""),
		},
		{
			// PROTOP0201 is raised at two sites with two different sentences,
			// and this is the second. `unknown declaration` answers a word that
			// names no keyword; this answers a token that is not a word at all,
			// and tells the user what a declaration may start with instead of
			// quoting back something they did not write.
			name:      "a top-level token that is not a word at all",
			src:       "revision 1;\nnamespace A;\n;\n",
			wantCodes: []diag.Code{diag.UnknownDeclaration},
			wantMessageHas: []string{
				"expected a declaration",
				"found `;`",
			},
			wantLine: 3, wantCol: 1,
			wantSpan: underlines(";"),
		},
		{
			// PROTOP0402's second site, for the same reason: `unknown
			// annotation` quotes the word back and lists protop's own set,
			// which there is no word to do here.
			name: "an annotation that is not a word at all",
			src: `revision 1;
namespace A;
struct S {
  int64 a = 1 [1];
}
`,
			wantCodes: []diag.Code{diag.UnknownAnnotation},
			wantMessageHas: []string{
				"expected an annotation",
				"found an integer",
			},
			wantLine: 4, wantCol: 16,
			wantSpan: underlines("1"),
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
			assertGate(t, f, bag, tt.wantCodes)
			if pos := f.Position(bag.All()[0].Span.Start); pos.Line != tt.wantLine || pos.Column != tt.wantCol {
				t.Errorf("first diagnostic at %d:%d, want %d:%d", pos.Line, pos.Column, tt.wantLine, tt.wantCol)
			}
			switch {
			case tt.wantSpan != nil && tt.spanUnpinned != "":
				t.Errorf("row pins a span AND says why it does not; one of the two is stale")
			case tt.wantSpan != nil:
				if got := f.Slice(bag.All()[0].Span); got != *tt.wantSpan {
					t.Errorf("first diagnostic underlines %q, want %q", got, *tt.wantSpan)
				}
			case tt.spanUnpinned == "":
				t.Errorf("row pins no span and gives no reason: set wantSpan, or set spanUnpinned to the issue that owns the span")
			}

			msg := bag.All()[0].Message
			if tt.wantMessage != "" && len(tt.wantMessageHas) > 0 {
				t.Errorf("row pins the whole message AND parts of it; pick one")
			}
			if tt.wantMessage != "" && msg != tt.wantMessage {
				t.Errorf("first diagnostic reads\n\t%q\nwant\n\t%q", msg, tt.wantMessage)
			}
			for _, want := range tt.wantMessageHas {
				if !strings.Contains(msg, want) {
					t.Errorf("first diagnostic reads\n\t%q\nwant it to contain\n\t%q", msg, want)
				}
			}
		})
	}

	// Every code this package raises whose message carries more than its own
	// name is pinned by some row above. Without this the message column is the
	// span column before it was made to answer for itself: filled where
	// somebody thought of it, and silent about everywhere they did not.
	//
	// The list is #40's census, restricted to the codes this package raises. A
	// code is on it when its message says something the code name does not --
	// an enumeration of what was expected, a worked example, a position to go
	// and look at. The codes deliberately absent are the ones whose message is
	// their own name in a sentence: a golden on those is a second copy of
	// codes.go that has to be edited whenever a word moves.
	covered := map[diag.Code]bool{}
	for _, tt := range tests {
		if tt.wantMessage == "" && len(tt.wantMessageHas) == 0 {
			continue
		}
		covered[tt.wantCodes[0]] = true
	}
	for _, c := range loadBearingMessages {
		if !covered[c] {
			t.Errorf("%s carries a load-bearing message and no row pins it", c)
		}
	}
}

// loadBearingMessages is the codes raised in this package whose message text is
// worth more than the code number -- #40's census, its parser half. Two of them
// are raised at two sites with different wording and have a row for each:
// PROTOP0201 (`expected a declaration` against `unknown declaration`) and
// PROTOP0402 (`expected an annotation` against `unknown annotation`).
//
// PROTOP0200 is absent on purpose despite having six sites. Its message is
// `expected X, found Y` at every one of them, which is the code name with the
// two tokens the underline already shows; the census puts it among the 14 that
// earn no golden, and six rows of it would be six copies of the Kind stringer.
var loadBearingMessages = []diag.Code{
	diag.UnsupportedRevision,
	diag.ImportNotInHeader,
	diag.UnknownDeclaration,
	diag.KeyNotAllowed,
	diag.ViewSourceMissing,
	diag.TagOutOfRange,
	diag.PrecisionNotAllowed,
	diag.PrecisionRequired,
	diag.DuplicateTag,
	diag.TagIsReserved,
	diag.TypeArgumentCountWrong,
	diag.TypeArgumentsNotAllowed,
	diag.TagInViewMember,
	diag.ScalarInViewMember,
	diag.AnnotationNotAllowed,
	diag.UnknownAnnotationPrefix,
	diag.UnknownAnnotation,
}

// The severity of a parser diagnostic, end to end, on the two codes a demotion
// was reproduced on. A code sequence says what was noticed; this says what it
// costs, which is a different claim: bag.HasErrors is the gate every stage
// after the parser inherits, so a code that does not fail the compile is a file
// protopc reads, accepts, and generates C# from.
//
// Both of these were demoted by hand -- PROTOP0202 at the parser's `key` site,
// PROTOP0303 at its duplicate-tag one -- and the whole suite stayed green while
// protopc printed a warning and exited 0 on input it exists to reject. The
// checker's side of PROTOP0303 was never in that state, because
// TestACheckerDiagnosticIsWhatDecidesTheExitCode reads the exit code; the
// parser's side had nothing.
//
// diag's table pins what each code means. This pins that the meaning survives
// the trip from the code to the gate, and neither test makes the other one
// true.
func TestAParserDiagnosticFailsTheCompile(t *testing.T) {
	for _, tt := range []struct {
		name string
		src  string
		want diag.Code
	}{
		{
			// Identity is a table's, so a struct that declares a key is not a
			// struct with a harmless extra word -- nothing can point at it.
			name: "a key on a struct",
			src: `revision 1;
namespace A;
struct S {
  key int64 id = 1;
}
`,
			want: diag.KeyNotAllowed,
		},
		{
			// A tag is a position in a MessagePack array. Accepting this emits
			// two members writing one slot, which is a wire format nobody can
			// read back.
			name: "one tag on two members",
			src: `revision 1;
namespace A;
struct S {
  int64 a = 1;
  int64 b = 1;
}
`,
			want: diag.DuplicateTag,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, _, bag := parse(t, tt.src)
			if bag.Len() != 1 || bag.All()[0].Code != tt.want {
				t.Fatalf("want one %s, got:\n%s", tt.want, diag.RenderAll(f, bag))
			}
			if got := bag.All()[0].Severity; got != diag.Error {
				t.Errorf("%s was raised as a %s", tt.want, got)
			}
			if !bag.HasErrors() {
				t.Error("HasErrors is false, so protopc accepts this file and generates from it")
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

// The four messages this milestone adds, as a user reads them: severity, code,
// position, wording and the width of the underline, all in one golden each.
//
// Every part of that was unpinned. A diagnostic's Message was asserted nowhere
// in the suite -- the tables collect Code and the renders only ever appeared
// inside a t.Fatalf format string, printed on failure and never compared -- so
// PROTOP0308's message, which #36 asks to be the best in the milestone, could
// be replaced by "unexpected token" with everything green. Two of the four
// could be emptied entirely.
//
// Severity was unpinned too, and it is the worst of the three: bag.HasErrors
// gates every stage after the parser, so a code quietly demoted to a warning
// means protopc ACCEPTS a view that names no source and hands it to a backend.
// Demoting one now takes a line of internal/diag/codes.go rather than a word at
// this call site, and diag's own table holds all forty-five of them; the word
// `error` in each golden says so here as well, and HasErrors is asserted beside
// them in case the rendering ever stops printing it.
func TestRenderOfTheDiagnosticsThisMilestoneAdds(t *testing.T) {
	for _, tt := range []struct {
		name string
		src  string
		want string
	}{
		{
			// The one that explains rather than reports. A tag in a view is not
			// a token in the wrong place, so the message is about what a view
			// is and what writing a tag in one would do on the wire.
			name: "a tag on a view member",
			src:  "revision 1;\nnamespace A;\nview V of T {\n  sport_type_id = 5;\n}\n",
			want: "x.protop:4:17: error PROTOP0308: a view may not write a tag: `sport_type_id` is " +
				"pinned to the tag its source gives it, and only the source extends the tag space; " +
				"a tag added here is never written by the producer, so it would decode to nil " +
				"forever while squatting a slot the source may later claim\n" +
				"  |\n" +
				"4 |   sport_type_id = 5;\n" +
				"  |                 ^",
		},
		{
			// The message names the missing word and shows the shape, because
			// the mistake is usually not knowing a view has a source at all.
			name: "a view that names no source",
			src:  "revision 1;\nnamespace A;\nview V {\n  name;\n}\n",
			want: "x.protop:3:8: error PROTOP0203: a view is a subset of one source and declares no " +
				"members of its own; name that source, as in `view V of SportGame { ... }`\n" +
				"  |\n" +
				"3 | view V {\n" +
				"  |        ^",
		},
		{
			// The underline is the scalar alone and not the whole member: the
			// name being selected is fine, and the message says which word to
			// delete.
			name: "a scalar in a narrowing position",
			src:  "revision 1;\nnamespace A;\nview V of T {\n  int64 x;\n}\n",
			want: "x.protop:4:3: error PROTOP0309: `int64` is a scalar and a view member narrows to " +
				"another view; a member keeps the type its source gives it when it is written as " +
				"its name alone\n" +
				"  |\n" +
				"4 |   int64 x;\n" +
				"  |   ^^^^^",
		},
		{
			// The whole import is underlined, because the line is not wrong --
			// only where it is written is -- and the message says where it
			// belongs and why the position is not a matter of taste.
			name: "an import below a declaration",
			src:  "revision 1;\nnamespace A;\nstruct S {\n  int64 a = 1;\n}\nimport \"prague.protop\";\n",
			want: "x.protop:6:1: error PROTOP0106: an `import` belongs with the file's other imports, " +
				"above the first declaration; what a file depends on is a property of the whole " +
				"file and not of the lines below it\n" +
				"  |\n" +
				"6 | import \"prague.protop\";\n" +
				"  | ^^^^^^^^^^^^^^^^^^^^^^^",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, _, bag := parse(t, tt.src)
			if bag.Len() != 1 {
				t.Fatalf("got %d diagnostics, want 1:\n%s", bag.Len(), diag.RenderAll(f, bag))
			}
			if !bag.HasErrors() {
				t.Error("reported as a warning; everything downstream is gated on HasErrors, so a warning here is an accepted file")
			}
			if got := diag.Render(f, bag.All()[0]); got != tt.want {
				t.Errorf("rendered:\n%s\nwant:\n%s", got, tt.want)
			}
		})
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
//
// The declaration being skipped used to be a `view`, which the parser now
// reads. An unknown keyword is what is left, and it is the better test anyway:
// `view` was going to stop being unreadable, and `widget` never will be.
func TestRecoveryAtTopLevelConsumesTheDeclarationItSkips(t *testing.T) {
	f, file, bag := parse(t, `revision 1;
namespace A;
widget W {
  int64 a = 1;
}
struct S {
  int64 a = 1;
}
`)
	if bag.Len() != 1 || bag.All()[0].Code != diag.UnknownDeclaration {
		t.Fatalf("got %d diagnostics, want one UnknownDeclaration:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	assertDump(t, file, "revision 1\nnamespace A\nstruct S\n  int64 a = 1\n")
}

// `record` is `struct` with a different keyword: the parser reads one shape and
// section 4 gives the two different meanings. A field may name a declared type,
// and a collection is a type over one -- `list<T>` -- rather than a label in
// front of the field.
//
// Nothing here is resolved -- `Venue` names no declaration in this file and the
// parser has nothing to say about that, because whether a name is in scope is a
// question only the stage that knows the scope can ask.
func TestParseRecordNamedTypesAndLists(t *testing.T) {
	_, file := parseClean(t, `revision 1;
namespace A;
record StaticDataRef {
  int64  id   = 1;
  string name = 2;
}
struct S {
  StaticDataRef       sport_type = 1;
  StaticDataRef?      home_team  = 2;
  list<StaticDataRef> runners    = 3;
  Venue?              venue      = 4;
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
  list<StaticDataRef> runners = 3
  Venue? venue = 4
`)

	if _, ok := file.Decls[0].(*Record); !ok {
		t.Errorf("first declaration is %T, want *Record", file.Decls[0])
	}
	s := file.Decls[1].(*Struct)
	// The element type is an argument of the collection type, and the field
	// carries nothing extra: the whole of the repetition is in TypeRef.
	runners := s.Members[2].(*Field).Type
	if runners.Name.Text != "list" || len(runners.Args) != 1 || runners.Args[0].Name.Text != "StaticDataRef" {
		t.Errorf("runners is %s, want list<StaticDataRef>", typeString(runners))
	}
	if got := s.Members[0].(*Field).Type; len(got.Args) != 0 {
		t.Errorf("sport_type is %s, want no type arguments", typeString(got))
	}
}

// The reason repetition moved into the type: presence composes, and the three
// spellings are three types. `repeated Odd?` could only ever have been one of
// the last two.
func TestListComposesWithPresenceAndNests(t *testing.T) {
	_, file := parseClean(t, `revision 1;
namespace A;
struct S {
  list<Odd>        plain   = 1;
  list<Odd>?       maybe   = 2;
  list<Odd?>       holes   = 3;
  list<list<int64>> nested = 4;
}
`)
	assertDump(t, file, `revision 1
namespace A
struct S
  list<Odd> plain = 1
  list<Odd>? maybe = 2
  list<Odd?> holes = 3
  list<list<int64>> nested = 4
`)

	s := file.Decls[0].(*Struct)
	types := make([]TypeRef, 4)
	for i := range types {
		types[i] = s.Members[i].(*Field).Type
	}
	// `list<Odd>?` is Optional on the outer ref; `list<Odd?>` on the inner one.
	// One bool, two positions, and that is the whole of the disambiguation.
	if types[1].Optional == types[2].Optional {
		t.Errorf("list<Odd>? and list<Odd?> agree about Optional on the outer ref")
	}
	if !types[1].Optional || types[1].Args[0].Optional {
		t.Errorf("list<Odd>? is %+v, want the `?` on the collection", types[1])
	}
	if types[2].Optional || !types[2].Args[0].Optional {
		t.Errorf("list<Odd?> is %+v, want the `?` on the element", types[2])
	}
	// Nesting needs no code of its own: Args recurses.
	if inner := types[3].Args[0]; inner.Name.Text != "list" || inner.Args[0].Name.Text != "int64" {
		t.Errorf("nested is %s, want list<list<int64>>", typeString(types[3]))
	}
}

// `list` and `map` are words, not keyword token kinds -- the same property
// TestKeyIsAWordAndNotAKeyword pins for `key`. A member may be named either,
// and the parser tells a type position from a name position by where it is.
func TestListIsAWordAndNotAKeyword(t *testing.T) {
	_, file := parseClean(t, `revision 1;
namespace A;
struct S {
  int64      list = 1;
  int64      map  = 2;
  list<Odd>  odds = 3;
  list<int64> tags = 4;
}
`)
	assertDump(t, file, `revision 1
namespace A
struct S
  int64 list = 1
  int64 map = 2
  list<Odd> odds = 3
  list<int64> tags = 4
`)
}

// The three closed sets, written out here as literals so that the test has an
// oracle the implementation cannot move. Each of these sets is a decision --
// protop's scalars are a census of the spec (#32), the legal annotation
// prefixes are exactly the backends protopc has, and protop's own annotations
// are read rather than passed through -- and a set that can be widened or
// narrowed with the suite green is not closed in any sense that matters.
var (
	wantScalarTypes       = []string{"bool", "decimal", "double", "int32", "int64", "string"}
	wantBackends          = []string{"cs"}
	wantProtopAnnotations = []string{"references"}
	wantCollectionTypes   = map[string]int{"list": 1, "map": 2}
)

func TestClosedSetsAreExactlyWhatTheySay(t *testing.T) {
	for _, tt := range []struct {
		name string
		got  map[string]bool
		want []string
	}{
		{"scalarTypes", scalarTypes, wantScalarTypes},
		{"backends", backends, wantBackends},
		{"protopAnnotations", protopAnnotations, wantProtopAnnotations},
	} {
		t.Run(tt.name, func(t *testing.T) {
			names := make([]string, 0, len(tt.got))
			for name, ok := range tt.got {
				if !ok {
					t.Errorf("%s[%q] is false; membership is the map's only meaning", tt.name, name)
				}
				names = append(names, name)
			}
			slices.Sort(names)
			if !slices.Equal(names, tt.want) {
				t.Errorf("%s = %v, want %v", tt.name, names, tt.want)
			}
		})
	}

	// A collection carries its arity, so widening it is two mistakes rather
	// than one: a name that should not be generic, and a shape for it.
	t.Run("collectionTypes", func(t *testing.T) {
		if len(collectionTypes) != len(wantCollectionTypes) {
			t.Fatalf("collectionTypes = %v, want %v", collectionTypes, wantCollectionTypes)
		}
		for name, arity := range wantCollectionTypes {
			if got, ok := collectionTypes[name]; !ok || got != arity {
				t.Errorf("collectionTypes[%q] = %d, %v; want %d", name, got, ok, arity)
			}
		}
	})

	// `list` and `map` are not scalars, and the separation is the point: #32 is
	// about the scalar set and a type constructor is not in it.
	for _, name := range []string{"list", "map"} {
		if scalarTypes[name] {
			t.Errorf("%q is in scalarTypes; a collection is a type constructor, not a scalar", name)
		}
	}
}

// Whether protop's scalar set is closed at all is unresolved -- #32 -- and the
// parser no longer branches on it: a name outside the set is a reference to a
// declared type. What keeps the census honest is the fixture. Every type
// section 1 names is one of the six scalars, one of the two collections, or a
// declaration in the same file, so a seventh scalar appearing in the spec fails
// here rather than parsing quietly as a name that will resolve to nothing.
//
// The oracle is the literal above, not the map under test. Reading `scalarTypes`
// here would make widening it *weaken* the check, which is exactly how this
// test used to pass a mutation that added `float` and `datetime`.
func TestSpecSubsetNamesNoTypeItDoesNotDeclare(t *testing.T) {
	_, file := parseClean(t, readFixture(t, specSubsetFixture))

	known := map[string]bool{}
	for _, name := range wantScalarTypes {
		known[name] = true
	}
	for name := range wantCollectionTypes {
		known[name] = true
	}
	for _, d := range file.Decls {
		switch d := d.(type) {
		case *Struct:
			known[d.Name.Text] = true
		case *Record:
			known[d.Name.Text] = true
		case *Table:
			known[d.Name.Text] = true
		case *Enum:
			known[d.Name.Text] = true
		}
	}

	// A collection's arguments are types too, and `list<SomethingUndeclared>`
	// has to fail here for the same reason the bare name would.
	var check func(owner string, t2 TypeRef)
	check = func(owner string, t2 TypeRef) {
		if !known[t2.Name.Text] {
			t.Errorf("field %s names %s, which is neither a scalar, a collection, nor declared in the fixture",
				owner, t2.Name.Text)
		}
		for _, arg := range t2.Args {
			check(owner, arg)
		}
	}
	for _, d := range file.Decls {
		for _, m := range declMembers(d) {
			f, ok := m.(*Field)
			if !ok {
				continue
			}
			check(f.Name.Text, f.Type)
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
// verbatim, in source order, and both of the identical ones on `x` are kept,
// because protopc does not know what any of it would mean to the backend that
// reads it.
//
// Every run here is written in reverse alphabetical order on purpose. A stage
// that sorted annotations -- or that collected them into a map and wrote them
// back out -- would be invisible to a fixture whose runs happen to be sorted
// already, and the order a C# attribute list is emitted in is a diff in the
// generated file.
func TestParseAnnotations(t *testing.T) {
	_, file := parseClean(t, `revision 1;
namespace A;
record R
  [cs: DataCacheContract(typeof(SportGameCacheContract), Role = ContractRole.Producer)]
  [cs: DataCache(CacheClassName = nameof(BPSportGameCache))]
{
  int64 event_id  = 1 [references(SportGame)];
  int64 timestamp = 2 [cs: DataCacheIgnoreEquality] [cs: DataCacheFromTimestamp];
  int64 x         = 3 [cs: A] [cs: A];
  int64 y         = 4 [cs: Zeta] [cs: Mid] [cs: Alpha];
}
`)
	assertDump(t, file, `revision 1
namespace A
record R
  [cs: DataCacheContract(typeof(SportGameCacheContract), Role = ContractRole.Producer)]
  [cs: DataCache(CacheClassName = nameof(BPSportGameCache))]
  int64 event_id = 1 [references(SportGame)]
  int64 timestamp = 2 [cs: DataCacheIgnoreEquality] [cs: DataCacheFromTimestamp]
  int64 x = 3 [cs: A] [cs: A]
  int64 y = 4 [cs: Zeta] [cs: Mid] [cs: Alpha]
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
  list<StaticDataRef> runners = 47;
}
`)
	assertDump(t, file, `revision 1
namespace A
table SportGame
  [cs: DataCacheTopic(TopicNames.BrandFromConfigs.SportGame)]
  reserved 8..14, 16..23, 25
  key int64 event_id = 1
  int64? home_team_id = 5
  list<StaticDataRef> runners = 47
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
//
// A composite key and an alias each take annotations in the one position every
// member takes them in -- last, before the `;`. Both are emitted members and
// both reach a backend, so an annotation dropped on either is an attribute
// missing from the generated C#.
func TestParseCompositeKeyAndAlias(t *testing.T) {
	_, file := parseClean(t, `revision 1;
namespace A;
table BetType {
  key key = 1 (event_id, bet_type_id) [cs: Z];

  int64 event_id = 2 [references(BetType)];
  int64 id       = 3;

  alias bet_type_id = id [cs: W];
}
`)
	assertDump(t, file, `revision 1
namespace A
table BetType
  key key = 1 (event_id, bet_type_id) [cs: Z]
  int64 event_id = 2 [references(BetType)]
  int64 id = 3
  alias bet_type_id = id [cs: W]
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

// The tag census skips 0, and that is not an oversight. A tag that failed to
// parse leaves behind a zero the member never wrote, and two of those are not
// two members fighting over one position -- calling them duplicates would bury
// the mistake that made them.
func TestFailedAndZeroTagsAreNotDuplicates(t *testing.T) {
	for _, tt := range []struct {
		name      string
		src       string
		wantCodes []diag.Code
	}{
		{
			name:      "tags that failed to parse",
			src:       "revision 1;\nnamespace A;\nstruct S {\n  int64 a = ;\n  int64 b = ;\n}\n",
			wantCodes: []diag.Code{diag.UnexpectedToken, diag.UnexpectedToken},
		},
		{
			name:      "tags written as 0",
			src:       "revision 1;\nnamespace A;\nstruct S {\n  int64 a = 0;\n  int64 b = 0;\n}\n",
			wantCodes: []diag.Code{diag.TagOutOfRange, diag.TagOutOfRange},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, _, bag := parse(t, tt.src)
			var got []diag.Code
			for _, d := range bag.All() {
				got = append(got, d.Code)
			}
			if !slices.Equal(got, tt.wantCodes) {
				t.Errorf("codes = %v, want %v:\n%s", got, tt.wantCodes, diag.RenderAll(f, bag))
			}
		})
	}
}

// Spans, for every node that has one.
//
// A span is the parser's whole contribution to every message a user ever
// reads: a diagnostic, an editor's squiggle and a quoted line are all a span
// sliced back out of the source. A span that is short by a token is not caught
// by any test that only looks at the tree, and it is wrong in every message
// that quotes it.
//
// Every expectation below is written as the text it must slice to. A
// hand-computed byte offset would be a second implementation of the same
// arithmetic, and would agree with a mistake.
func TestSpansCoverWhatWasWritten(t *testing.T) {
	src := `revision 1;
namespace A.B;

enum E {
  UNKNOWN = 0;
  DONE    = 1;
}

table T
  [cs: Topic("t")]
{
  reserved 8..14, 25;
  key key = 1 (a, b) [cs: Z];
  list<Odd?>? odds = 2 [references(X)];
  decimal(4) amount = 3;
  list<Odd> plain = 4;
  alias d = odds;
}
`
	f, file := parseClean(t, src)

	// A span slices to what it claims to cover, named by what it is so a
	// failure says which node lost its ends.
	check := func(what string, span source.Span, want string) {
		t.Helper()
		if got := f.Slice(span); got != want {
			t.Errorf("%s spans %q, want %q", what, got, want)
		}
	}

	// The file is the whole file, which is what makes a diagnostic with no
	// better place to point still land inside the source.
	check("the file", file.Span, src)
	check("revision", file.Revision.Span, "revision 1;")
	check("namespace", file.Namespace.Span, "namespace A.B;")

	e, ok := file.Decls[0].(*Enum)
	if !ok {
		t.Fatalf("first declaration is %T, want *Enum", file.Decls[0])
	}
	check("the enum", e.Span, "enum E {\n  UNKNOWN = 0;\n  DONE    = 1;\n}")
	check("an enum member", e.Members[0].Span, "UNKNOWN = 0;")
	check("an enum member's name", e.Members[0].Name.Span, "UNKNOWN")
	check("an enum member's number", e.Members[0].Number.Span, "0")

	tbl, ok := file.Decls[1].(*Table)
	if !ok {
		t.Fatalf("second declaration is %T, want *Table", file.Decls[1])
	}
	// A declaration opens at its keyword and closes at its brace, and its
	// annotations are inside that even though they are written before the
	// brace rather than after it.
	check("the table", tbl.Span, src[strings.Index(src, "table T"):strings.LastIndex(src, "}")+1])
	check("a head annotation", tbl.Annotations[0].Span, `[cs: Topic("t")]`)

	res, ok := tbl.Members[0].(*Reserved)
	if !ok {
		t.Fatalf("the first member is %T, want *Reserved", tbl.Members[0])
	}
	check("reserved", res.Span, "reserved 8..14, 25;")
	check("a reserved range", res.Ranges[0].Span, "8..14")
	// A single tag is a range whose ends are equal, and its span is the one
	// number that was written -- not a `..` the source does not contain.
	check("a single reserved tag", res.Ranges[1].Span, "25")

	k, ok := tbl.Members[1].(*CompositeKey)
	if !ok {
		t.Fatalf("the second member is %T, want *CompositeKey", tbl.Members[1])
	}
	check("a composite key", k.Span, "key key = 1 (a, b) [cs: Z];")
	check("a composite key's name", k.Name.Span, "key")
	check("a composite key's part", k.Parts[1].Span, "b")
	check("a composite key's annotation", k.Annotations[0].Span, "[cs: Z]")

	odds, ok := tbl.Members[2].(*Field)
	if !ok {
		t.Fatalf("the third member is %T, want *Field", tbl.Members[2])
	}
	check("a field", odds.Span, "list<Odd?>? odds = 2 [references(X)];")
	// A collection's type closes at its `>`, and its `?` after that. This is
	// the assertion that says the outer `?` of `list<Odd?>?` belongs to the
	// outer type: the two are different types and a message that quotes the
	// wrong one says the wrong thing.
	check("a collection type", odds.Type.Span, "list<Odd?>?")
	check("a type argument", odds.Type.Args[0].Span, "Odd?")
	check("an unprefixed annotation", odds.Annotations[0].Span, "[references(X)]")

	amount, ok := tbl.Members[3].(*Field)
	if !ok {
		t.Fatalf("the fourth member is %T, want *Field", tbl.Members[3])
	}
	// A precision is part of the type, and is also its own span because the
	// diagnostic for a precision that does not belong underlines it alone.
	check("a scaled type", amount.Type.Span, "decimal(4)")
	check("a precision", amount.Type.Precision.Span, "4")
	check("a tag", amount.Tag.Span, "3")

	plain, ok := tbl.Members[4].(*Field)
	if !ok {
		t.Fatalf("the fifth member is %T, want *Field", tbl.Members[4])
	}
	// The same collection with no `?` after it, which is the one shape that
	// says the span reaches the `>` on its own rather than being carried there
	// by a trailing presence marker.
	check("a collection type with no presence marker", plain.Type.Span, "list<Odd>")

	alias, ok := tbl.Members[5].(*Alias)
	if !ok {
		t.Fatalf("the sixth member is %T, want *Alias", tbl.Members[5])
	}
	check("an alias", alias.Span, "alias d = odds;")
	check("an alias source", alias.Source.Span, "odds")
}

// `key` is written in front of the type, so a member that carries it has to
// span from the `key` and not from the type -- a stage that quotes the member
// by span has to get back the line that was written. This is the last leading
// word there is: `repeated` used to be the other one and is now part of the
// type, which is exactly why the extension still has to be pinned.
func TestAKeyFieldSpansFromItsLeadingWord(t *testing.T) {
	f, file := parseClean(t, `revision 1;
namespace A;
table T {
  key int64 id = 1;
}
`)
	id, ok := file.Decls[0].(*Table).Members[0].(*Field)
	if !ok {
		t.Fatalf("the member is %T, want *Field", file.Decls[0].(*Table).Members[0])
	}
	if got := f.Slice(id.Span); got != "key int64 id = 1;" {
		t.Errorf("the field spans %q, want %q", got, "key int64 id = 1;")
	}
	// And the type is the type alone, which is what the second half of the
	// extension is for: the field grew at the front and the type did not.
	if got := f.Slice(id.Type.Span); got != "int64" {
		t.Errorf("the type spans %q, want %q", got, "int64")
	}
}

// The three spellings of a view member, and the one position an annotation is
// written in. A view member reaches a backend as a property of the generated
// class exactly as a field does, so an annotation dropped from one is an
// attribute missing from the generated C#.
//
// The runs here are written in reverse alphabetical order on purpose, for the
// same reason TestParseAnnotations writes its runs that way: a stage that
// sorted them would be invisible to a fixture that was sorted already.
func TestParseViewMemberSpellings(t *testing.T) {
	_, file := parseClean(t, `revision 1;
namespace A;
view SlimBetType of BetType
  [cs: DataCache(CacheClassName = nameof(BPSlimBetTypeCache))]
{
  key as id;
  event_id;
  id as bet_type_id [cs: DataCacheIgnoreEquality];
  SlimOdd odds [cs: Zeta] [cs: Alpha];
}
`)
	assertDump(t, file, `revision 1
namespace A
view SlimBetType of BetType
  [cs: DataCache(CacheClassName = nameof(BPSlimBetTypeCache))]
  key as id
  event_id
  id as bet_type_id [cs: DataCacheIgnoreEquality]
  SlimOdd odds [cs: Zeta] [cs: Alpha]
`)

	v, ok := file.Decls[0].(*View)
	if !ok {
		t.Fatalf("declaration is %T, want *View", file.Decls[0])
	}
	if v.Name.Text != "SlimBetType" || v.Source.Text != "BetType" {
		t.Errorf("head is `view %s of %s`, want `view SlimBetType of BetType`", v.Name.Text, v.Source.Text)
	}
	// A plain selection is one ident: no type, because the type comes from the
	// source, and no tag, because writing one would be the error.
	if m := v.Members[1]; m.Narrow != nil || m.As != nil || m.Name.Text != "event_id" {
		t.Errorf("`event_id;` read as %+v, want a bare selection", m)
	}
	// A rename carries both names. The one on the left is what the source
	// calls the member; the one on the right is what this view calls it.
	if m := v.Members[2]; m.Narrow != nil || m.As == nil || m.Name.Text != "id" || m.As.Text != "bet_type_id" {
		t.Errorf("`id as bet_type_id;` read as %+v, want `id` renamed to `bet_type_id`", m)
	}
	// A narrowing's leading ident is the view, and the trailing one is still
	// the member selected out of the source -- not the other way round.
	if m := v.Members[3]; m.Narrow == nil || m.Narrow.Text != "SlimOdd" || m.Name.Text != "odds" {
		t.Errorf("`SlimOdd odds;` read as %+v, want `odds` narrowed to `SlimOdd`", m)
	}
}

// Nothing a view body contains is a reserved word, and this is where that is
// cashed in. `of` and `as` read as keywords in exactly two positions and are
// idents everywhere else; `key`, `alias` and `reserved` are the words section 1
// spells members with and are selected here by name.
//
// `as as of;` and `SlimOdd as;` are the pair that says the decision is
// positional rather than textual: in the first, a member named `as` is renamed
// to `of`; in the second, a member named `as` is narrowed. Only the ident after
// the second word tells a rename from a narrowing, and a lexer with keyword
// kinds could not read either line at all.
func TestViewMemberWordsAreNotKeywords(t *testing.T) {
	_, file := parseClean(t, `revision 1;
namespace A;
view V of of {
  of;
  as;
  as as of;
  key;
  key as id;
  alias;
  reserved;
  view;
  import;
  SlimOdd as;
}
`)
	assertDump(t, file, `revision 1
namespace A
view V of of
  of
  as
  as as of
  key
  key as id
  alias
  reserved
  view
  import
  SlimOdd as
`)

	v := file.Decls[0].(*View)
	if v.Source.Text != "of" {
		t.Errorf("the source is %q, want the declaration named `of`", v.Source.Text)
	}
	// `as as of;` is a rename: the middle word is the one in keyword position.
	if m := v.Members[2]; m.Narrow != nil || m.As == nil || m.Name.Text != "as" || m.As.Text != "of" {
		t.Errorf("`as as of;` read as %+v, want the member `as` renamed to `of`", m)
	}
	// `SlimOdd as;` is a narrowing of a member named `as`, which is the same
	// two words in the same order as a rename minus the third.
	if m := v.Members[9]; m.Narrow == nil || m.Narrow.Text != "SlimOdd" || m.Name.Text != "as" || m.As != nil {
		t.Errorf("`SlimOdd as;` read as %+v, want `as` narrowed to `SlimOdd`", m)
	}
}

// Spans for the nodes this milestone adds, written as the text each one must
// slice back to -- a hand-computed offset would be a second implementation of
// the same arithmetic and would agree with a mistake.
func TestViewAndImportSpansCoverWhatWasWritten(t *testing.T) {
	src := `revision 1;
namespace A;

import "prague.protop";

view SlimBetType of BetType
  [cs: DataCache]
{
  key as id;
  event_id;
  SlimOdd odds [cs: Z];
}
`
	f, file := parseClean(t, src)

	check := func(what string, span source.Span, want string) {
		t.Helper()
		if got := f.Slice(span); got != want {
			t.Errorf("%s spans %q, want %q", what, got, want)
		}
	}

	im := file.Imports[0]
	check("an import", im.Span, `import "prague.protop";`)
	// The path's span is the literal and its quotes, which is what the lexer
	// emits and what a message about an unresolvable path has to underline.
	check("an import path", im.Path.Span, `"prague.protop"`)
	// And the text is that literal as written. Nothing in the frontend decodes
	// a string, so half-decoding one -- quotes off, escapes untouched -- would
	// hand a later stage something it could not tell from a raw literal.
	if got := im.Path.Text; got != `"prague.protop"` {
		t.Errorf("the path is %s, want %s", got, `"prague.protop"`)
	}

	v, ok := file.Decls[0].(*View)
	if !ok {
		t.Fatalf("declaration is %T, want *View", file.Decls[0])
	}
	check("a view", v.Span, src[strings.Index(src, "view SlimBetType"):strings.LastIndex(src, "}")+1])
	check("a view's name", v.Name.Span, "SlimBetType")
	check("a view's source", v.Source.Span, "BetType")
	check("a view's head annotation", v.Annotations[0].Span, "[cs: DataCache]")

	check("a renamed member", v.Members[0].Span, "key as id;")
	check("a renamed member's new name", v.Members[0].As.Span, "id")
	check("a plain selection", v.Members[1].Span, "event_id;")
	check("a narrowed member", v.Members[2].Span, "SlimOdd odds [cs: Z];")
	// The narrowing is the leading ident alone: the diagnostic for a scalar
	// written there underlines that word and not the member.
	check("a narrowing", v.Members[2].Narrow.Span, "SlimOdd")
	check("a view member's annotation", v.Members[2].Annotations[0].Span, "[cs: Z]")
}

// Imports are a file property and are held in source order, which is the order
// a lock file and a diagnostic quoting two of them both want.
func TestImportsAreGroupedAndOrdered(t *testing.T) {
	_, file := parseClean(t, `revision 1;
namespace A;
import "prague.protop";
import "brand.protop";
`)
	assertDump(t, file, `revision 1
namespace A
import "prague.protop"
import "brand.protop"
`)
	if len(file.Decls) != 0 {
		t.Errorf("an import landed in Decls; it is a property of the file, not a declaration")
	}
}

// An import below a declaration is reported and still recorded. The tree is
// best-effort and a later stage reads Imports whatever their position: a file
// that is wrong about where it wrote its dependency is not a file with no
// dependency.
func TestAMisplacedImportIsStillRecorded(t *testing.T) {
	f, file, bag := parse(t, `revision 1;
namespace A;
struct S {
  int64 a = 1;
}
import "prague.protop";
`)
	if bag.Len() != 1 || bag.All()[0].Code != diag.ImportNotInHeader {
		t.Fatalf("got %d diagnostics, want one ImportNotInHeader:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	if len(file.Imports) != 1 || file.Imports[0].Path.Text != `"prague.protop"` {
		t.Errorf("imports are %+v, want the one that was written", file.Imports)
	}
}

// A view member ends with its `;` like every other line in the language, and
// the parser requires one rather than merely accepting one. Nothing wrote a
// view member without its semicolon, so the `;` could be made optional with the
// suite green -- and an optional terminator is not a small thing here, because
// a view body is idents in a row and the semicolons are the only boundary
// between one member and the next.
func TestAViewMemberNeedsItsSemicolon(t *testing.T) {
	f, file, bag := parse(t, `revision 1;
namespace A;
view V of T {
  name
}
`)
	if bag.Len() != 1 || bag.All()[0].Code != diag.UnexpectedToken {
		t.Fatalf("got %d diagnostics, want one UnexpectedToken:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	// Reported where the `;` should have been, which is the token that is not
	// one -- here the brace that ends the body.
	if got := f.Slice(bag.All()[0].Span); got != "}" {
		t.Errorf("reported at %q, want the token that should have been a `;`", got)
	}
	// And the member is still in the tree: it is missing its terminator, not
	// unreadable.
	assertDump(t, file, "revision 1\nnamespace A\nview V of T\n  name\n")
}

// A view body that is never closed is reported. parseTypeDecl and parseEnum
// have the same shape and the struct one is covered by the recovery tests; the
// view one was covered by nothing, so the closing brace could be made optional
// and a file that trails off mid-view would have compiled.
func TestAnUnterminatedViewBodyIsReported(t *testing.T) {
	f, file, bag := parse(t, "revision 1;\nnamespace A;\nview V of T {\n  name;\n")
	if bag.Len() != 1 || bag.All()[0].Code != diag.UnexpectedToken {
		t.Fatalf("got %d diagnostics, want one UnexpectedToken:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	if got := diag.Render(f, bag.All()[0]); !strings.Contains(got, "expected `}`, found end of file") {
		t.Errorf("rendered:\n%s\nwant a message about the missing `}`", got)
	}
	// What was read before the file ran out is kept, which is what lets an
	// editor outline a view that is still being typed.
	assertDump(t, file, "revision 1\nnamespace A\nview V of T\n  name\n")
}

// A view body line that is not a name at all -- the case every other view
// diagnostic misses, because all of them enter parseViewMember with an ident in
// hand. Three separate things are pinned here, each of which could be broken on
// its own with the rest of the suite green:
//
// the offending token is CONSUMED before recovery, without which the parser
// would sit on it; the member is NOT appended, so a nil never reaches the tree
// for a later stage to dereference; and recovery is recoverStmt, so the `}` is
// left for the body loop and the declaration after the view still parses.
func TestAViewBodyLineThatIsNotAName(t *testing.T) {
	f, file, bag := parse(t, `revision 1;
namespace A;
view V of T {
  5;
  name;
}
struct S {
  int64 x = 1;
}
`)
	var got []diag.Code
	for _, d := range bag.All() {
		got = append(got, d.Code)
	}
	if !slices.Equal(got, []diag.Code{diag.UnexpectedToken}) {
		t.Fatalf("codes = %v, want one UnexpectedToken:\n%s", got, diag.RenderAll(f, bag))
	}
	if s := f.Slice(bag.All()[0].Span); s != "5" {
		t.Errorf("the diagnostic underlines %q, want the token that is not a name", s)
	}

	// The members are exactly the ones that were well formed -- one, not two
	// with a nil in it.
	v := file.Decls[0].(*View)
	if len(v.Members) != 1 || v.Members[0] == nil || v.Members[0].Name.Text != "name" {
		t.Errorf("members are %v, want only the line that parsed", v.Members)
	}
	assertDump(t, file, `revision 1
namespace A
view V of T
  name
struct S
  int64 x = 1
`)

	// Without the `;` there is no semicolon for recovery to stop at, which is
	// the input that tells recoverStmt from recoverDecl: the body's `}` has to
	// be left for the body loop. recoverDecl would eat it, and the struct below
	// would be read as more of the view.
	f, file, bag = parse(t, `revision 1;
namespace A;
view V of T {
  5
}
struct S {
  int64 x = 1;
}
`)
	if bag.Len() != 1 || bag.All()[0].Code != diag.UnexpectedToken {
		t.Fatalf("got %d diagnostics, want one UnexpectedToken:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	assertDump(t, file, `revision 1
namespace A
view V of T
struct S
  int64 x = 1
`)
}

// A stray `;` is the one token where it matters that the offending token is
// left for recovery rather than consumed first. Everywhere else the two are the
// same, because recoverStmt consumes anything that is not a boundary -- but a
// `;` IS the boundary, so consuming it sends recovery hunting for the next one
// and the member after the stray semicolon disappears from the tree with
// nothing said about it. One mistake costs one diagnostic and one line.
func TestAStrayTerminatorDoesNotEatTheMemberAfterIt(t *testing.T) {
	f, file, bag := parse(t, "revision 1;\nnamespace A;\nview V of T {\n  ;\n  name;\n}\n")
	if bag.Len() != 1 || bag.All()[0].Code != diag.UnexpectedToken {
		t.Fatalf("got %d diagnostics, want one UnexpectedToken:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	if got := f.Slice(bag.All()[0].Span); got != ";" {
		t.Errorf("the diagnostic underlines %q, want the stray terminator", got)
	}
	assertDump(t, file, "revision 1\nnamespace A\nview V of T\n  name\n")
}

// A tag in a view member ends the member and not the view. Recovery here is
// recoverStmt, which leaves the `}` for the body loop; recoverDecl would
// consume the balanced brace, so the view body would run on into the next
// declaration and swallow it -- one mistake would cost three diagnostics and a
// struct.
//
// The partial member is kept for the same reason a field with a failed tag is:
// the tree is best-effort, and a member that wrote a tag it may not write is
// still a member that was selected.
func TestATagInAViewMemberEndsTheMemberAndNotTheView(t *testing.T) {
	f, file, bag := parse(t, `revision 1;
namespace A;
view V of T {
  a = 1;
  b;
}
struct S {
  int64 x = 1;
}
`)
	if bag.Len() != 1 || bag.All()[0].Code != diag.TagInViewMember {
		t.Fatalf("got %d diagnostics, want one TagInViewMember:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	assertDump(t, file, `revision 1
namespace A
view V of T
  a
  b
struct S
  int64 x = 1
`)
	v := file.Decls[0].(*View)
	if len(v.Members) != 2 || v.Members[0].Name.Text != "a" {
		t.Errorf("members are %v, want the member that wrote the tag and the one after it", v.Members)
	}

	// The same mistake with an annotation on the member, which is where the
	// check used to be in the wrong place. An annotation follows what it
	// describes, so a view member is fully read only once its annotations are;
	// with the tag checked first, the `[` was not an `=`, the member fell
	// through to the annotations, and the `=` after them was reported as a
	// missing `;` -- two PROTOP0200 at one column, and the milestone's
	// best-worded diagnostic never said.
	f, file, bag = parse(t, `revision 1;
namespace A;
view V of T {
  a [cs: X] = 1;
  b;
}
`)
	if bag.Len() != 1 || bag.All()[0].Code != diag.TagInViewMember {
		t.Fatalf("got %d diagnostics, want one TagInViewMember:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	if got := f.Slice(bag.All()[0].Span); got != "=" {
		t.Errorf("the diagnostic underlines %q, want the tag's `=`", got)
	}
	assertDump(t, file, `revision 1
namespace A
view V of T
  a [cs: X]
  b
`)

	// The same mistake without its `;`, which is the input that tells the two
	// recoveries apart: with no semicolon to stop at, recovery runs up to the
	// body's `}` and must LEAVE it for the body loop. recoverDecl consumes the
	// balanced brace, so the view would run on into the struct below and take
	// it with it.
	f, file, bag = parse(t, `revision 1;
namespace A;
view V of T {
  a = 1
}
struct S {
  int64 x = 1;
}
`)
	if bag.Len() != 1 || bag.All()[0].Code != diag.TagInViewMember {
		t.Fatalf("got %d diagnostics, want one TagInViewMember:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	assertDump(t, file, `revision 1
namespace A
view V of T
  a
struct S
  int64 x = 1
`)
}

// A view whose head is unreadable is still a declaration in the tree. The
// parser appends what it read and returns -- ast.go and the parser's own
// preamble both rest on that -- so an editor outlining a file gets the view's
// name even though the view names no source, and bag.HasErrors is what stops
// anything downstream trusting it.
func TestParseKeepsASourcelessViewInTheTree(t *testing.T) {
	f, file, bag := parse(t, "revision 1;\nnamespace A;\nview V {\n  name;\n}\n")
	if !bag.HasErrors() {
		t.Fatal("a view with no source was accepted")
	}
	if bag.Len() != 1 || bag.All()[0].Code != diag.ViewSourceMissing {
		t.Fatalf("got %d diagnostics, want one ViewSourceMissing:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	if len(file.Decls) != 1 {
		t.Fatalf("decls = %d, want the view that was written", len(file.Decls))
	}
	v, ok := file.Decls[0].(*View)
	if !ok {
		t.Fatalf("declaration is %T, want *View", file.Decls[0])
	}
	if v.Name.Text != "V" {
		t.Errorf("the view is named %q, want V", v.Name.Text)
	}
	if v.Source.Text != "" {
		t.Errorf("the source is %q, want nothing; there was none to read", v.Source.Text)
	}
}

// The narrowing position takes a view, and the six scalars are the one set the
// parser can rule out by itself. The boundary is tested from both sides,
// because it was untested in both: a check that knew only `int64` passed, a
// check that exempted `decimal` passed, and a check that ALSO rejected `list`
// and `map` passed -- and that last one would have made `list<Odd>` narrowings
// unwritable the day M4 decides what they mean.
//
// The six come from the literal oracle, not from scalarTypes. Reading the map
// under test would make widening it weaken the check, which is precisely the
// tautology M2 had to unpick.
func TestEveryScalarIsRejectedInANarrowingPosition(t *testing.T) {
	for _, scalar := range wantScalarTypes {
		t.Run(scalar, func(t *testing.T) {
			f, file, bag := parse(t, "revision 1;\nnamespace A;\nview V of T {\n  "+scalar+" x;\n}\n")
			var got []diag.Code
			for _, d := range bag.All() {
				got = append(got, d.Code)
			}
			if !slices.Equal(got, []diag.Code{diag.ScalarInViewMember}) {
				t.Fatalf("codes = %v, want one ScalarInViewMember:\n%s", got, diag.RenderAll(f, bag))
			}
			if s := f.Slice(bag.All()[0].Span); s != scalar {
				t.Errorf("the diagnostic underlines %q, want %q", s, scalar)
			}
			// The line is still read: what is wrong is the narrowing, not the
			// member it narrows.
			v := file.Decls[0].(*View)
			if len(v.Members) != 1 || v.Members[0].Name.Text != "x" {
				t.Errorf("members are %v, want the member that was written", v.Members)
			}
		})
	}

	// A collection is not a scalar and is not rejected here. Whether `list` can
	// be narrowed to is a question about what `list` resolves to, which is not
	// a question this file is allowed to ask.
	_, file := parseClean(t, "revision 1;\nnamespace A;\nview V of T {\n  list odds;\n  map m;\n}\n")
	assertDump(t, file, "revision 1\nnamespace A\nview V of T\n  list odds\n  map m\n")
}

// peekAt clamps to the EOF token the lexer always emits, and the suite never
// put a two-token lookahead within two tokens of the end of the file. The
// composite-key lookahead is the one that reaches furthest -- `key` decides
// which spelling it is by looking at the token after next -- so a file that
// stops on the word `key` is what walks off the end.
//
// Without the clamp this input panics with an index out of range, which is a
// crash rather than a diagnostic and is the one failure mode a compiler may not
// have on malformed input. It is also exactly what a file looks like halfway
// through being typed.
func TestLookaheadPastTheEndOfTheFileDoesNotPanic(t *testing.T) {
	f, file, bag := parse(t, "revision 1;\nnamespace A;\ntable T {\n  key")
	if !bag.HasErrors() {
		t.Fatal("a file that stops mid-member was accepted")
	}
	var got []diag.Code
	for _, d := range bag.All() {
		got = append(got, d.Code)
	}
	if !slices.Equal(got, []diag.Code{diag.UnexpectedToken, diag.UnexpectedToken}) {
		t.Fatalf("codes = %v, want the member and the body each reporting what ran out:\n%s", got, diag.RenderAll(f, bag))
	}
	// The table is in the tree with no members, which is all there was.
	assertDump(t, file, "revision 1\nnamespace A\ntable T\n")
}

// A view member is idents and a `;`, and the count of them is what tells the
// four shapes apart: one name selects, two narrow, three are a rename when the
// middle one is the word `as` with a name after it, and four narrow and rename
// at once on that same condition read one word along. Nothing wrote a member
// with the wrong number of words, so the arithmetic was free -- and getting it
// wrong does not produce a diagnostic, it produces a DIFFERENT PROGRAM. With
// the `as` test dropped from the rename lookahead, `event_id ass id;` compiles
// as a rename and carries the wrong name into every backend; with the narrowing
// read in a loop, `a b c d;` collapses to a member named `d` and the rest is
// thrown away in silence.
//
// The fourth shape itself is `SlimOdd odds as o;` and is held by
// TestANarrowedViewMemberMayAlsoBeRenamed. What is here is the arithmetic that
// tells it apart from the four-ident line that is not one.
func TestAViewMemberIsOneOfFourShapes(t *testing.T) {
	// Three names where a rename would have `as` in the middle. The third word
	// is where the line stops making sense, so that is where it is reported.
	f, file, bag := parse(t, `revision 1;
namespace A;
view V of T {
  a b c;
  keep;
}
struct S {
  int64 x = 1;
}
`)
	if bag.Len() != 1 || bag.All()[0].Code != diag.UnexpectedToken {
		t.Fatalf("got %d diagnostics, want one UnexpectedToken:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	if got := f.Slice(bag.All()[0].Span); got != "c" {
		t.Errorf("the diagnostic underlines %q, want the word the line stops at", got)
	}
	assertDump(t, file, `revision 1
namespace A
view V of T
  a b
  c
  keep
struct S
  int64 x = 1
`)

	// Four idents that are not the fourth shape: the third word is not `as`,
	// so this is a narrowing and then a word too many. It is also what a
	// narrowing read in a loop would swallow whole.
	f, file, bag = parse(t, `revision 1;
namespace A;
view V of T {
  a b c d;
  keep;
}
`)
	if bag.Len() != 1 || bag.All()[0].Code != diag.UnexpectedToken {
		t.Fatalf("got %d diagnostics, want one UnexpectedToken:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	assertDump(t, file, `revision 1
namespace A
view V of T
  a b
  c d
  keep
`)

	// The typo the rename lookahead exists for. `ass` is not `as`, so this is a
	// narrowing followed by a word too many -- not a rename that quietly
	// renames `event_id` to `id` and takes that name downstream.
	f, file, bag = parse(t, "revision 1;\nnamespace A;\nview V of T {\n  event_id ass id;\n}\n")
	if bag.Len() != 1 || bag.All()[0].Code != diag.UnexpectedToken {
		t.Fatalf("got %d diagnostics, want one UnexpectedToken:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	v := file.Decls[0].(*View)
	if v.Members[0].As != nil {
		t.Errorf("`event_id ass id;` was read as a rename to %q", v.Members[0].As.Text)
	}

	// And the other half of the lookahead: `as` in the middle with no name
	// after it is not a rename either. Without the name checked, the `;` is
	// read as the new name and its text goes into the tree.
	f, file, bag = parse(t, "revision 1;\nnamespace A;\nview V of T {\n  a b as;\n}\n")
	if bag.Len() != 1 || bag.All()[0].Code != diag.UnexpectedToken {
		t.Fatalf("got %d diagnostics, want one UnexpectedToken:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	if got := f.Slice(bag.All()[0].Span); got != "as" {
		t.Errorf("the diagnostic underlines %q, want the `as` that renames nothing", got)
	}
	v = file.Decls[0].(*View)
	if v.Members[0].As != nil {
		t.Errorf("`a b as;` renamed to %q; there was no name to rename to", v.Members[0].As.Text)
	}
}

// section5NarrowAndRename is the rule the test below rests on, quoted out of
// section 5 and held against it by TestSection5StatesTheNarrowAndRename. The
// span is chosen for what it carries rather than for its length: the rule
// itself, the example this file parses, and the tag -- 26, BetType's, and
// neither half of the member's doing. Each of the three can be falsified on its
// own, and nothing else in the suite would notice; `26` changed to `1` reads as
// a spec that says the member may invent a number after all.
//
// It is quoted once, here, rather than in the comment below it as well. Two
// copies of a sentence in one file are two things to keep true, and the one
// that is not compared to anything is the one that goes stale.
const section5NarrowAndRename = "**A view may narrow and rename in one member.** " +
	"`SlimOdd odds as o;` is still tag 26: the leading type says what the member holds, " +
	"`as o` says what the consumer calls it, and the tag is the source's as it is on " +
	"every other view line — neither half is about the tag."

// The citation, checked. A comment may quote the spec; only a test can say the
// spec still reads that way.
func TestSection5StatesTheNarrowAndRename(t *testing.T) {
	assertSpecStates(t, specSection5, section5NarrowAndRename)
}

// Narrowing and renaming at once, which section 5 now rules in -- the rule is
// section5NarrowAndRename above, quoted from the section and pinned to it.
//
// The parser accepted the combination before the section said so, because the
// shapes compose by construction -- a narrowing is two idents, a rename is `as`
// and a third -- and refusing a combination the spec did not forbid would have
// been this file deciding a question that was not its to decide. The question
// is decided now, and decided the same way, so the absence of a diagnostic here
// is a rule rather than a silence.
//
// What this test holds is the shape the parser read. What the line resolves to
// -- the name from the rename, the type from the narrowing, the tag from the
// source -- is held on the model by internal/check's
// TestAViewMemberMayNarrowAndRenameAtOnce.
func TestANarrowedViewMemberMayAlsoBeRenamed(t *testing.T) {
	_, file := parseClean(t, "revision 1;\nnamespace A;\nview V of T {\n  SlimOdd odds as o;\n}\n")
	assertDump(t, file, "revision 1\nnamespace A\nview V of T\n  SlimOdd odds as o\n")
}

// assertRenamed holds a fixture to the thing it was written to ask about. Each
// of these fixtures is a bad view member with a rename on it, and the rename is
// the whole point: without it the fixture is a mistake the un-renamed spellings
// already diagnose and already test, so the case would keep its name while
// asking a question answered elsewhere -- and the one-token guard it exists to
// catch would survive again with the suite green.
func assertRenamed(t *testing.T, src string) {
	t.Helper()
	if !strings.Contains(src, " as o") {
		t.Fatalf("the fixture carries no rename, so it no longer asks what the test is named for:\n%s", src)
	}
}

// The same two diagnostics this file already reports on a view member, with a
// rename on the member that earns them. A rename is the last thing read before
// either check runs -- the `= 3` is looked for after it, and the narrowing is
// checked with the parser already sitting on the `as` -- so both are one
// condition away from being asked only when no rename is present. Section 5
// rules the combination in; it does not exempt it from anything.
func TestAViewMemberDiagnosticSurvivesARename(t *testing.T) {
	// A tag is a tag whatever the member is called. Gating the `=` on the
	// absence of a rename reports a missing `;` instead, and the milestone's
	// best-worded diagnostic never gets said.
	const tagged = "revision 1;\nnamespace A;\nview V of T {\n  SlimOdd odds as o = 3;\n}\n"
	assertRenamed(t, tagged)
	f, _, bag := parse(t, tagged)
	if bag.Len() != 1 || bag.All()[0].Code != diag.TagInViewMember {
		t.Fatalf("got %d diagnostics, want one TagInViewMember:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	if got := f.Slice(bag.All()[0].Span); got != "=" {
		t.Errorf("the diagnostic underlines %q, want the tag's `=`", got)
	}

	// And a scalar in the narrowing position is a scalar whatever the member
	// is called. The rename is read after the narrowing and says nothing about
	// what the leading word is allowed to be.
	const scalar = "revision 1;\nnamespace A;\nview V of T {\n  int64 odds as o;\n}\n"
	assertRenamed(t, scalar)
	f, _, bag = parse(t, scalar)
	if bag.Len() != 1 || bag.All()[0].Code != diag.ScalarInViewMember {
		t.Fatalf("got %d diagnostics, want one ScalarInViewMember:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	if got := f.Slice(bag.All()[0].Span); got != "int64" {
		t.Errorf("the diagnostic underlines %q, want the scalar in the narrowing position", got)
	}
}

// A view member's name carries the span of the name itself, on both the plain
// and the narrowed path. That is the span the next stage underlines for "no
// such member `odds` on `SportGame`", and a resolver has no other way to point
// at the word it could not find: the member's own span covers the narrowing and
// the rename too, so quoting it would underline the whole line for a mistake in
// one word of it.
func TestAViewMemberNameCarriesItsOwnSpan(t *testing.T) {
	f, file := parseClean(t, "revision 1;\nnamespace A;\nview V of T {\n  sport_type_id;\n  SlimOdd odds;\n}\n")
	v := file.Decls[0].(*View)
	if len(v.Members) != 2 {
		t.Fatalf("members = %d, want 2", len(v.Members))
	}
	for i, want := range []string{"sport_type_id", "odds"} {
		if got := f.Slice(v.Members[i].Name.Span); got != want {
			t.Errorf("member %d's name spans %q, want %q", i, got, want)
		}
	}
	// The narrowing's own span is the narrowing, and the member's span is the
	// line: three spans, three jobs.
	if got := f.Slice(v.Members[1].Narrow.Span); got != "SlimOdd" {
		t.Errorf("the narrowing spans %q, want SlimOdd", got)
	}
	if got := f.Slice(v.Members[1].Span); got != "SlimOdd odds;" {
		t.Errorf("the member spans %q, want the whole line", got)
	}
}

// Recovery from a malformed view HEAD, which is the half of recovery that
// e8e3033 did not reach: it pinned what happens after a bad line inside a body,
// and left what happens after a bad line before one.
//
// The distinction is recoverDecl against recoverStmt and it is invisible to a
// count: the diagnostic is the same either way, and only the declarations after
// the view tell the two apart. recoverDecl stops at the brace that closes what
// it entered; recoverStmt keeps going to the next `;` at depth zero, which is
// somewhere inside the next declaration -- so the file after a single typo'd
// view head quietly loses a struct, and nothing that counts errors can see it.
func TestAMalformedViewHeadStopsAtItsOwnBody(t *testing.T) {
	const tail = `struct S {
  int64 x = 1;
}
`
	for _, tt := range []struct {
		name string
		src  string
		code diag.Code
	}{
		{"no name", "view {\n  a;\n}\n", diag.UnexpectedToken},
		{"no source", "view V {\n  a;\n}\n", diag.ViewSourceMissing},
		// An unterminated annotation on the head stops at the `}` that closes
		// the body it sits in -- scanAnnotation will not swallow a brace it did
		// not open -- so recovery starts ON that brace and has to consume it.
		{"unterminated annotation", "view V of T [cs: X\n  a;\n}\n", diag.UnterminatedAnnotation},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f, file, bag := parse(t, "revision 1;\nnamespace A;\n"+tt.src+tail)
			var got []diag.Code
			for _, d := range bag.All() {
				got = append(got, d.Code)
			}
			if !slices.Equal(got, []diag.Code{tt.code}) {
				t.Fatalf("codes = %v, want just %v:\n%s", got, tt.code, diag.RenderAll(f, bag))
			}
			if len(file.Decls) != 2 {
				t.Fatalf("decls = %d, want the view and the struct after it; the file below a bad head is not part of it", len(file.Decls))
			}
			if _, ok := file.Decls[1].(*Struct); !ok {
				t.Errorf("the declaration after the view is %T, want *Struct", file.Decls[1])
			}
		})
	}

	// A head that goes wrong before its `{` has no body brace to stop at, so
	// recovery runs to the end of the NEXT declaration and that one is lost.
	// That is the cost of the mistake and it is bounded: the declaration after
	// that still parses. Under recoverStmt there is no bound -- it would
	// consume every brace it met and take the rest of the file with it.
	f, file, bag := parse(t, `revision 1;
namespace A;
view V of T x
struct S {
  int64 x = 1;
}
enum E {
  ONE = 1;
}
`)
	if bag.Len() != 1 || bag.All()[0].Code != diag.UnexpectedToken {
		t.Fatalf("got %d diagnostics, want one UnexpectedToken:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	assertDump(t, file, `revision 1
namespace A
view V of T
enum E
  ONE = 1
`)
}

// The same for a malformed import, and the same invisibility: one diagnostic
// either way, and only what survives below it says which recovery ran.
//
// The node is kept whatever happens to the path. A best-effort tree is the
// stated contract, and an import with no readable path is still the statement
// that this file depends on something -- which is what an editor lists, and
// what M4 counts when it decides whether a name could have come from anywhere.
func TestAMalformedImportStopsAtItsOwnLine(t *testing.T) {
	f, file, bag := parse(t, `revision 1;
namespace A;
import }
struct S {
  int64 x = 1;
}
`)
	if bag.Len() != 1 || bag.All()[0].Code != diag.UnexpectedToken {
		t.Fatalf("got %d diagnostics, want one UnexpectedToken:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	if len(file.Imports) != 1 {
		t.Errorf("imports = %d, want the import that was written", len(file.Imports))
	}
	assertDump(t, file, `revision 1
namespace A
import 
struct S
  int64 x = 1
`)

	// Two independent mistakes on one line: the path is not a string, and the
	// import is below a declaration. Neither excuses the other -- a reader who
	// fixes the path has not moved the import, and would find the second
	// diagnostic waiting on the next compile.
	f, file, bag = parse(t, `revision 1;
namespace A;
struct S {
  int64 x = 1;
}
import prague;
`)
	var got []diag.Code
	for _, d := range bag.All() {
		got = append(got, d.Code)
	}
	if !slices.Equal(got, []diag.Code{diag.UnexpectedToken, diag.ImportNotInHeader}) {
		t.Fatalf("codes = %v, want the bad path and the misplaced import:\n%s", got, diag.RenderAll(f, bag))
	}
	if len(file.Imports) != 1 {
		t.Errorf("imports = %d, want the import that was written", len(file.Imports))
	}
}

// A `}` that closes nothing ends the declaration recovery is in, rather than
// putting it into deficit. The depth counter can go negative -- recovery is
// started at a token the parser could not read, which may already be inside a
// body whose `{` was consumed by whatever went wrong -- and a check for exactly
// zero would sail past this brace and every following one, ending only at the
// first `;` that happened to land at depth zero again. That is somewhere in the
// middle of the next declaration, so one stray brace would cost two.
func TestRecoveryEndsAtABraceItDidNotOpen(t *testing.T) {
	f, file, bag := parse(t, `revision 1;
namespace A;
struct S {
  int64 x = 1;
}
}
struct T {
  int64 y = 1;
}
`)
	if bag.Len() != 1 || bag.All()[0].Code != diag.UnknownDeclaration {
		t.Fatalf("got %d diagnostics, want one UnknownDeclaration:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	assertDump(t, file, `revision 1
namespace A
struct S
  int64 x = 1
struct T
  int64 y = 1
`)
}

// Recovery inside a line stops at the line's own `;`, and a `;` nested inside
// braces is not it. Unguarded, the first semicolon anywhere would end recovery
// -- leaving the parser inside a brace run it never left, so the `}` that
// followed would be read as the end of the enclosing body and the rest of that
// body would cascade out into the file.
func TestRecoveryInsideALineIgnoresANestedSemicolon(t *testing.T) {
	f, file, bag := parse(t, `revision 1;
namespace A;
view V of T {
  a = { x; };
  b;
}
struct S {
  int64 x = 1;
}
`)
	if bag.Len() != 1 || bag.All()[0].Code != diag.TagInViewMember {
		t.Fatalf("got %d diagnostics, want one TagInViewMember:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	assertDump(t, file, `revision 1
namespace A
view V of T
  a
  b
struct S
  int64 x = 1
`)
}

// An unterminated annotation on a view member costs one diagnostic and nothing
// else: the member is kept, recovery runs, and the declaration after the view
// still parses. All three are separable and all three were free -- dropping the
// member passed, and skipping recovery passed.
func TestAnUnterminatedAnnotationOnAViewMemberKeepsTheMember(t *testing.T) {
	f, file, bag := parse(t, `revision 1;
namespace A;
view V of T {
  a [cs: X
}
struct S {
  int64 x = 1;
}
`)
	if bag.Len() != 1 || bag.All()[0].Code != diag.UnterminatedAnnotation {
		t.Fatalf("got %d diagnostics, want one UnterminatedAnnotation:\n%s", bag.Len(), diag.RenderAll(f, bag))
	}
	v := file.Decls[0].(*View)
	if len(v.Members) != 1 || v.Members[0].Name.Text != "a" {
		t.Errorf("members are %v, want the member the annotation was written on", v.Members)
	}
	assertDump(t, file, `revision 1
namespace A
view V of T
  a [cs: X]
struct S
  int64 x = 1
`)
}
