package check

import (
	"slices"
	"strings"
	"unicode"

	"github.com/praguedb/praguec/internal/diag"
	"github.com/praguedb/praguec/internal/source"
	"github.com/praguedb/praguec/internal/syntax"
)

// Unit is one file as the checker takes it: the tree, and the text its spans
// index into. The two travel together because a diagnostic about a declaration
// in one file may have to quote a position in another, and a span alone cannot
// say which file it is an offset into.
type Unit struct {
	Source *source.File
	File   *syntax.File
}

// FileSet is the whole of what the checker knows about where files come from:
// a map from an import string to the file that import names.
//
// `import "prague.prague"` resolves against this map and against nothing else.
// Nothing about git, tags, caches, manifests or lock files is decided here, and
// nothing here should be read as deciding any of them: the checker is handed
// files, and who found them -- a path on disk, a module fetched by immutable
// tag, an editor's unsaved buffer -- is the driver's question and the
// distribution model's after that. An import that names no key in the map is a
// diagnostic, because the checker can say that the file it was promised is not
// here; it cannot say where it should have been.
//
// The key is the import string with its quotes stripped and nothing else done
// to it. The lexer decodes no escape -- it recognises `\"` only well enough to
// keep it from closing the literal -- so a path is matched as written, and a
// caller builds the map out of the same strings it read from the import lines.
type FileSet map[string]Unit

// Check turns a set of parsed files into one schema model, appending every
// diagnostic it finds to bag.
//
// It returns nil when bag already has errors. The parser's contract is that a
// tree is best-effort and bag.HasErrors is what says whether it means anything,
// and every stage after it inherits that: a file that lost its brace has
// declarations the user never wrote and members that fell out of the ones they
// did, so checking it would bury the one mistake under a hundred names that
// look undeclared. This is also what keeps `map` reported once, by the parser,
// as the not-yet-implemented construct it is.
func Check(files FileSet, bag *diag.Bag) *Schema {
	if bag.HasErrors() {
		return nil
	}
	c := &checker{bag: bag, set: files, byPath: make(map[string]*fileWork, len(files))}

	// Sorted, because a map has no order and a compiler that reported its
	// diagnostics in a different order on every run could not be diffed.
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	slices.Sort(paths)

	// The passes are over the whole set rather than over one file at a time,
	// which is what a cross-file reference costs: a type declared in an
	// imported file has to exist before any file resolves a name, and a view's
	// source has to be resolved before the view can be expanded against it.
	// Diagnostics come out in pass order, then file order, then source order.
	for _, path := range paths {
		c.declare(path, files[path])
	}
	for _, w := range c.work {
		c.resolveImports(w)
	}
	for _, w := range c.work {
		c.resolveDecls(w)
	}
	for _, w := range c.work {
		c.expandViews(w)
	}
	for _, w := range c.work {
		c.resolveRelations(w)
	}

	schema := &Schema{Files: make([]*File, len(c.work))}
	for i, w := range c.work {
		schema.Files[i] = w.file
	}
	return schema
}

type checker struct {
	bag    *diag.Bag
	set    FileSet
	work   []*fileWork
	byPath map[string]*fileWork
}

// fileWork is a model file beside the tree it came from. The AST is not kept on
// the model: a backend sees the model and never the tree, and a field pointing
// back at the syntax would be an invitation to read around the resolution.
type fileWork struct {
	file  *File
	ast   *syntax.File
	sink  *diag.Sink
	decls []*declWork
}

type declWork struct {
	decl    *Decl
	view    *syntax.View // nil unless the declaration is a view
	members []*memberWork
}

type memberWork struct {
	member *Member
	ast    syntax.Member      // nil for a view member
	view   *syntax.ViewMember // nil for every other member
}

// declare builds a file's model shell: its declarations by name and their
// members by name, with nothing resolved. It is a pass of its own because a
// name may be used above where it is declared, and in another file entirely.
func (c *checker) declare(path string, u Unit) {
	f := &File{Path: path, Source: u.Source, byName: map[string]*Decl{}}
	if u.File.Namespace != nil {
		f.Namespace = u.File.Namespace.String()
	}
	w := &fileWork{file: f, ast: u.File, sink: c.bag.In(u.Source)}
	c.work = append(c.work, w)
	c.byPath[path] = w

	for _, d := range u.File.Decls {
		dw := c.declareOne(w, d)
		if dw == nil {
			continue
		}
		w.decls = append(w.decls, dw)
		f.Decls = append(f.Decls, dw.decl)
	}
}

