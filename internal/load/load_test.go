package load

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/nanov/protobuf-to-messagepack-sharp/internal/check"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/diag"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/syntax"
)

// The fixtures are written here rather than read from testdata because what
// this package is under test for is the *filesystem*: which directory a file
// sits in is half of every assertion, so the layout has to be part of the test
// and not part of a directory somebody could rearrange. Nothing here is a
// golden and `-update` rewrites none of it.

// file is one file to plant: a path relative to the test's root, and its text.
type file struct {
	path string
	text string
}

// src is a minimal parsable protop file: revision, namespace, imports, one
// declaration. The imports are what the loader is being asked about; the rest
// is the least the frontend will accept, so that a loading test never fails on
// a parse.
func src(ns string, imports ...string) string {
	var b strings.Builder
	b.WriteString("revision 1;\n")
	fmt.Fprintf(&b, "namespace %s;\n", ns)
	for _, im := range imports {
		fmt.Fprintf(&b, "import %q;\n", im)
	}
	fmt.Fprintf(&b, "struct %s {\n  int64 id = 1;\n}\n", ns)
	return b.String()
}

// plant writes the files under a fresh temp directory and returns it.
func plant(t *testing.T, files ...file) string {
	t.Helper()
	root := t.TempDir()
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f.path))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f.text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// keys is the set's keys, sorted, which is what a load is measured by: the
// checker looks an import up by exactly these strings.
func keys(set check.FileSet) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// codes is the diagnostics as a code sequence, which is the suite's convention:
// codes are protopc's interface and a count is not.
func codes(bag *diag.Bag) []string {
	out := make([]string, 0, bag.Len())
	for _, d := range bag.All() {
		out = append(out, d.Code.String())
	}
	return out
}

// assertCodes holds a bag against the sequence of codes it should contain, and
// then against what those codes imply about the compile.
//
// bag.HasErrors is the gate the driver reads. The loader is the first stage to
// fill a bag, so a loader diagnostic that does not reach the gate is a missing
// import that protopc prints, and then goes on to parse, check and generate C#
// around the hole it just named.
//
// The gate expectation is derived and never written down: the compile fails
// exactly when one of the expected codes is an error by diag's own table. So
// this says nothing about what a severity ought to be -- the day a code becomes
// advisory the expectation moves with it and no fixture here needs an edit --
// and everything about whether the severity a code has survives the trip from
// Raise to the gate. No expected codes derives to false, which is the clean
// case, and is why the clean assertions come through here too.
//
// What it does not catch is a demotion made in diag's table and its oracle
// together, because then the derivation agrees with the demotion. The tests
// that name a severity in a literal are what hold that.
//
// What a gate comparison catches that nothing else does, on the record,
// because deriving the expectation from diag's table makes it look like it
// holds nothing at all:
//
// Bag.All returns the live backing slice and Diagnostic.Severity is an
// exported field, so any stage can demote a diagnostic AFTER raising it. diag
// cannot see that -- the stage doing it is not diag -- and neither can any
// table, because nothing about the code changed. Measured in the two stages
// that have a code without a literal-severity fixture: the demotion appended
// to syntax/parser.go's TagIsReserved site, and to check.go's
// AliasTargetNotFound site, each fails that package's gate comparison and
// nothing else in the repo.
//
// The loader is the exception and it is worth writing down rather than
// implying otherwise. It raises two codes and both of them have a literal
// fixture in TestALoaderDiagnosticFailsTheCompile, so for a demotion of either
// one this comparison is measurably redundant today -- verified, the demotion
// at the ImportNotFound site fails that test too. It is the same comparison
// all the same, and it is what covers the third code the loader raises,
// whenever there is one, without anybody having to remember to add a fixture
// for it.
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
func assertCodes(t *testing.T, bag *diag.Bag, want ...diag.Code) {
	t.Helper()
	wantStrings := make([]string, len(want))
	fails := false
	for i, c := range want {
		wantStrings[i] = c.String()
		if c.Severity() == diag.Error {
			fails = true
		}
	}
	if got := codes(bag); !slices.Equal(got, wantStrings) {
		t.Fatalf("codes = %v, want %v", got, wantStrings)
	}
	if got := bag.HasErrors(); got != fails {
		t.Fatalf("HasErrors = %v, want %v for %v", got, fails, wantStrings)
	}
}

