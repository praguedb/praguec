// Package check turns the parsed files of a program into one schema model.
//
// The frontend produces one AST per file, faithful to what was written and
// resolving nothing: a type is a name, a view's source is a name, and whether
// either names anything is a question about a whole program rather than about
// one file. This package answers those questions once, and every backend sees
// the answer instead of the tree.
//
// The model inverts the AST's shape deliberately. The AST makes its
// declarations four unrelated types and has every reader type-switch, because a
// parser writes what the grammar says and the four bodies have little in
// common. A backend asks the opposite kind of question -- is this a table, what
// is this member's tag, what does this type resolve to -- so here a declaration
// is one struct with a kind, and the kind is a field rather than a Go type.
//
// The conventions are the frontend's, unchanged: diagnostics are collected and
// never thrown, codes are stable, positions come from spans, and a result is
// partial rather than absent. An unresolved type is a type whose kind says so,
// not a nil that every caller has to guard.
package check

import (
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/source"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/syntax"
)

// Schema is every file of one program, resolved.
//
// Files are in the order of the import strings they are known by, sorted, and
// not in the order the caller's map happened to iterate. A compiler that
// reported its diagnostics in a different order on every run would be unusable
// in a build log, and the map the checker is handed has no order at all.
type Schema struct {
	Files []*File
}

// File returns the file known by path, or nil.
func (s *Schema) File(path string) *File {
	for _, f := range s.Files {
		if f.Path == path {
			return f
		}
	}
	return nil
}

// File is one resolved source file.
type File struct {
	// Path is the import string this file is known by within the set -- the
	// key the caller supplied it under, and what another file writes to import
	// it.
	Path string

	// Source is the text spans in this file index into. Every diagnostic about
	// this file is rendered against it, and a message that quotes a position in
	// another file computes it from that file's own.
	Source *source.File

	// Namespace is the file's `namespace A.B.C;`, dotted. Every file has one:
	// a file without is PROTOP0104 at the parser, and the checker does not run
	// on a file that failed to parse.
	Namespace string

	// Imports are the files this one imports, in source order, with the ones
	// that resolved to nothing left out. What a file can see is its own
	// declarations and these, and nothing else.
	Imports []*File

	// Decls are this file's declarations in source order.
	Decls []*Decl

	byName map[string]*Decl
	scope  map[string]*Decl
}

// Decl returns the declaration this file declares under name, or nil. It does
// not look through imports: a file's own declarations are what it owns, and
// what it can *see* is a different question, asked at the point a name is
// resolved.
func (f *File) Decl(name string) *Decl { return f.byName[name] }

// DeclKind is which of protop's five declaration keywords a declaration was
// written with. Section 4 gives each of them a different identity, a different
// emission and a different answer to whether it can be referenced, and the
// model carries the keyword so that a backend can ask.
type DeclKind int

const (
	// DeclStruct is `struct Venue { ... }`: value semantics, no identity.
	DeclStruct DeclKind = iota
	// DeclRecord is `record StaticDataRef { ... }`: no identity either, and a
	// C# class rather than a C# struct.
	DeclRecord
	// DeclTable is `table SportGame { ... }`: the one declaration with a key,
	// and so the only one a relation can name.
	DeclTable
	// DeclView is `view SlimSportGame of SportGame { ... }`: a field subset of
	// one source, at that source's tags.
	DeclView
	// DeclEnum is `enum EventStatus { ... }`.
	DeclEnum
)

func (k DeclKind) String() string {
	switch k {
	case DeclStruct:
		return "struct"
	case DeclRecord:
		return "record"
	case DeclTable:
		return "table"
	case DeclView:
		return "view"
	case DeclEnum:
		return "enum"
	}
	return "declaration"
}

// Decl is one resolved declaration.
type Decl struct {
	Kind DeclKind
	Name string

	// File is where the declaration was written, which is what lets a
	// diagnostic in one file quote a position in another.
	File *File

	// Members are the declaration's members in source order. A view's are
	// expanded: one entry per line of the view body, each carrying the tag,
	// type and presence of the source member it selects.
	Members []*Member

	// Key is the member that carries the declaration's identity, or nil. A
	// table's is the `key` it declares; a view's is whichever member it
	// inherited from its source's key, because section 4 says a view's identity
	// is the source's -- and nil when the view does not select it, which is a
	// view that cannot be cached by key and is not this milestone's to
	// diagnose.
	Key *Member

	// Of is the declaration a view is a view of, and nil on everything else.
	Of *Decl

	// EnumMembers are an enum's arms in source order, and nil on everything
	// else. An arm's number is a value on the wire rather than a position in an
	// array, which is why 0 is legal here and is not a tag.
	EnumMembers []*EnumMember

	// Annotations are carried through verbatim, in source order. protopc does
	// not know what a backend's annotation means and is not entitled to an
	// opinion; the backend that reads them is the one that checks them.
	Annotations []*syntax.Annotation

	Span   source.Span
	byName map[string]*Member
}

// Member returns the member this declaration holds under name, or nil. For a
// view the name is the view's -- `event_id as id;` is looked up as `id`,
// because that is what the view calls it and the rename is the view's alone.
func (d *Decl) Member(name string) *Member { return d.byName[name] }

// EnumMember is one arm of an enum.
type EnumMember struct {
	Name   string
	Number uint64
	Span   source.Span
}

// MemberKind is which shape of member a line of a body was.
type MemberKind int

const (
	// MemberField is a tagged member, `decimal(4)? live_limit_manual_single = 36;`.
	MemberField MemberKind = iota
	// MemberCompositeKey is `key key = 1 (event_id, bet_type_id);`: a name over
	// members that keep their own ordinals, which spends an ordinal of its own.
	MemberCompositeKey
	// MemberAlias is `alias bet_type_id = id;`: get and set passthrough, no
	// wire slot.
	MemberAlias
)

