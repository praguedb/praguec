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

	"github.com/praguedb/praguec/internal/check"
	"github.com/praguedb/praguec/internal/diag"
	"github.com/praguedb/praguec/internal/source"
	"github.com/praguedb/praguec/internal/syntax"
)

// The corpus is the oracle.
//
// The goldens under testdata/golden are the emitter held against itself: they
// catch a change, and `-update` can rewrite them. Nothing in this file can be
// rewritten by a flag. corpus/prague/ is 29 hand-written C# files copied from a
// running system, and every assertion here is praguec's output measured against
// them.
//
// The shape of the measurement is a coverage table with one row per corpus
// file. A row either names the declaration that reproduces it or names the
// ticket that blocks it, and TestCoverageTableNamesEveryCorpusFile makes the
// table exhaustive -- so a corpus file this milestone cannot emit is a line of
// evidence rather than a silence. Within a covered row, a key praguec does not
// write is listed in Missing with its ticket, and a key it writes differently
// is listed in Diverges with what the difference is.

const corpusRoot = "../../corpus/prague"

const examplesDir = "../../examples/prague"

// corpusRow is one corpus file and what praguec does about it.
type corpusRow struct {
	// Path is relative to corpus/prague.
	Path string

	// Decl is the declaration in examples/prague that reproduces this file, or
	// "" when nothing does.
	Decl string

	// Blocked says why, when Decl is "". Every one of them names a ticket or a
	// reason that is not praguec's to fix.
	Blocked string

	// Extra are the corpus's using lines praguec cannot derive. Section 3 has
	// praguec never inspect an annotation's contents, so a namespace that is
	// only reachable through an attribute is not reachable at all -- and a
	// member praguec does not write takes its namespace with it.
	Extra []string

	// Missing are corpus keys the emission does not contain, and why. Each is
	// a `reserved` tag in the protop.
	Missing map[int]string

	// Diverges are corpus keys the emission contains and writes differently.
	Diverges map[int]string
}

