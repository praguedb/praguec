package csharp

import (
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

// The corpus is the oracle.
//
// The goldens under testdata/golden are the emitter held against itself: they
// catch a change, and `-update` can rewrite them. Nothing in this file can be
// rewritten by a flag. corpus/prague/ is 29 hand-written C# files copied from a
// running system, and every assertion here is protopc's output measured against
// them.
//
// The shape of the measurement is a coverage table with one row per corpus
// file. A row either names the declaration that reproduces it or names the
// ticket that blocks it, and TestCoverageTableNamesEveryCorpusFile makes the
// table exhaustive -- so a corpus file this milestone cannot emit is a line of
// evidence rather than a silence. Within a covered row, a key protopc does not
// write is listed in Missing with its ticket, and a key it writes differently
// is listed in Diverges with what the difference is.

const corpusRoot = "../../corpus/prague"

const examplesDir = "../../examples/prague"

// corpusRow is one corpus file and what protopc does about it.
type corpusRow struct {
	// Path is relative to corpus/prague.
	Path string

	// Decl is the declaration in examples/prague that reproduces this file, or
	// "" when nothing does.
	Decl string

	// Blocked says why, when Decl is "". Every one of them names a ticket or a
	// reason that is not protopc's to fix.
	Blocked string

	// Extra are the corpus's using lines protopc cannot derive. Section 3 has
	// protopc never inspect an annotation's contents, so a namespace that is
	// only reachable through an attribute is not reachable at all -- and a
	// member protopc does not write takes its namespace with it.
	Extra []string

	// Missing are corpus keys the emission does not contain, and why. Each is
	// a `reserved` tag in the protop.
	Missing map[int]string

	// Diverges are corpus keys the emission contains and writes differently.
	Diverges map[int]string
}

const (
	cacheCore  = "B2Tech.Feed.Cache.Core"         // every [DataCache*] attribute
	jsonIgnore = "System.Text.Json.Serialization" // [JsonIgnore] on the alias
	liveObject = "Common.IntegratedEventApi.Models.LiveObject"
)

// temporal is #15: the corpus carries instants as DateTime and protop has no
// temporal type. It is the single largest blocker in the corpus -- 8 of the 14
// payload files lose at least one member to it.
const temporal = "#15, no temporal type; corpus DateTime"

// unsigned is #32: the corpus's `byte ProviderId` has no protop spelling and
// widening the scalar set to give it one is #32's ruling, not this milestone's.
const unsigned = "#32, closed scalar set; corpus byte"

var coverage = []corpusRow{
	// --- deps ---------------------------------------------------------------
	{Path: "deps/BaseProviderObject.cs", Decl: "BaseProviderObject"},
	{Path: "deps/IBaseLiveData.cs",
		Blocked: "#9, unions. A 19-arm interface, and protop has no sum type."},
	{Path: "deps/SportGameSlimExtensions.cs",
		Blocked: "not a MessagePack type: a static projection helper, which is code rather than schema."},
	{Path: "deps/StaticDataRef.cs",
		Blocked: "#11, inheritance. The corpus writes `class StaticDataRef : BaseProviderObject` with an empty body; #11 ruled protop flattens, so the emission carries the four inherited members and the C# hierarchy forks."},

	// --- payloads: the branded producer -------------------------------------
	{Path: "payloads/Venue.cs", Decl: "Venue"},
	{Path: "payloads/BPSportGameCacheItem.cs", Decl: "BPSportGameCacheItem",
		Extra: []string{cacheCore},
		Missing: map[int]string{
			11: unsigned, 27: temporal, 28: temporal, 29: temporal,
			30: temporal, 31: temporal, 50: temporal,
		}},
	{Path: "payloads/BPBetTypeCacheItem.cs", Decl: "BPBetTypeCacheItem",
		Extra:   []string{cacheCore, jsonIgnore},
		Missing: map[int]string{15: temporal, 16: temporal, 17: temporal}},
	{Path: "payloads/BPSearchCacheItem.cs", Decl: "BPSearchCacheItem",
		Extra:   []string{cacheCore},
		Missing: map[int]string{31: temporal}},
	{Path: "payloads/BPLiveObjectCacheItem.cs", Decl: "BPLiveObjectCacheItem",
		Extra:   []string{cacheCore, liveObject},
		Missing: map[int]string{1: "#9, unions; corpus IBaseLiveData?"}},

	// --- payloads: the back office ------------------------------------------
	{Path: "payloads/BOVenue.cs", Decl: "BOVenue"},
	{Path: "payloads/BOSportGameCacheItem.cs", Decl: "BOSportGameCacheItem",
		Extra: []string{cacheCore},
		Missing: map[int]string{
			11: unsigned, 27: temporal, 28: temporal, 29: temporal,
			30: temporal, 31: temporal, 50: temporal,
		}},
	{Path: "payloads/BOBetTypeCacheItem.cs", Decl: "BOBetTypeCacheItem",
		Extra:   []string{cacheCore, jsonIgnore},
		Missing: map[int]string{15: temporal, 16: temporal, 17: temporal}},
	{Path: "payloads/BOSearchCacheItem.cs", Decl: "BOSearchCacheItem",
		Extra:   []string{cacheCore},
		Missing: map[int]string{31: temporal}},
	{Path: "payloads/BOLiveObjectCacheItem.cs", Decl: "BOLiveObjectCacheItem",
		Extra:   []string{cacheCore, liveObject},
		Missing: map[int]string{1: "#9, unions; corpus IBaseLiveData?"}},

	// --- payloads: unbranded and the views ----------------------------------
	{Path: "payloads/LiveObjectCacheItem.cs", Decl: "LiveObjectCacheItem",
		Extra:   []string{cacheCore, liveObject},
		Missing: map[int]string{1: "#9, unions; corpus IBaseLiveData?"}},
	{Path: "payloads/SlimSportGame.cs", Decl: "SlimSportGame",
		Extra:   []string{cacheCore},
		Missing: map[int]string{11: unsigned, 27: temporal, 29: temporal},
		Diverges: map[int]string{
			53: "the corpus view types this `EventSource?` and the table it is a view of types it `int?`. A protop view selects a member; it cannot retype one (section 5), so the emission follows the table. Either the corpus is drifting between two hand-written files or views want a narrowing this language does not have -- and the corpus alone cannot say which.",
			57: "the corpus view types this `StaticDataRef` and the table types it `StaticDataRef?`. A view cannot change presence either.",
			58: "same as 57.",
		}},
	{Path: "payloads/SlimBetType.cs", Decl: "SlimBetType",
		Extra: []string{cacheCore},
		Diverges: map[int]string{
			25: "#24. The corpus writes `SlimOdd[]` here and `List<T>` at its other six collection sites; protopc writes the majority form. Inferring `a view emits T[]` from one sample is a language ruling.",
		}},
	{Path: "payloads/SlimOdd.cs", Decl: "SlimOdd"},

	// --- shells -------------------------------------------------------------
	// The 11 hand-written halves. #25 owns whether protopc emits a shell at
	// all; until it rules, none of these is protopc's output to check.
	//
	// SOURCE.md says every shell is empty. Eight of the eleven are not: the
	// BO and BP shells carry a `Clear()`, and the Search ones carry more. That
	// is evidence for #25 rather than against the corpus -- the hand-written
	// half is not a formality that could be generated away.
	{Path: "shells/BOBetTypeCache.cs", Blocked: shellBlocked},
	{Path: "shells/BOLiveObjectCache.cs", Blocked: shellBlocked},
	{Path: "shells/BOSearchCache.cs", Blocked: shellBlocked},
	{Path: "shells/BOSportGameCache.cs", Blocked: shellBlocked},
	{Path: "shells/BPBetTypeCache.cs", Blocked: shellBlocked},
	{Path: "shells/BPLiveObjectCache.cs", Blocked: shellBlocked},
	{Path: "shells/BPSearchCache.cs", Blocked: shellBlocked},
	{Path: "shells/BPSportGameCache.cs", Blocked: shellBlocked},
	{Path: "shells/LiveObjectCache.cs", Blocked: shellBlocked},
	{Path: "shells/SlimBetTypeCache.cs", Blocked: shellBlocked},
	{Path: "shells/SlimSportGameCache.cs", Blocked: shellBlocked},
}

const shellBlocked = "#25, the hand-written half. Not a [MessagePackObject] type and not protopc's output."

// blockedPaths is the corpus this milestone does not reproduce, written out
// rather than counted.
//
// TestCoverageTableNamesEveryCorpusFile holds the table against the tree, and a
// row that moves from reproduced to blocked satisfies it just as well: the file
// is still listed, still carries a reason, and stops being compared against
// anything. That is how coverage is lost -- not by deleting a row but by
// demoting one -- and a type that contributes no normalisation site can be
// demoted today with nothing failing at all. So the blocked set is a list: it
// takes an edit here, next to the eleven shells and the three deps, to stop
// measuring a payload.
var blockedPaths = []string{
	"deps/IBaseLiveData.cs",
	"deps/SportGameSlimExtensions.cs",
	"deps/StaticDataRef.cs",
	"shells/BOBetTypeCache.cs",
	"shells/BOLiveObjectCache.cs",
	"shells/BOSearchCache.cs",
	"shells/BOSportGameCache.cs",
	"shells/BPBetTypeCache.cs",
	"shells/BPLiveObjectCache.cs",
	"shells/BPSearchCache.cs",
	"shells/BPSportGameCache.cs",
	"shells/LiveObjectCache.cs",
	"shells/SlimBetTypeCache.cs",
	"shells/SlimSportGameCache.cs",
}

// --- loading -----------------------------------------------------------------

// exampleUnits compiles every file of examples/prague and returns the emission
// by declaration name.
//
// examples/prague is the protop that renders the corpus. Section 1 declares
// `SportGame` once and the corpus has BO, BP and Slim variants of it, so the
// file the oracle needs did not exist and writing it is part of this milestone.
func exampleUnits(t *testing.T) map[string]Unit {
	t.Helper()
	ents, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatal(err)
	}
	var bag diag.Bag
	set := make(check.FileSet, len(ents))
	var any *source.File
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".protop") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(examplesDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		f := source.NewFile(e.Name(), string(b))
		any = f
		set[e.Name()] = check.Unit{Source: f, File: syntax.Parse(f, &bag)}
	}
	if len(set) == 0 {
		t.Fatal("examples/prague has no .protop files")
	}
	schema := check.Check(set, &bag)
	if bag.Len() != 0 {
		t.Fatalf("examples/prague does not check cleanly:\n%s", diag.RenderAll(any, &bag))
	}
	out := map[string]Unit{}
	for _, u := range Emit(schema) {
		out[u.TypeName] = u
	}
	return out
}

