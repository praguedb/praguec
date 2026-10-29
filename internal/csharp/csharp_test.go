package csharp

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/nanov/protobuf-to-messagepack-sharp/internal/check"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/diag"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/source"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/syntax"
)

// update rewrites every golden from the emitter's current output.
//
// The flag exists because maintaining C# by hand in a Go string is worse than
// reviewing a diff, and it is dangerous for exactly the same reason: a
// regenerated golden proves only that the emitter agrees with itself.
//
// What -update is and is not backstopped by, stated exactly, because the
// previous version of this comment overclaimed and a mutation run caught it:
//
//   - -update writes nothing outside testdata/golden. So it cannot turn a red
//     corpus assertion green, and every assertion in corpus_test.go is a corpus
//     assertion. Emitting `{ get; init; }` -- the #20 defect the reviewer used
//     to break the old claim -- now fails TestTheEmissionNeedsNoNormalisation
//     before any golden is consulted, because normalisation sites are harvested
//     from the *emission* side as well as the corpus side and the emission's
//     must be empty. Emitting an alias that reads its own name fails
//     TestCorpusMembers. Neither is reachable by regenerating anything.
//
//   - That backstop reaches exactly as far as the coverage table does. A
//     declaration no corpus row reproduces -- presence.protop, the MessagePack
//     namespace fixture, the spec sections beyond the reproduced set -- has no
//     oracle at all, and for those a regenerated golden is a record of what the
//     emitter did, not evidence that it was right. They are small and reviewed,
//     and that is the whole of their warrant.
//
// So: run -update, then read the diff as a proposal rather than as a result.
// The suite failing after it is the signal; the suite passing after it is not.
var update = flag.Bool("update", false, "rewrite the goldens in testdata/golden")

// The two spec sections, under the names the program knows them by. They are
// copies of the frontend's and the checker's fixtures, and copies rather than a
// reach into another package's testdata because testdata belongs to the package
// that uses it; TestSpecFixturesAreVerbatim is what holds them to the spec.
const (
	pragueFixture   = "prague.protop"
	consumerFixture = "consumer.protop"
)

var (
	specSection1 = filepath.Join("..", "..", "docs", "spec", "protop", "shared-repository.md")
	specSection2 = filepath.Join("..", "..", "docs", "spec", "protop", "service-repository.md")
	// Section 3 lends this package no fixture, only the rule its emission obeys.
	// One sentence of it makes a claim about the pin -- see
	// TestSection3sCommaIllustrationIsOneThePinCarries.
	specSection3 = filepath.Join("..", "..", "docs", "spec", "protop", "annotations.md")
)