const (
	cacheCore  = "Prague.Feed.Cache.Core"         // every [DataCache*] attribute
	jsonIgnore = "System.Text.Json.Serialization" // [JsonIgnore] on the alias
	liveObject = "Common.EventApi.Models.LiveObject"
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
	{Path: "deps/ShowSlimExtensions.cs",
		Blocked: "not a MessagePack type: a static projection helper, which is code rather than schema."},
	{Path: "deps/StaticDataRef.cs",
		Blocked: "#11, inheritance. The corpus writes `class StaticDataRef : BaseProviderObject` with an empty body; #11 ruled protop flattens, so the emission carries the four inherited members and the C# hierarchy forks."},

	// --- payloads: the branded producer -------------------------------------
	{Path: "payloads/Venue.cs", Decl: "Venue"},
	{Path: "payloads/BPShowCacheItem.cs", Decl: "BPShowCacheItem",
		Extra: []string{cacheCore},
		Missing: map[int]string{
			11: unsigned, 27: temporal, 28: temporal, 29: temporal,
			30: temporal, 31: temporal, 50: temporal,
		}},
	{Path: "payloads/BPTicketTypeCacheItem.cs", Decl: "BPTicketTypeCacheItem",
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
	{Path: "payloads/BOShowCacheItem.cs", Decl: "BOShowCacheItem",
		Extra: []string{cacheCore},
		Missing: map[int]string{
			11: unsigned, 27: temporal, 28: temporal, 29: temporal,
			30: temporal, 31: temporal, 50: temporal,
		}},
	{Path: "payloads/BOTicketTypeCacheItem.cs", Decl: "BOTicketTypeCacheItem",
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
	{Path: "payloads/SlimShow.cs", Decl: "SlimShow",
		Extra:   []string{cacheCore},
		Missing: map[int]string{11: unsigned, 27: temporal, 29: temporal},
		Diverges: map[int]string{
			53: "the corpus view types this `EventSource?` and the table it is a view of types it `int?`. A protop view selects a member; it cannot retype one (section 5), so the emission follows the table. Either the corpus is drifting between two hand-written files or views want a narrowing this language does not have -- and the corpus alone cannot say which.",
			57: "the corpus view types this `StaticDataRef` and the table types it `StaticDataRef?`. A view cannot change presence either.",
			58: "same as 57.",
		}},
	{Path: "payloads/SlimTicketType.cs", Decl: "SlimTicketType",
		Extra: []string{cacheCore},
		Diverges: map[int]string{
			25: "#24. The corpus writes `SlimQuote[]` here and `List<T>` at its other six collection sites; praguec writes the majority form. Inferring `a view emits T[]` from one sample is a language ruling.",
		}},
	{Path: "payloads/SlimQuote.cs", Decl: "SlimQuote"},

	// --- shells -------------------------------------------------------------
	// The 11 hand-written halves. #25 owns whether praguec emits a shell at
	// all; until it rules, none of these is praguec's output to check.
	//
	// SOURCE.md records that only three of the eleven are empty --
	// LiveObjectCache, SlimTicketTypeCache and SlimShowCache. The other eight
	// carry a `Clear()` written against members the generator emits, and the
	// Search ones carry more. That is evidence for #25 rather than against the
	// corpus -- the hand-written half is not a formality that could be
	// generated away.
	{Path: "shells/BOTicketTypeCache.cs", Blocked: shellBlocked},
	{Path: "shells/BOLiveObjectCache.cs", Blocked: shellBlocked},
	{Path: "shells/BOSearchCache.cs", Blocked: shellBlocked},
	{Path: "shells/BOShowCache.cs", Blocked: shellBlocked},
	{Path: "shells/BPTicketTypeCache.cs", Blocked: shellBlocked},
	{Path: "shells/BPLiveObjectCache.cs", Blocked: shellBlocked},
	{Path: "shells/BPSearchCache.cs", Blocked: shellBlocked},
	{Path: "shells/BPShowCache.cs", Blocked: shellBlocked},
	{Path: "shells/LiveObjectCache.cs", Blocked: shellBlocked},
	{Path: "shells/SlimTicketTypeCache.cs", Blocked: shellBlocked},
	{Path: "shells/SlimShowCache.cs", Blocked: shellBlocked},
}

const shellBlocked = "#25, the hand-written half. Not a [MessagePackObject] type and not praguec's output."

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
	"deps/ShowSlimExtensions.cs",
	"deps/StaticDataRef.cs",
	"shells/BOTicketTypeCache.cs",
	"shells/BOLiveObjectCache.cs",
	"shells/BOSearchCache.cs",
	"shells/BOShowCache.cs",
	"shells/BPTicketTypeCache.cs",
	"shells/BPLiveObjectCache.cs",
	"shells/BPSearchCache.cs",
	"shells/BPShowCache.cs",
	"shells/LiveObjectCache.cs",
	"shells/SlimTicketTypeCache.cs",
	"shells/SlimShowCache.cs",
}

// --- loading -----------------------------------------------------------------

// exampleUnits compiles every file of examples/prague and returns the emission
// by declaration name.
//
// examples/prague is the protop that renders the corpus. Section 1 declares
// `Show` once and the corpus has BO, BP and Slim variants of it, so the
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
		if !strings.HasSuffix(e.Name(), ".prague") {
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
		t.Fatal("examples/prague has no .prague files")
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
		if !strings.HasSuffix(e.Name(), ".prague") {
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
// payloads/BPTicketTypeCacheItem.cs, lines 1 to 5) and praguec sorts ordinally;
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
			// `[MessagePackObject]` and praguec writes it on every non-enum
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
// is a tag the next member may take. Tag 51 of BPShowCacheItem is the
// corpus's `DateTime? PurchaseDataLastUpdateAt`; dropping it from the reserved list
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
	"EventStatus":      "CommonEventApi enum; SOURCE.md does not copy the declaration",
	"BookingStatus":    "CommonEventApi enum; SOURCE.md does not copy the declaration",
	"Channel":          "CommonEventApi enum; SOURCE.md does not copy the declaration",
	"TicketTypeStatus": "CommonFeedModels enum; SOURCE.md does not copy the declaration",
	"QuoteStatus":      "CommonFeedModels enum; SOURCE.md does not copy the declaration",
	"WinStatus":        "CommonFeedModels enum; SOURCE.md does not copy the declaration",
	"WatchDogAction":   "CommonFeedModels enum; SOURCE.md does not copy the declaration",

	// deps/StaticDataRef.cs is in the coverage table and is blocked by #11:
	// the corpus writes `class StaticDataRef : BaseProviderObject` with an
	// empty body and protop flattens. So the row names no declaration, and the
	// declaration is still needed -- both Show twins carry eight members
	// typed by it.
	"StaticDataRef": "#11; the corpus file is blocked and the type is still referenced",

	// Not copied by SOURCE.md at all, and referenced by members that are:
	// `Quote` is what SlimQuote is a view of and what the corpus's six `List<Quote>`
	// members hold, and `Performer` is the element type of `List<Performer>` at key
	// 46 of both Show twins. Quote's tags are recoverable from SlimQuote,
	// which is a view of it; Performer's are not, so Performer is empty.
	"Quote":     "SOURCE.md does not copy it; SlimQuote is a view of it and six members hold it",
	"Performer": "SOURCE.md does not copy it; `List<Performer>` at key 46 of both Show twins",
}

// TestTheExamplesDeclareNothingUnreviewed makes the input side of the oracle
// exhaustive, the way the coverage table makes the output side exhaustive.
//
// examples/prague is what praguec compiles to produce everything this file
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

		// #20 forbids praguec emitting an immutable type: a computed member at
		// key 0 breaks deserializing-constructor matching on both MessagePack
		// paths. So `init` becomes `set`, at all fifteen corpus sites.
		"init:Id",
		"init:IsBalancedLine", "init:IsFeatured", "init:IsDynamic", "init:Line",
		"init:Name", "init:QuotesActual", "init:QuotesFair", "init:QuotesManual",
		"init:CustomerId", "init:Status", "init:StatusManual", "init:Timestamp",
		"init:WinStatusId", "init:WinStatusIdManual",

		// protop has no default-value syntax. Every one of these is a C#
		// nullable-reference suppression rather than a fact about the wire --
		// `= string.Empty`, `= []`, `= null!` and one enum default -- but that
		// is a judgement about C#, and the list is here so that the judgement
		// is visible rather than assumed.
		"initialiser:Opener = null!;", "initialiser:Opener = null!;", "initialiser:Opener = null!;",
		"initialiser:Country = null!;", "initialiser:Country = null!;", "initialiser:Country = null!;",
		"initialiser:EventStatus = EventStatus.Unknown;", "initialiser:EventStatus = EventStatus.Unknown;",
		"initialiser:Groups = [];", "initialiser:Groups = [];",
		"initialiser:Headliner = null!;", "initialiser:Headliner = null!;", "initialiser:Headliner = null!;",
		"initialiser:Tour = null!;", "initialiser:Tour = null!;", "initialiser:Tour = null!;",
		"initialiser:Name = string.Empty;", "initialiser:Name = string.Empty;",
		"initialiser:Name = string.Empty;", "initialiser:Name = string.Empty;",
		"initialiser:Name = string.Empty;", "initialiser:Name = string.Empty;",
		"initialiser:Quotes = [];", "initialiser:Quotes = [];", "initialiser:Quotes = [];",
		"initialiser:Performers = [];", "initialiser:Performers = [];",
		"initialiser:Genre = null!;", "initialiser:Genre = null!;", "initialiser:Genre = null!;",
		"initialiser:AssignedSeatId = string.Empty;", "initialiser:AssignedSeatId = string.Empty;",

		// The one computed member the corpus writes in the generated half:
		// `public int ListingIdIndex => ListingId ?? 0;`, which
		// BOShowCacheItem's own `[DataCacheIndex(nameof(ListingIdIndex),
		// ...)]` names. Section 5 puts computed members in the hand-written
		// partial; this one is not there, and protop has no expression syntax
		// to write it with, so the annotation passes through and the member it
		// points at does not exist in the emission. That is a gap in the
		// annotation passthrough rule -- an attribute can name a member protop
		// cannot produce -- and it is one site, so it is listed rather than
		// generalised.
		"computed:ListingIdIndex",

		// #25, the hand-written half arriving in the same file. Two of the
		// three views carry a parameterless and a full constructor.
		"ctor:SlimTicketType", "ctor:SlimTicketType", "ctor:SlimQuote", "ctor:SlimQuote",
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
// payloads/BOShowCacheItem.cs:61 -- it sits on the computed
// `ListingIdIndex`, which the reader skips on both sides and records as
// `computed:ListingIdIndex` above.
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

		// The index attributes, which praguec never derives and only ever
		// copies: dropping `index` from the language moved these into the
		// annotation passthrough, and this list is what that passthrough is
		// carrying today.
		"DataCacheIndex on TicketTypeId",
		"DataCacheIndex on BookingStatus",
		"DataCacheIndex on ChannelId", "DataCacheIndex on ChannelId",
		"DataCacheIndex on CountryId",
		"DataCacheIndex on EventId", "DataCacheIndex on EventId", "DataCacheIndex on EventId",
		"DataCacheIndex on EventId", "DataCacheIndex on EventId",
		"DataCacheIndex on EventStatus", "DataCacheIndex on EventStatus", "DataCacheIndex on EventStatus",
		"DataCacheIndex on EventTypeId",
		"DataCacheIndex on IsLive", "DataCacheIndex on IsLive",
		"DataCacheIndex on TourId",
		"DataCacheIndex on GenreId",
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
		"JsonIgnore on TicketTypeId", "JsonIgnore on TicketTypeId",
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
// the generated half, which is what praguec is an oracle for; `deps/` is
// hand-written supporting types, two of which praguec cannot emit at all; and
// `shells/` is eleven cache classes that #25 blocks entirely. A claim about
// what praguec emits is a claim about `payloads/`, and counting the other two
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
				"that a `Dictionary<,>` does appear transitively and string-keyed via `Quote.Specifiers`; " +
				"SOURCE.md's `Upstream paths` section lists `Quote` among the fourteen types deliberately not " +
				"copied. So the zero is an artefact of where the copy stopped, and " +
				"examples/prague/branded.prague's stand-in `record Quote` omits the member that would have needed " +
				"one. Evidence for #24, which is human-in-the-loop and unruled. " +
				"(Cited by section rather than by line: both regions moved in #38, and a line number into a " +
				"prose document is a citation that rots without telling anyone)"},
		{"enum ", "corpus", 0,
			"SOURCE.md does not copy the enum declarations, so writeEnum has no oracle at all"},

		// --- the ruling that was taken on the whole tree ---
		{"[Key(", "corpus", 339,
			"the 339 sites the `Key = tag - 1` rule was ruled on; 335 of them are in payloads/ and 4 in deps/"},

		// --- claims about what praguec emits: payloads/ only ---
		{"List<", "payloads", 6,
			"the majority collection form in the generated half, which is why `list<T>` emits List<T> (deps/ adds two more, in types praguec does not emit)"},
		{"SlimQuote[]", "payloads", 1,
			"the corpus's one array, and #24's whole question"},
		{"DateTime", "payloads", 22,
			"#15: the members protop cannot spell at all"},
		{"byte ", "payloads", 3,
			"#32: the corpus's one scalar with no protop keyword"},
		{"decimal", "payloads", 30,
			"#13: every one of them unscaled on the wire today"},
		{"[DataCacheKey]", "payloads", 11,
			"protop's `key` is the only thing that produces it; 11 of the 14 payload types have a key and the other three -- Venue, BOVenue, SlimQuote -- have none"},
		{"sealed partial class", "payloads", 11,
			"11 for 11 in the generated half, which is why a table emits `sealed partial`; the 11 shells/ files make 22 corpus-wide and #25 still owns whether `partial` is mandatory"},
		{"public struct", "payloads", 2,
			"Venue and BOVenue, the two declarations section 4's `struct` row is measured against"},
		{"public class", "payloads", 1,
			"SlimQuote, and the whole of the correction to #45: it is a view, and it is the only view that is not `sealed partial` because it is the only one whose source is a record"},
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

// Section 1 says it transcribes rather than aspires, and this is what makes
// that sentence something other than a promise.
//
// Every `[cs: ...]` is passthrough -- section 3 has praguec copy an
// annotation's contents out without inspecting them -- so no assertion about
// the emission can tell an annotation the pin carries from one somebody wished
// for. The goldens are worse than silent about it: `-update` writes the wish
// into testdata/golden and the suite goes green. That is how `DataCacheContract`
// and `DataCacheIgnoreEquality` sat in section 1 through every run of this
// suite at zero occurrences under corpus/, and how `DataCacheTopic` sat there
// in a member-access form where all eleven corpus sites pass a string literal
// (#43).
//
// So the oracle has to be the corpus, as it is everywhere else in this file:
// every attribute name section 1 writes is an attribute name the pin uses.
//
// What it does not check, said rather than implied: the arguments. Section 1's
// `nameof(BPShowCache)` against the corpus's
// `nameof(Prague.Models.BPShowCache)` is a real divergence this test
// cannot see, because an argument list is exactly the part praguec is forbidden
// to understand. Names are the part a census can settle, and all three defects
// #43 found were names or forms of names.
// Section 1 transcribes the pin, and this is what holds it to that.
//
// Three questions, and the block has to answer all three.
//
// *Is it the pin's text?* Not its name: `DataCacheTopic` was always a real name
// in the corpus, and what #43 was filed about was the form it was written in. A
// census that matched names certified
// `[cs: DataCacheTopic(TopicNames.Invented.Nonsense)]`, the exact defect it was
// built to fail. So the whole annotation is normalised for whitespace and
// matched whole.
//
// *Is it the pin's text on this subject?* The pin is 29 files and matching
// against all of them at once is a bag of 38 head texts and 6 member texts for
// the seven slots section 1 uses -- enough slack to swap the two topic strings
// between the two tables, or to move `[BsonIgnoreExtraElements]` off the record
// onto a table, with every character still occurring somewhere in the corpus.
// The prose under the block already says which corpus file each of these
// follows; section1Follows is that sentence, and each annotation is matched
// against the one declaration it names. Prose that can go false becomes the
// thing the test reads.
//
// *Is it praguec's to write?* An annotation naming an attribute the emitter
// already emits is not a transcription question at all -- it is CS0579 in the
// one block of the spec that claims to be emittable. `[cs: MessagePackObject]`
// on a table emits the attribute twice and the worked example stops compiling.
// That set is derived from the emitter rather than listed here, by emitting
// section 1 with its annotations stripped and reading back what praguec wrote
// unbidden, so a fifth attribute added to the backend joins it without anyone
// remembering to.
//
// Where the census gives up rather than widens: normalisation is whitespace and
// nothing else, so an annotation naming a section 1 declaration -- `Show`,
// where the pin's class is `BPShowCacheItem` -- would match nothing and
// fail here. None does today. Order within one member's stack is pinned by
// `want` against section 1 and is not measured against the corpus; it is
// meaningful to praguec, which emits in source order and never sorts, but
// inert to C#.
func TestSection1AnnotatesOnlyWhatThePinCarries(t *testing.T) {
	got := csAnnotations(readFixture(t, pragueFixture))

	// Identity, not a count. A floor of three is satisfied by three different
	// annotations, so swapping one for another held it; and the moment section 1
	// grew a fourth, a floor would have permitted deleting one of the original
	// three. These are the seven, in source order, and any edit to the block's
	// annotations is meant to be an edit to this list as well.
	want := []csAnnotation{
		{decl: "StaticDataRef", text: "[BsonIgnoreExtraElements]"},
		{decl: "Show", text: "[DataCache(CacheClassName = nameof(Prague.Models.BPShowCache))]"},
		{decl: "Show", text: `[DataCacheTopic("Cache.[v:brandId].Show")]`},
		{decl: "TicketType", text: "[DataCache(CacheClassName = nameof(Prague.Models.BPTicketTypeCache))]"},
		{decl: "TicketType", text: `[DataCacheTopic("Cache.[v:brandId].TicketType")]`},
		{decl: "TicketType", member: "timestamp", text: "[DataCacheIndex(DataCacheIndexType.Range)]"},
		{decl: "TicketType", member: "timestamp", text: "[DataCacheFromTimestamp]"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("section 1 writes these annotations:\n  %v\nand this test expects:\n  %v", got, want)
	}

	// Everything below runs over what section 1 actually writes rather than over
	// the list above, so that editing both in lockstep -- which is what a change
	// to the block looks like -- still has to answer to the corpus.
	if len(got) == 0 {
		t.Fatalf("section 1 writes no `[cs: ...]` at all; nothing was censused")
	}

	emits := praguecsOwnAttributes(t)
	if len(emits) == 0 {
		t.Fatal("read no attributes at all out of an emission of section 1; the collision check would pass for free")
	}
	seen := map[csAnnotation]bool{}
	for _, a := range got {
		if n := attributeName(a.text); slices.Contains(emits, n) {
			t.Errorf("section 1 writes %s and praguec emits [%s] on its own; "+
				"the two land in the same attribute list and C# rejects the duplicate (CS0579), "+
				"so this is not a form the block can carry however the pin writes it", a, n)
		}
		if seen[a] {
			t.Errorf("section 1 writes %s twice; praguec emits annotations in source order and "+
				"never deduplicates, so the emission carries both and C# rejects it (CS0579)", a)
		}
		seen[a] = true
	}

	for _, a := range got {
		subject := a.decl
		if a.member != "" {
			subject += "." + a.member
		}
		file, ok := section1Follows[subject]
		if !ok {
			t.Errorf("section 1 annotates %s and the paragraph under the block does not say which "+
				"corpus declaration %s follows; an annotation with no named subject has no oracle, so "+
				"either the paragraph gains a clause and section1Follows gains a row, or the annotation "+
				"belongs in a ticket", subject, subject)
			continue
		}
		onType, onMember := corpusFileAttributes(t, file)
		if len(onType)+len(onMember) == 0 {
			t.Fatalf("read no attributes at all out of %s; a census of nothing passes for free", file)
		}
		where, carried := "on its type", onType[a.text]
		if a.member != "" {
			where, carried = "on its property "+pascal(a.member), onMember[[2]string{pascal(a.member), a.text}]
		}
		if !carried {
			t.Errorf("section 1 writes %s and %s -- the file the paragraph under the block says it "+
				"follows -- does not carry that attribute %s; section 1 transcribes the pin, so a form "+
				"the pin does not carry there belongs in a ticket, not in the spec", a, file, where)
		}
	}
}

// section1Follows is the paragraph under section 1's block, as a map.
//
// The paragraph is docs/spec/protop/shared-repository.md:114, the one that
// opens "This section transcribes; it does not aspire."
//
// That paragraph names a corpus file for every annotation in the block: the two
// table heads follow the two BP payload files, the record head follows
// deps/StaticDataRef.cs, and the pair on `timestamp` follows SlimTicketType.cs --
// which is a different file from the one TicketType's head follows, because
// BPTicketTypeCacheItem.cs writes `Timestamp` with nothing on it but its `[Key]`.
//
// It is hand-written because it is that sentence's own claim and not an
// inference from anything: no rule relates section 1's `Show` to the pin's
// `BPShowCacheItem`, the coverage table above is where that relation is
// stated, and a map derived from a guess would agree with whatever it guessed.
// The two must move together -- editing the paragraph without editing this map
// leaves the spec asserting one thing and the test checking another, which is
// the shape of the defect #43 was filed about.
var section1Follows = map[string]string{
	"StaticDataRef":        corpusRoot + "/deps/StaticDataRef.cs",
	"Show":                 corpusRoot + "/payloads/BPShowCacheItem.cs",
	"TicketType":           corpusRoot + "/payloads/BPTicketTypeCacheItem.cs",
	"TicketType.timestamp": corpusRoot + "/payloads/SlimTicketType.cs",
}

// praguecsOwnAttributes is every attribute name praguec writes without being
// asked, read off an emission of section 1 with its annotations stripped.
//
// Derived rather than listed: the emitter writes four today -- csharp.go:197,
// :242, :322 and :349 -- and section 1 reaches all four, having a simple key, a
// composite key and an alias in it. A fifth added to the backend appears here
// the moment it is emitted, which a hand-written list would not do.
func praguecsOwnAttributes(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, u := range emitSource(t, map[string]string{"stripped.prague": stripCSAnnotations(readFixture(t, pragueFixture))}) {
		for _, ln := range strings.Split(u.Source, "\n") {
			trimmed := strings.TrimSpace(ln)
			if !strings.HasPrefix(trimmed, "[") {
				continue
			}
			if n := attributeName(trimmed); n != "" && !slices.Contains(out, n) {
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

// stripCSAnnotations removes every `[cs: ...]` from a protop source and leaves
// the rest of it parseable.
func stripCSAnnotations(src string) string {
	var b strings.Builder
	for _, ln := range strings.Split(src, "\n") {
		for _, br := range bracketed(ln) {
			if strings.HasPrefix(br, "[cs:") {
				ln = strings.Replace(ln, br, "", 1)
			}
		}
		b.WriteString(strings.TrimRight(ln, " \t"))
		b.WriteString("\n")
	}
	return b.String()
}

// attributeName is the identifier an attribute leads with: `Key` out of
// `[Key(24)]`, `DataCacheTopic` out of `[DataCacheTopic("...")]`.
func attributeName(attr string) string {
	m := regexp.MustCompile(`^\[\s*([A-Za-z_][A-Za-z0-9_]*)`).FindStringSubmatch(attr)
	if m == nil {
		return ""
	}
	return m[1]
}

func corpusFileAttributes(t *testing.T, path string) (map[string]bool, map[[2]string]bool) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return corpusAttributes(string(b))
}

// Section 1's `record StaticDataRef` is the same declaration as
// examples/prague/branded.prague's, and only one of the two has an oracle: the
// example's four members are corpus/prague/deps/BaseProviderObject.cs's four
// properties, and TestCorpusMembers/BaseProviderObject fails the moment they
// stop matching. The copy in section 1 was tied to nothing -- dropping the `?`
// off `provider_source_key`, or swapping the two int64 members' types for the
// string one's, was invisible to the whole suite while the identical edit to
// branded.prague failed instantly. This is the tie.
//
// It compares the declarations whole rather than member by member, so the
// annotation, the tags and the order are all in it. Alignment is not: the two
// files pad their columns differently and a pin that failed on a realignment
// would teach the next reader to delete it.
func TestSection1sStaticDataRefIsTheOneTheCorpusChecks(t *testing.T) {
	const decl = "record StaticDataRef"
	spec := declText(t, readFixture(t, pragueFixture), decl)
	branded := declText(t, readExample(t, "branded.prague"), decl)
	if collapseSpace(spec) != collapseSpace(branded) {
		t.Errorf("section 1 declares\n\n%s\n\nand %s/branded.prague -- the copy the corpus checks -- declares\n\n%s",
			spec, examplesDir, branded)
	}
}

// section1PerformersCaveat is the note section 1 carries above the member it is
// about, quoted with that member on the end of it. The member is the citation:
// #38 was filed because line numbers into these documents had rotted, and a pin
// that named a line would rot the same way the next time the block moves.
const section1PerformersCaveat = "  // Element type unverified against the pin. The corpus writes `List<Performer>`\n" +
	"  // in this slot and `Performer` was never copied, so the pin cannot settle whether\n" +
	"  // `StaticDataRef` here is faithful, a substitution, or an error.\n" +
	"  // corpus/prague/SOURCE.md records the gap; tracked on #38.\n" +
	"  list<StaticDataRef> performers = 47;"

// performerDeclared matches a C# declaration of Performer. The corpus USES the name
// -- `List<Performer>` in both Show twins -- so an occurrence count would say
// nothing; what is absent is a definition, which is what an oracle would be.
var performerDeclared = regexp.MustCompile(`\b(?:class|struct|record|interface|enum)\s+Performer\b`)

// The one member of section 1 whose element type the pin cannot check, held to
// the gap that makes that true.
//
// Everything else in that block is tied to a corpus line -- by
// TestSection1AnnotatesOnlyWhatThePinCarries, by the StaticDataRef tie above,
// by the coverage table. The note is the only thing saying that one member is
// not, and two adversarial rounds found it deletable in lockstep with the suite
// green: section 1 would then read as uniformly pin-backed, which is #43's
// defect arriving by deletion instead of by invention.
//
// What goes red, and when. Three assertions and only one of them is about a
// sentence:
//
//   - `List<Performer>` still in the pin, twice. ANCHOR. The note's first clause
//     is a claim about the corpus; if the next pin drops the member or renames
//     the type, the clause is false and nothing else in this file would notice.
//
//   - no corpus file DECLARES Performer. ANCHOR, and the half that is usually
//     missed: it fails when the gap CLOSES. The day `Performer` is copied, the
//     element type has an oracle, the note is a lie, and this is what says so.
//     A test that only checked the note was present would let that lie stand
//     forever -- which is the anchor-versus-decoration mistake this repository
//     has now made six times, in its quieter direction.
//
//   - the note is in section 1, above `performers`. DECORATION, and named as
//     such: what turns it red is someone editing or deleting the note. That is
//     the half the ticket asked for and it is worth exactly what it costs --
//     with the two above it, neither side of the pair can move alone.
//
// Why not the stronger, derived version. The obvious upgrade is to stop pinning
// the note's text and instead compute, from the corpus, which of section 1's
// element types have no declaration there, then require a caveat on exactly
// those. It does not survive contact: `Quote` has no corpus declaration either
// and correctly carries no caveat, because its tags are recoverable from
// SlimQuote, which is a view of it, and Performer's are not. "Has an oracle" is not
// "is declared", and the difference is a judgement per type -- so the derived
// rule needs a hand-maintained exception beside it, which is the copy this
// anchor exists to avoid. The conjunction is what can actually be held.
func TestSection1sPerformersCaveatStandsExactlyWhileTheGapDoes(t *testing.T) {
	whole := corpusText(t, corpusRoot)

	if n := strings.Count(whole, "List<Performer>"); n != 2 {
		t.Errorf("the pin has %d `List<Performer>` sites, not the two Show twins section 1's note "+
			"describes; the note's first clause is about the corpus and has stopped matching it", n)
	}
	if decl := performerDeclared.FindString(whole); decl != "" {
		t.Errorf("the corpus now declares Performer (`%s`), so the element type of section 1's "+
			"`list<StaticDataRef> performers = 47` has an oracle and the #38 caveat above it is false. "+
			"The caveat goes, this assertion goes with it, and the transcription gets checked instead", decl)
	}

	spec, err := os.ReadFile(specSection1)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(spec), section1PerformersCaveat) {
		t.Errorf("section 1 no longer carries, above its `performers` member:\n\n%s\n\n"+
			"The gap it records is still open -- the two assertions above just measured it -- so "+
			"section 1 now reads as uniformly pin-backed when one member is not", section1PerformersCaveat)
	}
}

// declText is one top-level declaration, from its head line to the `}` in
// column zero that closes it.
func declText(t *testing.T, src, head string) string {
	t.Helper()
	i := strings.Index(src, "\n"+head)
	if i < 0 {
		t.Fatalf("no declaration %q", head)
	}
	i++
	j := strings.Index(src[i:], "\n}")
	if j < 0 {
		t.Fatalf("declaration %q is never closed", head)
	}
	return src[i : i+j+2]
}

func readExample(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(examplesDir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// csAnnotation is one `[cs: ...]` site in a protop source: the C# attribute it
// emits and the subject it is written on -- a declaration, or a member of one.
type csAnnotation struct {
	decl   string
	member string
	text   string
}

func (a csAnnotation) String() string {
	if a.member == "" {
		return a.text + " on " + a.decl
	}
	return a.text + " on " + a.decl + "." + a.member
}

// csAnnotations is every `[cs: ...]` in a protop source, in source order, each
// occurrence kept -- a duplicate that was deleted should be as visible as one
// that was added.
//
// A declaration's annotations are the ones between its head line, which starts
// in column zero, and the `{` that opens its body; everything else belongs to a
// member.
var declStart = regexp.MustCompile(`^(?:table|record|struct|view|enum)\s+([A-Za-z_][A-Za-z0-9_]*)`)

func csAnnotations(src string) []csAnnotation {
	var out []csAnnotation
	decl, inHead := "", false
	for _, ln := range strings.Split(src, "\n") {
		if m := declStart.FindStringSubmatch(ln); m != nil {
			decl, inHead = m[1], true
		}
		for _, br := range bracketed(ln) {
			body, ok := strings.CutPrefix(br, "[cs:")
			if !ok {
				continue // `[references(...)]` is protop's own and emits nothing
			}
			body = strings.TrimSuffix(body, "]")
			a := csAnnotation{decl: decl, text: "[" + collapseSpace(body) + "]"}
			if !inHead {
				a.member = memberName(ln)
			}
			out = append(out, a)
		}
		if strings.Contains(ln, "{") {
			inHead = false
		}
	}
	return out
}

// bracketed is every outermost `[...]` run on a line.
//
// Depth is what makes `[DataCacheTopic("Cache.[v:brandId].Show")]` come
// back whole: a scanner that stopped at the first `]` would truncate the pin's
// most-written annotation to something no corpus file carries. The inner pair
// happens to balance, so depth alone would carry today's section 1 -- the
// string tracking is for a bracket in a literal that does not, which is a state
// no annotation reaches from any fixture. TestBracketedReadsWholeAnnotations
// exercises it directly for that reason.
func bracketed(ln string) []string {
	var out []string
	depth, start, inStr := 0, 0, false
	for i := 0; i < len(ln); i++ {
		switch c := ln[i]; {
		case c == '"':
			inStr = !inStr
		case inStr:
		case c == '[':
			if depth == 0 {
				start = i
			}
			depth++
		case c == ']':
			if depth--; depth == 0 {
				out = append(out, ln[start:i+1])
			}
		}
	}
	return out
}

// The annotation scanner, on the two inputs section 1 supplies and on the one
// it does not. A `[` inside a string literal with no `]` to answer it would run
// the depth count negative and swallow every annotation after it on the line;
// nothing in the corpus or the spec writes one today, and the guard against it
// is unreachable from every fixture in the package.
func TestBracketedReadsWholeAnnotations(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{`  [cs: DataCacheTopic("Cache.[v:brandId].Show")]`, []string{`[cs: DataCacheTopic("Cache.[v:brandId].Show")]`}},
		{"  int64 timestamp = 25 [cs: A] [cs: B];", []string{"[cs: A]", "[cs: B]"}},
		{"  int64 id = 1;", nil},
		{`  [cs: T("a [ b")] [cs: After]`, []string{`[cs: T("a [ b")]`, "[cs: After]"}},
	}
	for _, c := range cases {
		if got := bracketed(c.in); !slices.Equal(got, c.want) {
			t.Errorf("bracketed(%s) = %q, want %q", c.in, got, c.want)
		}
	}
}

// memberName is the identifier a protop member line binds, which is the token
// before its `=`.
func memberName(ln string) string {
	head, _, ok := strings.Cut(ln, "=")
	if !ok {
		return ""
	}
	f := strings.Fields(head)
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}

// csProperty matches a C# property declaration and captures its name: the last
// identifier before the accessor list or the expression body, so that
// `public (long EventId, Channel ChannelId) Key =>` declares `Key` and not
// `EventId`. The `[^A-Za-z0-9_]` before the group is what makes it the whole
// identifier -- RE2 has no greedy submatch, and without that boundary the group
// is satisfied by the name's last letter alone.
var csProperty = regexp.MustCompile(`^\s+public\s.*[^A-Za-z0-9_]([A-Za-z_][A-Za-z0-9_]*)\s*(?:\{|=>)`)

// The two levels corpusAttributes draws, asserted directly rather than only
// through section 1.
//
// Both are load-bearing against a real mutation -- `[cs: JsonIgnore]` on a table
// head is admissible without the column-zero split, and `[cs: DataCacheKey]` on
// `timestamp` is admissible without the property binding, since the pin carries
// both texts in the file each maps to. But a guard is only exercised by the
// mutation it stops, and neither level is visible in a green run: collapsing
// either one leaves the whole suite passing until someone writes the annotation
// it was there to catch. This is the test that fails instead.
//
// The input is the shape of corpus/prague/payloads/SlimTicketType.cs:83-86 and of
// a class head, which is where the distinction comes from.
func TestCorpusAttributesKeepsHeadAndPropertyApart(t *testing.T) {
	onType, onMember := corpusAttributes(`[MessagePackObject]
[DataCacheTopic("Cache.[v:brandId].TicketType")]
public sealed partial class BPTicketTypeCacheItem
{
    [Key(24)]
    [DataCacheIndex(DataCacheIndexType.Range)]
    [DataCacheFromTimestamp]
    public long Timestamp { get; set; }

    [JsonIgnore]
    public long TicketTypeId { get; set; }
}
`)
	for _, want := range []string{"[MessagePackObject]", `[DataCacheTopic("Cache.[v:brandId].TicketType")]`} {
		if !onType[want] {
			t.Errorf("%s is written in column zero and did not land on the type", want)
		}
	}
	for _, notWanted := range []string{"[JsonIgnore]", "[DataCacheFromTimestamp]", "[Key(24)]"} {
		if onType[notWanted] {
			t.Errorf("%s is written on a property and landed on the type; "+
				"without that split, a real property attribute is admissible on a table head", notWanted)
		}
	}
	for _, want := range [][2]string{
		{"Timestamp", "[DataCacheIndex(DataCacheIndexType.Range)]"},
		{"Timestamp", "[DataCacheFromTimestamp]"},
		{"TicketTypeId", "[JsonIgnore]"},
	} {
		if !onMember[want] {
			t.Errorf("%s did not land on the property %s", want[1], want[0])
		}
	}
	for _, notWanted := range [][2]string{
		{"TicketTypeId", "[DataCacheFromTimestamp]"},
		{"Timestamp", "[JsonIgnore]"},
	} {
		if onMember[notWanted] {
			t.Errorf("%s landed on the property %s, which is not the one it annotates; "+
				"without that binding, any property attribute of a file is admissible on any member of it",
				notWanted[1], notWanted[0])
		}
	}
}

// corpusAttributes is every C# attribute the pin writes, keyed by what it is
// written on: a line starting in column zero annotates the type, and an
// indented run of them annotates the property that closes the run.
func corpusAttributes(text string) (onType map[string]bool, onMember map[[2]string]bool) {
	onType, onMember = map[string]bool{}, map[[2]string]bool{}
	var pending []string
	for _, ln := range strings.Split(text, "\n") {
		ln = strings.TrimRight(ln, "\r")
		trimmed := strings.TrimSpace(ln)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if ln == trimmed {
				onType[collapseSpace(trimmed)] = true
			} else {
				pending = append(pending, collapseSpace(trimmed))
			}
			continue
		}
		if m := csProperty.FindStringSubmatch(ln); m != nil {
			for _, a := range pending {
				onMember[[2]string{m[1], a}] = true
			}
		}
		pending = pending[:0]
	}
	return onType, onMember
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

// backtickedCall is a backticked span containing a `(`, which in section 3's
// juxtaposition paragraph is an illustration and nothing else.
var backtickedCall = regexp.MustCompile("`([^`]*\\([^`]*)`")

// Every illustration section 3's juxtaposition paragraph argues from, read out
// of the document rather than copied into this file.
//
// Reading them rather than pinning them is the whole difference between this
// and a sentence pin. A pinned string would fail when the sentence was edited,
// which is what "decoration" means here; this fails when an illustration stops
// being something the corpus carries, and passes when someone swaps one for a
// different real one. The document is free to change its mind; it is not free
// to argue from an attribute that does not exist.
//
// All of them, not the first. The first version of this read one illustration
// -- `FindStringSubmatch` is leftmost -- and an adversarial reviewer put a real
// attribute in front of the invented one and left the invention standing in the
// same sentence, still inside the "which the pin carries" clause, with the
// suite green. That is the defect this test is named after, reinstated by the
// test that was supposed to hold it. A paragraph is allowed more than one
// illustration and every one of them is a claim, so every one of them is read.
func section3CommaIllustrations(t *testing.T) []string {
	t.Helper()
	spec, err := os.ReadFile(specSection3)
	if err != nil {
		t.Fatal(err)
	}
	const lead = "**One bracket, one annotation. Stack by juxtaposition.**"
	i := strings.Index(string(spec), lead)
	if i < 0 {
		t.Fatalf("%s no longer carries the juxtaposition rule, which is what this test is about", specSection3)
	}
	para := string(spec)[i:]
	if j := strings.Index(para, "\n\n"); j >= 0 {
		para = para[:j]
	}
	ms := backtickedCall.FindAllStringSubmatch(para, -1)
	if len(ms) == 0 {
		t.Fatalf("%s argues that annotation contents already contain commas and now shows nothing "+
			"that does; the paragraph is:\n\n%s", specSection3, para)
	}
	var out []string
	for _, m := range ms {
		out = append(out, m[1])
	}
	return out
}

// Section 3's reason for having no separator between annotations is that the
// contents already contain commas, and it shows one. `DataCacheContract` was
// that one and occurs zero times under corpus/ -- the same name #43 measured at
// zero and removed from section 1, surviving one file over because #43's census
// was scoped to section 1 (#59).
//
// What goes red, and when. Both assertions are anchors; neither is about the
// wording, and both hold for every illustration in the paragraph rather than
// for whichever one comes first:
//
//   - the illustration is an attribute the pin writes. Red when a re-pin stops
//     carrying it, and red when someone substitutes another invention. Not red
//     when the document swaps in a different attribute the corpus does have,
//     which is correct: the claim is about the pin, not about which name was
//     chosen.
//
//   - the illustration contains a comma. Red when it stops being evidence for
//     the sentence it is evidence for. The paragraph's reasoning is "a
//     separator that competes with them buys nothing", and an illustration
//     whose arguments contain no comma leaves that standing on nothing at all
//     -- which is a way for this to rot that replacing the name does not fix.
//
// Why the corpus is searched with the brackets on. A bare `strings.Contains`
// accepts any substring, and `ListingIdIndex), nameof(ListingId` is a substring
// of the pin that carries a comma, is not an attribute, and is not even
// balanced. Searching for `[` + illustration + `]` asks the question the
// sentence actually asks -- is this an annotation the pin writes -- and no file
// under corpus/ stacks two calls inside one bracket, so nothing real is
// excluded by requiring the brackets to be its own.
//
// Why this one paragraph and not a census of the file. Section 1 is a
// transcription and every name in it is measured; section 3 is a rules file and
// its illustrations are allowed to be synthetic -- `[go: json:"show"]` is
// explicitly for a backend that does not exist, and syntax's parser fixture
// uses a fictional attribute on purpose, because it is testing verbatim copying
// and makes no claim about the corpus. Extending section 1's census here would
// forbid all of that. This paragraph is different because it says "which the
// pin carries" out loud: prose that can go false becomes the thing the test
// reads.
//
// The chosen site is the corpus's only multi-argument attribute outside
// deps/IBaseLiveData.cs, and there is exactly one of it. That is a thin thread
// and this is what makes it a loud one: if the next pin drops it, this fails
// and the illustration gets re-chosen, rather than quietly reverting to the
// state #59 was filed about.
func TestSection3sCommaIllustrationIsOneThePinCarries(t *testing.T) {
	illustrations := section3CommaIllustrations(t)
	whole := collapseSpace(corpusText(t, corpusRoot))

	for _, illustration := range illustrations {
		if !strings.Contains(illustration, ",") {
			t.Errorf("section 3 illustrates \"annotation contents already contain commas\" with `%s`, "+
				"which contains none; the paragraph's reasoning has nothing left to stand on", illustration)
		}
		if !strings.Contains(whole, collapseSpace("["+illustration+"]")) {
			t.Errorf("section 3 argues from `%s` and says the pin carries it; no file under %s writes it "+
				"as an annotation. That is the defect #43 removed from section 1 and #59 found here",
				illustration, corpusRoot)
		}
	}
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
// Each of the seven sites names a construct praguec does not emit and must not.
// `init`, `required` and `virtual` are #20, #12 and #11 ruling; `base` is #11's
// flattening; `ctor` and `computed` are the hand-written half (#25); and
// `initialiser` is a default-value syntax protop does not have. So the emitted
// side's list is empty, and empty is the assertion -- a normalisation firing on
// praguec's own output is either a construct it was ruled out of or a defect
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
// `[DataCacheIndex]` and an unkeyed `ListingIdIndex`, neither of which is a
// keyed member. Dropping BO is what makes the document's "22 annotations sit on
// members" true, and the document now says so and gives the raw 31 beside it.
var detwinnedDrops = []string{
	"BOTicketTypeCacheItem.cs",
	"BOLiveObjectCacheItem.cs",
	"BOSearchCacheItem.cs",
	"BOShowCacheItem.cs",
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
	"TicketTypeStatus", "BookingStatus", "Channel", "EventSource",
	"EventStatus", "QuoteStatus", "WatchDogAction", "WinStatus",
}

// tallyScalars and tallyRefTypes are every other base type a keyed payload
// member is written with. Together with corpusEnums they are exhaustive over
// the pin, and tallyBucket enforces that.
var (
	tallyScalars  = []string{"DateTime", "bool", "byte", "decimal", "double", "int", "long", "string"}
	tallyRefTypes = []string{"BOVenue", "IBaseLiveData", "Quote", "SlimQuote", "StaticDataRef", "Venue"}
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
// It follows cmd/praguec's TestTheExclusionRulesAreTheOnesTheResearchRecorded
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

	// The prose around the entries restates numbers the entries already carry.
	// A restatement is a second copy of a fact, so it is a second place the
	// document can be wrong in -- and it was: a heading reading 203 over a
	// paragraph reading 999 is exactly the drift #38 was opened about, one
	// section later. Every figure below is one this test already derived.
	t.Run("tally prose", func(t *testing.T) {
		raw, detwinned := corpusTally(t)
		u := unwrapped(doc)
		by := map[string]int{}
		for _, e := range detwinned {
			by[e.Label] = e.N
		}
		total := sumEntries(detwinned)
		_, _, payloads := corpusDeclShapes(t)

		// The de-twinning rule's own arithmetic: how many files it starts from,
		// how many pairs it collapses, how many it counts, and what it gets.
		m := docPhrase(t, u, reRawProse)
		wantPayloads, _ := strconv.Atoi(m[1])
		wantRaw, _ := strconv.Atoi(m[2])
		if wantPayloads != payloads || wantRaw != raw {
			t.Errorf("the rule reads %d payload files for %d keyed members; the corpus has %d files and %d",
				wantPayloads, wantRaw, payloads, raw)
		}
		if got := docPhrase(t, u, reTwinPairs)[1]; got != cardinal(t, len(detwinnedDrops)) {
			t.Errorf("the rule drops %d files, one per twin pair, and calls that %q", len(detwinnedDrops), got)
		}
		if got := docPhrase(t, u, reRemaining)[1]; got != cardinal(t, payloads-len(detwinnedDrops)) {
			t.Errorf("the rule counts %d files and calls that %q", payloads-len(detwinnedDrops), got)
		}
		if n, _ := strconv.Atoi(docPhrase(t, u, reDetwinnedProse)[1]); n != total {
			t.Errorf("the rule's own paragraph says it totals %d; the entries total %d", n, total)
		}

		// Collapsing relationship 3 as well is the road not taken, and the
		// document quotes where it leads. It is LiveObjectCacheItem's keys and
		// nothing else -- the twins' copies of them are already dropped.
		if n, _ := strconv.Atoi(docPhrase(t, u, reCollapse)[1]); n != total-corpusKeyed(t, "LiveObjectCacheItem.cs") {
			t.Errorf("collapsing the LiveObject triplet as well gives %d, not the %d the document quotes",
				total-corpusKeyed(t, "LiveObjectCacheItem.cs"), n)
		}

		// The sentence that exists because the pair under it was transposed.
		// Both halves of it are the tally: the ratio and the sum.
		nonNull, nullable := by["StaticDataRef"], by["StaticDataRef?"]
		if nullable == 0 || nonNull%nullable != 0 {
			t.Fatalf("StaticDataRef %d against StaticDataRef? %d is not a whole ratio, so the "+
				"document's \"N to one\" phrasing cannot be checked as written", nonNull, nullable)
		}
		if got := docPhrase(t, u, reRatio)[1]; got != cardinal(t, nonNull/nullable) {
			t.Errorf("StaticDataRef %d outnumbers StaticDataRef? %d %s to one, not %q",
				nonNull, nullable, cardinal(t, nonNull/nullable), got)
		}
		if got := docPhrase(t, u, reTenUses)[1]; got != cardinal(t, nonNull+nullable) {
			t.Errorf("StaticDataRef is used by %d members, not %q", nonNull+nullable, got)
		}
		// Written once as a word above and as a digit in the section it cites.
		// Every place it is written has to agree, and one of them has to exist.
		uses := reUsedBy.FindAllStringSubmatch(u, -1)
		if len(uses) == 0 {
			t.Fatalf("%s no longer says how many members use StaticDataRef anywhere", invariantsDoc)
		}
		for _, hit := range uses {
			if n, _ := strconv.Atoi(hit[1]); n != nonNull+nullable {
				t.Errorf("%q, but %d members are typed StaticDataRef or StaticDataRef?", hit[0], nonNull+nullable)
			}
		}

		// The money/quotes split and the int count, each restated a paragraph
		// after the entry it restates.
		m = docPhrase(t, u, reMoneyQuotes)
		for i, label := range []string{"decimal?", "double", "double?"} {
			if n, _ := strconv.Atoi(m[i+1]); n != by[label] {
				t.Errorf("the money/quotes sentence says %s %d; the tally says %d", label, n, by[label])
			}
		}
		if got := docPhrase(t, u, reNinthInt)[1]; got != ordinal(t, by["int"]+1) {
			t.Errorf("`int` is %d keyed members, so the unkeyed ListingIdIndex is the %s in the "+
				"directory, not the %q", by["int"], ordinal(t, by["int"]+1), got)
		}
		if n, _ := strconv.Atoi(docPhrase(t, u, reValueTuple)[1]); n != by["ValueTuple"] {
			t.Errorf("the document counts %d ValueTuple members in prose and %d in the tally", n, by["ValueTuple"])
		}

		// `Absent entirely` is the tally's complement, so a type cannot be in
		// both. Nothing else about the list is checked -- `Guid` and `float`
		// are absences, and a test cannot derive a name that is not there.
		for _, name := range backticked(docParagraph(t, u, "Absent entirely: ")) {
			for _, label := range []string{name, name + "?"} {
				if _, in := by[label]; in {
					t.Errorf("`%s` is listed as absent entirely and carries %d members in the tally",
						name, by[label])
				}
			}
		}
	})

	// The one figure in this document that changes with the de-twinning rule,
	// and the reason the rule is keep-BP rather than keep-BO. It decided which
	// copy the tally counts, and until now nothing derived it: on a re-pin,
	// TestMemberAttributesAreEnumerated fails and gets updated, and this rots
	// in silence.
	t.Run("attribute co-tenancy", func(t *testing.T) {
		dci := corpusDataCacheIndex(t)
		u := unwrapped(doc)

		// Under the rule, they are all on members. If a re-pin ever leaves a
		// type-level one standing, the sentence below stops being true and no
		// count in it would say so.
		if dci.OnTypes != 0 {
			t.Errorf("the de-twinning rule leaves %d type-level [DataCacheIndex] standing, and the "+
				"document says the annotations sit on members", dci.OnTypes)
		}
		for _, re := range []*regexp.Regexp{reDCIMembers, reDCIAgain} {
			if n, _ := strconv.Atoi(docPhrase(t, u, re)[1]); n != dci.OnMembers {
				t.Errorf("%v reads %d member-level [DataCacheIndex]; under the rule there are %d",
					re, n, dci.OnMembers)
			}
		}

		// The raw count is the one the document tells its reader to trust,
		// because it is the one no rule can move.
		_, _, payloads := corpusDeclShapes(t)
		m := docPhrase(t, u, reDCIRaw)
		wantPayloads, _ := strconv.Atoi(m[1])
		wantRaw, _ := strconv.Atoi(m[2])
		wantMembers, _ := strconv.Atoi(m[3])
		if wantPayloads != payloads {
			t.Errorf("the raw count reads %d payload files; there are %d", wantPayloads, payloads)
		}
		if wantRaw != dci.RawOnMembers+dci.RawOnTypes || wantMembers != dci.RawOnMembers {
			t.Errorf("the corpus writes %d [DataCacheIndex], %d of them on members; the document says %d and %d",
				dci.RawOnMembers+dci.RawOnTypes, dci.RawOnMembers, wantRaw, wantMembers)
		}
		if got := m[4]; got != cardinal(t, dci.RawOnTypes) {
			t.Errorf("%d sit on a type, not %q", dci.RawOnTypes, got)
		}

		// And the road not taken, quoted so the choice can be re-examined
		// rather than taken on faith.
		m = docPhrase(t, u, reDCIKeepBO)
		if n, _ := strconv.Atoi(m[1]); n != dci.KeepBOOnMembers {
			t.Errorf("keeping the BO side gives %d member-level [DataCacheIndex], not %d", dci.KeepBOOnMembers, n)
		}
		if got := m[2]; got != cardinal(t, dci.KeepBOOnTypes) {
			t.Errorf("keeping the BO side leaves %d type-level, not %q", dci.KeepBOOnTypes, got)
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
		// wrong: SlimQuote is a bare `public class`, which csharp.go's view row
		// and TestCorpusCensus's three declaration rows both already encode,
		// and which the document used to contradict.
		//
		// Copy-editor's warning: this reads every backticked name up to the
		// first ". ", so the sentence has to name the three and then stop.
		// "`BOVenue` and `Venue` are both `struct`, and `SlimQuote`" is true and
		// fails here, because `struct` is not a type name. That is the safe
		// direction -- it fails loudly and in front of you rather than
		// silently agreeing -- but reword the second sentence, not the first.
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
// member is keyed or it is not: `ListingIdIndex` is expression-bodied and
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

// The figures the prose restates. These are matched against the document with
// its paragraphs unwrapped, because prose wraps and a claim that moved onto the
// next line is not a claim that changed. Each still has to match in exactly one
// place: a number restated twice is a number that can disagree with itself.
var (
	reRawProse       = regexp.MustCompile(`across all (\d+) payload files that is (\d+)\.`)
	reTwinPairs      = regexp.MustCompile(`the (\w+) BO \x{2194} BP twin pairs`)
	reRemaining      = regexp.MustCompile(`counts the remaining (\w+) files once each`)
	reDetwinnedProse = regexp.MustCompile(`That is (\d+)\.`)
	reCollapse       = regexp.MustCompile(`Collapsing that as well gives (\d+)\.`)
	reRatio          = regexp.MustCompile("`StaticDataRef` outnumbers `StaticDataRef[?]` (\\w+) to one")
	reTenUses        = regexp.MustCompile(`the (\w+) uses line up with`)
	reUsedBy         = regexp.MustCompile(`Used by (\d+) members`)
	reMoneyQuotes    = regexp.MustCompile("The `decimal[?]` (\\d+) are all money and the `double` (\\d+) / `double[?]` (\\d+) are all quotes")
	reNinthInt       = regexp.MustCompile("`ListingIdIndex` is a (\\w+) `public int`")
	reValueTuple     = regexp.MustCompile(`ValueTuple counts [*]members[*], (\d+) of them`)

	reDCIMembers = regexp.MustCompile("(\\d+) `\\[DataCacheIndex\\]` annotations sit on [*][*]members[*][*] under the de-twinning rule")
	reDCIAgain   = regexp.MustCompile(`That (\d+) is the one figure here`)
	reDCIRaw     = regexp.MustCompile(`across all (\d+) payload files there are (\d+) \x{2014} (\d+) on members and (\w+) on a type`)
	reDCIKeepBO  = regexp.MustCompile(`Keeping the BO side instead gives (\d+) member-level and that (\w+) type-level`)
)

// dataCacheIndexCensus is how many `[DataCacheIndex]` the payload directory
// writes, split by where they sit and by which twin a rule keeps.
//
// The split by rule is the whole point. The keyed members are identical on both
// sides of every twin pair, so the tally does not move; this does, and it is the
// figure that chose the rule. Keeping BP leaves 22 on members and none on a
// type; keeping BO leaves 23 and the corpus's one type-level annotation.
type dataCacheIndexCensus struct {
	RawOnMembers, RawOnTypes       int
	OnMembers, OnTypes             int // under the document's rule: drop BO
	KeepBOOnMembers, KeepBOOnTypes int
}

// detwinnedDropsKeepingBO is the other half of the choice: the BP-side copy of
// each pair, dropped by the rule the document did not take. It is here so the
// road not taken is derived rather than remembered, and so a re-pin that adds a
// twin pair breaks both lists at once instead of only the live one.
var detwinnedDropsKeepingBO = []string{
	"BPTicketTypeCacheItem.cs",
	"BPLiveObjectCacheItem.cs",
	"BPSearchCacheItem.cs",
	"BPShowCacheItem.cs",
	"Venue.cs",
}

// corpusDataCacheIndex counts the annotations off the source text rather than
// through readCS.
//
// readCS is right for members and wrong here. It drops the `[DataCacheIndex]`
// on BOShowCacheItem's computed `ListingIdIndex`, which is why the census
// TestMemberAttributesAreEnumerated holds says 29 where the document says 30 --
// both correct, about different things. The document counts what the corpus
// writes, so this counts that: every attribute occurrence, type-level ones
// separated by their indentation, which is how C# writes the distinction.
func corpusDataCacheIndex(t *testing.T) dataCacheIndexCensus {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(corpusRoot, "payloads"))
	if err != nil {
		t.Fatal(err)
	}
	var c dataCacheIndexCensus
	seen := map[string]bool{}
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".cs") {
			continue
		}
		seen[e.Name()] = true
		b, err := os.ReadFile(filepath.Join(corpusRoot, "payloads", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		var onMembers, onTypes int
		for _, line := range strings.Split(string(b), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "[DataCacheIndex") {
				continue
			}
			if line == strings.TrimSpace(line) {
				onTypes++ // unindented: it annotates the type
			} else {
				onMembers++
			}
		}
		c.RawOnMembers += onMembers
		c.RawOnTypes += onTypes
		if !slicesContains(detwinnedDrops, e.Name()) {
			c.OnMembers += onMembers
			c.OnTypes += onTypes
		}
		if !slicesContains(detwinnedDropsKeepingBO, e.Name()) {
			c.KeepBOOnMembers += onMembers
			c.KeepBOOnTypes += onTypes
		}
	}
	// A twin pair has exactly two sides, so the two drop lists have to be the
	// same length and name no file in common. Without this, a name going
	// missing from the keep-BO list is invisible whenever the file it names
	// carries no annotation -- which `Venue.cs` and `BPLiveObjectCacheItem.cs`
	// both do today, so two fifths of that list would be holding nothing.
	if len(detwinnedDropsKeepingBO) != len(detwinnedDrops) {
		t.Fatalf("the rule drops %d files and its mirror drops %d; a twin pair has two sides",
			len(detwinnedDrops), len(detwinnedDropsKeepingBO))
	}
	for _, name := range detwinnedDropsKeepingBO {
		if !seen[name] {
			t.Fatalf("the rule this document did not take drops payloads/%s, which is not in the corpus", name)
		}
		if slicesContains(detwinnedDrops, name) {
			t.Fatalf("payloads/%s is dropped by both rules, so it is nobody's twin", name)
		}
	}
	return c
}

// corpusKeyed is how many keyed members one payload file has.
func corpusKeyed(t *testing.T, file string) int {
	t.Helper()
	d := readCorpus(t, filepath.Join("payloads", file))
	n := 0
	for _, id := range d.Order {
		if _, wire := keyNumber(id); wire {
			n++
		}
	}
	return n
}

// cardinal and ordinal are how the document writes a small number when it
// writes it as a word. The lists stop at fourteen because the corpus does: a
// figure past the end is a figure this check was not built for, and it says so
// rather than comparing against an empty string.
var (
	cardinals = []string{"zero", "one", "two", "three", "four", "five", "six", "seven",
		"eight", "nine", "ten", "eleven", "twelve", "thirteen", "fourteen"}
	ordinals = []string{"zeroth", "first", "second", "third", "fourth", "fifth", "sixth", "seventh",
		"eighth", "ninth", "tenth", "eleventh", "twelfth", "thirteenth", "fourteenth"}
)

func cardinal(t *testing.T, n int) string { return numberWord(t, cardinals, n) }
func ordinal(t *testing.T, n int) string  { return numberWord(t, ordinals, n) }

func numberWord(t *testing.T, words []string, n int) string {
	t.Helper()
	if n < 0 || n >= len(words) {
		t.Fatalf("%d is outside the range this check spells out; write it in digits or extend the list", n)
	}
	return words[n]
}

// unwrapped is doc with each paragraph joined onto one line, so that a phrase
// can be matched without its line breaks mattering. Markdown's own line breaks
// inside a paragraph are not content, and a copy-edit that rewraps a paragraph
// must not change what this test finds in it.
func unwrapped(doc string) string {
	var out, para []string
	flush := func() {
		if len(para) > 0 {
			out = append(out, strings.Join(para, " "))
			para = nil
		}
	}
	for _, line := range strings.Split(doc, "\n") {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		para = append(para, strings.TrimSpace(line))
	}
	flush()
	return strings.Join(out, "\n")
}

// docPhrase is the submatches of the one place in the unwrapped document that
// re matches. Exactly one, for docLine's two reasons: a claim restated
// somewhere else means this is reading whichever came first, and a claim
// deleted rather than corrected has to fail here rather than pass by leaving
// nothing to disagree with.
func docPhrase(t *testing.T, unwrappedDoc string, re *regexp.Regexp) []string {
	t.Helper()
	m := re.FindAllStringSubmatch(unwrappedDoc, -1)
	if len(m) != 1 {
		t.Fatalf("%d places in %s match %v, want exactly 1", len(m), invariantsDoc, re)
	}
	return m[0]
}

// docParagraph is the one unwrapped paragraph beginning with prefix.
func docParagraph(t *testing.T, unwrappedDoc, prefix string) string {
	t.Helper()
	var found []string
	for _, para := range strings.Split(unwrappedDoc, "\n") {
		if strings.HasPrefix(para, prefix) {
			found = append(found, para)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%d paragraphs of %s begin %q, want exactly 1", len(found), invariantsDoc, prefix)
	}
	return found[0]
}

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