func (c *checker) declareOne(w *fileWork, d syntax.Decl) *declWork {
	dw := &declWork{decl: &Decl{File: w.file, byName: map[string]*Member{}}}
	switch d := d.(type) {
	case *syntax.Struct:
		dw.decl.Kind, dw.decl.Name, dw.decl.Annotations, dw.decl.Span = DeclStruct, d.Name.Text, d.Annotations, d.Span
		dw.decl.NameSpan = d.Name.Span
		c.declareMembers(w, dw, d.Members)
	case *syntax.Record:
		dw.decl.Kind, dw.decl.Name, dw.decl.Annotations, dw.decl.Span = DeclRecord, d.Name.Text, d.Annotations, d.Span
		dw.decl.NameSpan = d.Name.Span
		c.declareMembers(w, dw, d.Members)
	case *syntax.Table:
		dw.decl.Kind, dw.decl.Name, dw.decl.Annotations, dw.decl.Span = DeclTable, d.Name.Text, d.Annotations, d.Span
		dw.decl.NameSpan = d.Name.Span
		c.declareMembers(w, dw, d.Members)
	case *syntax.View:
		dw.decl.Kind, dw.decl.Name, dw.decl.Annotations, dw.decl.Span = DeclView, d.Name.Text, d.Annotations, d.Span
		dw.decl.NameSpan = d.Name.Span
		dw.view = d
		c.declareViewMembers(w, dw, d.Members)
	case *syntax.Enum:
		dw.decl.Kind, dw.decl.Name, dw.decl.Span = DeclEnum, d.Name.Text, d.Span
		dw.decl.NameSpan = d.Name.Span
		for _, m := range d.Members {
			dw.decl.EnumMembers = append(dw.decl.EnumMembers, &EnumMember{Name: m.Name.Text, Number: m.Number.Value, Span: m.Span})
		}
	default:
		return nil
	}

	// A declaration whose name failed to parse has none. It is in the tree
	// because the parser returns what it read, and a nameless entry in the
	// symbol table would collide with the next one.
	if dw.decl.Name == "" {
		return nil
	}
	// A declaration cannot take a name protop's own type system already holds.
	// There are no keyword token kinds, so `record list { ... }` parses like any
	// other declaration -- and then nothing can ever name it: a type written
	// `list` is read as the collection and a type written `string` as the
	// scalar, so the declaration has no second spelling that reaches it. The
	// name was never the schema's to take.
	//
	// Reported here rather than in the parser because it is a fact about a
	// file's names, and the parser reads one declaration at a time. It does not
	// return: the declaration is well formed, the collision is with the language
	// and not with anything else in the file, and dropping it would turn one
	// mistake into a second wave of them at every place the file's OTHER
	// declarations mention it. The set is read, never copied -- see
	// syntax.IsBuiltinTypeName.
	if syntax.IsBuiltinTypeName(dw.decl.Name) {
		w.sink.Raise(diag.BuiltinTypeDeclared, dw.decl.Span,
			"`%s` already names protop's built-in %s type; a type written `%s` is read as the built-in, so nothing can ever spell its way to this declaration",
			dw.decl.Name, builtinKindOf(dw.decl.Name), dw.decl.Name)
	}
	// The name and not the declaration, at the caret. What is duplicated is the
	// word `S`, not the three lines under it; underlining the whole of the
	// second declaration said that all of it was wrong, when renaming one word
	// fixes it.
	//
	// The position INSIDE the message stays at the first declaration's keyword,
	// which is TestAMessageQuotesADeclarationAtItsKeyword's rule: a message that
	// says where a declaration is says where it begins, keyword and all. The two
	// are different questions -- what to underline here, and where to send a
	// reader next -- and moving the caret is not a reason to move the other one.
	if first, dup := w.file.byName[dw.decl.Name]; dup {
		w.sink.Raise(diag.DuplicateDeclaration, dw.decl.NameSpan,
			"`%s` is already declared at %s; which of the two a reference means is not a question protop answers",
			dw.decl.Name, w.file.Source.Position(first.Span.Start))
		return nil
	}
	w.file.byName[dw.decl.Name] = dw.decl
	return dw
}