// exampleDecl is one declaration of examples/prague as the AST has it.
type exampleDecl struct {
	// Kind is the keyword it was written with.
	Kind string

	// Reserved is the tags its `reserved` lists retire. A view has none, and
	// the nil is the point rather than an omission: a view selects, and a
	// member it does not select is not a retired tag, because the source still
	// owns it.
	Reserved map[uint64]bool
}

// exampleDeclarations is every declaration in examples/prague, by name.
//
// It reads the AST rather than the checked model, because `reserved` is not in
// the checked model at all -- it names nothing and holds nothing, so the
// checker records the ranges and drops them. That is the right shape for a
// backend and the wrong shape for these assertions, which are about what the
// protop says rather than about what it emits.
func exampleDeclarations(t *testing.T) map[string]exampleDecl {
	t.Helper()
	ents, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]exampleDecl{}
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".protop") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(examplesDir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var bag diag.Bag
		f := syntax.Parse(source.NewFile(e.Name(), string(b)), &bag)
		for _, d := range f.Decls {
			var name, kind string
			var members []syntax.Member
			switch d := d.(type) {
			case *syntax.Struct:
				name, kind, members = d.Name.Text, "struct", d.Members
			case *syntax.Record:
				name, kind, members = d.Name.Text, "record", d.Members
			case *syntax.Table:
				name, kind, members = d.Name.Text, "table", d.Members
			case *syntax.View:
				name, kind = d.Name.Text, "view"
			case *syntax.Enum:
				name, kind = d.Name.Text, "enum"
			default:
				t.Fatalf("%s declares a %T, which this census does not know how to read", e.Name(), d)
			}
			if _, dup := out[name]; dup {
				t.Fatalf("examples/prague declares %s twice", name)
			}
			tags := map[uint64]bool{}
			for _, m := range members {
				r, ok := m.(*syntax.Reserved)
				if !ok {
					continue
				}
				for _, rng := range r.Ranges {
					for tag := rng.Lo.Value; tag <= rng.Hi.Value; tag++ {
						tags[tag] = true
					}
				}
			}
			if kind == "view" {
				tags = nil
			}
			out[name] = exampleDecl{Kind: kind, Reserved: tags}
		}
	}
	if len(out) == 0 {
		t.Fatal("examples/prague declares nothing at all")
	}
	return out
}

func readCorpus(t *testing.T, path string) *csDecl {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(corpusRoot, path))
	if err != nil {
		t.Fatal(err)
	}
	d, err := readCS(path, string(b))
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// --- the table is exhaustive --------------------------------------------------