// assertUnderlines holds a loader diagnostic against the source it points at:
// where its span starts, and what that span covers.
//
// The loader was the one stage in the frontend with no way to say either. Both
// of its codes are raised at an import's span, and a span that is wrong --
// collapsed to nothing, or sitting on the import above the one at fault --
// paints the caret under the wrong line while every code sequence in this file
// stays green. Measured: collapsing PROTOP0108's span inside Bag.add failed
// nothing in the repo before this existed.
//
// The file comes off the diagnostic and not off the fixture. The loader is the
// first stage that holds several files at once, so which file a span indexes
// into is the diagnostic's own answer to give.
//
// The position is a line and a column rather than the printed `path:line:col`
// because the path is a fresh temp directory: the only part of it a fixture
// could write down is the suffix, and which file the stamp names is already
// held by TestADiagnosticNamesTheFileThatWroteTheImport.
func assertUnderlines(t *testing.T, bag *diag.Bag, i, wantLine, wantCol int, wantText string) {
	t.Helper()
	d := bag.All()[i]
	if d.File == nil {
		t.Fatalf("diagnostic %d names no file, so there is nothing to slice its span out of", i)
	}
	pos := d.File.Position(d.Span.Start)
	if pos.Line != wantLine || pos.Column != wantCol {
		t.Errorf("diagnostic %d starts at %d:%d, want %d:%d", i, pos.Line, pos.Column, wantLine, wantCol)
	}
	if got := d.File.Slice(d.Span); got != wantText {
		t.Errorf("diagnostic %d underlines %q, want %q", i, got, wantText)
	}
}

func loadIn(t *testing.T, dir string, roots, inputs []string) (*Result, *diag.Bag, error) {
	t.Helper()
	abs := func(paths []string) []string {
		out := make([]string, len(paths))
		for i, p := range paths {
			out[i] = filepath.Join(dir, filepath.FromSlash(p))
		}
		return out
	}
	var bag diag.Bag
	res, err := Load(abs(inputs), abs(roots), &bag)
	return res, &bag, err
}

// An import resolves against the include roots and against nothing else, and a
// file reached through one is in the set under the import string that named it
// -- not under the path it was opened from.
func TestAnImportResolvesAgainstTheIncludeRoots(t *testing.T) {
	dir := plant(t,
		file{"schemas/a.protop", src("A", "b.protop")},
		file{"schemas/b.protop", src("B")},
	)
	res, bag, err := loadIn(t, dir, []string{"schemas"}, []string{"schemas/a.protop"})
	if err != nil {
		t.Fatal(err)
	}
	assertCodes(t, bag)
	if got, want := keys(res.Set), []string{"a.protop", "b.protop"}; !slices.Equal(got, want) {
		t.Errorf("set keys = %v, want %v", got, want)
	}
	if got, want := res.Targets, []string{"a.protop"}; !slices.Equal(got, want) {
		t.Errorf("targets = %v, want %v", got, want)
	}
}