// builtinKindOf says which of protop's two closed type sets a name is in, so
// that a message can name the thing the declaration collided with. Derived from
// the sets themselves rather than from a word list here: a name added to either
// one is described correctly with no edit at this end.
func builtinKindOf(name string) string {
	if _, isCollection := syntax.CollectionArity(name); isCollection {
		return "generic"
	}
	return "scalar"
}

func (c *checker) declareMembers(w *fileWork, dw *declWork, members []syntax.Member) {
	for _, m := range members {
		mw := &memberWork{ast: m}
		switch m := m.(type) {
		case *syntax.Field:
			mw.member = &Member{Kind: MemberField, Name: m.Name.Text, Tag: m.Tag.Value, Annotations: m.Annotations, Span: m.Span}
			if m.Key {
				dw.decl.Key = mw.member
			}
		case *syntax.CompositeKey:
			mw.member = &Member{Kind: MemberCompositeKey, Name: m.Name.Text, Tag: m.Tag.Value, Annotations: m.Annotations, Span: m.Span}
			dw.decl.Key = mw.member
		case *syntax.Alias:
			mw.member = &Member{Kind: MemberAlias, Name: m.Name.Text, Annotations: m.Annotations, Span: m.Span}
		default:
			// `reserved` names nothing and holds nothing. It is a fact about
			// the declaration's tags, checked where the tags are, and there is
			// no member for a backend to emit.
			continue
		}
		c.addMember(w, dw, mw)
	}
}

func (c *checker) declareViewMembers(w *fileWork, dw *declWork, members []*syntax.ViewMember) {
	for _, m := range members {
		// The name the view knows the member by is the rename when there is
		// one: `event_id as id;` is `id` everywhere above this line, and
		// `event_id` only below it, in the source.
		name := m.Name.Text
		if m.As != nil {
			name = m.As.Text
		}
		c.addMember(w, dw, &memberWork{
			view:   m,
			member: &Member{Name: name, Annotations: m.Annotations, Span: m.Span},
		})
	}
}

func (c *checker) addMember(w *fileWork, dw *declWork, mw *memberWork) {
	if mw.member.Name == "" {
		return // the parser could not read the name; see declareOne
	}
	if first, dup := dw.decl.byName[mw.member.Name]; dup {
		w.sink.Raise(diag.DuplicateMember, mw.member.Span,
			"`%s` already names a member of `%s` at %s",
			mw.member.Name, dw.decl.Name, w.file.Source.Position(first.Span.Start))
		return
	}
	dw.decl.byName[mw.member.Name] = mw.member
	dw.decl.Members = append(dw.decl.Members, mw.member)
	dw.members = append(dw.members, mw)
}

// resolveImports gives a file its scope: its own declarations, and those of the
// files it imports. Nothing else, and not transitively -- a file that wants
// what its import imports writes the import itself, so that what a file can see
// is on the file rather than spread across the graph behind it.
func (c *checker) resolveImports(w *fileWork) {
	scope := make(map[string]*Decl, len(w.file.byName))
	for name, d := range w.file.byName {
		scope[name] = d
	}

	for _, im := range w.ast.Imports {
		path := ImportPath(im.Path)
		imported, ok := c.byPath[path]
		if !ok {
			w.sink.Raise(diag.ImportNotFound, im.Span,
				"no file named %q was given to the compiler; an import names a file and where that file comes from is the build's question, not the schema's",
				path)
			continue
		}
		w.file.Imports = append(w.file.Imports, imported.file)
		for name, d := range imported.file.byName {
			// A file's own declaration wins over an imported one, and the
			// first import to bring a name wins over a later one. Two files
			// claiming one name is a question about how a repository is laid
			// out, and this milestone does not answer it.
			if _, taken := scope[name]; !taken {
				scope[name] = d
			}
		}
	}
	w.file.scope = scope
}