func (k MemberKind) String() string {
	switch k {
	case MemberCompositeKey:
		return "key"
	case MemberAlias:
		return "alias"
	}
	return "field"
}

// Member is one resolved member of a declaration.
type Member struct {
	Kind MemberKind

	// Name is what the declaration holding this member calls it, which for a
	// view member is the rename when there is one.
	Name string

	// Type is resolved. A composite key has none: it is a name over other
	// members rather than a thing with a type of its own, and its kind is where
	// a backend learns that.
	Type Type

	// Tag is the member's position in the MessagePack array, or 0 for a member
	// that has no wire slot -- an alias, and a view member that selects one. A
	// tag is 1-based because a MessagePack key is tag - 1.
	//
	// A view member's tag is its source's. The view never has one of its own:
	// only the source extends the tag space, which is section 5's whole
	// argument and the reason a tag written in a view body is PROTOP0308.
	Tag uint64

	// Parts are the members a composite key names, in the order it names them,
	// and nil on every other kind. They are members rather than names because
	// the corpus's own composite key -- `key key = 1 (event_id, bet_type_id)`
	// -- names an alias in its second part, and a backend reading the tuple
	// needs to arrive at the field the alias passes through to.
	Parts []*Member

	// Aliased is the member an alias passes through to, and nil on every other
	// kind.
	Aliased *Member

	// Relations are what the member's `[references(T)]` annotations resolved
	// to, with their cardinality derived. A member carries more than one only
	// if it was written more than once.
	Relations []*Relation

	// From is the source member a view member selects, and nil outside a view.
	// It is what a backend follows to answer a question the view does not hold
	// the answer to, and what makes the view's tag traceable to the declaration
	// that owns it.
	From *Member

	// Annotations are the member's own, verbatim and in source order. A view
	// member's are its own; the ones it inherits are on From, because section 6
	// has a view inherit the relations of the members it lists.
	Annotations []*syntax.Annotation

	Span source.Span
}

// HasTag reports whether the member occupies a slot on the wire. An alias does
// not, and neither does a view member that selects one.
func (m *Member) HasTag() bool { return m.Tag != 0 }

// TypeKind is what a type reference resolved to.
type TypeKind int

const (
	// TypeUnresolved is a type the checker could not resolve. It is a kind
	// rather than an absence so that a partial model is still walkable: a
	// diagnostic has already been raised, and bag.HasErrors is what stops a
	// backend from reading the model at all.
	TypeUnresolved TypeKind = iota
	// TypeScalar is one of protop's scalar keywords.
	TypeScalar
	// TypeDecl is a reference to a declaration in scope.
	TypeDecl
	// TypeCollection is `list<T>` or `map<K, V>` over resolved arguments.
	TypeCollection
)

// Type is a resolved type: a scalar, a declared type, or a collection over
// resolved arguments.
//
// Presence is a property of the type rather than of the field, which is what
// lets it compose: `list<Odd>?` is Optional on the outer type and `list<Odd?>`
// on the inner one.
type Type struct {
	Kind TypeKind

	// Name is the type as written -- the scalar keyword, the collection
	// keyword, or the declaration's name. It is kept on every kind so that a
	// message can quote what the user typed without a type switch.
	Name string

	// Decl is the declaration a TypeDecl resolves to, and nil otherwise.
	Decl *Decl

	// Args are a collection's arguments, resolved: one for a list, two for a
	// map. Arity is the parser's check, PROTOP0306.
	Args []Type

	// Precision is the N of `decimal(N)` and 0 on everything else. A decimal
	// without one is PROTOP0302 at the parser, so a resolved decimal always
	// carries a precision.
	Precision uint64

	// Optional is the trailing `?`.
	Optional bool

	Span source.Span
}

// Element is the type a narrowing applies to: the type itself, or what a
// collection holds. `SlimOdd odds;` narrows a `list<Odd>` because what is
// narrowed is what the collection holds -- section 5 gives that member tag 26,
// and tag 26 in the corpus is `list<Odd> odds`.
//
// A map's element is its value. Nothing reaches that yet: `map` is
// PROTOP0900 at the parser, so a file containing one never gets here.
func (t Type) Element() Type {
	if t.Kind == TypeCollection && len(t.Args) > 0 {
		return t.Args[len(t.Args)-1].Element()
	}
	return t
}

// Cardinality is how many of the declaring type relate to one of the target,
// derived from the key rather than declared. Section 6: a declared cardinality
// asserts a uniqueness nothing checks, and an unchecked assertion is worse than
// a missing one because code will trust it.
type Cardinality int

const (
	// ManyToOne is the answer whenever the key does not force the other one:
	// the foreign key is part of a composite, or in no key at all. It may be
	// one-to-one in truth with nothing in the schema saying so, which is what
	// "sound, not complete" means -- the rule declines to promise.
	ManyToOne Cardinality = iota
	// OneToOne is a foreign key that is the whole of the declaring type's key,
	// so the key itself enforces that there is exactly one.
	OneToOne
)

func (c Cardinality) String() string {
	if c == OneToOne {
		return "one-to-one"
	}
	return "many-to-one"
}

// Relation is one `[references(T)]` resolved.
type Relation struct {
	// Target is the table the relation points at. A record has no identity and
	// cannot be one, which is PROTOP0318.
	Target *Decl

	// Cardinality is derived from the declaring type's key, never declared.
	Cardinality Cardinality

	// Span is the annotation the relation was written as, whole. The annotation
	// is what a message about it has to underline: its contents are raw text to
	// the frontend, and it carries no span of its own for the name inside it.
	Span source.Span
}
