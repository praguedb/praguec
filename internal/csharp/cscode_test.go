package csharp

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// A very small C# reader, used on both sides of every corpus comparison.
//
// It exists because the corpus is hand-written and praguec is not, and the
// interesting differences between them are never whitespace. Both the corpus
// file and the emitted text go through this one function, so a difference that
// survives it is a difference in what the type *is* -- a member, a key, a type,
// an attribute -- rather than in how a human happened to lay it out.
//
// Every normalisation it performs is named, and every one that can hide an
// expressiveness gap records where it fired. TestNormalisationSitesAreEnumerated
// holds those sites against a checked-in list, so "praguec cannot write
// `required`" is a line in a test rather than a silence in a diff.

// errSkip says the line was recognised and is deliberately not a member.
var errSkip = errors.New("skip")

// csDecl is one C# type, read structurally.
type csDecl struct {
	Usings    []string
	Namespace string
	Attrs     []string // type-level attributes, in source order
	Head      string   // "public sealed partial class BPShowCacheItem"
	Order     []string // member ids, in source order
	Members   map[string]string

	// Sites are the normalisations that fired, as "<what>:<member>".
	Sites []string

	// MemberAttrs are the member-level attributes that survived into the
	// canonical text, as "<attr> on <member>". `[Key(N)]` and `[IgnoreMember]`
	// are not here: they are read as the member's identity above and compared
	// as such. What is left is the passthrough surface -- `[DataCacheKey]`,
	// `[DataCacheIndex(...)]`, `[JsonIgnore]` -- which TestCorpusMembers
	// compares only as part of a member's text, so a comparison that stopped
	// carrying attributes at all would agree on both sides and say nothing.
	// TestMemberAttributesAreEnumerated holds this against a checked-in census
	// for that reason.
	MemberAttrs []string
}

// noteAttrs records the attributes of one canonical member.
//
// It reads them back out of the text attrsJoin produced rather than off the
// list that went in, so that an attrsJoin which returned the body alone empties
// this census instead of quietly dropping 11 `[DataCacheKey]` sites on both
// sides of every comparison.
func (d *csDecl) noteAttrs(body, member string) {
	for _, ln := range strings.Split(body, "\n") {
		if m := reAttr.FindStringSubmatch(ln); m != nil {
			d.MemberAttrs = append(d.MemberAttrs, m[1]+" on "+member)
		}
	}
}

// Keys are the member ids that are wire slots, as integers, in source order.
func (d *csDecl) Keys() []int {
	var out []int
	for _, id := range d.Order {
		if n, err := strconv.Atoi(id); err == nil {
			out = append(out, n)
		}
	}
	return out
}

var (
	reUsing = regexp.MustCompile(`^using ([\w.]+);$`)
	reNs    = regexp.MustCompile(`^namespace ([\w.]+);$`)
	reAttr  = regexp.MustCompile(`^\[(.+)\]$`)
	reKey   = regexp.MustCompile(`^\[Key\((\d+)\)\]$`)
	reHead  = regexp.MustCompile(`^public [\w ]*(class|struct|enum|interface) (\w+)`)
	// `{ get; set; }` only. An `init` accessor is rewritten to `set` a few
	// lines above, with its site recorded; letting this regex accept `init`
	// too would normalise it a second time and silently, which is one rule in
	// two places and a site that stops being counted the day the explicit
	// replacement is removed.
	reProperty = regexp.MustCompile(`^public (.+?) (\w+) \{ get; set; \}(.*)$`)
	reTuple    = regexp.MustCompile(`^public (\(.+\)) (\w+) => (.+);$`)
	reBare     = regexp.MustCompile(`^public ([\w.<>?\[\], ]+) (\w+)$`)
	reComputed = regexp.MustCompile(`^public ([\w.<>?\[\], ]+) (\w+) => (.+);$`)
)