// ImportPath is the key an import names in the file set: the literal's
// contents, quotes stripped and nothing else done to them.
//
// It is exported because the stage that builds a FileSet has to key it the same
// way the stage that reads one looks names up in it, and those are now two
// packages. One definition, in the package that owns FileSet.
func ImportPath(lit syntax.StringLit) string {
	return strings.TrimSuffix(strings.TrimPrefix(lit.Text, `"`), `"`)
}

// resolveDecls resolves everything a declaration can be resolved to without its
// views expanded: a view's source, and every member of everything else.
func (c *checker) resolveDecls(w *fileWork) {
	for _, dw := range w.decls {
		if dw.decl.Kind == DeclView {
			c.resolveViewSource(w, dw)
			continue
		}
		// Two passes over the body, and the order is the point. An alias takes
		// its target's type by value, so every field has to have one before any
		// alias is resolved -- otherwise `alias bet_type_id = id;` written
		// *above* `int64 id = 3;` silently takes the zero type and emits a
		// member with no type at all. Section 1 happens to write the alias
		// below its target and the corpus writes it above, which is how this
		// was found: corpus/prague/payloads/BPBetTypeCacheItem.cs opens with
		// its alias.
		//
		// A composite key does not need the second pass -- its parts are
		// members rather than copies of them, so a part resolved before its own
		// type is filled in still sees the type later -- but it is in it
		// anyway, because "everything that reads another member runs after
		// everything that defines one" is a rule, and "aliases second, keys
		// wherever" is a coincidence.
		for _, mw := range dw.members {
			if m, ok := mw.ast.(*syntax.Field); ok {
				mw.member.Type = c.resolveType(w, m.Type)
			}
		}
		for _, mw := range dw.members {
			switch m := mw.ast.(type) {
			case *syntax.CompositeKey:
				c.resolveKeyParts(w, dw, mw, m)
			case *syntax.Alias:
				c.resolveAlias(w, dw, mw, m)
			}
		}
	}
}

// resolveType resolves a type as written to a scalar, a declared type, or a
// collection over resolved arguments. Which of the three it is comes from the
// name and from what is in scope, and nothing else: the parser has already held
// the type to its arity and to whether it may carry a precision.
func (c *checker) resolveType(w *fileWork, t syntax.TypeRef) Type {
	out := Type{Name: t.Name.Text, Optional: t.Optional, Span: t.Span}
	if t.Precision != nil {
		out.Precision = t.Precision.Value
	}

	if _, isCollection := syntax.CollectionArity(t.Name.Text); isCollection {
		out.Kind = TypeCollection
		for _, arg := range t.Args {
			out.Args = append(out.Args, c.resolveType(w, arg))
		}
		return out
	}
	if syntax.IsScalar(t.Name.Text) {
		out.Kind = TypeScalar
		return out
	}
	if d, ok := w.file.scope[t.Name.Text]; ok {
		out.Kind, out.Decl = TypeDecl, d
		return out
	}
	w.sink.Raise(diag.UnknownType, t.Name.Span,
		"unknown type `%s`; a file sees its own declarations and those of the files it imports, and nothing else",
		t.Name.Text)
	return out
}

// resolveKeyParts matches a composite key's parts against the members beside
// it. A part may name an alias: the corpus's own key is `key key = 1 (event_id,
// bet_type_id)` over a table whose second part is `alias bet_type_id = id;`, so
// requiring a field here would reject section 1.
func (c *checker) resolveKeyParts(w *fileWork, dw *declWork, mw *memberWork, k *syntax.CompositeKey) {
	for _, part := range k.Parts {
		target := dw.decl.Member(part.Text)
		if target == nil || target == mw.member {
			w.sink.Raise(diag.CompositeKeyPartNotFound, part.Span,
				"`%s` names no member of `%s`; a composite key is a name over members of the declaration it belongs to",
				part.Text, dw.decl.Name)
			continue
		}
		mw.member.Parts = append(mw.member.Parts, target)
	}
}

// resolveAlias matches `alias bet_type_id = id;` against the member it passes
// through to. The type is the target's when the target has one; an alias of an
// alias, or of a composite key, leaves it unresolved rather than following a
// chain the language says nothing about and the corpus does not write.
func (c *checker) resolveAlias(w *fileWork, dw *declWork, mw *memberWork, a *syntax.Alias) {
	target := dw.decl.Member(a.Source.Text)
	if target == nil || target == mw.member {
		w.sink.Raise(diag.AliasTargetNotFound, a.Source.Span,
			"`%s` names no other member of `%s`; an alias is a get and set passthrough and needs something to pass through to",
			a.Source.Text, dw.decl.Name)
		return
	}
	mw.member.Aliased = target
	if target.Kind == MemberField {
		mw.member.Type = target.Type
	}
}