// TestCoverageTableNamesEveryCorpusFile is the assertion the rest of this file
// rests on. Without it the harness could cover three files and look complete;
// with it, a corpus file that is not reproduced has to carry a written reason.
func TestCoverageTableNamesEveryCorpusFile(t *testing.T) {
	var onDisk []string
	err := filepath.Walk(corpusRoot, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(p, ".cs") {
			return nil
		}
		rel, err := filepath.Rel(corpusRoot, p)
		if err != nil {
			return err
		}
		onDisk = append(onDisk, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(onDisk)

	var listed []string
	for _, r := range coverage {
		listed = append(listed, r.Path)
		if r.Decl == "" && r.Blocked == "" {
			t.Errorf("%s: neither reproduced nor blocked. A corpus file is covered or it names a ticket; there is no third option.", r.Path)
		}
		if r.Decl != "" && r.Blocked != "" {
			t.Errorf("%s: both reproduced by %s and blocked by %q", r.Path, r.Decl, r.Blocked)
		}
	}
	sort.Strings(listed)

	if strings.Join(onDisk, "\n") != strings.Join(listed, "\n") {
		t.Errorf("the coverage table and corpus/prague/ disagree.\nonly on disk: %v\nonly in table: %v",
			missingFrom(onDisk, listed), missingFrom(listed, onDisk))
	}

	var blocked []string
	for _, r := range coverage {
		if r.Decl == "" {
			blocked = append(blocked, r.Path)
		}
	}
	sort.Strings(blocked)
	want := slices.Clone(blockedPaths)
	sort.Strings(want)
	if strings.Join(blocked, "\n") != strings.Join(want, "\n") {
		t.Errorf("the blocked set changed.\nnewly blocked: %v\nnewly reproduced: %v",
			missingFrom(blocked, want), missingFrom(want, blocked))
	}
}

// TestBlockedCorpusFilesNameATicket keeps the blocked column from decaying into
// a shrug. Every reason is either a ticket number or an explicit statement that
// the file is not schema.
func TestBlockedCorpusFilesNameATicket(t *testing.T) {
	for _, r := range coverage {
		if r.Decl != "" {
			continue
		}
		if !strings.Contains(r.Blocked, "#") && !strings.Contains(r.Blocked, "not a MessagePack type") {
			t.Errorf("%s: blocked by %q, which names no ticket", r.Path, r.Blocked)
		}
	}
}

// --- the covered files --------------------------------------------------------

// TestCorpusHeaders holds the emitted header against the corpus's: the same
// usings, the same namespace, the same type attributes and the same
// declaration line.
//
// Usings compare as a set. The corpus sorts `System.*` first (see
// payloads/BPBetTypeCacheItem.cs, lines 1 to 5) and protopc sorts ordinally;
// which order is right is a style question that no ticket owns and that nothing
// on the wire depends on. What the set comparison still catches is a using that
// should have been derived and was not, which is the part that matters.
func TestCorpusHeaders(t *testing.T) {
	units := exampleUnits(t)
	for _, r := range coverage {
		if r.Decl == "" {
			continue
		}
		t.Run(r.Decl, func(t *testing.T) {
			want := readCorpus(t, r.Path)
			u, ok := units[r.Decl]
			if !ok {
				t.Fatalf("examples/prague declares no %s", r.Decl)
			}
			got, err := readCS(r.Decl, u.Source)
			if err != nil {
				t.Fatal(err)
			}

			if got.Namespace != want.Namespace {
				t.Errorf("namespace = %q, corpus has %q", got.Namespace, want.Namespace)
			}
			if got.Head != want.Head {
				t.Errorf("declaration = %q, corpus has %q", got.Head, want.Head)
			}
			if strings.Join(got.Attrs, "\n") != strings.Join(want.Attrs, "\n") {
				t.Errorf("type attributes =\n%v\ncorpus has\n%v", got.Attrs, want.Attrs)
			}
			// The comparison above is two empty lists when the reader stops
			// recording type-level attributes, and then `[MessagePackObject]`
			// and every `[cs: DataCache(...)]` passthrough agree by being
			// absent from both sides. All 15 covered corpus files carry
			// `[MessagePackObject]` and protopc writes it on every non-enum
			// declaration, so both lists are non-empty and both name it.
			for what, attrs := range map[string][]string{r.Path: want.Attrs, "the emission": got.Attrs} {
				if !slicesContains(attrs, "MessagePackObject") {
					t.Errorf("%s carries no [MessagePackObject]: type attributes are %v", what, attrs)
				}
			}

			derivable := missingFrom(want.Usings, r.Extra)
			if missed := missingFrom(derivable, got.Usings); len(missed) > 0 {
				t.Errorf("usings missing from the emission: %v", missed)
			}
			if extra := missingFrom(got.Usings, want.Usings); len(extra) > 0 {
				t.Errorf("usings the corpus does not have: %v", extra)
			}
			for _, e := range r.Extra {
				if !slicesContains(want.Usings, e) {
					t.Errorf("Extra lists %q, which %s does not use", e, r.Path)
				}
				if slicesContains(got.Usings, e) {
					t.Errorf("Extra lists %q, but the emission derived it after all", e)
				}
			}
		})
	}
}

// TestCorpusMembers is the milestone's central claim: for every corpus key the
// emission writes, it writes the same member.
//
// The keys it does not write are Missing, each with the ticket that blocks it,
// and the test checks that set exactly -- a member that silently stopped being
// emitted fails here even though nothing compares it.
func TestCorpusMembers(t *testing.T) {
	units := exampleUnits(t)
	for _, r := range coverage {
		if r.Decl == "" {
			continue
		}
		t.Run(r.Decl, func(t *testing.T) {
			want := readCorpus(t, r.Path)
			got, err := readCS(r.Decl, units[r.Decl].Source)
			if err != nil {
				t.Fatal(err)
			}

			for _, id := range want.Order {
				n, keyed := keyNumber(id)
				if keyed {
					if why, skip := r.Missing[n]; skip {
						if _, emitted := got.Members[id]; emitted {
							t.Errorf("key %d is listed Missing (%s) but the emission writes it", n, why)
						}
						continue
					}
				}
				g, ok := got.Members[id]
				if !ok {
					t.Errorf("member %s is in the corpus and not in the emission, and not listed Missing", id)
					continue
				}
				if keyed {
					if why, diverges := r.Diverges[n]; diverges {
						if g == want.Members[id] {
							t.Errorf("key %d is listed as diverging (%s) but the emission matches the corpus", n, why)
						}
						continue
					}
				}
				if g != want.Members[id] {
					t.Errorf("member %s:\n got %s\ncorpus %s", id, g, want.Members[id])
				}
			}
			for _, id := range got.Order {
				if _, ok := want.Members[id]; !ok {
					t.Errorf("member %s is emitted and is not in the corpus", id)
				}
			}
			// And the count, which is the same claim as the loop above made
			// arithmetic: the emission writes the corpus's members, minus the
			// ones listed Missing, and nothing else. Without it the reverse
			// direction is one deletable loop -- delete it and the test stops
			// checking that emission invents nothing, while every remaining
			// assertion still passes.
			if n, w := len(got.Order), len(want.Members)-len(r.Missing); n != w {
				t.Errorf("the emission writes %d members; %s has %d, of which %d are listed Missing",
					n, r.Path, len(want.Members), len(r.Missing))
			}
			for n := range r.Missing {
				if _, ok := want.Members[itoa(n)]; !ok {
					t.Errorf("Missing lists key %d, which %s does not have", n, r.Path)
				}
			}
			for n := range r.Diverges {
				if _, ok := want.Members[itoa(n)]; !ok {
					t.Errorf("Diverges lists key %d, which %s does not have", n, r.Path)
				}
			}
		})
	}
}

// TestCorpusKeyOrder holds the order of the emitted keys against the corpus's.
//
// Order is not the wire format -- a MessagePack key is an index and the array
// is written by key, not by declaration order -- so this is a readability
// claim rather than a correctness one. It is still worth pinning: a backend
// that emitted its members in map order would pass every other test in this
// file and produce a diff against the corpus on every run.
func TestCorpusKeyOrder(t *testing.T) {
	units := exampleUnits(t)
	for _, r := range coverage {
		if r.Decl == "" {
			continue
		}
		t.Run(r.Decl, func(t *testing.T) {
			want := readCorpus(t, r.Path)
			got, err := readCS(r.Decl, units[r.Decl].Source)
			if err != nil {
				t.Fatal(err)
			}
			var trimmed []int
			for _, k := range want.Keys() {
				if _, missing := r.Missing[k]; !missing {
					trimmed = append(trimmed, k)
				}
			}
			if fmtInts(got.Keys()) != fmtInts(trimmed) {
				t.Errorf("key order = %s, corpus (minus the reserved) has %s",
					fmtInts(got.Keys()), fmtInts(trimmed))
			}
		})
	}
}

// assertTheCensusReachesTheCoverageTable is what keeps the two tests above from
// being switched off by the thing they read.
//
// Both of them iterate exampleDeclarations, one over the rows and one over the
// census itself, and neither can tell "this declaration retires nothing" from
// "this declaration was never read". An exampleDeclarations returning an empty
// map, or one that stopped reading `table`, makes the whole Missing <-> reserved
// cross-check vacuous with nothing failing -- so the census is held to the
// coverage table first, and every reproduced declaration has to be in it.
func assertTheCensusReachesTheCoverageTable(t *testing.T, decls map[string]exampleDecl) {
	t.Helper()
	reached := 0
	for _, r := range coverage {
		if r.Decl == "" {
			continue
		}
		d, ok := decls[r.Decl]
		if !ok {
			t.Errorf("%s is reproduced by %s and the declaration census does not reach it", r.Path, r.Decl)
			continue
		}
		if d.Kind == "view" && d.Reserved != nil {
			t.Errorf("%s is a view and the census gave it a reserved list", r.Decl)
		}
		if d.Kind != "view" && d.Reserved == nil {
			t.Errorf("%s is a %s and the census gave it no reserved list", r.Decl, d.Kind)
		}
		reached++
	}
	if reached == 0 {
		t.Fatal("the coverage table names no reproduced declaration at all")
	}
}

// TestEveryMissingKeyIsReservedInTheProtop closes the coverage table's one
// remaining silence.
//
// Every other assertion here is about emitted text, and `reserved` emits
// nothing -- so deleting a tag from a `reserved` list changes no output and
// fails nothing. What it does change is the language's cross-repo guarantee:
// section 7 retires a tag where it was declared, and a tag that is not retired
// is a tag the next member may take. Tag 51 of BPSportGameCacheItem is the
// corpus's `DateTime? BetDataLastUpdateAt`; dropping it from the reserved list
// leaves the slot free for a future field to silently collide with a member of
// a running system.
//
// So: a key listed Missing must be reserved in the declaration that claims it.
// Views are exempt, and the exemption is the definition of a view -- it selects
// from its source, and not selecting is not retiring.
func TestEveryMissingKeyIsReservedInTheProtop(t *testing.T) {
	decls := exampleDeclarations(t)
	assertTheCensusReachesTheCoverageTable(t, decls)
	for _, r := range coverage {
		if r.Decl == "" || len(r.Missing) == 0 {
			continue
		}
		// The exemption is a view *being* a view, not a declaration being
		// absent from the census. Read the other way round -- "no entry, so
		// nothing to check" -- this test skips every row the moment the census
		// stops reading `table`, and skips all of them the moment it returns
		// nothing.
		if decls[r.Decl].Kind == "view" {
			continue
		}
		tags := decls[r.Decl].Reserved
		for key, why := range r.Missing {
			tag := uint64(key) + 1
			if !tags[tag] {
				t.Errorf("%s: key %d is listed Missing (%s) but tag %d is not reserved; the slot is free for the next member to take",
					r.Decl, key, why, tag)
			}
		}
	}
}

// The other direction: a reserved tag that is not listed Missing is a tag
// retired for no recorded reason. `reserved` is cheap to write and the coverage
// table is where the reason lives, so the two are held to each other -- a
// reservation with no row is a gap nobody can look up.
func TestEveryReservedTagIsListedMissing(t *testing.T) {
	decls := exampleDeclarations(t)
	assertTheCensusReachesTheCoverageTable(t, decls)
	byDecl := map[string]corpusRow{}
	for _, r := range coverage {
		if r.Decl != "" {
			byDecl[r.Decl] = r
		}
	}
	for decl, d := range decls {
		r, ok := byDecl[decl]
		if !ok {
			continue
		}
		for tag := range d.Reserved {
			if _, listed := r.Missing[int(tag)-1]; !listed {
				t.Errorf("%s reserves tag %d, and the coverage table gives no reason for it", decl, tag)
			}
		}
	}
}

// --- the input side of the oracle ---------------------------------------------

// exampleSupport is every declaration in examples/prague that no coverage row
// reproduces, and the reason it is written anyway.
//
// The coverage table makes the *output* side exhaustive: a corpus file is
// reproduced or it names a ticket. Nothing made the input side exhaustive, so a
// declaration could be added to examples/prague -- reviewed by nobody, compared
// against nothing, emitting whatever it liked -- and the whole suite stayed
// green. This map is the other half: a name here is a declaration the corpus
// cannot check and that has a written reason for existing.
var exampleSupport = map[string]string{
	// SOURCE.md lists the enum declarations among the types deliberately not
	// copied, so the corpus carries every use of these names and not one of
	// their definitions. They are declared with no arms for that reason: the
	// numbers are the whole of an enum's wire contract and inventing them would
	// put unchecked values on the wire. What they exist for is to make the
	// names resolve, so members carry a type and files carry a using.
	"EventStatus":    "CommonIntegratedEventApi enum; SOURCE.md does not copy the declaration",
	"BookingStatus":  "CommonIntegratedEventApi enum; SOURCE.md does not copy the declaration",
	"Channel":        "CommonIntegratedEventApi enum; SOURCE.md does not copy the declaration",
	"BetTypeStatus":  "CommonFeedModels enum; SOURCE.md does not copy the declaration",
	"OddStatus":      "CommonFeedModels enum; SOURCE.md does not copy the declaration",
	"WinStatus":      "CommonFeedModels enum; SOURCE.md does not copy the declaration",
	"WatchDogAction": "CommonFeedModels enum; SOURCE.md does not copy the declaration",

	// deps/StaticDataRef.cs is in the coverage table and is blocked by #11:
	// the corpus writes `class StaticDataRef : BaseProviderObject` with an
	// empty body and protop flattens. So the row names no declaration, and the
	// declaration is still needed -- both SportGame twins carry eight members
	// typed by it.
	"StaticDataRef": "#11; the corpus file is blocked and the type is still referenced",

	// Not copied by SOURCE.md at all, and referenced by members that are:
	// `Odd` is what SlimOdd is a view of and what the corpus's six `List<Odd>`
	// members hold, and `Runner` is the element type of `List<Runner>` at key
	// 46 of both SportGame twins. Odd's tags are recoverable from SlimOdd,
	// which is a view of it; Runner's are not, so Runner is empty.
	"Odd":    "SOURCE.md does not copy it; SlimOdd is a view of it and six members hold it",
	"Runner": "SOURCE.md does not copy it; `List<Runner>` at key 46 of both SportGame twins",
}

// TestTheExamplesDeclareNothingUnreviewed makes the input side of the oracle
// exhaustive, the way the coverage table makes the output side exhaustive.
//
// examples/prague is what protopc compiles to produce everything this file
// measures, and until this test existed an `table UnreviewedJunk { ... }`
// appended to any of its six files emitted C# that no assertion in the package
// so much as read. Every declaration is now either a corpus row's Decl or a
// line of exampleSupport with a reason.
func TestTheExamplesDeclareNothingUnreviewed(t *testing.T) {
	declared := exampleDeclarations(t)

	want := map[string]string{}
	for name, why := range exampleSupport {
		want[name] = why
	}
	for _, r := range coverage {
		if r.Decl == "" {
			continue
		}
		if why, support := exampleSupport[r.Decl]; support {
			t.Errorf("%s is both reproduced by %s and listed as support (%s)", r.Decl, r.Path, why)
		}
		want[r.Decl] = "corpus/prague/" + r.Path
	}

	for name := range declared {
		if _, ok := want[name]; !ok {
			t.Errorf("examples/prague declares %s, which no coverage row reproduces and exampleSupport does not explain", name)
		}
	}
	for name, why := range want {
		if _, ok := declared[name]; !ok {
			t.Errorf("%s is expected in examples/prague (%s) and is not declared there", name, why)
		}
	}
}

// --- normalisation ------------------------------------------------------------

// TestNormalisationSitesAreEnumerated holds every normalisation that can hide
// an expressiveness gap against a checked-in list of where it fires.
//
// The whitespace-shaped ones are not here: a BOM, a trailing newline and a
// blank line between members cannot hide a member. These six can, so each is
// counted and named. The list is the shape of the gap between protop and the
// C# the corpus writes, and it is short enough to read.
func TestNormalisationSitesAreEnumerated(t *testing.T) {
	want := []string{
		// #12 closed on omitting `required` and #19 measured that MessagePack
		// does not enforce it. Two struct members, twice over.
		"required:Id", "required:Id", "required:Name", "required:Name",

		// #11 ruled that protop flattens rather than inherits, and under
		// flattening nothing overrides anything.
		"virtual:Name",

		// #20 forbids protopc emitting an immutable type: a computed member at
		// key 0 breaks deserializing-constructor matching on both MessagePack
		// paths. So `init` becomes `set`, at all fifteen corpus sites.
		"init:Id",
		"init:IsBalancedLine", "init:IsBoosted", "init:IsDynamic", "init:Line",
		"init:Name", "init:OddsActual", "init:OddsFair", "init:OddsManual",
		"init:PlayerId", "init:Status", "init:StatusManual", "init:Timestamp",
		"init:WinStatusId", "init:WinStatusIdManual",

		// protop has no default-value syntax. Every one of these is a C#
		// nullable-reference suppression rather than a fact about the wire --
		// `= string.Empty`, `= []`, `= null!` and one enum default -- but that
		// is a judgement about C#, and the list is here so that the judgement
		// is visible rather than assumed.
		"initialiser:AwayTeam = null!;", "initialiser:AwayTeam = null!;", "initialiser:AwayTeam = null!;",
		"initialiser:Country = null!;", "initialiser:Country = null!;", "initialiser:Country = null!;",
		"initialiser:EventStatus = EventStatus.Unknown;", "initialiser:EventStatus = EventStatus.Unknown;",
		"initialiser:Groups = [];", "initialiser:Groups = [];",
		"initialiser:HomeTeam = null!;", "initialiser:HomeTeam = null!;", "initialiser:HomeTeam = null!;",
		"initialiser:League = null!;", "initialiser:League = null!;", "initialiser:League = null!;",
		"initialiser:Name = string.Empty;", "initialiser:Name = string.Empty;",
		"initialiser:Name = string.Empty;", "initialiser:Name = string.Empty;",
		"initialiser:Name = string.Empty;", "initialiser:Name = string.Empty;",
		"initialiser:Odds = [];", "initialiser:Odds = [];", "initialiser:Odds = [];",
		"initialiser:Runners = [];", "initialiser:Runners = [];",
		"initialiser:SportType = null!;", "initialiser:SportType = null!;", "initialiser:SportType = null!;",
		"initialiser:WinningSelectionId = string.Empty;", "initialiser:WinningSelectionId = string.Empty;",

		// The one computed member the corpus writes in the generated half:
		// `public int FixtureIdIndex => FixtureId ?? 0;`, which
		// BOSportGameCacheItem's own `[DataCacheIndex(nameof(FixtureIdIndex),
		// ...)]` names. Section 5 puts computed members in the hand-written
		// partial; this one is not there, and protop has no expression syntax
		// to write it with, so the annotation passes through and the member it
		// points at does not exist in the emission. That is a gap in the
		// annotation passthrough rule -- an attribute can name a member protop
		// cannot produce -- and it is one site, so it is listed rather than
		// generalised.
		"computed:FixtureIdIndex",

		// #25, the hand-written half arriving in the same file. Two of the
		// three views carry a parameterless and a full constructor.
		"ctor:SlimBetType", "ctor:SlimBetType", "ctor:SlimOdd", "ctor:SlimOdd",
	}
	var got []string
	for _, r := range coverage {
		if r.Decl == "" {
			continue
		}
		got = append(got, readCorpus(t, r.Path).Sites...)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("normalisation sites changed.\nnot in the list: %v\nno longer firing: %v",
			missingFrom(got, want), missingFrom(want, got))
	}
}

// TestMemberAttributesAreEnumerated holds the member-level attribute surface
// against a checked-in census, for the same reason the normalisation sites are
// held against one.
//
// TestCorpusMembers compares attributes only as part of a member's canonical
// text, and that text is built by attrsJoin on both sides of the comparison. An
// attrsJoin that returned the body alone therefore agrees with itself: 11
// `[DataCacheKey]` sites, 29 `[DataCacheIndex...]`, the two `[JsonIgnore]` and
// every future `[cs: ...]` passthrough stop being compared at all, and no test
// in this package says a word. The census is read back out of the joined text
// for exactly that reason -- it is empty the moment the join stops carrying
// them.
//
// `[Key(N)]` and `[IgnoreMember]` are not in the list: the reader takes those
// as the member's identity and TestCorpusMembers compares them as the key and
// the `alias:` id. The one attribute missing from the census that the corpus
// does write on a member is the `[DataCacheIndex]` on
// payloads/BOSportGameCacheItem.cs:61 -- it sits on the computed
// `FixtureIdIndex`, which the reader skips on both sides and records as
// `computed:FixtureIdIndex` above.
func TestMemberAttributesAreEnumerated(t *testing.T) {
	want := []string{
		// protop's `key` is the only thing that produces a [DataCacheKey], and
		// the 11 sites are the census types.go quotes: 11 of the 14 payload
		// types have a key, on a field in seven of them and on the composite in
		// four.
		"DataCacheKey on EventId", "DataCacheKey on EventId", "DataCacheKey on EventId",
		"DataCacheKey on EventId", "DataCacheKey on EventId",
		"DataCacheKey on Id", "DataCacheKey on Id",
		"DataCacheKey on Key", "DataCacheKey on Key", "DataCacheKey on Key", "DataCacheKey on Key",

		// The index attributes, which protopc never derives and only ever
		// copies: dropping `index` from the language moved these into the
		// annotation passthrough, and this list is what that passthrough is
		// carrying today.
		"DataCacheIndex on BetTypeId",
		"DataCacheIndex on BookingStatus",
		"DataCacheIndex on ChannelId", "DataCacheIndex on ChannelId",
		"DataCacheIndex on CountryId",
		"DataCacheIndex on EventId", "DataCacheIndex on EventId", "DataCacheIndex on EventId",
		"DataCacheIndex on EventId", "DataCacheIndex on EventId",
		"DataCacheIndex on EventStatus", "DataCacheIndex on EventStatus", "DataCacheIndex on EventStatus",
		"DataCacheIndex on EventTypeId",
		"DataCacheIndex on IsLive", "DataCacheIndex on IsLive",
		"DataCacheIndex on LeagueId",
		"DataCacheIndex on SportTypeId",
		"DataCacheIndex on Status",
		"DataCacheIndex on WatchDogAction", "DataCacheIndex on WatchDogAction", "DataCacheIndex on WatchDogAction",
		"DataCacheIndex(DataCacheIndexType.Range) on EventDate",
		"DataCacheIndex(DataCacheIndexType.Range) on EventDate",
		"DataCacheIndex(DataCacheIndexType.Range) on EventDate",
		"DataCacheIndex(DataCacheIndexType.Range) on InternalTimestamp",
		"DataCacheIndex(DataCacheIndexType.Range) on ProviderTimestamp",
		"DataCacheIndex(DataCacheIndexType.Range) on ProviderTimestamp",
		"DataCacheIndex(DataCacheIndexType.Range) on Timestamp",
		"DataCacheFromTimestamp on Timestamp",

		// The two aliases that are also hidden from System.Text.Json, and the
		// only member attribute in the corpus that belongs to neither
		// MessagePack nor the cache generator.
		"JsonIgnore on BetTypeId", "JsonIgnore on BetTypeId",
	}
	var got []string
	for _, r := range coverage {
		if r.Decl == "" {
			continue
		}
		got = append(got, readCorpus(t, r.Path).MemberAttrs...)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("the member attributes being compared changed.\nnot in the list: %v\nno longer compared: %v",
			missingFrom(got, want), missingFrom(want, got))
	}
}

// --- the census ---------------------------------------------------------------

// TestCorpusCensus pins the counts the provisional rows of the mapping table
// were justified by.
//
// Each of these is a number quoted in a comment in types.go or csharp.go as the
// reason a row was written the way it was. A census that is quoted and never
// checked is a number that was true once; these fail the day the corpus is
// re-pinned and the justification stops holding.
//
// Every row carries its own scope, and the scope is load-bearing rather than
// tidy. `corpus/prague/` is three different things in one tree: `payloads/` is
// the generated half, which is what protopc is an oracle for; `deps/` is
// hand-written supporting types, two of which protopc cannot emit at all; and
// `shells/` is eleven cache classes that #25 blocks entirely. A claim about
// what protopc emits is a claim about `payloads/`, and counting the other two
// into it inflates the number that justifies the row. Three rows genuinely are
// corpus-wide -- the `Key = tag - 1` ruling was taken on all 339 sites, and an
// absence is only an absence if nowhere has one -- and they say so.
func TestCorpusCensus(t *testing.T) {
	payloads := corpusText(t, filepath.Join(corpusRoot, "payloads"))
	whole := corpusText(t, corpusRoot)

	for _, tt := range []struct {
		what  string
		scope string
		want  int
		why   string
	}{
		// --- absences: only corpus-wide proves them ---
		{"[MessagePackFormatter", "corpus", 0,
			"#13's scaled decimal would arrive as a formatter attribute; there are none anywhere, so `decimal(N)` drops its scale"},
		{"Dictionary<", "corpus", 0,
			"#24: the corpus *cannot check* a Dictionary row, which is not the same as saying no map is needed. " +
				"INVARIANTS.md's `Type tally` section carries, marked as not re-derivable from the pin, a report " +
				"that a `Dictionary<,>` does appear transitively and string-keyed via `Odd.Specifiers`; " +
				"SOURCE.md's `Upstream paths` section lists `Odd` among the fourteen types deliberately not " +
				"copied. So the zero is an artefact of where the copy stopped, and " +
				"examples/prague/branded.protop's stand-in `record Odd` omits the member that would have needed " +
				"one. Evidence for #24, which is human-in-the-loop and unruled. " +
				"(Cited by section rather than by line: both regions moved in #38, and a line number into a " +
				"prose document is a citation that rots without telling anyone)"},
		{"enum ", "corpus", 0,
			"SOURCE.md does not copy the enum declarations, so writeEnum has no oracle at all"},

		// --- the ruling that was taken on the whole tree ---
		{"[Key(", "corpus", 339,
			"the 339 sites the `Key = tag - 1` rule was ruled on; 335 of them are in payloads/ and 4 in deps/"},

		// --- claims about what protopc emits: payloads/ only ---
		{"List<", "payloads", 6,
			"the majority collection form in the generated half, which is why `list<T>` emits List<T> (deps/ adds two more, in types protopc does not emit)"},
		{"SlimOdd[]", "payloads", 1,
			"the corpus's one array, and #24's whole question"},
		{"DateTime", "payloads", 22,
			"#15: the members protop cannot spell at all"},
		{"byte ", "payloads", 3,
			"#32: the corpus's one scalar with no protop keyword"},
		{"decimal", "payloads", 30,
			"#13: every one of them unscaled on the wire today"},
		{"[DataCacheKey]", "payloads", 11,
			"protop's `key` is the only thing that produces it; 11 of the 14 payload types have a key and the other three -- Venue, BOVenue, SlimOdd -- have none"},
		{"sealed partial class", "payloads", 11,
			"11 for 11 in the generated half, which is why a table emits `sealed partial`; the 11 shells/ files make 22 corpus-wide and #25 still owns whether `partial` is mandatory"},
		{"public struct", "payloads", 2,
			"Venue and BOVenue, the two declarations section 4's `struct` row is measured against"},
		{"public class", "payloads", 1,
			"SlimOdd, and the whole of the correction to #45: it is a view, and it is the only view that is not `sealed partial` because it is the only one whose source is a record"},
	} {
		body, ok := map[string]string{"payloads": payloads, "corpus": whole}[tt.scope]
		if !ok {
			t.Fatalf("row %q has no scope", tt.what)
		}
		if n := strings.Count(body, tt.what); n != tt.want {
			t.Errorf("%s has %d %q, the mapping table was written against %d (%s)",
				tt.scope, n, tt.what, tt.want, tt.why)
		}
	}
}

func corpusText(t *testing.T, root string) string {
	t.Helper()
	var b strings.Builder
	err := filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".cs") {
			return err
		}
		c, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		b.Write(c)
		b.WriteString("\n")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// --- small helpers ------------------------------------------------------------

func keyNumber(id string) (int, bool) {
	n := 0
	for _, r := range id {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int(r-'0')
	}
	return n, id != ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func fmtInts(ns []int) string {
	var parts []string
	for _, n := range ns {
		parts = append(parts, itoa(n))
	}
	return strings.Join(parts, ",")
}

func slicesContains(hay []string, needle string) bool {
	for _, h := range hay {
		if h == needle {
			return true
		}
	}
	return false
}

// TestTheEmissionNeedsNoNormalisation is the other half of the census above,
// and without it that census is only half an oracle.
//
// readCS normalises on *both* sides, and TestNormalisationSitesAreEnumerated
// harvests its sites from the corpus side alone. So a construct introduced on
// the emission side is normalised into agreement and counted nowhere: emitting
// `{ get; init; }` -- which #20 forbids outright, tag 1 being key 0 and
// MessagePack needing a settable member -- passes every corpus assertion in
// this file, and the goldens that do fail are rewritten by -update.
//
// Each of the seven sites names a construct protopc does not emit and must not.
// `init`, `required` and `virtual` are #20, #12 and #11 ruling; `base` is #11's
// flattening; `ctor` and `computed` are the hand-written half (#25); and
// `initialiser` is a default-value syntax protop does not have. So the emitted
// side's list is empty, and empty is the assertion -- a normalisation firing on
// protopc's own output is either a construct it was ruled out of or a defect
// being normalised into agreement with the corpus, and neither may pass.
func TestTheEmissionNeedsNoNormalisation(t *testing.T) {
	units := exampleUnits(t)
	read := 0
	for _, r := range coverage {
		if r.Decl == "" {
			continue
		}
		u, ok := units[r.Decl]
		if !ok {
			t.Fatalf("examples/prague declares no %s", r.Decl)
		}
		got, err := readCS(r.Decl, u.Source)
		if err != nil {
			t.Fatal(err)
		}
		read++
		if len(got.Sites) > 0 {
			t.Errorf("%s: the emission needs normalising to match the corpus: %v", r.Decl, got.Sites)
		}
	}
	if read == 0 {
		t.Fatal("no emission was read; the coverage table stopped naming a reproduced declaration")
	}
}

// --- the documents ------------------------------------------------------------

// corpus/prague carries two prose documents, and until #38 nothing in the repo
// opened either one.
//
// INVARIANTS.md opens by telling its readers to cite it rather than re-derive
// what it says. That instruction is only safe if something keeps it true, and
// nothing did: it accumulated ten wrong numbers, one of them -- `StaticDataRef?`
// 8 against `StaticDataRef` 2, exactly backwards -- an instruction to emit ten
// members with the wrong nullability, and another asserting the opposite of what
// TestCorpusCensus above already pins and csharp.go already implements.
//
// So the tally and the declaration shapes are derived here and held against the
// document. Not every sentence in it: a claim the pin cannot check must not be
// given a test that pretends otherwise, and the document marks three of those
// as carried from upstream rather than established. What is anchored is what a
// generator would act on and what was in fact wrong.

const invariantsDoc = corpusRoot + "/INVARIANTS.md"

// detwinnedDrops are the five payload files INVARIANTS.md's tally does not
// count: the BO-side copy of each BO <-> BP twin pair.
//
// The rule is written out in the document, in the section this checks, because
// a tally whose de-twinning rule is unstated is a number nobody can reproduce
// -- which is how the old one survived being wrong. Five and not four: BOVenue
// / Venue is a twin pair too, and the document did not used to say so.
//
// Which side is dropped matters for exactly one figure. The keyed members are
// identical either way; the BO side carries the corpus's only type-level
// `[DataCacheIndex]` and an unkeyed `FixtureIdIndex`, neither of which is a
// keyed member. Dropping BO is what makes the document's "22 annotations sit on
// members" true, and the document now says so and gives the raw 31 beside it.
var detwinnedDrops = []string{
	"BOBetTypeCacheItem.cs",
	"BOLiveObjectCacheItem.cs",
	"BOSearchCacheItem.cs",
	"BOSportGameCacheItem.cs",
	"BOVenue.cs",
}

// corpusEnums are the type names the tally's two enum buckets aggregate.
//
// They are listed rather than detected, and the list is the one step of this
// derivation the pin cannot justify: SOURCE.md copies no enum declaration at
// all, which TestCorpusCensus pins as `enum ` appearing zero times corpus-wide.
// Nothing in corpus/prague/ says these names are enums.
//
// tallyBucket fails on a member type that is in neither this list nor
// tallyScalars nor tallyRefTypes, so the list cannot go stale in the direction
// that matters: a re-pin introducing a new type name stops the suite instead of
// landing in an enum bucket unnoticed.
var corpusEnums = []string{
	"BetTypeStatus", "BookingStatus", "Channel", "EventSource",
	"EventStatus", "OddStatus", "WatchDogAction", "WinStatus",
}

// tallyScalars and tallyRefTypes are every other base type a keyed payload
// member is written with. Together with corpusEnums they are exhaustive over
// the pin, and tallyBucket enforces that.
var (
	tallyScalars  = []string{"DateTime", "bool", "byte", "decimal", "double", "int", "long", "string"}
	tallyRefTypes = []string{"BOVenue", "IBaseLiveData", "Odd", "SlimOdd", "StaticDataRef", "Venue"}
)

// tallyEntry is one bucket of the tally, on either side of the comparison.
type tallyEntry struct {
	Label string
	N     int
}

// TestTheInvariantsDocumentMatchesTheCorpus derives INVARIANTS.md's two
// machine-checkable claims from the .cs files and fails if the document
// disagrees.
//
// It follows cmd/protopc's TestTheExclusionRulesAreTheOnesTheResearchRecorded
// rather than a golden: the right-hand side is a document nothing regenerates,
// there is no -update path, and neither side can move without the other. The
// difference is direction. That test transcribes a rule *out of* a document
// into code; this one derives a fact *from* the corpus and holds the document
// to it, because here the corpus is the oracle and the prose is the copy.
//
// The regions are found by their own text -- a heading, a marker sentence --
// and not by line number, so an edit above them does not quietly stop this
// test from finding anything. Every lookup requires exactly one match and
// fatals otherwise, which is what stops the test passing vacuously when the
// number it checks is deleted rather than changed.
func TestTheInvariantsDocumentMatchesTheCorpus(t *testing.T) {
	b, err := os.ReadFile(invariantsDoc)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)

	t.Run("tally", func(t *testing.T) {
		raw, detwinned := corpusTally(t)

		// The heading carries both totals: the de-twinned one the buckets sum
		// to, and the raw one that says how much the rule collapsed.
		m := docMatch(t, doc, reTallyHeading)
		wantDetwinned, _ := strconv.Atoi(m[1])
		wantRaw, _ := strconv.Atoi(m[2])

		if n := sumEntries(detwinned); n != wantDetwinned {
			t.Errorf("the de-twinned corpus has %d keyed members, the tally heading says %d", n, wantDetwinned)
		}
		if raw != wantRaw {
			t.Errorf("the 14 payload files have %d keyed members, the tally heading says %d raw", raw, wantRaw)
		}

		got := docTally(t, doc)
		if entriesText(got) != entriesText(detwinned) {
			t.Errorf("the tally in %s is not the corpus's tally.\ndocument:\n%s\ncorpus:\n%s",
				invariantsDoc, entriesText(got), entriesText(detwinned))
		}
	})

	t.Run("declaration shape", func(t *testing.T) {
		sealed, exceptions, total := corpusDeclShapes(t)

		m := docMatch(t, doc, reShapeBullet)
		wantSealed, _ := strconv.Atoi(m[1])
		wantTotal, _ := strconv.Atoi(m[2])
		if sealed != wantSealed || total != wantTotal {
			t.Errorf("%d of %d payloads are `public sealed partial class`, the document says %d of %d",
				sealed, total, wantSealed, wantTotal)
		}

		// The exceptions are the rest, and naming them is the half that was
		// wrong: SlimOdd is a bare `public class`, which csharp.go's view row
		// and TestCorpusCensus's three declaration rows both already encode,
		// and which the document used to contradict.
		exLine := docLine(t, doc, "- The exceptions are ")
		if cut := strings.Index(exLine, ". "); cut >= 0 {
			exLine = exLine[:cut]
		}
		named := backticked(exLine)
		sort.Strings(named)
		if strings.Join(named, ", ") != strings.Join(exceptions, ", ") {
			t.Errorf("the document names %v as the exceptions; the corpus's are %v\n  %s",
				named, exceptions, exLine)
		}
	})
}

// corpusTally counts keyed payload members by bucket, raw and de-twinned.
//
// It reads through readCS, the same reader every other corpus comparison in
// this package goes through, so "a member" means here what it means there. A
// member is keyed or it is not: `FixtureIdIndex` is expression-bodied and
// `[IgnoreMember]`, so readCS skips it as a computed member and it is in no
// count below -- which is the whole of the old tally's `int` 9.
func corpusTally(t *testing.T) (raw int, detwinned []tallyEntry) {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(corpusRoot, "payloads"))
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	dropped := map[string]bool{}
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".cs") {
			continue
		}
		d := readCorpus(t, filepath.Join("payloads", e.Name()))
		drop := slicesContains(detwinnedDrops, e.Name())
		if drop {
			dropped[e.Name()] = true
		}
		for _, id := range d.Order {
			if _, wire := keyNumber(id); !wire {
				continue
			}
			raw++
			if !drop {
				counts[tallyBucket(t, memberType(t, e.Name(), d.Members[id]))]++
			}
		}
	}
	for _, name := range detwinnedDrops {
		if !dropped[name] {
			t.Fatalf("the tally's de-twinning rule drops payloads/%s, which is not in the corpus", name)
		}
	}

	for label, n := range counts {
		detwinned = append(detwinned, tallyEntry{label, n})
	}
	// Count descending, then label ordinal -- the order the document states it
	// writes its entries in, so that a bucket cannot be appended out of place.
	sort.Slice(detwinned, func(i, j int) bool {
		if detwinned[i].N != detwinned[j].N {
			return detwinned[i].N > detwinned[j].N
		}
		return detwinned[i].Label < detwinned[j].Label
	})
	return raw, detwinned
}

