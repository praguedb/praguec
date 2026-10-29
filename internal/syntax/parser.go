package syntax

import (
	"slices"
	"strconv"
	"strings"

	"github.com/nanov/protobuf-to-messagepack-sharp/internal/diag"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/source"
)

// protopRevision is the dialect this compiler implements. `revision 1;` stamps
// the language's version, not the schema's.
const protopRevision = 1

// scaledScalar is the one scalar type that carries a precision: decimal(4).
const scaledScalar = "decimal"

// scalarTypes is the closed set of protop's scalar type keywords, hard-coded
// from a census of every fenced protop block in docs/spec/protop.md -- which
// returns these six and nothing else.
//
// Whether the set is closed at all is one of the four things the spec
// deliberately leaves open, tracked as nanov/protobuf-to-messagepack-sharp#32.
// Widening protop's scalars is one edit here.
var scalarTypes = map[string]bool{
	"int64":      true,
	"int32":      true,
	"string":     true,
	"double":     true,
	"bool":       true,
	scaledScalar: true,
}

// collectionTypes is protop's closed set of generic types, each mapped to the
// number of type arguments it takes. It is deliberately separate from
// scalarTypes: a collection is not a scalar, it is a type constructor, and the
// question of whether protop's scalar set is closed (#32) has nothing to say
// about it.
//
// Repetition is spelled here rather than as a field label because a type is
// where presence can compose: `list<Odd>?` and `list<Odd?>` are different
// types, and a `repeated Odd?` could only have been one of them.
var collectionTypes = map[string]int{
	"list": 1,
	"map":  2,
}

// notYetCollections are collections the spec defines and this milestone does
// not read. `map` parses in full and is then reported not-yet-implemented: the
// corpus has no map, and the syntax exists so that adding one later fills a
// defined slot rather than inventing a third collection form.
var notYetCollections = map[string]bool{
	"map": true,
}

// IsScalar reports whether name is one of protop's scalar type keywords.
//
// The set is read here and nowhere else. A second stage that needed to know
// what a scalar is would otherwise keep a copy of the list, and the list is
// #32's single edit: two copies is two edits, and the second one is the one
// that gets forgotten.
func IsScalar(name string) bool { return scalarTypes[name] }

// ScalarTakesPrecision reports whether name is the one scalar that carries a
// precision, `decimal(4)`.
func ScalarTakesPrecision(name string) bool { return name == scaledScalar }

// CollectionArity reports the number of type arguments the collection named
// takes, and whether name is a collection at all.
func CollectionArity(name string) (int, bool) {
	arity, ok := collectionTypes[name]
	return arity, ok
}

// IsBuiltinTypeName reports whether name is one protop's own type system
// already holds: one of its scalars, or one of its collections.
//
// It reads the two closed sets and copies neither, which is the whole of why it
// lives here rather than in the stage that asks. A caller outside this package
// that kept its own list would have to be edited a second time the day either
// set grows -- and the second edit is the one that gets forgotten. Whether the
// scalar set is the right set is #32's question; this one is only whether the
// name is already taken.
func IsBuiltinTypeName(name string) bool {
	_, isCollection := collectionTypes[name]
	return scalarTypes[name] || isCollection
}