// resolveViewSource resolves `view V of T`. T is a table or a record: a view is
// a subset of members at their source's tags, so its source is something that
// has tags to lend. A struct has members and no identity to inherit, an enum
// has no members at all, and a view is already pinned to somewhere else's tags
// -- the narrowing in `SlimOdd odds;` is how a view reaches another view.
func (c *checker) resolveViewSource(w *fileWork, dw *declWork) {
	name := dw.view.Source
	if name.Text == "" {
		return // the parser reported the view as sourceless, PROTOP0203
	}
	src, ok := w.file.scope[name.Text]
	if !ok {
		w.sink.Raise(diag.ViewSourceNotFound, name.Span,
			"unknown view source `%s`; a file sees its own declarations and those of the files it imports, and nothing else",
			name.Text)
		return
	}
	if src.Kind != DeclTable && src.Kind != DeclRecord {
		w.sink.Raise(diag.ViewSourceNotSelectable, name.Span,
			"`%s` is a %s, and a view is a subset of a table or a record; there are no tags here for a view to be pinned to",
			name.Text, src.Kind)
		return
	}
	dw.decl.Of = src
}

// expandViews fills in what a view's members take from their source -- tag,
// type and presence -- and checks that each of them names something the source
// declares. The view records what it was given; it never has a tag, a type or a
// presence of its own, which is what keeps N views from drifting apart from the
// one table they are subsets of.
func (c *checker) expandViews(w *fileWork) {
	for _, dw := range w.decls {
		if dw.decl.Kind != DeclView || dw.decl.Of == nil {
			continue
		}
		for _, mw := range dw.members {
			c.expandViewMember(w, dw, mw)
		}
		c.checkViewTags(w, dw)
	}
}

func (c *checker) expandViewMember(w *fileWork, dw *declWork, mw *memberWork) {
	src := dw.decl.Of
	from := src.Member(mw.view.Name.Text)
	if from == nil {
		// The diagnostic the design exists to produce. A view is a second copy
		// of a field list held to the first one at compile time, and this is
		// the moment that pays: the shared repo renamed or retired the member,
		// and the repo that drifted is the one that fails to build.
		w.sink.Raise(diag.NoSuchMember, mw.view.Name.Span,
			"`%s` names no member of `%s`, declared at %s; a view selects members its source declares, so a rename or a removal there is a compile error here rather than a field that decodes to nil",
			mw.view.Name.Text, src.Name, src.File.Source.Position(src.Span.Start))
		return
	}

	mw.member.From = from
	mw.member.Kind = from.Kind
	mw.member.Tag = from.Tag
	mw.member.Type = from.Type
	mw.member.Parts = from.Parts
	mw.member.Aliased = from.Aliased

	// A view's identity is its source's, section 4. It has one when it selects
	// the member the source's key is, under whatever name the view gives it.
	if from == src.Key {
		dw.decl.Key = mw.member
	}

	if mw.view.Narrow != nil {
		c.narrow(w, mw)
	}
}

// narrow resolves the leading name of `SlimOdd odds;`. It asks for the member's
// type to be narrowed to another view, and the view has to be a view of what
// the member already holds -- a narrowing that points elsewhere would give the
// member a set of tags belonging to some other declaration, which is the
// mistake the construct exists to catch. The tags stay the source's either way:
// the member keeps the tag its own source gave it, and the narrowed view keeps
// the tags of the declaration it is a view of.
func (c *checker) narrow(w *fileWork, mw *memberWork) {
	name := *mw.view.Narrow
	view, ok := w.file.scope[name.Text]
	if !ok {
		w.sink.Raise(diag.UnknownType, name.Span,
			"unknown type `%s`; a file sees its own declarations and those of the files it imports, and nothing else",
			name.Text)
		return
	}
	if view.Kind != DeclView {
		w.sink.Raise(diag.NarrowingNotAView, name.Span,
			"`%s` is a %s, and a view member narrows to another view; a member that keeps the type its source gives it is written as its name alone",
			name.Text, view.Kind)
		return
	}

	elem := mw.member.Type.Element()
	if elem.Kind != TypeDecl || elem.Decl != view.Of {
		w.sink.Raise(diag.NarrowingSourceMismatch, name.Span,
			"`%s` is a view of `%s` and `%s` holds `%s`; a narrowing pins the member to a subset of what it already holds, and a view of anything else carries a different declaration's tags",
			name.Text, sourceName(view), mw.member.Name, elem.Name)
		return
	}
	mw.member.Type = withElement(mw.member.Type, Type{Kind: TypeDecl, Name: name.Text, Decl: view})
}