// readCS reads C# source into a csDecl. It is deliberately not a parser: it
// recognises the handful of member shapes praguec can emit and reports anything
// else, so a corpus file that contains something new fails loudly instead of
// being quietly half-read.
func readCS(name, src string) (*csDecl, error) {
	d := &csDecl{Members: map[string]string{}}
	lines := normaliseLines(src)

	i := 0
	// --- header ---
	for ; i < len(lines); i++ {
		s := lines[i]
		if m := reUsing.FindStringSubmatch(s); m != nil {
			d.Usings = append(d.Usings, m[1])
			continue
		}
		if m := reNs.FindStringSubmatch(s); m != nil {
			d.Namespace = m[1]
			continue
		}
		if reHead.MatchString(s) {
			break
		}
		if m := reAttr.FindStringSubmatch(s); m != nil {
			d.Attrs = append(d.Attrs, m[1])
			continue
		}
		return nil, fmt.Errorf("%s:%d: unrecognised header line %q", name, i, s)
	}
	if i == len(lines) {
		return nil, fmt.Errorf("%s: no type declaration", name)
	}
	head := lines[i]
	// N-inherit: a base list is dropped. Nothing praguec emits has one, and the
	// one corpus file that does is a listed gap (#11).
	if b := strings.Index(head, " : "); b >= 0 {
		d.Sites = append(d.Sites, "base:"+strings.TrimSpace(head[b+3:]))
		head = head[:b]
	}
	d.Head = head
	i++
	if i < len(lines) && lines[i] == "{" {
		i++
	}

	typeName := reHead.FindStringSubmatch(d.Head)[2]

	// --- body ---
	var attrs []string
	id := ""
	for ; i < len(lines); i++ {
		s := lines[i]
		if s == "}" && attrs == nil && id == "" {
			continue
		}
		if m := reKey.FindStringSubmatch(s); m != nil {
			id = m[1]
			attrs = nil
			continue
		}
		if s == "[IgnoreMember]" {
			id = "alias"
			attrs = nil
			continue
		}
		if m := reAttr.FindStringSubmatch(s); m != nil {
			attrs = append(attrs, m[1])
			continue
		}
		// N-ctor: a constructor is the hand-written half arriving in the same
		// file (#25). praguec emits none, so both sides drop them.
		if strings.HasPrefix(s, "public "+typeName+"(") {
			d.Sites = append(d.Sites, "ctor:"+typeName)
			for ; i < len(lines) && lines[i] != "}"; i++ {
			}
			continue
		}
		body, memberID, used, err := readMember(name, lines, i, id, attrs, d)
		if err == errSkip {
			i += used - 1
			id, attrs = "", nil
			continue
		}
		if err != nil {
			return nil, err
		}
		if _, dup := d.Members[memberID]; dup {
			return nil, fmt.Errorf("%s: member %q twice", name, memberID)
		}
		d.Members[memberID] = body
		d.Order = append(d.Order, memberID)
		i += used - 1
		id, attrs = "", nil
	}
	return d, nil
}

// readMember reads one member starting at lines[i] and returns its canonical
// text, its id, and how many lines it consumed.
func readMember(name string, lines []string, i int, id string, attrs []string, d *csDecl) (string, string, int, error) {
	s := lines[i]

	// N-required (#12 closed on omitting it, #19 measured it unenforced) and
	// N-virtual (#11 ruled flattening, so nothing overrides anything).
	for _, w := range []string{"required ", "virtual "} {
		if k := strings.Index(s, "public "+w); k == 0 {
			d.Sites = append(d.Sites, strings.TrimSpace(w)+":"+memberNameOf(s))
			s = strings.Replace(s, w, "", 1)
		}
	}
	// N-init: `init` and `set` differ, and #20 forbids praguec emitting the
	// immutable side. Every site is listed.
	if strings.Contains(s, "{ get; init; }") {
		d.Sites = append(d.Sites, "init:"+memberNameOf(s))
		s = strings.Replace(s, "{ get; init; }", "{ get; set; }", 1)
	}

	if m := reProperty.FindStringSubmatch(s); m != nil {
		// N-initialiser: protop has no default-value syntax, so the corpus's
		// null-forgiving and empty-collection initialisers have nowhere to go.
		if tail := strings.TrimSpace(m[3]); tail != "" {
			d.Sites = append(d.Sites, "initialiser:"+m[2]+" "+tail)
		}
		if id == "" {
			return "", "", 0, fmt.Errorf("%s: member %q has no [Key]", name, m[2])
		}
		body := attrsJoin(attrs, fmt.Sprintf("public %s %s { get; set; }", m[1], m[2]))
		d.noteAttrs(body, m[2])
		return body, memberID(id, m[2]), 1, nil
	}
	if m := reTuple.FindStringSubmatch(s); m != nil {
		if id == "" {
			return "", "", 0, fmt.Errorf("%s: member %q has no [Key]", name, m[2])
		}
		body := attrsJoin(attrs, fmt.Sprintf("public %s %s => %s;", m[1], m[2], m[3]))
		d.noteAttrs(body, m[2])
		return body, memberID(id, m[2]), 1, nil
	}
	// N-computed: an expression-bodied member with no wire slot is a computed
	// member, and section 5 puts those in the hand-written partial half. One
	// corpus file writes one in the generated half anyway --
	// payloads/BOShowCacheItem.cs:63, `public int ListingIdIndex =>
	// ListingId ?? 0;`, which is the member the type's own
	// `[DataCacheIndex(nameof(ListingIdIndex), ...)]` names. The attribute
	// passes through as an annotation; the member it names does not, and
	// protop has no expression syntax to write one with.
	//
	// The guard is "no [Key(N)]": a numeric id is a wire member, and both ""
	// and "alias" (the [IgnoreMember] the corpus writes above this one) mean
	// there is no slot. An alias is a block body and reaches reBare instead.
	if m := reComputed.FindStringSubmatch(s); m != nil && (id == "" || id == "alias") {
		d.Sites = append(d.Sites, "computed:"+m[2])
		return "", "", 1, errSkip
	}
	if m := reBare.FindStringSubmatch(s); m != nil && i+4 < len(lines) && lines[i+1] == "{" {
		get := strings.TrimSpace(lines[i+2])
		set := strings.TrimSpace(lines[i+3])
		body := attrsJoin(attrs, fmt.Sprintf("public %s %s { %s %s }", m[1], m[2], get, set))
		d.noteAttrs(body, m[2])
		return body, memberID(id, m[2]), 5, nil
	}
	return "", "", 0, fmt.Errorf("%s:%d: unrecognised member %q", name, i, s)
}

