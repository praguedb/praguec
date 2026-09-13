package csharp

import (
	"os"
	"path/filepath"
	"sort"
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

// exampleReservations is the `reserved` lists of examples/prague, by
// declaration name, as a set of retired tags.
//
// It reads the AST rather than the checked model, because `reserved` is not in
// the checked model at all -- it names nothing and holds nothing, so the
// checker records the ranges and drops them. That is the right shape for a
// backend and the wrong shape for this one assertion, which is about what the
// protop says rather than about what it emits.
//
// A view has no reserved list. A view selects, and a member it does not select
// is not a retired tag: the source still owns it.
func exampleReservations(t *testing.T) map[string]map[uint64]bool {
	t.Helper()
	ents, err := os.ReadDir(examplesDir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]map[uint64]bool{}
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
			var name string
			var members []syntax.Member
			switch d := d.(type) {
			case *syntax.Struct:
				name, members = d.Name.Text, d.Members
			case *syntax.Record:
				name, members = d.Name.Text, d.Members
			case *syntax.Table:
				name, members = d.Name.Text, d.Members
			default:
				continue
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
			out[name] = tags
		}
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
	reserved := exampleReservations(t)
	for _, r := range coverage {
		if r.Decl == "" || len(r.Missing) == 0 {
			continue
		}
		tags, isDecl := reserved[r.Decl]
		if !isDecl {
			continue // a view
		}
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
	reserved := exampleReservations(t)
	byDecl := map[string]corpusRow{}
	for _, r := range coverage {
		if r.Decl != "" {
			byDecl[r.Decl] = r
		}
	}
	for decl, tags := range reserved {
		r, ok := byDecl[decl]
		if !ok {
			continue
		}
		for tag := range tags {
			if _, listed := r.Missing[int(tag)-1]; !listed {
				t.Errorf("%s reserves tag %d, and the coverage table gives no reason for it", decl, tag)
			}
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
			"#24: there is no map anywhere in the corpus, so protopc has no Dictionary row to check"},
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