// sourceName is what a view is a view of, for a message about one whose own
// source did not resolve.
func sourceName(view *Decl) string {
	if view.Of == nil {
		return "nothing"
	}
	return view.Of.Name
}

// withElement rebuilds a type with its element replaced, which is how a
// narrowing reaches through `list<Odd>` to the `Odd` it holds. Presence is the
// outer type's throughout: `list<Odd>?` narrowed is `list<SlimOdd>?`, and
// `Odd?` narrowed is `SlimOdd?`.
func withElement(t, elem Type) Type {
	if t.Kind == TypeCollection && len(t.Args) > 0 {
		args := slices.Clone(t.Args)
		args[len(args)-1] = withElement(args[len(args)-1], elem)
		t.Args = args
		return t
	}
	elem.Optional = t.Optional
	elem.Span = t.Span
	return elem
}

// checkViewTags reports two members of one view holding one tag. A view cannot
// write a tag, so the only way to collide is to select one source member twice
// -- `id as a; id as b;` -- and the collision is as real as any other: two
// members cannot hold one position in the array.
func (c *checker) checkViewTags(w *fileWork, dw *declWork) {
	seen := map[uint64]*Member{}
	for _, m := range dw.decl.Members {
		if m.Tag == 0 {
			continue
		}
		if first, dup := seen[m.Tag]; dup {
			w.sink.Raise(diag.DuplicateTag, m.Span,
				"tag %d is already used at %s; `%s` and `%s` select the same member of `%s`, and a tag is a position in the MessagePack array that two members cannot hold",
				m.Tag, w.file.Source.Position(first.Span.Start), first.Name, m.Name, dw.decl.Of.Name)
			continue
		}
		seen[m.Tag] = m
	}
}

// resolveRelations reads protop's own `references` annotation, which the parser
// carries as raw text: it checks an annotation's position and its prefix and
// nothing else, because the contents of a prefixed annotation are a backend's
// language. protop's own set is closed, and this is the stage that can look a
// name up.
//
// It runs after the views are expanded, so that a relation written on a view
// member is derived against the key the view inherited.
func (c *checker) resolveRelations(w *fileWork) {
	for _, dw := range w.decls {
		for _, m := range dw.decl.Members {
			for _, a := range m.Annotations {
				c.resolveRelation(w, dw, m, a)
			}
		}
	}
}

func (c *checker) resolveRelation(w *fileWork, dw *declWork, m *Member, a *syntax.Annotation) {
	if a.Prefix != nil || !isRelation(a) {
		return // a backend's annotation, or one the parser already rejected
	}
	target, at, ok := relationTarget(a)
	if !ok {
		// The whole bracket here, and only here. What is wrong is the shape of
		// the annotation rather than anything inside it -- there is no target
		// to point at, which is the complaint -- so there is nothing narrower
		// that would be true.
		w.sink.Raise(diag.MalformedRelation, a.Span,
			"`%s` names the table a member points at, as in `[%s(SportGame)]`",
			syntax.RelationAnnotation, syntax.RelationAnnotation)
		return
	}

	decl, found := w.file.scope[target]
	if !found {
		w.sink.Raise(diag.RelationTargetNotFound, at,
			"unknown relation target `%s`; a file sees its own declarations and those of the files it imports, and nothing else",
			target)
		return
	}
	if decl.Kind != DeclTable {
		w.sink.Raise(diag.RelationTargetNotATable, at,
			"`%s` is a %s and cannot be the target of a relation; a relation resolves against a key, and only a `table` has one",
			target, decl.Kind)
		return
	}
	m.Relations = append(m.Relations, &Relation{
		Target:      decl,
		Cardinality: cardinality(dw.decl, m),
		Span:        a.Span,
	})
}

