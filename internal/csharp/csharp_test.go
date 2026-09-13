package csharp

import (
	"flag"
	"os"
	"path/filepath"
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
// The flag exists because hand-maintaining C# by hand in a Go string is worse
// than reviewing a diff, and it is dangerous for exactly the same reason: a
// regenerated golden proves only that the emitter agrees with itself. What
// makes these goldens evidence is that they are small and reviewed -- and that
// the oracle they ultimately answer to is corpus/prague/, which -update cannot
// touch.
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
	if got := aliasTarget(&check.Member{Name: "other"}); got != "" {
		t.Errorf("aliasTarget(unresolved) = %q, want the empty string; anything else names a member that is not there", got)
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