// memberID is the key for a wire member and `alias:Name` for one without a
// slot, so that the two halves of a comparison line up by what they are rather
// than by where they were written.
func memberID(id, member string) string {
	if id == "alias" {
		return "alias:" + member
	}
	return id
}

func memberNameOf(s string) string {
	f := strings.Fields(strings.TrimSuffix(s, ";"))
	for i, w := range f {
		if w == "{" && i > 0 {
			return f[i-1]
		}
	}
	if len(f) >= 3 {
		return f[2]
	}
	return s
}

func attrsJoin(attrs []string, body string) string {
	if len(attrs) == 0 {
		return body
	}
	var b strings.Builder
	for _, a := range attrs {
		b.WriteString("[" + a + "]\n")
	}
	b.WriteString(body)
	return b.String()
}

// bom is U+FEFF, which four corpus files start with.
const bom = "\uFEFF"

// normaliseLines applies the whitespace-shaped normalisations, none of which
// can hide anything: N-bom, N-trailing-newline, N-trailing-whitespace,
// N-indent, N-blank (the corpus is internally inconsistent about blank lines
// between members -- payloads separate them, Venue and BaseProviderObject do
// not) and N-comment (protop's lexer discards comments and the model does not
// carry them).
func normaliseLines(src string) []string {
	src = strings.TrimPrefix(src, bom)
	var out []string
	for _, ln := range strings.Split(src, "\n") {
		s := strings.TrimSpace(strings.TrimSuffix(ln, "\r"))
		if s == "" || strings.HasPrefix(s, "//") {
			continue
		}
		out = append(out, s)
	}
	return out
}

// missingFrom is the elements of want that got does not have.
// missingFrom is a *multiset* difference, and the multiset is the point. The
// normalisation sites it reports are counts -- five `= string.Empty` and not
// four -- so a set difference reports nothing at all when a site starts firing
// one extra time, which is exactly the change worth seeing.
func missingFrom(want, got []string) []string {
	have := map[string]int{}
	for _, g := range got {
		have[g]++
	}
	var out []string
	for _, w := range want {
		if have[w] == 0 {
			out = append(out, w)
			continue
		}
		have[w]--
	}
	sort.Strings(out)
	return out
}

// --- the reader, held against itself -------------------------------------------
//
// Everything above is harness rather than product, and a harness is where a
// defect hides best: a reader that silently reads nothing makes every
// comparison in corpus_test.go pass. These are the assertions that keep the
// reader from being able to agree with the corpus by saying less about it.