func readFixture(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// emitFixtures parses and checks a set of fixture files and emits all of it,
// failing the test if the frontend had anything at all to say. A backend test
// is about the backend: a fixture that does not check would be testing the
// checker by accident, and a backend never runs on a schema with errors in the
// first place.
func emitFixtures(t *testing.T, names ...string) []Unit {
	t.Helper()
	files := make(map[string]string, len(names))
	for _, n := range names {
		files[n] = readFixture(t, n)
	}
	return emitSource(t, files)
}

func emitSource(t *testing.T, files map[string]string) []Unit {
	t.Helper()
	return Emit(schemaOf(t, files))
}

func schemaOf(t *testing.T, files map[string]string) *check.Schema {
	t.Helper()
	var bag diag.Bag
	set := make(check.FileSet, len(files))
	var any *source.File
	for path, src := range files {
		f := source.NewFile(path, src)
		any = f
		set[path] = check.Unit{Source: f, File: syntax.Parse(f, &bag)}
	}
	schema := check.Check(set, &bag)
	if bag.Len() != 0 {
		t.Fatalf("fixtures do not check cleanly:\n%s", diag.RenderAll(any, &bag))
	}
	return schema
}

// --- goldens -----------------------------------------------------------------

// assertGoldens holds every emitted unit against one file of its own, and the
// directory against the set of units.
//
// One golden per declaration rather than one per case is the whole point. A
// suite with a single golden catches every mutation with a hundred-line diff
// and names none of them; here a change to how an alias is written fails the
// two declarations that have an alias and nothing else, which is the difference
// between a failure that reports a bug and a failure that reports that
// something changed.
func assertGoldens(t *testing.T, dir string, units []Unit) {
	t.Helper()
	root := filepath.Join("testdata", "golden", dir)
	if *update {
		if err := os.RemoveAll(root); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	want := make([]string, 0, len(units))
	for _, u := range units {
		name := u.Namespace + "." + u.TypeName + ".cs"
		want = append(want, name)
		path := filepath.Join(root, name)
		if *update {
			if err := os.WriteFile(path, []byte(u.Source), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		t.Run(u.TypeName, func(t *testing.T) {
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("no golden for %s: %v (run go test ./internal/csharp -update)", u.TypeName, err)
			}
			if got := u.Source; got != string(b) {
				t.Errorf("emission differs from %s\n--- got ---\n%s\n--- want ---\n%s", path, got, b)
			}
		})
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var have []string
	for _, e := range entries {
		have = append(have, e.Name())
	}
	sort.Strings(have)
	sorted := slices.Clone(want)
	sort.Strings(sorted)
	if !slices.Equal(have, sorted) {
		t.Errorf("testdata/golden/%s holds %v, the schema emits %v", dir, have, sorted)
	}
}

// The spec's own two sections, emitted. Section 1 is the shared repository and
// section 2 is a consumer of it, so between them they carry every settled row
// of the mapping table that a corpus type also carries: a struct, an enum, two
// records, a simple key, a composite key, an alias, a relation, a decimal, a
// list, three views, a rename and a narrowing.
func TestSpecSectionsEmit(t *testing.T) {
	assertGoldens(t, "spec", emitFixtures(t, pragueFixture, consumerFixture))
}

// The other half of "lifted verbatim out of the spec": these fixtures are
// copies of section 1 and section 2, not paraphrases of them.
func TestSpecFixturesAreVerbatim(t *testing.T) {
	assertVerbatim(t, specSection1, pragueFixture)
	assertVerbatim(t, specSection2, consumerFixture)
}

func assertVerbatim(t *testing.T, specPath, fixtureName string) {
	t.Helper()
	fixture := readFixture(t, fixtureName)
	spec, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(spec), fixture) {
		t.Errorf("testdata/%s no longer appears verbatim in %s", fixtureName, specPath)
	}
}

// The sentence of section 1 that says which of the two things the block is:
// a transcription of the pin, or a picture of what the schema ought to carry.
// #43 removed three annotations that were the second kind -- `DataCacheContract`
// and `DataCacheIgnoreEquality` at zero occurrences under corpus/, and
// `DataCacheTopic` in a member-access form where all eleven corpus sites pass a
// string literal -- and the sentence is what keeps the next widening from being
// a judgement call.
//
// The rule itself is measured against the corpus by
// TestSection1AnnotatesOnlyWhatThePinCarries. This pins the statement of it, and
// the two are not substitutes: a rule no reader can find is broken by the next
// person to widen the block, and a rule nothing measures is broken by the next
// `-update`. The seam between them is real and worth knowing -- deleting the
// paragraph fails this test and not that one; widening the block fails that one
// and not this.
//
// This package carries its own copy of the pin helper for the reason it carries
// its own fixtures: what a package rests on belongs to the package resting on
// it.
func TestSpecSection1SaysItTranscribes(t *testing.T) {
	assertSpecStates(t, specSection1, section1Transcribes)
	assertSpecStates(t, specSection1, section1RuleForWhatIsAbsent)
}

// Two paragraphs, and the second is the load-bearing one.
//
// The first says what the block does; TestSection1AnnotatesOnlyWhatThePinCarries
// is what actually holds the block to it, and this pin only keeps the section
// from quietly dropping the claim. The second states the forward-looking rule --
// a form the pin does not yet show belongs in a ticket -- which no test can
// derive from the block, because it is about what is *absent* from it. Deleting
// that paragraph used to pass green; it is pinned here for that reason.
const section1Transcribes = "**This section transcribes; it does not aspire.** " +
	"Every annotation above is written in the form some corpus file writes it, on " +
	"the kind of declaration that file writes it on: the two table heads follow " +
	"`corpus/prague/payloads/BPSportGameCacheItem.cs` and `BPBetTypeCacheItem.cs` " +
	"down to the qualified `nameof`, the record head follows " +
	"`corpus/prague/deps/StaticDataRef.cs`, and the pair on `timestamp` follows " +
	"`corpus/prague/payloads/SlimBetType.cs`. The topic is a string literal, " +
	"because all eleven `[DataCacheTopic]` sites in the pin pass one and not one " +
	"of them names a `TopicNames` constant. Annotations no corpus file carries are " +
	"not shown here at all: `DataCacheContract` and `DataCacheIgnoreEquality` " +
	"stood in this block until #43 measured them at zero occurrences corpus-wide " +
	"and removed them."

const section1RuleForWhatIsAbsent = "The rule that produced those edits is the " +
	"one worth keeping. A form protop should be able to carry, but that the pin " +
	"does not yet show, belongs in a ticket and not in this section — because the " +
	"emission of this block is checked against golden files, and a golden file " +
	"cannot tell aspiration from transcription. It will certify either."

// assertSpecStates is assertVerbatim for prose. A fenced block is a fixture and
// can be held byte for byte; a paragraph is written to be read, and a pin that
// failed on a rewrap would teach the next reader to delete it. Runs of
// whitespace are collapsed on both sides and nothing else is -- not the words,
// not the emphasis, not the backticks and not the numbers -- so a reflow passes
// and an inversion does not.
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

// collapseSpace renders every run of whitespace as one space, which is the whole
// of what a prose pin is allowed to forgive.
func collapseSpace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// Presence is the whole of protop's nullability axis and the reason it is a
// property of the type rather than of the member: `list<Odd>?` and `list<Odd?>`
// are different C#, and a rule that hung the `?` off the member could not tell
// them apart. Nothing in section 1 or section 2 writes either, and nothing in
// them writes a `bool` either, so this fixture exists to carry the rows the
// spec's own text happens not to reach.
func TestPresenceComposes(t *testing.T) {
	assertGoldens(t, "presence", emitFixtures(t, "presence.protop"))
}

// A declaration never writes a using for the namespace it is already in.
//
// The rule and the special case are two different lines of `usings`, and only
// one of them is about MessagePack. This is the rule: an ordinary namespace,
// one declaration naming another in the same file, and the only using emitted
// is the MessagePack one. Deleting the own-namespace seed at csharp.go:112
// emits `using A;` from inside namespace A -- legal C#, and noise in every file
// protopc writes.
//
// It is asserted directly rather than through a golden because a golden is
// regenerable: `-update` would rewrite the expectation to match the defect and
// say nothing. The corpus cannot reach this either -- every cross-namespace
// reference it makes is genuinely cross-namespace.
func TestOwnNamespaceIsNeverAUsing(t *testing.T) {
	units := emitSource(t, map[string]string{
		"a.protop": `revision 1;
namespace A;
record Inner { int64 id = 1; }
table Outer {
  int64 id = 1;
  Inner inner = 2;
  key k = 3 (id);
}
`,
	})
	for _, u := range units {
		got := usingsOf(u.Source)
		if want := []string{"MessagePack"}; strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("%s emits usings %v, want %v -- A is the namespace it is declared in", u.TypeName, got, want)
		}
	}
}

// And the special case: a file whose own namespace *is* MessagePack needs no
// using at all, so the list is empty rather than one short. No corpus file does
// this; the branch is defensive and pinned rather than left unexercised.
func TestTheMessagePackNamespaceNeedsNoUsingOfItself(t *testing.T) {
	assertGoldens(t, "messagepack", emitFixtures(t, "messagepack.protop"))
	for _, u := range emitFixtures(t, "messagepack.protop") {
		if got := usingsOf(u.Source); len(got) != 0 {
			t.Errorf("%s emits usings %v from inside namespace MessagePack, want none", u.TypeName, got)
		}
	}
}

// usingsOf is the using list of an emitted unit, read off the text so that the
// assertion is about what a C# compiler sees rather than about an internal.
func usingsOf(src string) []string {
	var out []string
	for _, ln := range strings.Split(src, "\n") {
		if !strings.HasPrefix(ln, "using ") {
			continue
		}
		out = append(out, strings.TrimSuffix(strings.TrimPrefix(ln, "using "), ";"))
	}
	return out
}

// Emit returns units in the schema's own order: file order, then source order
// within a file. Nothing downstream of a backend sorts them, and #25 is going
// to build file names out of these, so an emission whose order depends on a map
// iteration would give a generator a different diff on every run for no change
// at all. The goldens cannot see this -- assertGoldens keys by declaration name
// -- so it is pinned here.
func TestEmitKeepsTheSchemasOrder(t *testing.T) {
	schema := schemaOf(t, map[string]string{
		"a.protop": `revision 1;
namespace A;
struct First { int64 a = 1; }
struct Second { int64 a = 1; }
struct Third { int64 a = 1; }
`,
	})
	var got []string
	for _, u := range Emit(schema) {
		got = append(got, u.TypeName)
	}
	want := []string{"First", "Second", "Third"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("Emit returned %v, want %v -- source order, not any other order", got, want)
	}
}

// Section 3 gives every member one annotation position, and "every member"
// includes the two that are not fields. The corpus cannot check this: it never
// annotates a composite key -- all five of its keys carry `[DataCacheKey]` and
// nothing else -- so the passthrough on that branch has no oracle and is pinned
// here instead.
//
// The three branches of writeMember each copy annotations with their own line
// of code, which is three places for one rule to be forgotten in. This asserts
// on all three at once for that reason.
func TestEveryMemberKindCarriesItsAnnotations(t *testing.T) {
	units := emitSource(t, map[string]string{
		"a.protop": `revision 1;
namespace A;
table T {
  int64 event_id = 2 [cs: OnAField];
  int64 id       = 3;
  alias other    = id [cs: OnAnAlias];
  key k = 1 (event_id, other) [cs: OnACompositeKey];
}
`,
	})
	src := units[0].Source
	for _, want := range []string{"[OnAField]", "[OnAnAlias]", "[OnACompositeKey]"} {
		if !strings.Contains(src, want) {
			t.Errorf("the emission does not carry %s:\n%s", want, src)
		}
	}
}

// A Unit points back at the declaration it came from. Namespace and TypeName
// are convenience copies; Decl is the model, and #25 is going to need it to
// decide what a file is called and where it goes. An emission that forgot it
// would pass every text comparison in this package.
func TestEveryUnitCarriesItsDeclaration(t *testing.T) {
	schema := schemaOf(t, map[string]string{
		"a.protop": `revision 1;
namespace A;
struct S { int64 a = 1; }
enum E { UNKNOWN = 0; }
`,
	})
	for _, u := range Emit(schema) {
		if u.Decl == nil {
			t.Errorf("%s emitted a Unit with no Decl", u.TypeName)
			continue
		}
		if u.Decl.Name != u.TypeName {
			t.Errorf("Unit %q carries Decl %q", u.TypeName, u.Decl.Name)
		}
		if u.Decl.File.Namespace != u.Namespace {
			t.Errorf("Unit %q has namespace %q and its Decl has %q", u.TypeName, u.Namespace, u.Decl.File.Namespace)
		}
	}
}

// The emitter's three defensive branches, called directly.
//
// None is reachable from a schema: a backend runs only on a bag with no
// errors, and each of these states is an error the checker has already
// reported -- an unresolved view source is PROTOP0205, an unresolved alias
// target and an unresolved type name are diagnosed the same way. They exist so
// the functions are total rather than to describe a real case, and they are
// pinned because a branch nobody exercises is a branch nobody notices going
// wrong -- a missing nil guard here is a compiler panic on input the compiler
// had already rejected, which is the worst possible way to report a known
// error.
func TestTheEmittersTotalityGuards(t *testing.T) {
	// The declaration has a member, and that is the whole point of it. Against
	// an empty one aliasTarget's loop never runs, so the guard this pins is
	// never reached and the assertion holds nothing: found by mutation --
	// deleting the guard survived an empty declaration and is caught by this
	// one, because `c.From == m.Aliased` is nil == nil on every member outside
	// a view and would hand back the first member of whatever was passed in.
	somewhere := &check.Decl{Members: []*check.Member{{Name: "held"}}}
	if got := aliasTarget(somewhere, &check.Member{Name: "other"}); got != nil {
		t.Errorf("aliasTarget(unresolved) = %v, want nil; anything else names a member that is not there", got)
	}
	if got := aliasTargetName(somewhere, &check.Member{Name: "other"}); got != "" {
		t.Errorf("aliasTargetName(unresolved) = %q, want the empty string; anything else names a member that is not there", got)
	}
	if got := typeNamespaces(check.Type{Kind: check.TypeDecl}); len(got) != 0 {
		t.Errorf("typeNamespaces(unresolved decl type) = %v, want none", got)
	}
	if got := typeNamespaces(check.Type{Kind: check.TypeDecl, Decl: &check.Decl{}}); len(got) != 0 {
		t.Errorf("typeNamespaces(decl with no file) = %v, want none", got)
	}
}

// modifiers is total: a view whose source did not resolve still returns a
// shape. No schema reaches a backend in that state -- an unresolved view source
// is PROTOP0205, an error, and bag.HasErrors stops emission -- so no fixture
// can produce it and the branch is called directly.
//
// A defensive branch nobody exercises is a branch nobody notices going wrong.
// What it returns is the table shape, because a view of nothing is still a view
// and `sealed partial` is the shape that has a hand-written half to fall back
// on.
func TestAViewWithNoSourceStillHasAShape(t *testing.T) {
	if got := modifiers(&check.Decl{Kind: check.DeclView}); got != "public sealed partial class" {
		t.Errorf("modifiers(view with no source) = %q, want the table shape", got)
	}
}

// --- the rows, pinned as facts rather than as text ---------------------------

// TestScalarRowsAreClosed is the guard the whole mapping table hangs on.
//
// The list below is written out rather than read off csScalars or off
// internal/syntax, because a test that derives its expectation from the thing
// under test passes by construction the moment the thing under test grows a
// row. Whether protop's scalar set is closed at all is #32 and is unresolved;
// what this test says is only that no one widened it here without going there.
func TestScalarRowsAreClosed(t *testing.T) {
	want := map[string]string{
		"int64":   "long",
		"int32":   "int",
		"string":  "string",
		"double":  "double",
		"bool":    "bool",
		"decimal": "decimal",
	}
	for protop, cs := range want {
		got, ok := csScalars[protop]
		if !ok {
			t.Errorf("csScalars has no row for %q", protop)
			continue
		}
		if got != cs {
			t.Errorf("csScalars[%q] = %q, want %q", protop, got, cs)
		}
	}
	for protop := range csScalars {
		if _, ok := want[protop]; !ok {
			t.Errorf("csScalars has an extra row %q; protop's scalar set is #32's "+
				"question and widening it here rules it through the back door", protop)
		}
	}
}

// Every emitted [Key(N)] is some member's tag minus one, and every member with
// a tag emits one. Asserted as a multiset over the spec's own schema rather
// than as a line of text, so that dropping the offset, doubling it, or emitting
// a key for a member that has none all fail here and not only in a golden.
func TestEveryEmittedKeyIsItsTagMinusOne(t *testing.T) {
	schema := schemaOf(t, map[string]string{
		pragueFixture:   readFixture(t, pragueFixture),
		consumerFixture: readFixture(t, consumerFixture),
	})
	total := 0
	for _, f := range schema.Files {
		for _, d := range f.Decls {
			var want []string
			for _, m := range d.Members {
				if m.HasTag() {
					want = append(want, "[Key("+strconv.FormatUint(m.Tag-1, 10)+")]")
				}
			}
			got := keyAttributes(EmitDecl(d))
			if !slices.Equal(got, want) {
				t.Errorf("%s emits keys %v, its tags are %v", d.Name, got, want)
			}
			total += len(want)
		}
	}
	if total == 0 {
		t.Fatal("no keys were compared; the fixture stopped carrying tagged members")
	}
}

func keyAttributes(src string) []string {
	var out []string
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[Key(") {
			out = append(out, line)
		}
	}
	return out
}

// A view member that narrows and renames at once -- section 5's `SlimOdd odds
// as o;` -- emits the narrowing view's type under the rename's name, at the
// source's key.
//
// Its anchor is the corpus in two halves, because no corpus member does both:
//
//   - the narrowing half is corpus/prague/payloads/SlimBetType.cs key 25, where
//     `SlimOdd` -- the narrowing view's type -- sits at BetType's tag 26;
//   - the rename half is corpus/prague/payloads/SlimSportGame.cs key 0, where
//     section 2's `event_id as id;` emits `Id` and keeps SportGame's tag 1.
//
// The expectation below is those two anchored rules applied to one member,
// written out rather than regenerated: a golden for a member the corpus does
// not write would agree only with the emitter that produced it, which is what
// #45's review broke. The source tag is 7, so a key read off the member's
// position, off the narrowing view, or without the tag-minus-one offset is a
// different number and not this one.
func TestANarrowedAndRenamedMemberEmitsAtItsSourcesKey(t *testing.T) {
	units := emitSource(t, map[string]string{
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
	for _, u := range units {
		if u.TypeName != "SlimOuter" {
			continue
		}
		d, err := readCS(u.TypeName, u.Source)
		if err != nil {
			t.Fatalf("the emission does not read back: %v\n%s", err, u.Source)
		}
		const want = "public SlimInner O { get; set; }"
		if got := d.Members["6"]; got != want {
			t.Errorf("SlimOuter at [Key(6)] is %q, want %q -- the narrowing types it, the rename names it, and tag 7 keys it:\n%s", got, want, u.Source)
		}
		return
	}
	t.Fatal("nothing emitted SlimOuter")
}

// protop's own annotations are read by the checker and are not C# attributes.
// `[references(SportGame)]` is a fact about the schema; emitting it would
// produce an attribute no corpus file has and no C# project defines.
func TestProtopsOwnAnnotationsEmitNothing(t *testing.T) {
	units := emitFixtures(t, pragueFixture, consumerFixture)
	for _, u := range units {
		if strings.Contains(u.Source, syntax.RelationAnnotation) {
			t.Errorf("%s emits protop's own %q annotation", u.TypeName, syntax.RelationAnnotation)
		}
	}
}

// --- an alias a view carries through (#55) -----------------------------------

// aliasShapes is one table with an alias and the three views that carry it
// through: renamed, narrowed, and both at once. The view holds the alias's
// target under a name and at a type that are the view's own, and the
// passthrough body has to say those and not the source's.
//
// The fourth shape -- a view that selects the alias and does not select its
// target -- is deliberately not here. It is #66, it is unresolved, and it is
// held by TestAViewThatSelectsAnAliasAndNotItsTargetStillEmits below, which
// pins that the emitter is total on it and nothing about what it emits.
const aliasShapes = `revision 1;
namespace A;

record Inner {
  int64 x = 1;
}

view SlimInner of Inner {
  x;
}

table Outer {
  key int64 id = 1;
  Inner odds = 7;
  Inner? spare = 8;
  alias thing = odds;
  alias spare_thing = spare;
}

view RenamedOnly of Outer {
  id;
  odds as o;
  thing as t;
  spare as sp;
  spare_thing as st;
}

view NarrowedOnly of Outer {
  id;
  SlimInner odds;
  thing as t;
}

view RenamedAndNarrowed of Outer {
  id;
  SlimInner odds as o;
  thing as t;
}
`

// aliasChain is an alias whose target is itself an alias.
//
// It is here because the rule this ticket applies -- an alias is declared at
// the declared type of the member its body names -- does not know what kind of
// member that is, and `Second` reading `First` compiles or does not for exactly
// the reason `T` reading `O` does. The checker deliberately leaves the chain's
// *type* unresolved (internal/check, resolveAlias: it follows no chain the
// language has not described), which before #55 reached the emitter as a
// missing type and emitted `public  Second` -- not valid C# and not a
// diagnostic either. Nothing here says a chain should be legal.
//
// What it holds is one link, and that is the whole of the claim. `second`'s
// target `first` is an alias whose own type the checker *did* resolve, so the
// member this lookup finds has a type and `Second` gets one. Add a third link
// and the fixture stops holding: `alias third = second;` emits `public  Third`
// today, at exit 0, because `second` is the alias whose type was left
// unresolved and the lookup faithfully reports that. That is #67 and it is
// unresolved -- an alias of an alias may turn out to be a diagnostic rather
// than a chain to follow -- so there is deliberately no depth-3 fixture and no
// golden for one: #40 ruled that pinning a defect turns it into a golden the
// fixing ticket then has to argue with. The repro is on #67 as text.
//
// The obvious fix is one line -- have the lookup recurse on the target it
// found -- and it passes this suite and stack-overflows on `alias a = b; alias
// b = a;`, which protopc accepts today with no diagnostic. Whatever rules #67
// needs a visited set or a depth bound before it recurses.
const aliasChain = `revision 1;
namespace A;

table Chained {
  key int64 id = 1;
  int64 event_id = 2;
  alias first = event_id;
  alias second = first;
}
`

// compositeKeyShapes is a composite key whose two parts are at *different*
// types, carried through a view that renames nothing.
//
// The corpus's only composite key is `(event_id, bet_type_id)` and both parts
// are `int64`, so on the fixtures alone every tuple element's declared type and
// every named member's declared type are `long` and the second half of the
// clause below -- element i is typed as the member read at position i -- agrees
// with itself no matter which of the two it read. `(long N, string S)` is what
// makes it an assertion: an emitter that wrote the first part's type for every
// element, or the parts' types in the wrong order, has to disagree here.
const compositeKeyShapes = `revision 1;
namespace A;

table Mixed {
  key key = 1 (n, s);

  int64  n = 2;
  string s = 3;
}

view MixedView of Mixed {
  key;
  n;
  s;
}
`

// aliasAcrossNamespaces is the same narrowing spread over three namespaces, so
// that the two types the fix chooses between -- the source's `Inner` and the
// view's `SlimInner` -- do not live in the same one. With both in one namespace
// the import list cannot tell which of them the emission named.
var aliasAcrossNamespaces = map[string]string{
	"a.protop": `revision 1;
namespace A;
record Inner {
  int64 x = 1;
}
`,
	"c.protop": `revision 1;
namespace C;
import "a.protop";
view SlimInner of Inner {
  x;
}
`,
	"b.protop": `revision 1;
namespace B;
import "a.protop";
import "c.protop";
table Outer {
  key int64 id = 1;
  Inner odds = 7;
  alias thing = odds;
}
view Narrowing of Outer {
  id;
  SlimInner odds;
  thing as t;
}
`,
}

// The general form of #55, rather than the three spellings of it -- and the
// general form is not about aliases.
//
// Two of the three member shapes protopc emits have a body that *reads other
// members*: the alias's `get => X; set => X = value;` and the composite key's
// `=> (A, B)`. Two things make such a body compile, and neither is about which
// shape or which fixture produced it: every name the body reads has to be a
// property *the same emitted file declares*, and the type the body's own
// declaration claims for that name has to be the type that property is declared
// at -- a property cannot return `Inner` and assign to a `SlimInner`, and a
// tuple element typed `long` cannot be built out of a `string`.
//
// Both are read back out of the emitted C#, so the assertion sees what the
// compiler would see rather than what the model intended. #55 was two
// independent breakages of exactly these two clauses: writeAlias named the
// member as the *source* holds it, which a rename falsifies, and typed it from
// the *source's* alias, which a narrowing falsifies. Neither spelling appears
// below; a fixture that grew a fourth way to break the rule would fail here
// without this test being edited.
//
// Holding the composite key to the same clause is why it is stated as the
// general form. #55 fixed the alias and left the identical defect standing in
// writeCompositeKey next door: a view that renames a member a key is built out
// of emits `public (long Left, long Right) Id => (L, R)`'s bad twin --
// `=> (Left, Right)` in a class declaring `L` and `R`, two CS0103 at exit 0.
// That is #68 and it is a language question, not a bug: whether the element
// names follow the view, stay the source's, or make the rename a diagnostic
// decides what the emitter should write, and none of the three is protopc's to
// pick. So it is not fixed and not pinned -- #40 ruled that pinning a defect
// turns it into a golden the fixing ticket has to argue with -- and the schema
// that reaches it is recorded in #68 as text rather than as a fixture. What
// this test does is make the case impossible to reach quietly: the day a
// fixture renames a key part, the body fails here instead of compiling nowhere.
func TestAnExpressionBodyNamesOnlyMembersItsOwnFileDeclares(t *testing.T) {
	units := emitFixtures(t, pragueFixture, consumerFixture)
	units = append(units, emitSource(t, map[string]string{"a.protop": aliasShapes})...)
	units = append(units, emitSource(t, aliasAcrossNamespaces)...)
	units = append(units, emitSource(t, map[string]string{"a.protop": aliasChain})...)
	units = append(units, emitSource(t, map[string]string{"a.protop": compositeKeyShapes})...)

	checked := assertBodiesNameDeclaredMembers(t, units)

	// An assertion over bodies says nothing if there were no bodies, and these
	// are the extent this ticket owes. Named here so that a fixture losing one
	// fails as a missing case rather than as a smaller loop.
	for _, want := range []string{
		"BetType.BetTypeId",    // the corpus's own alias, in the declaration that wrote it
		"Outer.Thing",          // the same alias in the table it belongs to
		"RenamedOnly.T",        // the target renamed, not narrowed
		"RenamedOnly.St",       // and the same with the target optional: presence is part of the type
		"NarrowedOnly.T",       // the target narrowed, not renamed
		"RenamedAndNarrowed.T", // both at once
		"Narrowing.T",          // and the same again, with the two types in different namespaces
		"Chained.Second",       // an alias of an alias, which is a member like any other
		"BetType.Key",          // the other body shape: a composite key where it is declared
		"SlimBetType.Id",       // and carried through a view, which is where #68 lives
		"Mixed.Key",            // a key whose parts are at two different types
		"MixedView.Key",        // and the same one carried through a view
	} {
		if !slices.Contains(checked, want) {
			t.Errorf("no alias was checked on %s; the assertion passed by not running, and the fixtures owe this case", want)
		}
	}
}

// assertBodiesNameDeclaredMembers holds every expression-bodied member in every
// emitted unit to the two clauses above and returns "<Type>.<Member>" for each
// one it checked, so the caller can say which cases it expected to exist.
//
// Which members those are is decided by the *shape of the emitted text* and not
// by the member's id, deliberately. An alias is identifiable from its id --
// readCS keys it `alias:<Name>` because it has no wire slot -- but a composite
// key has a `[Key(N)]` like any field and is keyed by its number, so filtering
// on the id is exactly how the general clause came to be stated over aliases
// and enforced over aliases only. A member is checked here because its emission
// reads other members, which is the property the clause is about.
func assertBodiesNameDeclaredMembers(t *testing.T, units []Unit) []string {
	t.Helper()
	var checked []string
	for _, u := range units {
		// An enum has no members in readCS's sense and has no bodies.
		if u.Decl.Kind == check.DeclEnum {
			continue
		}
		d, err := readCS(u.TypeName, u.Source)
		if err != nil {
			t.Fatalf("%s does not read back: %v\n%s", u.TypeName, err, u.Source)
		}
		declared := declaredMembers(t, u, d)
		for _, id := range d.Order {
			line := lastLine(d.Members[id])
			if name, isAlias := strings.CutPrefix(id, "alias:"); isAlias {
				checked = append(checked, u.TypeName+"."+name)
				assertPassthroughBody(t, u, declared, name, line, d.Members[id])
				continue
			}
			if m := reCompositeKeyBody.FindStringSubmatch(line); m != nil {
				checked = append(checked, u.TypeName+"."+m[2])
				assertTupleBody(t, u, declared, m[2], m[1], m[3])
			}
		}
	}
	return checked
}

// assertPassthroughBody is the alias half: one name, read and written.
func assertPassthroughBody(t *testing.T, u Unit, declared map[string]string, name, line, whole string) {
	t.Helper()
	m := reAliasPassthrough.FindStringSubmatch(line)
	if m == nil {
		t.Errorf("%s.%s is not a passthrough at all: %q", u.TypeName, name, whole)
		return
	}
	typ, target, written := m[1], m[3], m[4]
	if target != written {
		t.Errorf("%s.%s reads %s and writes %s; a passthrough is one member", u.TypeName, name, target, written)
	}
	if target == name {
		t.Errorf("%s.%s passes through to itself, which is infinite recursion and not an alias", u.TypeName, name)
		return
	}
	held, ok := declared[target]
	if !ok {
		t.Errorf("%s.%s passes through to %s, which %s does not declare -- CS0103:\n%s",
			u.TypeName, name, target, u.TypeName, u.Source)
		return
	}
	if held != typ {
		t.Errorf("%s.%s is declared %s and passes through to %s, which is declared %s; a property cannot return one type and assign another:\n%s",
			u.TypeName, name, typ, target, held, u.Source)
	}
}

// assertTupleBody is the composite key half: one name per tuple element, read
// and not written, and each element's declared type is a claim about the member
// the same position reads.
//
// The arity check is not ceremony. The two lists are written by two separate
// loops over m.Parts in writeCompositeKey, and a body that dropped an element
// would otherwise be compared position by position against the wrong names and
// could agree on every one it still had.
func assertTupleBody(t *testing.T, u Unit, declared map[string]string, name, elements, body string) {
	t.Helper()
	elems := splitTopLevel(elements)
	reads := splitTopLevel(body)
	if len(elems) != len(reads) {
		t.Errorf("%s.%s declares %d tuple elements and builds the tuple out of %d names (%q => (%q))",
			u.TypeName, name, len(elems), len(reads), elements, body)
		return
	}
	for i, e := range elems {
		sp := strings.LastIndex(e, " ")
		if sp < 0 {
			t.Errorf("%s.%s: tuple element %q has no name", u.TypeName, name, e)
			continue
		}
		typ, read := e[:sp], reads[i]
		held, ok := declared[read]
		if !ok {
			t.Errorf("%s.%s builds its tuple out of %s, which %s does not declare -- CS0103. "+
				"This is #68: a view renaming a member the key is built out of.\n%s",
				u.TypeName, name, read, u.TypeName, u.Source)
			continue
		}
		if held != typ {
			t.Errorf("%s.%s declares tuple element %d as %s and builds it out of %s, which is declared %s:\n%s",
				u.TypeName, name, i, typ, read, held, u.Source)
		}
	}
}

// splitTopLevel splits a comma-separated list on the commas that are not inside
// brackets, so that `long Id, List<Dictionary<string, long>> Xs` is two
// elements and not four. A naive split reports the wrong arity, which reads as
// a dropped tuple element and sends the next reader looking in the emitter.
func splitTopLevel(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '<', '(', '[':
			depth++
		case '>', ')', ']':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}

// declaredMembers is every property the emitted file declares, as a map from
// the emitted name to the emitted declared type. It is read off the emission
// rather than off the model on purpose: #55 was a body naming a member the
// model held and the file did not.
func declaredMembers(t *testing.T, u Unit, d *csDecl) map[string]string {
	t.Helper()
	out := make(map[string]string, len(d.Order))
	for _, id := range d.Order {
		typ, name, ok := declaredTypeAndName(lastLine(d.Members[id]))
		if !ok {
			t.Fatalf("%s: member %q does not read back as a declaration: %q", u.TypeName, id, d.Members[id])
		}
		if prev, dup := out[name]; dup {
			t.Errorf("%s declares %s twice, as %s and as %s", u.TypeName, name, prev, typ)
		}
		out[name] = typ
	}
	return out
}

// reAliasPassthrough reads the canonical one-line form readCS folds an alias
// body into.
var reAliasPassthrough = regexp.MustCompile(`^public (.+) (\w+) \{ get => (\w+); set => (\w+) = value; \}$`)

// reCompositeKeyBody reads a composite key's emission: the tuple it declares,
// its name, and the tuple it builds. It matches the shape rather than the
// member, so a computed member that protopc does not emit today would be held
// to the same clause on the day it does.
var reCompositeKeyBody = regexp.MustCompile(`^public \((.+)\) (\w+) => \((.+)\);$`)

// declaredTypeAndName splits `public List<Odd?>? MaybeOdds { get; set; }` into
// its declared type and its name, for every member shape the emitter writes:
// the plain property, the composite key's expression body, and the alias's
// block. The type is whatever precedes the last identifier before the body
// starts, so a composite key's `(long EventId, long BetTypeId)` survives.
func declaredTypeAndName(line string) (typ, name string, ok bool) {
	rest, ok := strings.CutPrefix(line, "public ")
	if !ok {
		return "", "", false
	}
	i := strings.Index(rest, " { ")
	if i < 0 {
		i = strings.Index(rest, " => ")
	}
	if i < 0 {
		return "", "", false
	}
	head := rest[:i]
	sp := strings.LastIndex(head, " ")
	if sp < 0 {
		return "", "", false
	}
	return head[:sp], head[sp+1:], true
}

// lastLine drops the attribute lines readCS folds in above a member's text.
func lastLine(s string) string {
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		return s[i+1:]
	}
	return s
}

// A declaration imports the namespaces its emitted text names, so an alias's
// import has to follow its declared type.
//
// #55 moved that type from the source's alias to the member the emitting
// declaration actually holds. `Narrowing` names `SlimInner` twice -- on the
// narrowed member and on the passthrough -- and names `Inner` nowhere, so the
// import list is `C` and not `A`. Emitting `using A;` here is not a compile
// error, which is exactly why it needs a test: it is the half of the fix a
// green build would not notice.
func TestAnAliasCarriedThroughAViewImportsTheTypeItEmits(t *testing.T) {
	for _, u := range emitSource(t, aliasAcrossNamespaces) {
		if u.TypeName != "Narrowing" {
			continue
		}
		head, _, _ := strings.Cut(u.Source, "namespace ")
		if got, want := head, "using C;\nusing MessagePack;\n\n"; got != want {
			t.Errorf("Narrowing imports %q, want %q -- the alias emits SlimInner, so A is an import for a type the file does not name:\n%s", got, want, u.Source)
		}
		return
	}
	t.Fatal("nothing emitted Narrowing")
}

// The fourth shape: a view that selects an alias and does not select the
// alias's target at all. There is then no member of the view for the emitter's
// lookup to find.
//
// What protopc *should* do about it is #66 -- a diagnostic, an implicit
// selection, or something else -- and it is unresolved and the user's to rule.
// So what this test holds is only that the lookup coming up empty is total:
// the emitter still produces the unit rather than panicking on the missing
// member. It deliberately does not assert the text, and there is deliberately
// no golden for this schema: #40 ruled that pinning a defect turns it into a
// golden the fixing ticket has to argue with, and the emission here is the
// pre-#55 fallback rather than an answer.
func TestAViewThatSelectsAnAliasAndNotItsTargetStillEmits(t *testing.T) {
	units := emitSource(t, map[string]string{"a.protop": `revision 1;
namespace A;
record Inner {
  int64 x = 1;
}
table Outer {
  key int64 id = 1;
  Inner odds = 7;
  alias thing = odds;
}
view TargetNotSelected of Outer {
  id;
  thing as t;
}
`})
	for _, u := range units {
		if u.TypeName != "TargetNotSelected" {
			continue
		}
		if _, err := readCS(u.TypeName, u.Source); err != nil {
			t.Fatalf("the emission does not read back: %v\n%s", err, u.Source)
		}
		return
	}
	t.Fatal("nothing emitted TargetNotSelected")
}

// A backend's annotation is text and nothing else. protopc does not know what
// `DataCache(CacheClassName = nameof(BPSportGameCache))` names, cannot know
// which namespace defines it, and therefore never adds a using for one --
// which is a real limit of section 3's passthrough rule and the reason the
// corpus's own `using B2Tech.Feed.Cache.Core;` is unreachable from here.
func TestAnnotationsNeverProduceAUsing(t *testing.T) {
	units := emitFixtures(t, pragueFixture, consumerFixture)
	for _, u := range units {
		if u.TypeName != "SportGame" {
			continue
		}
		head, _, _ := strings.Cut(u.Source, "namespace ")
		if got, want := head, "using MessagePack;\n\n"; got != want {
			t.Errorf("SportGame usings = %q, want %q; an annotation contributed one", got, want)
		}
		return
	}
	t.Fatal("the fixture stopped declaring SportGame")
}

func TestPascalIsOneRuleForMembersAndEnumArms(t *testing.T) {
	cases := []struct{ in, want string }{
		{"id", "Id"},
		{"event_id", "EventId"},
		{"provider_source_key", "ProviderSourceKey"},
		{"NOT_STARTED", "NotStarted"},
		{"UNKNOWN", "Unknown"},
		{"t", "T"},
		{"events_hub_received_timestamp_ms", "EventsHubReceivedTimestampMs"},

		// Empty parts vanish rather than becoming empty parts of the output.
		// No corpus name and no spec name has a doubled or edge underscore, so
		// these are here precisely because nothing else reaches them -- a rule
		// checked only where its input is well-formed is a rule with an
		// unchecked half.
		{"a__b", "AB"},
		{"_leading", "Leading"},
		{"trailing_", "Trailing"},
		{"_", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := pascal(c.in); got != c.want {
			t.Errorf("pascal(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The scalar fallback: a protop scalar with no row in csScalars emits the
// protop keyword, so the generated C# fails at the C# compiler rather than
// emitting a member with no type at all.
//
// Nothing can reach it today -- csScalars and syntax.scalarTypes hold the same
// six keywords, which TestScalarRowsAreClosed pins -- so the branch is called
// directly with the one scalar #32 is actually about. If #32 rules that the set
// widens, this test is the thing that says what the half-widened state does.
func TestAScalarWithNoRowEmitsItsProtopKeyword(t *testing.T) {
	got := TypeName(check.Type{Kind: check.TypeScalar, Name: "byte"})
	if got != "byte" {
		t.Errorf("TypeName(unmapped scalar) = %q, want the protop keyword back", got)
	}
	if got == "" {
		t.Error("an empty type name emits `public  Name`, which compiles nowhere and is diagnosed nothing")
	}
}

// Section 4's emission column, pinned by name so that changing one row fails
// one line rather than four goldens. `record Odd` emitting a C# *class* is the
// row to watch: #20 forbids a C# record, and the two keywords collide in
// spelling while meaning opposite things.
//
// The view rows are the correction described on modifiers: `SlimOdd` is a view
// of a record and the corpus writes `public class SlimOdd`, where `SlimBetType`
// and `SlimSportGame` are views of tables and the corpus writes
// `public sealed partial class`.
func TestDeclarationShapes(t *testing.T) {
	want := map[string]string{
		"Venue":         "public struct Venue",
		"StaticDataRef": "public class StaticDataRef",
		"Odd":           "public class Odd",
		"SportGame":     "public sealed partial class SportGame",
		"BetType":       "public sealed partial class BetType",
		"SlimSportGame": "public sealed partial class SlimSportGame",
		"SlimBetType":   "public sealed partial class SlimBetType",
		"SlimOdd":       "public class SlimOdd",
		"EventStatus":   "public enum EventStatus",
	}
	seen := map[string]bool{}
	for _, u := range emitFixtures(t, pragueFixture, consumerFixture) {
		head, ok := want[u.TypeName]
		if !ok {
			continue
		}
		seen[u.TypeName] = true
		if !strings.Contains(u.Source, "\n"+head+"\n{\n") {
			t.Errorf("%s is not emitted as %q:\n%s", u.TypeName, head, u.Source)
		}
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("the fixture stopped declaring %s", name)
		}
	}
}