// memberType is the declared type of one canonical member, read back out of the
// text readCS produced.
func memberType(t *testing.T, file, body string) string {
	t.Helper()
	lines := strings.Split(body, "\n")
	decl := lines[len(lines)-1]
	if m := reProperty.FindStringSubmatch(decl); m != nil {
		return m[1]
	}
	if m := reTuple.FindStringSubmatch(decl); m != nil {
		return m[1]
	}
	t.Fatalf("%s: %q is a member shape this tally cannot read", file, decl)
	return ""
}

// tallyBucket is which entry of the tally a member type counts towards.
//
// Four buckets aggregate -- the two enum ones, ValueTuple and collections --
// and everything else counts under its own spelling, nullability included.
// That last part is the point of the exercise: `StaticDataRef` and
// `StaticDataRef?` are two entries, and swapping them is what #38 found.
func tallyBucket(t *testing.T, typ string) string {
	t.Helper()
	base := strings.TrimSuffix(typ, "?")
	switch {
	case strings.HasPrefix(base, "("):
		return "ValueTuple"
	case strings.HasPrefix(base, "List<"), strings.HasSuffix(base, "[]"):
		return "collections"
	}
	if slicesContains(corpusEnums, base) {
		if base == typ {
			return "non-null enums"
		}
		return "nullable enums"
	}
	if !slicesContains(tallyScalars, base) && !slicesContains(tallyRefTypes, base) {
		t.Fatalf("the corpus has a member of type %q, which is in none of corpusEnums, "+
			"tallyScalars or tallyRefTypes. Those three lists are what makes the enum buckets "+
			"meaningful; put %q in whichever it belongs to before trusting this tally again.", typ, base)
	}
	return typ
}