// cardinality derives how many of the declaring type relate to one of the
// target, by comparing the foreign key against the declaring type's own key.
// Section 6's table, in the order it is written:
//
//	the FK is the whole key       one-to-one, because the key enforces it
//	the FK is part of a composite many-to-one, the rest of the composite
//	                              distinguishing the many
//	the FK is in no key           many-to-one, unconstrained
//
// Sound and not complete. Every one-to-one it reports is real; a many-to-one
// may be one-to-one in truth with nothing in the schema saying so, and the rule
// declines to promise rather than asserting a uniqueness nothing checks.
func cardinality(d *Decl, fk *Member) Cardinality {
	key := d.Key
	switch {
	case key == nil:
		return ManyToOne
	case key.Kind != MemberCompositeKey:
		if key == fk {
			return OneToOne
		}
		return ManyToOne
	case len(key.Parts) == 1 && isPart(key.Parts[0], fk):
		// A composite of one part is the whole key spelled the long way.
		return OneToOne
	default:
		return ManyToOne
	}
}

// isPart reports whether a key part is the foreign key, following an alias to
// what it passes through to -- the corpus's own key names one.
func isPart(part, fk *Member) bool {
	return part == fk || (part.Kind == MemberAlias && part.Aliased == fk)
}

// isRelation reports whether an unprefixed annotation is protop's `references`.
// Anything else unprefixed is PROTOP0402 at the parser, where the set is
// checked; this only has to tell the relation apart from the rest of a set that
// will grow.
func isRelation(a *syntax.Annotation) bool {
	word, _ := splitWord(strings.TrimSpace(a.Text))
	return word == syntax.RelationAnnotation
}

// relationTarget reads the T of `references(T)` and says where it was written.
//
// The span is computed from an offset into the annotation's text rather than
// taken off a token, because the frontend reads an annotation's contents no
// further than the prefix and there are no tokens in here to take one off. That
// is sound and not a guess: syntax.Annotation.Text is the slice of exactly
// TextSpan, so an offset into the one is an offset into the other, and the two
// cannot drift apart without the parser changing both.
//
// Which is what lets PROTOP0317 and PROTOP0318 underline the name they are
// about. PROTOP0404 still underlines the whole bracket, and rightly: there the
// annotation has no target to point at -- that is the mistake.
func relationTarget(a *syntax.Annotation) (string, source.Span, bool) {
	text := a.Text
	i := skipSpace(text, 0)
	_, i = wordAt(text, i) // `references` itself; isRelation matched it already
	i = skipSpace(text, i)
	if i == len(text) || text[i] != '(' {
		return "", source.Span{}, false
	}
	i = skipSpace(text, i+1)
	name, end := wordAt(text, i)
	if name == "" || strings.TrimSpace(text[end:]) != ")" {
		return "", source.Span{}, false
	}
	return name, source.Span{
		Start: a.TextSpan.Start + source.Pos(i),
		End:   a.TextSpan.Start + source.Pos(end),
	}, true
}

// splitWord takes the leading identifier off s and returns it with the rest.
func splitWord(s string) (word, rest string) {
	_, end := wordAt(s, 0)
	return s[:end], s[end:]
}

// wordAt reads the identifier starting at i and returns it with the offset one
// past its last byte. An empty word means there was no identifier there.
func wordAt(s string, i int) (word string, end int) {
	end = i
	for end < len(s) && isIdentByte(s[end]) {
		end++
	}
	return s[i:end], end
}

// skipSpace returns the offset of the first non-space byte at or after i. It
// counts the same runes strings.TrimSpace does, so that a span computed by
// walking a string agrees with a check written by trimming one.
func skipSpace(s string, i int) int {
	rest := s[i:]
	return i + len(rest) - len(strings.TrimLeftFunc(rest, unicode.IsSpace))
}

func isIdentByte(b byte) bool {
	return b == '_' || ('a' <= b && b <= 'z') || ('A' <= b && b <= 'Z') || ('0' <= b && b <= '9')
}