// missingFrom is a multiset difference and its own comment argues why. A set
// difference passes every corpus test in this package -- and so does returning
// nil unconditionally, which switches both using-assertions in TestCorpusHeaders
// off at once. Both are pinned here because neither is visible anywhere else.
func TestMissingFromIsAMultisetDifference(t *testing.T) {
	for _, c := range []struct {
		what      string
		want, got []string
		missing   []string
		whyItPins string
	}{
		{
			what:      "a second occurrence is a second element",
			want:      []string{"a", "a", "b"},
			got:       []string{"a", "b"},
			missing:   []string{"a"},
			whyItPins: "a set difference reports nothing here, and a normalisation site that starts firing one extra time is exactly what this is for",
		},
		{
			what:      "nothing on the other side is everything missing",
			want:      []string{"a"},
			got:       nil,
			missing:   []string{"a"},
			whyItPins: "a missingFrom that returns nil reports no missing using, no extra using and no corpus-file disagreement",
		},
		{
			what:      "a matched element is consumed once",
			want:      []string{"a", "a"},
			got:       []string{"a", "a"},
			missing:   nil,
			whyItPins: "counting rather than matching would report the second `a` against itself",
		},
		{
			what:      "the result is sorted, since both callers compare it by eye",
			want:      []string{"c", "b", "a"},
			got:       nil,
			missing:   []string{"a", "b", "c"},
			whyItPins: "an unsorted diff reads as a different diff on every run",
		},
	} {
		t.Run(c.what, func(t *testing.T) {
			got := missingFrom(c.want, c.got)
			if strings.Join(got, ",") != strings.Join(c.missing, ",") {
				t.Errorf("missingFrom(%v, %v) = %v, want %v (%s)", c.want, c.got, got, c.missing, c.whyItPins)
			}
		})
	}
}

// The duplicate-member guard. Two members at one key is a corpus file the
// reader cannot represent -- Members is keyed by id, so the second overwrites
// the first and Order lists it twice -- and half-reading it quietly is the one
// failure mode a structural reader must not have.
func TestReadCSRejectsTwoMembersAtOneKey(t *testing.T) {
	_, err := readCS("dup.cs", `namespace N;

[MessagePackObject]
public sealed partial class T
{
    [Key(0)]
    public long Id { get; set; }

    [Key(0)]
    public string Name { get; set; }
}
`)
	if err == nil {
		t.Fatal("readCS accepted two members at key 0; the second silently replaces the first and the file is half-read")
	}
	if !strings.Contains(err.Error(), "twice") {
		t.Errorf("readCS error = %v, want it to name the duplicate", err)
	}
}

// The alias block, read as one canonical line.
//
// The five-line shape is the only member the reader assembles from more than
// one line, and it is assembled positionally: the accessor bodies are lines i+2
// and i+3 with no regex to say which is which. So blanking them reads every
// alias in the corpus as the same empty passthrough, and swapping them reads
// `get => Id; set => Id = value;` as `set => Id = value; get => Id;` -- which is
// still a get and a set, still matches the corpus after the same swap is applied
// to it, and is the half of an emitter that writes `get => TicketTypeId;` into
// TicketTypeId's own body. Neither is visible from a corpus comparison, so the
// canonical text is written out here.
func TestReadCSReadsAnAliasAsAPassthrough(t *testing.T) {
	d, err := readCS("alias.cs", `namespace N;

[MessagePackObject]
public sealed partial class T
{
    [IgnoreMember]
    [JsonIgnore]
    public long TicketTypeId
    {
        get => Id;
        set => Id = value;
    }

    [Key(0)]
    public long Id { get; set; }
}
`)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := strings.Join(d.Order, ","), "alias:TicketTypeId,0"; got != want {
		t.Fatalf("members = %s, want %s -- an alias has no wire slot and is keyed by name", got, want)
	}
	const want = "[JsonIgnore]\npublic long TicketTypeId { get => Id; set => Id = value; }"
	if got := d.Members["alias:TicketTypeId"]; got != want {
		t.Errorf("the alias reads as\n%s\nwant\n%s", got, want)
	}
}

// attrsJoin is what puts a member's attributes into the text that gets
// compared, and it runs on both sides -- so dropping them is invisible from any
// corpus assertion, and takes `[DataCacheKey]`, `[DataCacheIndex(...)]` and
// every `[cs: ...]` passthrough with it.
func TestAttrsJoinKeepsEveryAttribute(t *testing.T) {
	const body = "public long Id { get; set; }"
	for _, c := range []struct {
		attrs []string
		want  string
	}{
		{nil, body},
		{[]string{"DataCacheKey"}, "[DataCacheKey]\n" + body},
		{[]string{"DataCacheKey", "DataCacheIndex(DataCacheIndexType.Range)"},
			"[DataCacheKey]\n[DataCacheIndex(DataCacheIndexType.Range)]\n" + body},
	} {
		if got := attrsJoin(c.attrs, body); got != c.want {
			t.Errorf("attrsJoin(%v, ...) = %q, want %q", c.attrs, got, c.want)
		}
	}
}