// corpusDeclShapes is how many of the payload types are `public sealed partial
// class` and which ones are not.
func corpusDeclShapes(t *testing.T) (sealed int, exceptions []string, total int) {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(corpusRoot, "payloads"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".cs") {
			continue
		}
		total++
		d := readCorpus(t, filepath.Join("payloads", e.Name()))
		if strings.HasPrefix(d.Head, "public sealed partial class ") {
			sealed++
			continue
		}
		exceptions = append(exceptions, reHead.FindStringSubmatch(d.Head)[2])
	}
	sort.Strings(exceptions)
	return sealed, exceptions, total
}

// The two claims that are numbers in a sentence. Each is matched as a whole
// line, so a document that stops phrasing it this way fails docMatch's
// exactly-one requirement rather than silently matching nothing.
var (
	reTallyHeading = regexp.MustCompile(`^## Type tally \((\d+) de-twinned members; (\d+) raw\)$`)
	reShapeBullet  = regexp.MustCompile("^- (\\d+) of (\\d+) payloads are `public sealed partial class`")
)

// docMatch is the submatches of the one line of doc that re matches.
//
// Exactly one, for the same two reasons as docLine: a restated claim would make
// the test read whichever came first, and a deleted one has to fail rather than
// leave nothing to disagree with.
func docMatch(t *testing.T, doc string, re *regexp.Regexp) []string {
	t.Helper()
	var found [][]string
	for _, line := range strings.Split(doc, "\n") {
		if m := re.FindStringSubmatch(line); m != nil {
			found = append(found, m)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d lines of %s match %v, want exactly 1", len(found), invariantsDoc, re)
	}
	return found[0]
}

// docLine is the one line of doc beginning with prefix.
//
// Exactly one, and fatal otherwise: two would mean the claim is restated
// somewhere and this test is reading whichever came first, and zero is the
// vacuity case -- a number deleted from the document rather than corrected must
// fail this test, not pass it by having nothing left to disagree with.
func docLine(t *testing.T, doc, prefix string) string {
	t.Helper()
	var found []string
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(line, prefix) {
			found = append(found, line)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d lines of %s begin %q, want exactly 1", len(found), invariantsDoc, prefix)
	}
	return found[0]
}

// tallyMarker is the sentence the tally's own entries follow. The paragraph is
// found by it rather than by line number so that an edit anywhere above leaves
// this test still reading the tally.
const tallyMarker = "Entries are ordered by count, then by label:"

// docTally is the tally as the document writes it.
func docTally(t *testing.T, doc string) []tallyEntry {
	t.Helper()
	lines := strings.Split(doc, "\n")
	at := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == tallyMarker {
			if at >= 0 {
				t.Fatalf("%s carries the tally marker twice", invariantsDoc)
			}
			at = i
		}
	}
	if at < 0 {
		t.Fatalf("%s no longer contains %q, so the tally cannot be found at all", invariantsDoc, tallyMarker)
	}
	var para []string
	for i := at + 1; i < len(lines); i++ {
		s := strings.TrimSpace(lines[i])
		if s == "" {
			if len(para) > 0 {
				break
			}
			continue
		}
		para = append(para, s)
	}
	if len(para) == 0 {
		t.Fatalf("%s has nothing after %q", invariantsDoc, tallyMarker)
	}

	var out []tallyEntry
	for _, raw := range strings.Split(strings.Join(para, " "), "·") {
		e := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(raw, "`", ""), "**", ""))
		cut := strings.LastIndex(e, " ")
		if cut < 0 {
			t.Fatalf("%q is not a `label N` tally entry", raw)
		}
		n, err := strconv.Atoi(e[cut+1:])
		if err != nil {
			t.Fatalf("tally entry %q does not end in a count", raw)
		}
		out = append(out, tallyEntry{e[:cut], n})
	}
	return out
}

func sumEntries(es []tallyEntry) int {
	n := 0
	for _, e := range es {
		n += e.N
	}
	return n
}

func entriesText(es []tallyEntry) string {
	var b strings.Builder
	for _, e := range es {
		b.WriteString("  " + e.Label + " " + itoa(e.N) + "\n")
	}
	return b.String()
}

var backtickRE = regexp.MustCompile("`([^`]+)`")

// backticked is every backtick-quoted token of a line, in order.
func backticked(line string) []string {
	var out []string
	for _, m := range backtickRE.FindAllStringSubmatch(line, -1) {
		out = append(out, m[1])
	}
	return out
}