// BuiltinTypeNames is every name IsBuiltinTypeName answers yes to, sorted, for
// a caller that has to walk the set rather than ask about one name of it.
//
// Sorted because a map has no order and a test that iterated one would report a
// different failure on every run. Derived from the same two maps for the same
// reason as above: a rule held against this walks the language, so a name added
// to either map is covered with no second edit anywhere.
func BuiltinTypeNames() []string {
	names := make([]string, 0, len(scalarTypes)+len(collectionTypes))
	for name := range scalarTypes {
		names = append(names, name)
	}
	for name := range collectionTypes {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// RelationAnnotation is the name of protop's own relation annotation, the
// `references` of `[references(SportGame)]`. Its contents are read by the stage
// that can look a name up, which is not this one.
const RelationAnnotation = "references"

// backends is the legal set of annotation prefixes, which is exactly the
// backends protopc has. `[go: json:"sportGame"]` becomes legal by a Go backend
// being written, not by a word being added here.
var backends = map[string]bool{
	"cs": true,
}

// protopAnnotations is protop's own set, the annotations written with no
// prefix. It is closed and read -- an unknown one is an error -- which is the
// whole difference between it and the text protopc passes through untouched.
var protopAnnotations = map[string]bool{
	RelationAnnotation: true,
}

// ofWord and asWord are the two words that read as keywords inside a view and
// are idents everywhere else -- `view X of Y` and `event_id as id`. Neither is
// a token kind, and neither is reserved: `key as id;` selects a member named
// `key`, and a table may still hold a field named `of` or `as`. What makes a
// word one of these is the position it is written in, which is the whole of
// the no-keyword-token-kinds decision.
const (
	ofWord = "of"
	asWord = "as"
)

// Parse reads one file into an AST, appending every diagnostic it finds to bag.
//
// It always returns a File, even for source it could not parse: the tree is
// best-effort, and bag.HasErrors is what says whether it means anything.
// Partial trees are what let an editor outline a file that is still being
// typed, and the reason recovery skips to the next `;` or balanced `}` instead
// of giving up at the first error.
func Parse(f *source.File, bag *diag.Bag) *File {
	p := &parser{src: f, toks: Lex(f, bag), bag: bag}
	return p.parseFile()
}

type parser struct {
	src  *source.File
	toks []Token
	bag  *diag.Bag
	i    int // index into toks of the next unread token

	// reported says that a diagnostic has already been raised about the token
	// at i and that nothing has been consumed since. It exists for one shape:
	// a missing trailing `;` is reported where it is and the offending token
	// is deliberately left unconsumed (see recovery, below), so the body loop
	// re-enters on a token that has already been named and its member-start
	// report says the same thing a second time. That second report is a fact
	// about this parser's control flow, not about the source, so raiseMemberStart
	// drops it.
	//
	// next is what clears the flag, which is what makes it exact: set means
	// "still sitting on the token that was reported on". A report about a
	// different token, or about the same token once anything has moved past it,
	// is never suppressed -- including at EOF, where next advances no index but
	// still clears the flag.
	reported bool
}

func (p *parser) parseFile() *File {
	file := &File{Span: source.Span{Start: 0, End: source.Pos(len(p.src.Text))}}
	for !p.at(KindEOF) {
		p.parseDecl(file)
	}
	// Both file-level declarations are checked after the whole file, so that a
	// file missing its header still reports everything else wrong with it.
	if file.Revision == nil {
		p.bag.Raise(diag.MissingRevision, p.headSpan(), "a protop file must open with `revision %d;`", protopRevision)
	}
	if file.Namespace == nil {
		p.bag.Raise(diag.MissingNamespace, p.headSpan(), "a protop file must declare a `namespace`")
	}
	return file
}

func (p *parser) parseDecl(file *File) {
	// An annotation at the top level describes the declaration it precedes, and
	// the grammar has no slot there. Skipping it leaves that declaration to be
	// read on the next pass of the loop, with one diagnostic against it.
	p.rejectAnnotations("the declaration's name")

	tok := p.peek()
	if tok.Kind != KindIdent {
		p.bag.Raise(diag.UnknownDeclaration, tok.Span, "expected a declaration, found %s", tok.Kind)
		p.recoverDecl()
		return
	}

	switch word := p.text(tok); word {
	case "revision":
		p.parseRevision(file)
	case "namespace":
		p.parseNamespace(file)
	case "struct":
		file.Decls = append(file.Decls, p.parseStruct())
	case "record":
		file.Decls = append(file.Decls, p.parseRecord())
	case "table":
		file.Decls = append(file.Decls, p.parseTable())
	case "enum":
		file.Decls = append(file.Decls, p.parseEnum())
	case "view":
		file.Decls = append(file.Decls, p.parseView())
	case "import":
		p.parseImport(file)
	default:
		p.bag.Raise(diag.UnknownDeclaration, tok.Span,
			"unknown declaration `%s`; expected `struct`, `enum`, `record`, `table`, `view` or `import`", word)
		p.recoverDecl()
	}
}

// parseRevision reads `revision 1;`. A malformed stamp is still recorded, so
// that one mistake produces one diagnostic rather than also a missing-revision.
func (p *parser) parseRevision(file *File) {
	kw := p.next()
	rev := &Revision{Span: kw.Span}

	if num, ok := p.expectInt(); !ok {
		p.recoverDecl()
	} else {
		rev.Number = num
		rev.Span = rev.Span.To(num.Span)
		if num.Value != protopRevision {
			p.bag.Raise(diag.UnsupportedRevision, num.Span,
				"unknown revision %d; this compiler implements revision %d", num.Value, protopRevision)
		}
		if semi, ok := p.expect(KindSemi); ok {
			rev.Span = rev.Span.To(semi.Span)
		}
	}

	switch {
	case file.Revision != nil:
		p.bag.Raise(diag.DuplicateRevision, rev.Span, "`revision` is already declared at %s",
			p.src.Position(file.Revision.Span.Start))
		return // the first stamp wins
	case len(file.Decls) > 0 || len(file.Imports) > 0 || file.Namespace != nil:
		p.bag.Raise(diag.RevisionNotFirst, rev.Span, "`revision` must be the first declaration in the file")
	}
	file.Revision = rev
}

func (p *parser) parseNamespace(file *File) {
	kw := p.next()
	ns := &Namespace{Span: kw.Span}

	for {
		part, ok := p.expectIdent()
		if !ok {
			p.recoverDecl()
			break
		}
		ns.Parts = append(ns.Parts, part)
		ns.Span = ns.Span.To(part.Span)
		if !p.at(KindDot) {
			if semi, ok := p.expect(KindSemi); ok {
				ns.Span = ns.Span.To(semi.Span)
			}
			break
		}
		p.next() // the dot
	}

	if file.Namespace != nil {
		p.bag.Raise(diag.DuplicateNamespace, ns.Span, "`namespace` is already declared at %s",
			p.src.Position(file.Namespace.Span.Start))
		return // the first declaration wins
	}
	file.Namespace = ns
}

// parseImport reads `import "prague.prague";`.
//
// Imports are grouped under the file header, and an import written below a
// declaration is diagnosed. The reason: what a file depends on is a property of
// the whole file rather than of the lines that follow it. Nothing in protop is
// scoped by position, so an import half way down would come into force at the
// top of the file anyway, and a grammar that allowed it would be saying the
// order mattered when it does not. Grouped, a reader learns a file's
// dependencies from its head without reading it entire -- the same reason
// `revision` and `namespace` are pinned where they are, and the order section 2
// writes. An import above the namespace is still in the header and is not
// diagnosed: the rule is about declarations, not about the two header lines.
//
// Nothing is resolved. The string names a file; where that file is found is the
// distribution model -- git, by immutable tag, toolchain-fetched -- and every
// particular of it is open. No file is opened and no path is looked at.
func (p *parser) parseImport(file *File) {
	kw := p.next()
	im := &Import{Span: kw.Span}

	if path, ok := p.expectString(); !ok {
		p.recoverDecl()
	} else {
		im.Path = path
		im.Span = im.Span.To(path.Span)
		if semi, ok := p.expect(KindSemi); ok {
			im.Span = im.Span.To(semi.Span)
		}
	}

	if len(file.Decls) > 0 {
		p.bag.Raise(diag.ImportNotInHeader, im.Span,
			"an `import` belongs with the file's other imports, above the first declaration; what a file depends on is a property of the whole file and not of the lines below it")
	}
	file.Imports = append(file.Imports, im)
}

func (p *parser) parseStruct() *Struct {
	h := p.parseTypeDecl()
	return &Struct{Name: h.name, Annotations: h.anns, Members: h.members, Span: h.span}
}

// parseRecord reads `record StaticDataRef { ... }`. It is parseStruct with a
// different node, and deliberately so: section 4 gives the two keywords
// different emissions and different rules about being referenced, and none of
// that is a question the grammar asks.
func (p *parser) parseRecord() *Record {
	h := p.parseTypeDecl()
	return &Record{Name: h.name, Annotations: h.anns, Members: h.members, Span: h.span}
}

// parseTable reads `table SportGame [cs: ...] { ... }`. Its body is the same
// grammar as a struct's; what a table has that the others do not is identity,
// and identity is a member -- a `key` -- rather than anything in the head.
func (p *parser) parseTable() *Table {
	h := p.parseTypeDecl()
	return &Table{Name: h.name, Annotations: h.anns, Members: h.members, Span: h.span}
}

// parseView reads `view SlimSportGame of SportGame { ... }`.
//
// `of` is an ident in the third position, not a token kind -- which is the
// no-keyword-token-kinds decision paying out a third time, after `key key` and
// `alias`. A view may still hold a member named `of`, and a table a field named
// one.
//
// The source is a name and stays one. Whether `SportGame` is declared, whether
// it is a table or a record, and whether every member below selects something
// it actually has are all questions about a whole program rather than about
// this file.
func (p *parser) parseView() *View {
	kw := p.next()
	v := &View{Span: kw.Span}

	name, ok := p.expectIdent()
	if !ok {
		p.recoverDecl()
		return v
	}
	v.Name = name
	v.Span = v.Span.To(name.Span)

	src, ok := p.parseViewSource()
	if !ok {
		p.recoverDecl()
		return v
	}
	v.Source = src
	v.Span = v.Span.To(src.Span)

	if v.Annotations, ok = p.parseAnnotations(); !ok {
		p.recoverDecl()
		return v
	}
	if _, ok := p.expect(KindLBrace); !ok {
		p.recoverDecl()
		return v
	}
	for !p.at(KindRBrace) && !p.at(KindEOF) {
		if m := p.parseViewMember(); m != nil {
			v.Members = append(v.Members, m)
		}
	}
	if closing, ok := p.expect(KindRBrace); ok {
		v.Span = v.Span.To(closing.Span)
	}
	return v
}

// parseViewSource reads the `of SportGame` of a view head. The two ways of
// getting it wrong -- no `of` at all, and an `of` with nothing after it -- are
// one mistake said two ways, so they are one diagnostic: the view names nothing
// to be a view of.
func (p *parser) parseViewSource() (Ident, bool) {
	if p.at(KindIdent) && p.text(p.peek()) == ofWord {
		p.next()
		if p.at(KindIdent) {
			return p.identOf(p.next()), true
		}
	}
	// The underline is where the source belongs, and that is not the same
	// place in the two ways of getting it wrong. `view V from T` wrote a word
	// in the source position and got the word wrong, so the word is what the
	// caret has to sit under. `view V {` and `view V of {` wrote nothing there
	// at all, and the token the parser is looking at is the `{` -- the one part
	// of the head that is RIGHT. Pointing at it says "delete this brace", which
	// is the one repair that does not work. An empty span at the end of what
	// was read points at the gap instead, which is where the missing words go.
	span := source.SpanAt(p.prev().Span.End)
	if tok := p.peek(); tok.Kind == KindIdent {
		span = tok.Span
	}
	p.bag.Raise(diag.ViewSourceMissing, span,
		"a view is a subset of one source and declares no members of its own; name that source, as in `view V %s SportGame { ... }`", ofWord)
	return Ident{}, false
}

// parseViewMember reads one line of a view body, in all four of its spellings:
//
//	sport_type_id;      a plain selection
//	event_id as id;     the same, renamed
//	SlimOdd odds;       narrowed to another view
//	SlimOdd odds as o;  narrowed and renamed
//
// Each is idents and a `;`, so the shape is told by arity and by position and
// by nothing else: one ident selects, two narrow, three rename when the middle
// word is `as`, and four narrow and rename when the third word is. Two idents
// in a row are a narrowing -- unless the second is the `as` of a rename, and
// how many words the member has is what says whether it is. Nothing is
// reserved anywhere in here: `key as id;` selects the member named `key`, the
// composite identity's own word, and `SlimOdd as;` would narrow a member named
// `as`.
func (p *parser) parseViewMember() *ViewMember {
	p.rejectAnnotations("the member's name")

	tok := p.peek()
	if tok.Kind != KindIdent {
		p.raiseMemberStart(tok, "a view member's name")
		// The offending token is left for recovery rather than consumed
		// first. recoverStmt consumes everything that is not a boundary, so
		// skipping the token here would only matter when it IS one -- and a
		// stray `;` is the end of the line that went wrong, not the start of
		// it. Consuming it would send recovery hunting for the NEXT `;`, and
		// the member after the stray one would vanish from the tree without a
		// word said about it.
		p.recoverStmt()
		return nil
	}

	// The run is counted before a word of it is claimed, because the shape is
	// arity and position together and the two-token peek that reads `as <name>`
	// only has the position. Four idents whose third word is `as` are the
	// fourth shape, so the `as` the peek is sitting on is the member's own name
	// and the rename's keyword is the word after it: `SlimInner as as x;`
	// narrows the member named `as` and calls it `x`. Read on the peek alone,
	// the first `as` was claimed as the keyword and `x` was left over.
	//
	// Nothing else consults the count. A run that is none of the four shapes --
	// four idents with something other than `as` third, or a fifth ident, or a
	// `;` that was left out so that one member's run walks into the next one --
	// is not a shape to be read but a mistake to be reported, and it is
	// reported where it already was: by reading greedily and stopping at the
	// first word that cannot be there.
	run := p.identRun()
	lead := p.identOf(p.next())
	m := &ViewMember{Name: lead, Span: lead.Span}
	narrowAndRename := run == 4 && p.text(p.peekAt(1)) == asWord
	if p.at(KindIdent) && (!p.atRename() || narrowAndRename) {
		narrow := lead
		m.Narrow = &narrow
		m.Name = p.identOf(p.next())
		m.Span = m.Span.To(m.Name.Span)
		p.checkNarrowing(narrow)
	}
	if p.atRename() {
		p.next() // the `as`
		as := p.identOf(p.next())
		m.As = &as
		m.Span = m.Span.To(as.Span)
	}

	var ok bool
	if m.Annotations, ok = p.parseAnnotations(); !ok {
		// The recovery here cannot be observed, and is kept for the shape.
		// scanAnnotation fails in exactly two places -- at the end of the file,
		// and on a `}` that closes a brace the annotation did not open -- and
		// neither consumes the token it stopped on, so the parser is sitting on
		// EOF or on a `}` at depth zero. recoverStmt returns immediately on
		// both. Removing the call changes nothing; so would leaving it out of
		// the next branch, which is why it is written the same way as the rest.
		p.recoverStmt()
		return m
	}

	// A tag here is not a syntax slip. A view is pinned to its source's tags
	// and only the source extends the tag space, so the member is told what a
	// view is rather than which token was expected.
	//
	// The check comes after the annotations, not before them, because an
	// annotation follows what it describes and a view member is fully read
	// only once its annotations are. Checked first, `a [cs: X] = 1;` never
	// reached this diagnostic at all: the `[` is not an `=`, so the member
	// fell through to the annotations, and the `=` after them was reported as
	// a missing `;` instead of as the thing it actually is.
	//
	// The underline is the `=` and not the number after it, and that is a
	// ruling rather than an accident -- it was raised as one by the census that
	// filed nanov/protobuf-to-messagepack-sharp#57, on the grounds that the
	// offending thing is the tag. Three reasons it stays where it is:
	//
	// The number is not what is wrong. There is no tag that would make this
	// line right, so a caret under `5` invites the reader to try 6 -- which is
	// precisely the misunderstanding the message is written to undo. The `=` is
	// the token that turns a selection into a tagged field, and it is therefore
	// the first token in the line that could not be there.
	//
	// It is the only span every arm of this mistake has. The report is raised
	// on a peek, before the tag is read, and the member is recovered rather
	// than parsed further -- so `sport_type_id = ;` reaches here with no tag to
	// underline at all. A span that exists in one spelling of a mistake and not
	// in another is not a span the diagnostic can be written against.
	//
	// And the caret does not have to carry the identification: the message
	// names the member itself. What the caret is for here is the repair, and
	// the repair is to delete from the `=` to the `;`.
	if p.at(KindAssign) {
		p.bag.Raise(diag.TagInViewMember, p.peek().Span,
			"a view may not write a tag: `%s` is pinned to the tag its source gives it, and only the source extends the tag space; a tag added here is never written by the producer, so it would decode to nil forever while squatting a slot the source may later claim",
			m.Name.Text)
		p.recoverStmt()
		return m
	}
	if semi, ok := p.expect(KindSemi); ok {
		m.Span = m.Span.To(semi.Span)
	}
	return m
}

// atRename reports whether the parser is at the `as` of `event_id as id`. The
// ident after it is what makes it one: `SlimOdd as;` narrows a member named
// `as`, and only the third word tells the two apart.
//
// It answers for the position it is standing on and not for the member. In the
// fourth shape a member named `as` puts two of them side by side, and which is
// the keyword is a question about the whole run -- so parseViewMember counts
// the run and overrides this at the first of the two.
func (p *parser) atRename() bool {
	return p.at(KindIdent) && p.text(p.peek()) == asWord && p.peekAt(1).Kind == KindIdent
}

// identRun counts the idents that run from the parser's position to whatever
// ends them. In a view body that is the member's arity, which is what tells its
// four shapes apart -- and unlike a peek at a fixed offset it is a number the
// last word of the member can change, which is the whole of why the count is
// taken.
func (p *parser) identRun() int {
	n := 0
	for p.peekAt(n).Kind == KindIdent {
		n++
	}
	return n
}

// checkNarrowing reports a scalar written in a view member's leading position.
// That position asks for the member's type to be narrowed to another view, and
// a scalar is not one -- a member that keeps the type its source gives it is
// written as its name alone. Whether the name is a view at all is otherwise a
// question for the stage that resolves it; a scalar is the one case this file
// can answer by itself, because the scalars are a closed set and nothing can
// declare a view over that name.
func (p *parser) checkNarrowing(narrow Ident) {
	if !scalarTypes[narrow.Text] {
		return
	}
	p.bag.Raise(diag.ScalarInViewMember, narrow.Span,
		"`%s` is a scalar and a view member narrows to another view; a member keeps the type its source gives it when it is written as its name alone",
		narrow.Text)
}

// declHead is what struct, record and table are read into before they become nodes of
// their own: the shape the declarations share, which is all the parser sees.
type declHead struct {
	name    Ident
	anns    []*Annotation
	members []Member
	span    source.Span
}

// parseTypeDecl reads `<keyword> Name [<annotations>] { <members> }`, the
// keyword already peeked at by the caller that dispatched on it.
func (p *parser) parseTypeDecl() declHead {
	kw := p.next()
	h := declHead{span: kw.Span}
	// The keyword goes down into the body, because one member is legal in one
	// declaration and not in the others: only a table has identity.
	keyword := p.text(kw)

	name, ok := p.expectIdent()
	if !ok {
		p.recoverDecl()
		return h
	}
	h.name = name
	h.span = h.span.To(name.Span)
	if h.anns, ok = p.parseAnnotations(); !ok {
		p.recoverDecl()
		return h
	}

	if _, ok := p.expect(KindLBrace); !ok {
		p.recoverDecl()
		return h
	}
	for !p.at(KindRBrace) && !p.at(KindEOF) {
		if m := p.parseMember(keyword); m != nil {
			h.members = append(h.members, m)
		}
	}
	if closing, ok := p.expect(KindRBrace); ok {
		h.span = h.span.To(closing.Span)
	}
	p.checkTags(h.members)
	return h
}

// parseMember reads one line of a declaration body. The leading word decides
// which construct it is, and a word in leading position is the construct even
// though the same word is a legal name elsewhere: `reserved` opens a
// reservation and `alias` a passthrough, and a field may still be named either.
//
// `key` is the one word that needs a second look, because both spellings of
// identity open with it. The composite spelling names its member immediately,
// which puts its `=` two tokens in; the simple spelling still has a type to get
// through, so two tokens in is the field's name. That one lookahead is what the
// decision to have no keyword token kinds costs, and `key key = 1 (event_id,
// bet_type_id);` -- a member named `key`, of a kind named `key` -- is what it
// buys.
func (p *parser) parseMember(keyword string) Member {
	p.rejectAnnotations("the member's tag")

	tok := p.peek()
	if tok.Kind != KindIdent {
		p.raiseMemberStart(tok, "a field type")
		p.next()
		p.recoverStmt()
		return nil
	}
	word := p.text(tok)

	switch {
	case word == "reserved":
		return p.parseReserved()
	case word == "alias":
		return p.parseAlias()
	case word == "key" && p.peekAt(2).Kind == KindAssign:
		p.checkKeyAllowed(keyword, tok.Span)
		return p.parseCompositeKey()
	}

	var lead Token
	key := word == "key"
	if key {
		lead = p.next()
		p.checkKeyAllowed(keyword, lead.Span)
	}
	f := p.parseField()
	if f == nil {
		// A nil *Field has to be returned as a nil Member, not as an interface
		// holding a nil pointer, or the body loop appends a member that is not
		// there.
		return nil
	}
	f.Key = key
	if key {
		// The field's span opens at the `key`, not at the type: a stage that
		// quotes the member by span has to get back the line that was written.
		f.Span = lead.Span.To(f.Span)
	}
	return f
}

// parseCompositeKey reads `key key = 1 (event_id, bet_type_id);`. The parts are
// recorded as written: whether they name fields of this declaration is a
// question about the whole body, and belongs to the stage that has it.
func (p *parser) parseCompositeKey() *CompositeKey {
	kw := p.next()
	k := &CompositeKey{Span: kw.Span}

	name, ok := p.expectIdent()
	if !ok {
		p.recoverStmt()
		return k
	}
	k.Name = name
	k.Span = k.Span.To(name.Span)

	if _, ok := p.expect(KindAssign); !ok {
		p.recoverStmt()
		return k
	}
	tag, ok := p.expectInt()
	if !ok {
		p.recoverStmt()
		return k
	}
	k.Tag = tag
	k.Span = k.Span.To(tag.Span)
	p.checkTag(tag)

	if _, ok := p.expect(KindLParen); !ok {
		p.recoverStmt()
		return k
	}
	for {
		part, ok := p.expectIdent()
		if !ok {
			p.recoverStmt()
			return k
		}
		k.Parts = append(k.Parts, part)
		k.Span = k.Span.To(part.Span)
		if !p.at(KindComma) {
			break
		}
		p.next()
	}
	closing, ok := p.expect(KindRParen)
	if !ok {
		p.recoverStmt()
		return k
	}
	k.Span = k.Span.To(closing.Span)

	if k.Annotations, ok = p.parseAnnotations(); !ok {
		p.recoverStmt()
		return k
	}
	if semi, ok := p.expect(KindSemi); ok {
		k.Span = k.Span.To(semi.Span)
	}
	return k
}

// parseAlias reads `alias bet_type_id = id;`. There is no tag: an alias is a
// second name for a field that already has one, and naming no wire slot is the
// whole of what makes it free to add.
func (p *parser) parseAlias() *Alias {
	kw := p.next()
	a := &Alias{Span: kw.Span}

	name, ok := p.expectIdent()
	if !ok {
		p.recoverStmt()
		return a
	}
	a.Name = name
	a.Span = a.Span.To(name.Span)

	if _, ok := p.expect(KindAssign); !ok {
		p.recoverStmt()
		return a
	}
	src, ok := p.expectIdent()
	if !ok {
		p.recoverStmt()
		return a
	}
	a.Source = src
	a.Span = a.Span.To(src.Span)

	if a.Annotations, ok = p.parseAnnotations(); !ok {
		p.recoverStmt()
		return a
	}
	if semi, ok := p.expect(KindSemi); ok {
		a.Span = a.Span.To(semi.Span)
	}
	return a
}

// parseReserved reads `reserved 8..14, 16..23, 25;`, the list that retires tags
// where they were declared. The lexer makes `8..14` three tokens, so a range
// and a single differ by whether a `..` follows the first number.
func (p *parser) parseReserved() *Reserved {
	kw := p.next()
	r := &Reserved{Span: kw.Span}

	for {
		lo, ok := p.expectInt()
		if !ok {
			p.recoverStmt()
			return r
		}
		// A single tag is held as the range that begins and ends with it. The
		// two spellings retire exactly the same tags, and the span still says
		// which one was written.
		entry := ReservedRange{Lo: lo, Hi: lo, Span: lo.Span}
		if p.at(KindDotDot) {
			p.next()
			hi, ok := p.expectInt()
			if !ok {
				p.recoverStmt()
				return r
			}
			entry.Hi = hi
			entry.Span = entry.Span.To(hi.Span)
			if hi.Value < lo.Value {
				p.bag.Raise(diag.ReservedRangeReversed, entry.Span,
					"reserved range %d..%d ends before it begins and retires nothing", lo.Value, hi.Value)
			}
		}
		r.Ranges = append(r.Ranges, entry)
		r.Span = r.Span.To(entry.Span)

		if !p.at(KindComma) {
			break
		}
		p.next()
	}

	if semi, ok := p.expect(KindSemi); ok {
		r.Span = r.Span.To(semi.Span)
	}
	return r
}

// parseField reads one tagged member, `decimal(4)? live_limit_manual_single =
// 36;`. It consumes at least one token on every path, which is what keeps the
// body loop that calls it moving.
func (p *parser) parseField() *Field {
	if tok := p.peek(); tok.Kind != KindIdent {
		// Through raiseMemberStart like the other three, and measured never to
		// suppress anything there: this is reached only once parseMember has
		// consumed a leading `key`, and consuming is what clears the flag. That
		// is a fact about today's grammar and not a rule, so the question is
		// asked here too rather than the answer being hard-coded.
		p.raiseMemberStart(tok, "a field type")
		p.next()
		p.recoverStmt()
		return nil
	}

	typ, ok := p.parseTypeRef()
	if !ok {
		p.recoverStmt()
		return nil
	}
	p.rejectAnnotations("the member's tag")
	name, ok := p.expectIdent()
	if !ok {
		p.recoverStmt()
		return nil
	}
	f := &Field{Type: typ, Name: name, Span: typ.Span.To(name.Span)}

	p.rejectAnnotations("the member's tag")
	if _, ok := p.expect(KindAssign); !ok {
		p.recoverStmt()
		return f
	}
	tag, ok := p.expectInt()
	if !ok {
		p.recoverStmt()
		return f
	}
	f.Tag = tag
	f.Span = f.Span.To(tag.Span)
	p.checkTag(tag)

	if f.Annotations, ok = p.parseAnnotations(); !ok {
		p.recoverStmt()
		return f
	}

	if semi, ok := p.expect(KindSemi); ok {
		f.Span = f.Span.To(semi.Span)
	}
	return f
}

// checkKeyAllowed reports a `key` on a declaration that has none to give. A
// struct and a record are values: they are compared by what they hold, they are
// stored inside whatever holds them, and nothing can point at one. A key on
// either is a promise the language has no way to keep.
func (p *parser) checkKeyAllowed(keyword string, span source.Span) {
	if keyword != "table" {
		p.bag.Raise(diag.KeyNotAllowed, span,
			"only a `table` carries identity; a `%s` cannot declare a `key`", keyword)
	}
}

// checkTags runs the two checks a declaration can answer on its own: a tag
// written twice, and a tag the declaration's own `reserved` list has retired.
// Both need the whole body, so they run once it has been read rather than as
// each member arrives -- `reserved` is written above the members in section 1,
// but nothing says it has to be.
//
// Tag 0 is skipped. It has already been reported, and a member whose tag failed
// to parse carries a zero it never wrote; calling those duplicates of each
// other would bury the mistake that made them.
func (p *parser) checkTags(members []Member) {
	var reserved []ReservedRange
	for _, m := range members {
		if r, ok := m.(*Reserved); ok {
			reserved = append(reserved, r.Ranges...)
		}
	}

	seen := map[uint64]source.Span{}
	for _, m := range members {
		var tag IntLit
		switch m := m.(type) {
		case *Field:
			tag = m.Tag
		case *CompositeKey:
			// A composite key spends an ordinal, so it collides like any other
			// member -- which is the reason the spelling writes one down.
			tag = m.Tag
		default:
			continue
		}
		if tag.Value == 0 {
			continue
		}

		if first, dup := seen[tag.Value]; dup {
			p.bag.Raise(diag.DuplicateTag, tag.Span,
				"tag %d is already used at %s; a tag is a position in the MessagePack array and two members cannot hold one position",
				tag.Value, p.src.Position(first.Start))
			continue
		}
		seen[tag.Value] = tag.Span

		for _, r := range reserved {
			if r.Contains(tag.Value) {
				p.bag.Raise(diag.TagIsReserved, tag.Span,
					"tag %d is reserved at %s; a retired tag is never reused, because a reader built against the old schema would decode this field into the old one",
					tag.Value, p.src.Position(r.Span.Start))
				break
			}
		}
	}
}

// checkTag reports a tag outside the legal range. Every tagged member is held
// to it, the composite key included: it spends an ordinal exactly as a field
// does, which is the reason it writes one.
func (p *parser) checkTag(tag IntLit) {
	if tag.Value == 0 {
		p.bag.Raise(diag.TagOutOfRange, tag.Span,
			"field tag must be greater than 0; a MessagePack key is tag - 1, so tag 0 names no slot")
	}
}

// --- annotations -------------------------------------------------------------

// parseAnnotations reads the run of annotations at the current position, which
// may be empty. One bracket is one annotation and they stack by juxtaposition:
// there is no comma between them, because annotation contents already contain
// commas and a separator that competes with them buys nothing. Whitespace is
// insignificant, so a head carrying four wraps freely across lines.
//
// The order is the source's. protopc never sorts them and never drops a
// duplicate, because it does not know what either would mean to the backend
// reading them.
// An unterminated annotation ends the run and is reported false: everything
// after an unclosed `[` has already been read as its text, so the member it
// was written on is not there to be parsed and its caller recovers instead.
func (p *parser) parseAnnotations() ([]*Annotation, bool) {
	var anns []*Annotation
	for p.at(KindLBracket) {
		a, ok := p.parseAnnotation()
		anns = append(anns, a)
		if !ok {
			return anns, false
		}
	}
	return anns, true
}

// parseAnnotation reads one bracket. What it checks is the prefix and nothing
// else: a prefixed annotation is a backend's language and protopc is not
// entitled to an opinion about it, while an unprefixed one is protop's own,
// where the set is closed and an unknown name is a typo.
func (p *parser) parseAnnotation() (*Annotation, bool) {
	open := p.next()
	a := &Annotation{}

	switch tok := p.peek(); {
	case tok.Kind == KindIdent && p.peekAt(1).Kind == KindColon:
		prefix := p.identOf(p.next())
		p.next() // the colon
		a.Prefix = &prefix
		if !backends[prefix.Text] {
			p.bag.Raise(diag.UnknownAnnotationPrefix, prefix.Span,
				"unknown annotation prefix `%s`; protopc's backends are %s, and adding one means writing a backend rather than a keyword",
				prefix.Text, wordList(backends))
		}
	case tok.Kind != KindIdent:
		p.bag.Raise(diag.UnknownAnnotation, tok.Span, "expected an annotation, found %s", tok.Kind)
	default:
		if word := p.text(tok); !protopAnnotations[word] {
			p.bag.Raise(diag.UnknownAnnotation, tok.Span,
				"unknown annotation `%s`; protop's own annotations are %s, and an annotation for a backend carries its prefix, as in `[cs: %s]`",
				word, wordList(protopAnnotations), word)
		}
	}

	text, whole, ok := p.scanAnnotation(open)
	a.Text, a.TextSpan, a.Span = p.src.Slice(text), text, whole
	return a, ok
}

// scanAnnotation consumes an annotation's contents up to the `]` that closes
// open, returning the span of the contents and the span of the whole bracket.
//
// The contents are raw text, so the scan counts brackets rather than reading
// what is between them. It counts tokens rather than characters, which is what
// the complete lexer buys: a `]` inside a string literal is part of that
// literal's single token and cannot close the annotation, so `[cs: Foo("]")]`
// is one annotation rather than a broken one.
func (p *parser) scanAnnotation(open Token) (text, whole source.Span, ok bool) {
	start := p.peek().Span.Start
	end := start
	brackets, braces := 1, 0
	for {
		tok := p.peek()
		switch tok.Kind {
		case KindEOF:
			// A bracket left open was going to close somewhere past the end of
			// the file, and every token after it has already been read as
			// annotation text. Reporting at the `[` points at the one character
			// that has to be found.
			p.bag.Raise(diag.UnterminatedAnnotation, open.Span,
				"unterminated annotation; `[` is closed by the matching `]`")
			return source.Span{Start: start, End: end}, open.Span.To(tok.Span), false
		case KindLBracket:
			brackets++
		case KindRBracket:
			brackets--
			if brackets == 0 {
				p.next()
				return source.Span{Start: start, End: end}, open.Span.To(tok.Span), true
			}
		case KindLBrace:
			braces++
		case KindRBrace:
			// An annotation's contents balance their braces, so a `}` closing
			// nothing inside them is the brace of the body the annotation sits
			// in. Stopping short of it is what keeps one unclosed bracket from
			// swallowing the declaration around it.
			if braces == 0 {
				p.bag.Raise(diag.UnterminatedAnnotation, open.Span,
					"unterminated annotation; `[` is closed by the matching `]`")
				return source.Span{Start: start, End: end}, open.Span.To(tok.Span), false
			}
			braces--
		}
		end = tok.Span.End
		p.next()
	}
}

// rejectAnnotations reports an annotation written where the grammar has no slot
// for one, and discards it. An annotation follows what it describes and there
// is no before-slot, so the brackets are skipped and what follows them is read
// as though they had not been written -- which is what gets the rest of the
// line right and keeps one misplaced annotation to one diagnostic.
//
// The underline is the whole bracket and not the `[` that opens it. A message
// reading "write THIS ONE after the member's tag" has to say which one, and a
// caret under one character of seven does not -- while parseAnnotation, on the
// accepted path, has always underlined the whole. One construct does not get
// two spans depending on whether it was allowed.
//
// Which is why the scan runs before the report rather than after it: the whole
// bracket is not known until its `]` is found. The visible consequence is the
// order of two diagnostics about one `[`. An annotation that is both misplaced
// and unterminated now reports PROTOP0403 first, and that is the better order
// anyway -- the bracket that never closes is what has to be fixed before the
// question of where it belongs is worth asking, and it is also why the
// misplaced report underlines everything the scan swallowed.
func (p *parser) rejectAnnotations(after string) {
	for p.at(KindLBracket) {
		open := p.next()
		_, whole, ok := p.scanAnnotation(open)
		p.bag.Raise(diag.AnnotationNotAllowed, whole,
			"an annotation follows what it describes and there is no before-slot; write this one after %s", after)
		if !ok {
			return
		}
	}
}

// wordList renders a closed set the way a diagnostic wants to read it: sorted,
// quoted, and comma separated.
func wordList(set map[string]bool) string {
	words := make([]string, 0, len(set))
	for w := range set {
		words = append(words, "`"+w+"`")
	}
	slices.Sort(words)
	return strings.Join(words, ", ")
}

// parseTypeRef reads a type as written -- name, optional type arguments,
// optional precision, optional `?` -- and checks what can be checked with
// nothing resolved. It does not resolve anything: a type reference is a name
// until the checking stage says otherwise.
//
// Type arguments recurse into this same function, which is the whole of what
// `list<list<int64>>` costs.
func (p *parser) parseTypeRef() (TypeRef, bool) {
	name := p.identOf(p.next())
	t := TypeRef{Name: name, Span: name.Span}

	if p.at(KindLAngle) {
		args, closing, ok := p.parseTypeArgs()
		t.Args = args
		if !ok {
			return t, false
		}
		t.Span = t.Span.To(closing.Span)
	}

	if p.at(KindLParen) {
		p.next()
		prec, ok := p.expectInt()
		if !ok {
			return t, false
		}
		closing, ok := p.expect(KindRParen)
		if !ok {
			return t, false
		}
		t.Precision = &prec
		t.Span = t.Span.To(closing.Span)
	}
	// The `?` binds to the type, not to the field: presence is a property of
	// what is stored, and `decimal(4)?` is one type.
	if p.at(KindQuestion) {
		t.Optional = true
		t.Span = t.Span.To(p.next().Span)
	}

	p.checkTypeRef(t)
	return t, true
}

// parseTypeArgs reads `<T>` or `<K, V>`, the `<` already peeked at by the
// caller. It returns the arguments read, the token that closed them, and
// whether the list was closed at all -- a caller that gets false recovers,
// because everything after an unclosed `<` has been read as a type.
func (p *parser) parseTypeArgs() ([]TypeRef, Token, bool) {
	open := p.next()
	var args []TypeRef
	for {
		if p.peek().Kind != KindIdent {
			tok := p.peek()
			p.bag.Raise(diag.UnexpectedToken, tok.Span, "expected a type argument, found %s", tok.Kind)
			return args, open, false
		}
		arg, ok := p.parseTypeRef()
		args = append(args, arg)
		if !ok {
			return args, open, false
		}
		if !p.at(KindComma) {
			break
		}
		p.next()
	}
	closing, ok := p.expect(KindRAngle)
	return args, closing, ok
}

// checkTypeRef checks what a type can be checked for with nothing resolved: its
// precision, and its arity if it is one of protop's collections. A name outside
// both closed sets is a reference to a declared type -- `Venue? venue = 7;` --
// and whether that declaration exists is a question for the stage that knows
// what is in scope.
func (p *parser) checkTypeRef(t TypeRef) {
	switch {
	case t.Name.Text == scaledScalar && t.Precision == nil:
		p.bag.Raise(diag.PrecisionRequired, t.Name.Span,
			"`%s` requires a precision, as in `%s(4)`", scaledScalar, scaledScalar)
	case t.Name.Text != scaledScalar && t.Precision != nil:
		p.bag.Raise(diag.PrecisionNotAllowed, t.Precision.Span,
			"a precision is only allowed on `%s`, not on `%s`", scaledScalar, t.Name.Text)
	}

	// Both arity diagnostics point at the whole type rather than at the
	// arguments alone: what is wrong is the combination, and `list` written
	// bare has no arguments to underline.
	arity, isCollection := collectionTypes[t.Name.Text]
	switch {
	case !isCollection && len(t.Args) > 0:
		p.bag.Raise(diag.TypeArgumentsNotAllowed, t.Span,
			"`%s` takes no type arguments; protop's generic types are %s",
			t.Name.Text, wordList(collectionKeys()))
	case isCollection && len(t.Args) != arity:
		p.bag.Raise(diag.TypeArgumentCountWrong, t.Span,
			"`%s` takes %s and was given %d, as in `%s`",
			t.Name.Text, plural(arity, "type argument"), len(t.Args), collectionExample(t.Name.Text))
	}

	// A collection the spec defines and this milestone does not read is read in
	// full and then reported against, once per type written -- the difference
	// between "protopc is behind" and "you wrote this wrong".
	if notYetCollections[t.Name.Text] {
		p.bag.Raise(diag.NotYetImplemented, t.Name.Span, "`%s` is not yet implemented", t.Name.Text)
	}
}

// collectionKeys is collectionTypes as a set, for the diagnostic that lists it.
func collectionKeys() map[string]bool {
	set := make(map[string]bool, len(collectionTypes))
	for name := range collectionTypes {
		set[name] = true
	}
	return set
}

// collectionExample writes the shape a collection is meant to have, which is
// what a diagnostic about its arity is actually asking for.
func collectionExample(name string) string {
	switch collectionTypes[name] {
	case 1:
		return name + "<Odd>"
	case 2:
		return name + "<string, Odd>"
	}
	return name
}

// plural counts a noun the way a message reads it: "1 type argument", "2 type
// arguments".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

func (p *parser) parseEnum() *Enum {
	kw := p.next()
	e := &Enum{Span: kw.Span}

	name, ok := p.expectIdent()
	if !ok {
		p.recoverDecl()
		return e
	}
	e.Name = name
	e.Span = e.Span.To(name.Span)

	if _, ok := p.expect(KindLBrace); !ok {
		p.recoverDecl()
		return e
	}
	for !p.at(KindRBrace) && !p.at(KindEOF) {
		if m := p.parseEnumMember(); m != nil {
			e.Members = append(e.Members, m)
		}
	}
	if closing, ok := p.expect(KindRBrace); ok {
		e.Span = e.Span.To(closing.Span)
	}
	return e
}

// parseEnumMember reads `NOT_STARTED = 1;`. There is no lower bound on the
// number: an enum member's value is not a tag, and 0 is the idiomatic unknown
// arm.
func (p *parser) parseEnumMember() *EnumMember {
	tok := p.peek()
	if tok.Kind != KindIdent {
		p.raiseMemberStart(tok, "an enum member name")
		p.next()
		p.recoverStmt()
		return nil
	}
	name := p.identOf(p.next())
	m := &EnumMember{Name: name, Span: name.Span}

	if _, ok := p.expect(KindAssign); !ok {
		p.recoverStmt()
		return m
	}
	num, ok := p.expectInt()
	if !ok {
		p.recoverStmt()
		return m
	}
	m.Number = num
	m.Span = m.Span.To(num.Span)

	if semi, ok := p.expect(KindSemi); ok {
		m.Span = m.Span.To(semi.Span)
	}
	return m
}

// --- token access ------------------------------------------------------------

func (p *parser) peek() Token { return p.toks[p.i] }

func (p *parser) at(k Kind) bool { return p.toks[p.i].Kind == k }

// prev is the token most recently consumed, clamped to the first token so that
// a caller reading behind the start of the file needs no bounds check. It is
// what a diagnostic about something MISSING reports against: the gap opens at
// the end of the last thing that was actually written.
func (p *parser) prev() Token {
	if p.i == 0 {
		return p.toks[0]
	}
	return p.toks[p.i-1]
}

// peekAt looks n tokens ahead, clamped to the EOF token the lexer always emits
// so that a lookahead past the end of the file needs no bounds check.
func (p *parser) peekAt(n int) Token {
	if i := p.i + n; i < len(p.toks) {
		return p.toks[i]
	}
	return p.toks[len(p.toks)-1]
}

// next consumes and returns the current token. It never advances past EOF, so
// every peek is in bounds without a length check.
func (p *parser) next() Token {
	t := p.toks[p.i]
	if p.i < len(p.toks)-1 {
		p.i++
	}
	p.reported = false
	return t
}

func (p *parser) text(t Token) string { return p.src.Slice(t.Span) }

func (p *parser) identOf(t Token) Ident { return Ident{Text: p.text(t), Span: t.Span} }

// expect consumes the current token when it is of kind k. On a mismatch it
// reports and consumes nothing, leaving the caller to recover -- which is what
// keeps one misplaced token from being reported twice.
func (p *parser) expect(k Kind) (Token, bool) {
	if p.at(k) {
		return p.next(), true
	}
	tok := p.peek()
	p.bag.Raise(diag.UnexpectedToken, tok.Span, "expected %s, found %s", k, tok.Kind)
	p.reported = true
	return tok, false
}

// raiseMemberStart reports that a body member does not begin where one should,
// naming what that body was looking for -- unless the token it would name has
// already been reported on and nothing has moved past it since.
//
// Every failed expect sets that flag, but only one kind of failure can still be
// holding it by the time a body loop reports: a missing trailing `;`, which is
// reported in place and leaves its token where it is. Every other failure
// recovers, and recovery either consumes the token or stops on one that ends the
// body -- either way the loop never reaches here still holding the flag.
//
// The four member-start reports come through here and nothing else does. What
// the check is NOT is "a diagnostic already exists at this position". A position
// is an offset into a file the diagnostic does not name -- source.Span carries
// no file identity and a driver fills one bag from many files -- and two reports
// at one offset are not always one mistake anyway: a file that stops mid-member
// has the member and the enclosing body each reporting what ran out, at the same
// offset, under the same code, and both are worth saying.
func (p *parser) raiseMemberStart(tok Token, want string) {
	if p.reported {
		return
	}
	p.bag.Raise(diag.UnexpectedToken, tok.Span, "expected %s, found %s", want, tok.Kind)
}

func (p *parser) expectIdent() (Ident, bool) {
	tok, ok := p.expect(KindIdent)
	if !ok {
		return Ident{Span: tok.Span}, false
	}
	return p.identOf(tok), true
}

// expectString consumes a string literal, whose text is carried as written --
// quotes and all. See StringLit: nothing in the frontend decodes a string, and
// half-decoding one would be worse than not decoding it.
func (p *parser) expectString() (StringLit, bool) {
	tok, ok := p.expect(KindString)
	if !ok {
		return StringLit{Span: tok.Span}, false
	}
	return StringLit{Text: p.text(tok), Span: tok.Span}, true
}

func (p *parser) expectInt() (IntLit, bool) {
	tok, ok := p.expect(KindInt)
	if !ok {
		return IntLit{Span: tok.Span}, false
	}
	return IntLit{Value: tok.Int, Span: tok.Span}, true
}

// headSpan is where a missing file-level declaration is reported: the first
// token, or the start of an empty file.
func (p *parser) headSpan() source.Span {
	if len(p.toks) > 0 {
		return p.toks[0].Span
	}
	return source.SpanAt(0)
}

// --- recovery ----------------------------------------------------------------
//
// A missing trailing `;` is not one of these points. It is reported where it
// is and parsing continues, because the token that is not a semicolon is almost
// always the start of the next member -- skipping to the next `;` would report
// one mistake and swallow a member that was perfectly good.
//
// Both recovery points skip to the end of the construct that failed: the next
// `;`, or the `}` that closes a block the construct opened. The difference is
// what they do with a `}` they did not open -- recoverDecl consumes it, because
// at the top level it can only be the end of the declaration it is skipping;
// recoverStmt leaves it, because inside a body it belongs to the enclosing
// block and the loop that reads that body needs to see it.

func (p *parser) recoverDecl() {
	depth := 0
	for {
		switch p.peek().Kind {
		case KindEOF:
			return
		case KindSemi:
			if depth == 0 {
				p.next()
				return
			}
		case KindLBrace:
			depth++
		case KindRBrace:
			depth--
			if depth <= 0 {
				p.next()
				return
			}
		}
		p.next()
	}
}

func (p *parser) recoverStmt() {
	depth := 0
	for {
		switch p.peek().Kind {
		case KindEOF:
			return
		case KindSemi:
			if depth == 0 {
				p.next()
				return
			}
		case KindLBrace:
			depth++
		case KindRBrace:
			if depth == 0 {
				return
			}
			depth--
		}
		p.next()
	}
}
