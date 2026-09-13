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
	if got := codes(bag); len(got) != 0 {
		t.Fatalf("diagnostics %v", got)
	}
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
	if got := codes(bag); len(got) != 0 {
		t.Fatalf("diagnostics %v", got)
	}
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
	if got, want := codes(bag), []string{"PROTOP0107"}; !slices.Equal(got, want) {
		t.Errorf("codes = %v, want %v", got, want)
	}
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
	if got := codes(bag); len(got) != 0 {
		t.Fatalf("diagnostics %v", got)
	}
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
	if got, want := codes(bag), []string{"PROTOP0108"}; !slices.Equal(got, want) {
		t.Fatalf("codes = %v, want %v", got, want)
	}
	msg := bag.All()[0].Message
	for _, want := range []string{filepath.Join("one", "b.protop"), filepath.Join("two", "b.protop")} {
		if !strings.Contains(msg, want) {
			t.Errorf("message does not name %q:\n%s", want, msg)
		}
	}
}

// One root listed twice is one root. Two spellings of one directory provide one
// file, not two, and reporting that as an ambiguity would be a diagnostic about
// the command line rather than about the schema.
func TestOneRootGivenTwiceIsNotAnAmbiguity(t *testing.T) {
	dir := plant(t,
		file{"a.protop", src("A", "b.protop")},
		file{"one/b.protop", src("One")},
	)
	_, bag, err := loadIn(t, dir, []string{"one", "one/", "./one"}, []string{"a.protop"})
	if err != nil {
		t.Fatal(err)
	}
	if got := codes(bag); len(got) != 0 {
		t.Errorf("codes = %v, want none", got)
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
	if got := codes(bag); len(got) != 0 {
		t.Fatalf("diagnostics %v", got)
	}
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
	if got, want := codes(bag), []string{"PROTOP0107"}; !slices.Equal(got, want) {
		t.Fatalf("codes = %v, want %v", got, want)
	}
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
	if got := codes(bag); len(got) != 0 {
		t.Fatalf("diagnostics %v", got)
	}
	if len(res.Targets) != 1 || !strings.HasSuffix(res.Targets[0], "elsewhere/a.protop") {
		t.Errorf("targets = %v, want the input's own path", res.Targets)
	}
}

// The set the loader builds is the set the checker reads: an import resolves
// against the map keys, so a loader that keyed them differently would produce
// PROTOP0107 for a file it had just opened. This is the seam M4 left and M6
// fills, and nothing else in either suite crosses it.
func TestTheSetTheLoaderBuildsIsTheSetTheCheckerReads(t *testing.T) {
	dir := plant(t,
		file{"schemas/nested/a.protop", src("A", "nested/b.protop")},
		file{"schemas/nested/b.protop", src("B")},
	)
	res, bag, err := loadIn(t, dir, []string{"schemas"}, []string{"schemas/nested/a.protop"})
	if err != nil {
		t.Fatal(err)
	}
	schema := check.Check(res.Set, bag)
	if got := codes(bag); len(got) != 0 {
		t.Fatalf("diagnostics %v", got)
	}
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
}