// An input under a root is keyed by its path *relative to that root*, which is
// the import string another file would name it by. Keying it by the path as
// typed would put the same file in the set twice -- once as `schemas/a.protop`
// and once as `a.protop` -- and every declaration in it would be emitted twice.
func TestAnInputIsKeyedByItsPathRelativeToItsRoot(t *testing.T) {
	dir := plant(t,
		file{"schemas/nested/a.protop", src("A")},
	)
	res, _, err := loadIn(t, dir, []string{"schemas"}, []string{"schemas/nested/a.protop"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := keys(res.Set), []string{"nested/a.protop"}; !slices.Equal(got, want) {
		t.Errorf("set keys = %v, want %v", got, want)
	}
}

// The same file named as an input and reached as an import is one entry, under
// the key the import names. Two entries would be two files to the checker.
func TestAFileNamedAndImportedIsLoadedOnce(t *testing.T) {
	dir := plant(t,
		file{"schemas/a.protop", src("A", "b.protop")},
		file{"schemas/b.protop", src("B")},
	)
	res, _, err := loadIn(t, dir, []string{"schemas"}, []string{"schemas/a.protop", "schemas/b.protop"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := keys(res.Set), []string{"a.protop", "b.protop"}; !slices.Equal(got, want) {
		t.Errorf("set keys = %v, want %v", got, want)
	}
	if got, want := res.Targets, []string{"a.protop", "b.protop"}; !slices.Equal(got, want) {
		t.Errorf("targets = %v, want %v", got, want)
	}
}

// With no -I, the root is the directory of each input, so that `protopc
// generate schemas/a.protop` finds the `import "b.protop"` sitting next to it.
func TestTheDefaultRootIsTheDirectoryOfEachInput(t *testing.T) {
	dir := plant(t,
		file{"one/a.protop", src("A", "b.protop")},
		file{"one/b.protop", src("B")},
		file{"two/c.protop", src("C", "d.protop")},
		file{"two/d.protop", src("D")},
	)
	res, bag, err := loadIn(t, dir, nil, []string{"one/a.protop", "two/c.protop"})
	if err != nil {
		t.Fatal(err)
	}
	assertCodes(t, bag)
	want := []string{"a.protop", "b.protop", "c.protop", "d.protop"}
	if got := keys(res.Set); !slices.Equal(got, want) {
		t.Errorf("set keys = %v, want %v", got, want)
	}
}

// Once -I is given it is the whole list: the directory of the input is not
// added behind it. Combining the two would let a root the user never named
// shadow one they did, which is exactly the resolution-order question #48 is
// open on, decided by accident.
func TestAnExplicitRootReplacesTheDefaultRatherThanExtendingIt(t *testing.T) {
	dir := plant(t,
		file{"one/a.protop", src("A", "b.protop")},
		file{"one/b.protop", src("B")},
		file{"empty/.keep", ""},
	)
	_, bag, err := loadIn(t, dir, []string{"empty"}, []string{"one/a.protop"})
	if err != nil {
		t.Fatal(err)
	}
	assertCodes(t, bag, diag.ImportNotFound)
}

// Roots are searched in the order the user gave them, and the first to provide
// the import is the one opened. This is not a ruling on #48 -- it is what the
// loader does when there is only one root that has the file at all, which is
// the case a second root that *also* has it turns into a diagnostic below.
func TestTheRootsAreSearchedInTheOrderGiven(t *testing.T) {
	dir := plant(t,
		file{"a.protop", src("A", "b.protop")},
		file{"first/b.protop", src("First")},
		file{"second/.keep", ""},
	)
	res, bag, err := loadIn(t, dir, []string{"first", "second", "."}, []string{"a.protop"})
	if err != nil {
		t.Fatal(err)
	}
	assertCodes(t, bag)
	if got := res.Set["b.protop"].Source.Name; !strings.Contains(got, filepath.Join("first", "b.protop")) {
		t.Errorf("b.protop was opened from %q, want the first root", got)
	}
}

// Two roots providing one import string is PROTOP0108 and not a silent
// first-wins. Which root *should* win is #48, human-in-the-loop and open, so
// the loader reports the ambiguity and ranks nothing; the message names every
// file it found so that the user can see what they would be choosing between.
func TestTwoRootsProvidingOneImportIsDiagnosed(t *testing.T) {
	dir := plant(t,
		file{"a.protop", src("A", "b.protop")},
		file{"one/b.protop", src("One")},
		file{"two/b.protop", src("Two")},
	)
	_, bag, err := loadIn(t, dir, []string{"one", "two"}, []string{"a.protop"})
	if err != nil {
		t.Fatal(err)
	}
	assertCodes(t, bag, diag.ImportAmbiguous)
	msg := bag.All()[0].Message
	for _, want := range []string{filepath.Join("one", "b.protop"), filepath.Join("two", "b.protop")} {
		if !strings.Contains(msg, want) {
			t.Errorf("message does not name %q:\n%s", want, msg)
		}
	}
}

// The severity of a loader diagnostic, end to end. A code sequence says what
// was noticed; this says what it costs. bag.HasErrors is what cmd/protopc turns
// into an exit code, and the loader is the first stage to fill the bag, so a
// loader diagnostic that does not fail the compile is an import protopc reports
// as missing and then parses, checks and generates C# around.
//
// The wanted severity is written here as a literal, and that is the point --
// assertCodes derives what it expects from diag's own table and so agrees with
// a demotion made in that table. Two tests that cannot both be satisfied by the
// same edit: diag's table says what the code means, and this says the meaning
// survives the trip from Raise to the gate.
func TestALoaderDiagnosticFailsTheCompile(t *testing.T) {
	for _, tt := range []struct {
		name  string
		roots []string
		files []file
		want  diag.Code
	}{
		{
			name:  "an import nothing provides",
			roots: []string{"."},
			files: []file{{"a.protop", src("A", "gone.protop")}},
			want:  diag.ImportNotFound,
		},
		{
			name:  "an import two roots provide",
			roots: []string{"one", "two"},
			files: []file{
				{"a.protop", src("A", "b.protop")},
				{"one/b.protop", src("One")},
				{"two/b.protop", src("Two")},
			},
			want: diag.ImportAmbiguous,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := plant(t, tt.files...)
			_, bag, err := loadIn(t, dir, tt.roots, []string{"a.protop"})
			if err != nil {
				t.Fatal(err)
			}
			if bag.Len() != 1 || bag.All()[0].Code != tt.want {
				t.Fatalf("want one %s, got %v", tt.want, codes(bag))
			}
			if got := bag.All()[0].Severity; got != diag.Error {
				t.Errorf("%s raised as a %s, want an Error", tt.want, got)
			}
			if !bag.HasErrors() {
				t.Errorf("%s does not fail the compile; protopc would carry on past it", tt.want)
			}
		})
	}
}

// An ambiguity is a fact about the include roots, not about the line that
// tripped over it, so two files importing one ambiguous name report it once.
//
// This is what the memoisation in follow is for, and the cycle tests do not
// reach it: termination is held independently by the abs-path guard in load, so
// removing follow's guard leaves every cycle test green and only doubles this
// diagnostic. A user told twice that two roots disagree would go looking for
// two disagreements.
func TestAnAmbiguousImportIsReportedOnce(t *testing.T) {
	dir := plant(t,
		file{"a.protop", src("A", "b.protop")},
		file{"c.protop", src("C", "b.protop")},
		file{"one/b.protop", src("One")},
		file{"two/b.protop", src("Two")},
	)
	_, bag, err := loadIn(t, dir, []string{"one", "two", "."}, []string{"a.protop", "c.protop"})
	if err != nil {
		t.Fatal(err)
	}
	// The ambiguity is one fact about the roots, and so one diagnostic.
	assertCodes(t, bag, diag.ImportAmbiguous)
}

// One file reached under two import strings is one unit, under both of them.
//
// The two strings are real: with roots `lib` and `.`, the file `lib/b.protop`
// is `b.protop` to anything searching the first root and `lib/b.protop` to
// anything searching the second, and a schema may well contain both spellings.
// The key is the import string as written -- check.FileSet's contract, which
// this package does not get to change -- so the two are different keys by
// construction, and both have to be in the set or the checker raises
// PROTOP0107 for a file the loader opened. What must not happen twice is the
// *load*: a second read and parse would be two files to the checker, and every
// declaration in it would be emitted twice under two names.
func TestOneFileReachedUnderTwoImportStringsIsOneUnitUnderBoth(t *testing.T) {
	dir := plant(t,
		file{"a.protop", src("A", "b.protop")},
		file{"c.protop", src("C", "lib/b.protop")},
		file{"lib/b.protop", src("B")},
	)
	res, bag, err := loadIn(t, dir, []string{"lib", "."}, []string{"a.protop", "c.protop"})
	if err != nil {
		t.Fatal(err)
	}
	assertCodes(t, bag)
	want := []string{"a.protop", "b.protop", "c.protop", "lib/b.protop"}
	if got := keys(res.Set); !slices.Equal(got, want) {
		t.Fatalf("set keys = %v, want %v; both spellings resolved, so both must be findable", got, want)
	}
	// Both keys, one unit: the same parsed file, not a second copy of it.
	if a, b := res.Set["b.protop"], res.Set["lib/b.protop"]; a.Source != b.Source || a.File != b.File {
		t.Errorf("the two spellings name two units; lib/b.protop was loaded twice")
	}
	var bs int
	for _, u := range res.Set {
		for _, d := range u.File.Decls {
			if st, ok := d.(*syntax.Struct); ok && st.Name.Text == "B" {
				bs++
			}
		}
	}
	if bs != 2 {
		t.Errorf("B is reachable through %d entries of the set, want 2: %v", bs, keys(res.Set))
	}
}

// One root listed twice is one root. Two spellings of one directory provide one
// file, not two, and reporting that as an ambiguity would be a diagnostic about
// the command line rather than about the schema.
//
// The spellings have to reach Load *as written*, which is why this test calls
// Load itself rather than going through loadIn: loadIn builds every path with
// filepath.Join, and Join cleans, so three spellings would arrive as three
// copies of one string and the test would exercise nothing. A relative and an
// absolute spelling of one directory is the pair that survives every cleaning,
// and it is the pair a Makefile produces -- `-I inc` from one rule and `-I
// $(abspath inc)` from the next.
//
// Two mechanisms collapse them and they are asserted apart:
//
//   - resolveRoots cleans and dedups, which is what keeps `one` and `one/` from
//     being two roots. Its only effect is on the list a diagnostic prints,
//     because everything downstream joins and joining cleans -- so it is the
//     message that pins it. A user told protopc searched three directories
//     goes looking for three directories.
//   - search's seen[abs], which is what keeps `one` and an absolute spelling of
//     `one` -- which no amount of cleaning makes equal -- from providing one
//     file twice. Without it this is a spurious PROTOP0108 on a command line
//     that named one directory.
func TestOneRootGivenTwiceIsNotAnAmbiguity(t *testing.T) {
	dir := plant(t,
		file{"a.protop", src("A", "b.protop", "gone.protop")},
		file{"one/b.protop", src("One")},
	)
	// Resolved, so that the relative spelling below and the absolute one are
	// the same directory to filepath.Abs -- on macOS the temp directory is
	// reached through a symlink and Getwd returns the resolved path.
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	var bag diag.Bag
	roots := []string{"one", "one/", filepath.Join(dir, "one")}
	if _, err := Load([]string{"a.protop"}, roots, &bag); err != nil {
		t.Fatal(err)
	}

	// The import that is there resolved once: no PROTOP0108.
	// Three spellings of one directory are one root, and so one diagnostic.
	assertCodes(t, &bag, diag.ImportNotFound)
	// And the import that is not there says it looked in one directory, twice
	// over: `one` and its absolute spelling are two roots to resolveRoots and
	// `one/` is not a third.
	msg := bag.All()[0].Message
	_, searched, ok := strings.Cut(msg, "searched ")
	if !ok {
		t.Fatalf("the message does not say where it looked:\n%s", msg)
	}
	if got, want := strings.Count(searched, `"`)/2, 2; got != want {
		t.Errorf("the message names %d roots, want %d:\n%s", got, want, msg)
	}
}

// Two roots that both provide the import: which file is opened is the first
// root's, and the ambiguity is reported beside it.
//
// This is not a ruling on #48, which is about which root *should* win -- the
// diagnostic is what says that is unsettled. It is that -I is an ordered list
// and the order has to be the whole of what it means: a loader that sorted its
// roots, or took the last candidate rather than the first, would answer #48 by
// accident and in the direction nobody chose. Both orders are run, because one
// order alone passes for a loader that always takes `one`.
func TestTheFirstRootThatProvidesAnImportIsTheOneOpened(t *testing.T) {
	for _, first := range []string{"one", "two"} {
		t.Run(first, func(t *testing.T) {
			second := "two"
			if first == "two" {
				second = "one"
			}
			dir := plant(t,
				file{"a.protop", src("A", "b.protop")},
				file{"one/b.protop", src("One")},
				file{"two/b.protop", src("Two")},
			)
			res, bag, err := loadIn(t, dir, []string{first, second, "."}, []string{"a.protop"})
			if err != nil {
				t.Fatal(err)
			}
			assertCodes(t, bag, diag.ImportAmbiguous)
			want := filepath.Join(first, "b.protop")
			if got := res.Set["b.protop"].Source.Name; !strings.HasSuffix(got, want) {
				t.Errorf("b.protop was opened from %q, want the first root's %q", got, want)
			}
		})
	}
}

// A root that is a prefix of another root: the input is keyed against the first
// root that contains it, not the most specific one, so the same file on the
// same disk is `nested/a.protop` under `-I schemas -I schemas/nested` and
// `a.protop` under the two flags reversed.
//
// Neither of those is ruled right here. What is ruled is that the order the
// user gave decides it: a loader that preferred the longest matching root would
// be making `-I` unordered for inputs while it stays ordered for imports, and a
// file's key is what every import of it has to spell.
func TestANestedRootKeysAgainstTheFirstRootGiven(t *testing.T) {
	for _, tc := range []struct {
		roots []string
		want  string
	}{
		{[]string{"schemas", "schemas/nested"}, "nested/a.protop"},
		{[]string{"schemas/nested", "schemas"}, "a.protop"},
	} {
		t.Run(strings.Join(tc.roots, ","), func(t *testing.T) {
			dir := plant(t,
				file{"schemas/nested/a.protop", src("A")},
			)
			res, _, err := loadIn(t, dir, tc.roots, []string{"schemas/nested/a.protop"})
			if err != nil {
				t.Fatal(err)
			}
			if got, want := keys(res.Set), []string{tc.want}; !slices.Equal(got, want) {
				t.Errorf("set keys = %v, want %v", got, want)
			}
		})
	}
}

// A directory whose name is an import string does not provide that import. An
// include root full of directories named for the files that used to be in them
// is not exotic, and a root that "has" the import shadows every root behind it:
// the loader would open a directory, fail, and return an error where the next
// root held the file all along.
func TestADirectoryNamedLikeAnImportDoesNotProvideIt(t *testing.T) {
	dir := plant(t,
		file{"a.protop", src("A", "b.protop")},
		file{"one/b.protop/.keep", ""},
		file{"two/b.protop", src("Two")},
	)
	res, bag, err := loadIn(t, dir, []string{"one", "two"}, []string{"a.protop"})
	if err != nil {
		t.Fatalf("a directory in the way of an import must not be an error: %v", err)
	}
	assertCodes(t, bag)
	if got, want := res.Set["b.protop"].Source.Name, filepath.Join("two", "b.protop"); !strings.HasSuffix(got, want) {
		t.Errorf("b.protop was opened from %q, want %q", got, want)
	}
}

// A cycle terminates and produces both files. The checker's scope is not
// transitive, so a cycle is harmless to *visibility* -- but nothing stopped the
// resolver walking it forever, and this is the loader half of #48. It holds
// however #48 rules on scope, because it is about the walk and not about what
// a file can see.
func TestAnImportCycleTerminates(t *testing.T) {
	dir := plant(t,
		file{"a.protop", src("A", "b.protop")},
		file{"b.protop", src("B", "a.protop")},
	)
	res, bag, err := loadIn(t, dir, []string{"."}, []string{"a.protop"})
	if err != nil {
		t.Fatal(err)
	}
	assertCodes(t, bag)
	if got, want := keys(res.Set), []string{"a.protop", "b.protop"}; !slices.Equal(got, want) {
		t.Errorf("set keys = %v, want %v", got, want)
	}
}

// The degenerate cycle, which a visited-set that is checked in the wrong place
// walks forever: a file that imports itself.
func TestAFileThatImportsItselfTerminates(t *testing.T) {
	dir := plant(t,
		file{"a.protop", src("A", "a.protop")},
	)
	res, _, err := loadIn(t, dir, []string{"."}, []string{"a.protop"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := keys(res.Set), []string{"a.protop"}; !slices.Equal(got, want) {
		t.Errorf("set keys = %v, want %v", got, want)
	}
}

// A cycle three files long, because a two-file cycle is also what a loader that
// only remembers the file it came from survives.
func TestALongerCycleTerminates(t *testing.T) {
	dir := plant(t,
		file{"a.protop", src("A", "b.protop")},
		file{"b.protop", src("B", "c.protop")},
		file{"c.protop", src("C", "a.protop")},
	)
	res, _, err := loadIn(t, dir, []string{"."}, []string{"a.protop"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a.protop", "b.protop", "c.protop"}
	if got := keys(res.Set); !slices.Equal(got, want) {
		t.Errorf("set keys = %v, want %v", got, want)
	}
}

// An import naming no file under any root is a diagnostic, not an error: the
// user did not type that path on the command line, they wrote it in a file that
// can be pointed at. The message says where the loader looked, which is the
// only thing that distinguishes "you spelled it wrong" from "you forgot a -I".
func TestAnImportUnderNoRootIsADiagnosticThatSaysWhereItLooked(t *testing.T) {
	dir := plant(t,
		file{"one/a.protop", src("A", "gone.protop")},
		file{"two/.keep", ""},
	)
	_, bag, err := loadIn(t, dir, []string{"one", "two"}, []string{"one/a.protop"})
	if err != nil {
		t.Fatalf("a missing import must not be an error: %v", err)
	}
	assertCodes(t, bag, diag.ImportNotFound)
	msg := bag.All()[0].Message
	for _, want := range []string{"gone.protop", filepath.Join(dir, "one"), filepath.Join(dir, "two")} {
		if !strings.Contains(msg, want) {
			t.Errorf("message does not name %q:\n%s", want, msg)
		}
	}
}

// A missing import is reported against the file that wrote it. The bag holds
// one compile's worth of files and a diagnostic that names none of them renders
// its snippet against whichever file the printer happened to be handed.
func TestADiagnosticNamesTheFileThatWroteTheImport(t *testing.T) {
	dir := plant(t,
		file{"a.protop", src("A", "b.protop")},
		file{"b.protop", src("B", "gone.protop")},
	)
	_, bag, err := loadIn(t, dir, []string{"."}, []string{"a.protop"})
	if err != nil {
		t.Fatal(err)
	}
	if bag.Len() != 1 {
		t.Fatalf("codes = %v, want one", codes(bag))
	}
	f := bag.All()[0].File
	if f == nil || !strings.HasSuffix(f.Name, "b.protop") {
		t.Errorf("the diagnostic names %v, want b.protop", f)
	}
}

// A loader diagnostic underlines the import that caused it, and not the file,
// the first import, or nothing.
//
// Both fixtures write a good import above the bad one. That is the whole point
// of them: a span that named the file, or that named whichever import the
// loader looked at first, would slice to `import "there.protop";` and this row
// would say so. Without the good import above, a span pointing anywhere in the
// header would be indistinguishable from the right one.
//
// What is underlined is the import statement entire, `;` included, rather than
// the string inside it. That is deliberate and it is not the shape
// nanov/protobuf-to-messagepack-sharp#57 objects to elsewhere: there the span
// covers a container when the thing at fault is one name inside it, and here
// the whole statement is what protopc could not carry out. There is nothing
// inside it that is right.
func TestALoaderDiagnosticUnderlinesTheImportThatCausedIt(t *testing.T) {
	for _, tt := range []struct {
		name  string
		roots []string
		files []file
		want  diag.Code
		text  string
	}{
		{
			name:  "an import nothing provides",
			roots: []string{"."},
			files: []file{
				{"a.protop", src("A", "there.protop", "gone.protop")},
				{"there.protop", src("There")},
			},
			want: diag.ImportNotFound,
			text: `import "gone.protop";`,
		},
		{
			name:  "an import two roots provide",
			roots: []string{".", "one", "two"},
			files: []file{
				{"a.protop", src("A", "there.protop", "both.protop")},
				{"there.protop", src("There")},
				{"one/both.protop", src("One")},
				{"two/both.protop", src("Two")},
			},
			want: diag.ImportAmbiguous,
			text: `import "both.protop";`,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := plant(t, tt.files...)
			_, bag, err := loadIn(t, dir, tt.roots, []string{"a.protop"})
			if err != nil {
				t.Fatal(err)
			}
			assertCodes(t, bag, tt.want)
			// Line 4 is the second import: revision, namespace, import,
			// import, as src writes them.
			assertUnderlines(t, bag, 0, 4, 1, tt.text)
		})
	}
}

// A parse error is stamped with the file it was found in, too. The parser sees
// one file at a time and does not name one; the loader holds several and is the
// only thing that knows which parse produced which diagnostic.
func TestAParseErrorNamesTheFileItWasFoundIn(t *testing.T) {
	dir := plant(t,
		file{"a.protop", src("A", "b.protop")},
		file{"b.protop", "revision 1;\nnamespace B;\nstruct {\n"},
	)
	_, bag, err := loadIn(t, dir, []string{"."}, []string{"a.protop"})
	if err != nil {
		t.Fatal(err)
	}
	if bag.Len() == 0 {
		t.Fatal("a file that does not parse produced no diagnostic")
	}
	for i, d := range bag.All() {
		if d.File == nil || !strings.HasSuffix(d.File.Name, "b.protop") {
			t.Errorf("diagnostic %d (%s) names %v, want b.protop", i, d.Code, d.File)
		}
	}
}

// An input that is not there is the *command* being wrong, and the caller turns
// that into exit 2. It is held apart from a missing import on purpose: the user
// typed this path.
func TestAMissingInputIsAnError(t *testing.T) {
	dir := plant(t, file{"a.protop", src("A")})
	_, bag, err := loadIn(t, dir, []string{"."}, []string{"gone.protop"})
	if err == nil {
		t.Fatal("a missing input must be an error")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("err = %v, want a not-exist error", err)
	}
	if bag.Len() != 0 {
		t.Errorf("a missing input raised diagnostics: %v", codes(bag))
	}
}

// A -I that is not a directory is the command being wrong as well, and it is
// worth catching because the failure it otherwise produces is every import
// going missing at once -- which reads as a broken schema rather than as a
// mistyped flag.
func TestARootThatIsNotADirectoryIsAnError(t *testing.T) {
	dir := plant(t, file{"a.protop", src("A")})
	if _, _, err := loadIn(t, dir, []string{"nosuch"}, []string{"a.protop"}); err == nil {
		t.Error("a -I naming nothing must be an error")
	}
	if _, _, err := loadIn(t, dir, []string{"a.protop"}, []string{"a.protop"}); err == nil {
		t.Error("a -I naming a file must be an error")
	}
}

// Two inputs that resolve to one import string are an error rather than a
// silent loss. Under roots [one, two], both `one/x.protop` and `two/x.protop`
// want the key "x.protop", and an import of "x.protop" can only ever mean one
// of them -- so the other is a file the build can never name.
func TestTwoInputsClaimingOneImportPathIsAnError(t *testing.T) {
	dir := plant(t,
		file{"one/x.protop", src("One")},
		file{"two/x.protop", src("Two")},
	)
	_, _, err := loadIn(t, dir, []string{"one", "two"}, []string{"one/x.protop", "two/x.protop"})
	if err == nil {
		t.Fatal("two inputs claiming one key must be an error")
	}
	if !strings.Contains(err.Error(), "x.protop") {
		t.Errorf("err = %v, want it to name the key", err)
	}
}

// An input that lies under no root keeps its own path as its key. It is a file
// nothing can import, which is true and is not an error: a one-file compile
// with no imports is a legitimate thing to ask for.
func TestAnInputUnderNoRootKeepsItsOwnPath(t *testing.T) {
	dir := plant(t,
		file{"elsewhere/a.protop", src("A")},
		file{"roots/.keep", ""},
	)
	res, bag, err := loadIn(t, dir, []string{"roots"}, []string{"elsewhere/a.protop"})
	if err != nil {
		t.Fatal(err)
	}
	assertCodes(t, bag)
	if len(res.Targets) != 1 || !strings.HasSuffix(res.Targets[0], "elsewhere/a.protop") {
		t.Errorf("targets = %v, want the input's own path", res.Targets)
	}
}

// The set the loader builds is the set the checker reads: an import resolves
// against the map keys, so a loader that keyed them differently would produce
// PROTOP0107 for a file it had just opened. This is the seam M4 left and M6
// fills, and nothing else in either suite crosses it.
//
// The invariant is not "the canonical spelling is in the set". It is that
// *every* import string the loader resolved is, because the checker has no
// other index: it holds the string the user wrote and looks it up, and the
// loader is the only stage that knows the string it looked up is a file that
// is already open.
func TestTheSetTheLoaderBuildsIsTheSetTheCheckerReads(t *testing.T) {
	t.Run("one spelling", func(t *testing.T) {
		dir := plant(t,
			file{"schemas/nested/a.protop", src("A", "nested/b.protop")},
			file{"schemas/nested/b.protop", src("B")},
		)
		res, bag, err := loadIn(t, dir, []string{"schemas"}, []string{"schemas/nested/a.protop"})
		if err != nil {
			t.Fatal(err)
		}
		schema := check.Check(res.Set, bag)
		assertCodes(t, bag)
		if schema == nil || len(schema.Files) != 2 {
			t.Fatalf("schema = %v, want two files", schema)
		}
		a := schema.File("nested/a.protop")
		if a == nil {
			t.Fatal("the checker does not know the file by the key the loader used")
		}
		if len(a.Imports) != 1 {
			t.Errorf("a.protop has %d imports, want 1", len(a.Imports))
		}
	})

	// Two spellings of one path, in two files of one compile. Each file alone
	// loads and checks clean; the pair is what breaks a loader whose set only
	// answers for the spelling that reached the file first. `./b.protop`
	// resolves -- filepath.Join cleans it on the way to the disk -- and then
	// `b.protop` resolves to the same file and finds it already open, and a
	// loader that stops there has resolved an import it cannot hand over.
	//
	// Nothing here says `./b.protop` is a legal import string. Whether a
	// non-canonical spelling should be rejected outright is a language
	// question, unruled; what is ruled is that a *resolved* import is in the
	// set, so that the answer to that question is never "PROTOP0107, for a
	// file that is on disk and already loaded".
	t.Run("two spellings of one path", func(t *testing.T) {
		for _, alias := range []string{"./b.protop", "sub/../b.protop"} {
			t.Run(alias, func(t *testing.T) {
				dir := plant(t,
					file{"s/a.protop", src("A", alias)},
					file{"s/c.protop", src("C", "b.protop")},
					file{"s/b.protop", src("B")},
					file{"s/sub/.keep", ""},
				)
				res, bag, err := loadIn(t, dir, []string{"s"}, []string{"s/a.protop", "s/c.protop"})
				if err != nil {
					t.Fatal(err)
				}
				assertCodes(t, bag)
				if _, ok := res.Set["b.protop"]; !ok {
					t.Fatalf("%q resolved and %q did not reach the set: %v", alias, "b.protop", keys(res.Set))
				}
				check.Check(res.Set, bag)
				assertCodes(t, bag)
			})
		}
	})
}
