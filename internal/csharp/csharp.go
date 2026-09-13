// Package csharp is protop's C# backend: a checked schema becomes C# source
// text.
//
// The package is named for the annotation prefix rather than for what it does.
// Section 3 says the legal prefix set is exactly the backends protopc has, so
// `[cs: X]` and this package are one fact spelled twice; naming the package
// `emit` or `gen` would let the two drift, and adding a `go:` prefix means
// adding a package next to this one rather than a word to a list.
//
// What this package does not decide is where the text goes. A file name, a
// directory, a header comment and a `.g.cs` suffix are all
// nanov/protobuf-to-messagepack-sharp#25, and #25 is human-in-the-loop because
// the downstream generator silently skips any file that ends `.g.cs` or carries
// `auto-generated` in its first 500 bytes. So Emit returns text keyed by the
// declaration it came from, and nothing here writes a file or names one.
//
// The frontend's conventions are inherited whole. The model is read and never
// mutated, a backend runs only on a schema whose bag has no errors, and
// annotation contents are copied and never inspected.
package csharp

import (
	"slices"
	"strconv"
	"strings"

	"github.com/nanov/protobuf-to-messagepack-sharp/internal/check"
	"github.com/nanov/protobuf-to-messagepack-sharp/internal/syntax"
)

// Unit is one declaration's emission: the C# text, and enough about where it
// came from to let a driver decide what to call it.
//
// Namespace and TypeName are carried separately rather than left for a caller
// to re-derive from the model, because the two of them are what a file name is
// built out of and #25 is the ticket that gets to build it.
type Unit struct {
	Decl      *check.Decl
	Namespace string
	TypeName  string
	Source    string
}

// Emit turns a checked schema into one Unit per declaration, in the schema's
// own order -- file order, then source order within a file.
//
// Every declaration emits. `reserved` does not appear here because it retires
// tags rather than declaring a member and the model does not carry it at all,
// which is the mapping table's "emits nothing" row arriving as an absence
// rather than as a branch.
func Emit(s *check.Schema) []Unit {
	var units []Unit
	for _, f := range s.Files {
		for _, d := range f.Decls {
			units = append(units, Unit{
				Decl:      d,
				Namespace: f.Namespace,
				TypeName:  d.Name,
				Source:    EmitDecl(d),
			})
		}
	}
	return units
}

// EmitDecl renders one declaration as a complete C# compilation unit: usings,
// file-scoped namespace, and the type.
func EmitDecl(d *check.Decl) string {
	var b strings.Builder
	us := usings(d)
	for _, u := range us {
		b.WriteString("using ")
		b.WriteString(u)
		b.WriteString(";\n")
	}
	if len(us) > 0 {
		b.WriteString("\n")
	}
	b.WriteString("namespace ")
	b.WriteString(d.File.Namespace)
	b.WriteString(";\n\n")
	if d.Kind == check.DeclEnum {
		writeEnum(&b, d)
	} else {
		writeType(&b, d)
	}
	return b.String()
}

// messagePackNamespace is the one namespace protopc knows the name of. Every
// non-enum declaration carries `[MessagePackObject]` and every wire member
// carries `[Key(N)]`, so the using follows from the emission rather than from
// anything the schema says.
const messagePackNamespace = "MessagePack"

// usings is `using MessagePack;` plus the namespace of every declaration this
// one names, minus its own, sorted ordinally as the corpus sorts them.
//
// It is deliberately derived from types alone. protopc never inspects an
// annotation's contents (section 3), so the namespace `[cs: DataCache(...)]`
// needs is not a thing this function could know: an annotation that needs a
// using is written fully qualified, or the using is in the hand-written half.
// That is a real limit of the passthrough rule and not an oversight -- the
// corpus's own `using B2Tech.Feed.Cache.Core;` is unreachable from here.
//
// An enum names no types and carries no MessagePack attribute, so it emits no
// usings at all.
func usings(d *check.Decl) []string {
	if d.Kind == check.DeclEnum {
		return nil
	}
	seen := map[string]bool{d.File.Namespace: true}
	out := []string{messagePackNamespace}
	if d.File.Namespace == messagePackNamespace {
		out = nil
	}
	seen[messagePackNamespace] = true
	for _, m := range d.Members {
		for _, ns := range typeNamespaces(m.Type) {
			if seen[ns] {
				continue
			}
			seen[ns] = true
			out = append(out, ns)
		}
	}
	slices.Sort(out)
	return out
}

// typeNamespaces is every namespace a type reference reaches, which for a
// collection is its arguments' and for a declared type is the file that
// declares it.
func typeNamespaces(t check.Type) []string {
	switch t.Kind {
	case check.TypeDecl:
		if t.Decl == nil || t.Decl.File == nil {
			return nil
		}
		return []string{t.Decl.File.Namespace}
	case check.TypeCollection:
		var out []string
		for _, a := range t.Args {
			out = append(out, typeNamespaces(a)...)
		}
		return out
	}
	return nil
}

// modifiers is section 4's emission column, and the row that reads strangest is
// the right one: a protop `record` emits a C# *class*. #20 measured that a
// computed member at key 0 breaks deserializing-constructor matching on both
// MessagePack paths, so protopc may never emit a C# `record` or any other
// immutable type. The two keywords collide in spelling and mean opposite
// things.
//
// `sealed partial` is on tables and not on records and structs, which is what
// the corpus writes: 22 sites for the first, and `StaticDataRef` /
// `BaseProviderObject` / `Venue` / `BOVenue` for the second. Whether `partial`
// is *mandatory* is question 1 of #25 and is not ruled here; 22 for 22 is
// evidence and the emitter follows it.
//
// A view emits whatever its source emits, which is a correction to #45's
// mapping table rather than a reading of it. The ticket says a view emits "as
// table" and cites `Slim*.cs`; two of those three files agree and
// `corpus/prague/payloads/SlimOdd.cs:7` does not -- it is `public class
// SlimOdd`, and it is the one Slim type whose source is a `record` rather than
// a `table`. Section 4 already has a view *inherit its source's identity*, and
// the shape follows the identity: a view of a table is cached and has the
// hand-written partial half that `partial` exists for, and a view of a record
// is embedded in whatever holds it and has neither. Under that rule the corpus
// is 3 for 3 instead of 2 for 3. See the report on #45.
func modifiers(d *check.Decl) string {
	k := d.Kind
	if k == check.DeclView {
		if d.Of == nil {
			// A view whose source did not resolve never reaches a backend:
			// PROTOP0205 is an error and bag.HasErrors stops emission. Written
			// so the function is total rather than to describe a real case.
			return "public sealed partial class"
		}
		k = d.Of.Kind
	}
	switch k {
	case check.DeclStruct:
		return "public struct"
	case check.DeclRecord:
		return "public class"
	case check.DeclTable:
		return "public sealed partial class"
	}
	return "public class"
}

func writeType(b *strings.Builder, d *check.Decl) {
	b.WriteString("[MessagePackObject]\n")
	writeAnnotations(b, "", d.Annotations)
	b.WriteString(modifiers(d))
	b.WriteString(" ")
	b.WriteString(d.Name)
	b.WriteString("\n{\n")
	for i, m := range d.Members {
		if i > 0 {
			b.WriteString("\n")
		}
		writeMember(b, d, m)
	}
	b.WriteString("}\n")
}

// writeEnum emits the obvious thing, and nothing in the corpus checks it.
// `corpus/prague/SOURCE.md` lists the enum declarations among the types
// deliberately not copied, so there are zero enum declarations corpus-wide and
// this row has no oracle at all -- neither the shape below nor the trailing
// comma is held against anything. Whether an enum is open or closed on the wire
// is #23 and is untouched here: protop's arms carry numbers, and a number is
// what is written.
func writeEnum(b *strings.Builder, d *check.Decl) {
	writeAnnotations(b, "", d.Annotations)
	b.WriteString("public enum ")
	b.WriteString(d.Name)
	b.WriteString("\n{\n")
	for _, m := range d.EnumMembers {
		b.WriteString("    ")
		b.WriteString(pascal(m.Name))
		b.WriteString(" = ")
		b.WriteString(strconv.FormatUint(m.Number, 10))
		b.WriteString(",\n")
	}
	b.WriteString("}\n")
}

// dataCacheKeyAttribute marks the member that carries the declaration's
// identity. It is the one attribute protopc writes that belongs to
// `B2Tech.Feed.Cache.Core` rather than to MessagePack, and it is written
// because the corpus writes it on every key it has and protop's `key` keyword
// is the only thing that could produce it: 11 sites over the 14 payload types,
// the other three -- Venue, BOVenue and SlimOdd -- having no key at all. Which
// version of that package protop targets is #21 and is open; nothing here pins
// one.
const dataCacheKeyAttribute = "[DataCacheKey]\n"

func writeMember(b *strings.Builder, d *check.Decl, m *check.Member) {
	switch m.Kind {
	case check.MemberAlias:
		writeAlias(b, m)
	case check.MemberCompositeKey:
		writeCompositeKey(b, d, m)
	default:
		writeField(b, d, m)
	}
}

// writeField is the ordinary case and carries the milestone's central
// arithmetic: a MessagePack key is the tag minus one, so tag 1 is key 0. 339
// corpus sites agree and none disagree.
//
// A view member emits its source's tag, because a view never has one of its
// own. A view member that selects an alias carries no tag and arrives at
// writeAlias instead.
func writeField(b *strings.Builder, d *check.Decl, m *check.Member) {
	writeKey(b, m)
	if d.Key == m {
		b.WriteString("    ")
		b.WriteString(dataCacheKeyAttribute)
	}
	writeAnnotations(b, "    ", m.Annotations)
	b.WriteString("    public ")
	b.WriteString(TypeName(m.Type))
	b.WriteString(" ")
	b.WriteString(pascal(m.Name))
	b.WriteString(" { get; set; }\n")
}

// writeCompositeKey emits `key key = 1 (event_id, bet_type_id);` as the
// get-only ValueTuple the corpus writes. #20 measured what that costs: the
// member serializes on every message and is Skip()ped on the way back, so four
// of its eight bytes are never read. It is emitted anyway because the corpus's
// wire format already says so, and because writing the tag is what makes the
// duplication visible and the collision checkable.
//
// The tuple's element names are the parts' own names. In a view they are the
// *source's* part names, since a view member selecting a composite key carries
// the source's parts: a view that renamed one of the parts would emit a body
// naming a member it does not have. No corpus view does that -- SlimBetType
// renames `id` to `bet_type_id`, which is the name the source's own alias
// already gives it -- and the language does not currently say whether it may.
func writeCompositeKey(b *strings.Builder, d *check.Decl, m *check.Member) {
	writeKey(b, m)
	if d.Key == m {
		b.WriteString("    ")
		b.WriteString(dataCacheKeyAttribute)
	}
	writeAnnotations(b, "    ", m.Annotations)
	b.WriteString("    public (")
	for i, p := range m.Parts {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(TypeName(p.Type))
		b.WriteString(" ")
		b.WriteString(pascal(p.Name))
	}
	b.WriteString(") ")
	b.WriteString(pascal(m.Name))
	b.WriteString(" => (")
	for i, p := range m.Parts {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(pascal(p.Name))
	}
	b.WriteString(");\n")
}

// writeAlias emits `alias bet_type_id = id;` as a get and set passthrough with
// no wire slot. `[IgnoreMember]` is not decoration: #20 measured that `[Key(N)]`
// is not optional on a member MessagePack can see -- MsgPack004 is a hard error
// -- so the only way to say "no slot" is to say it explicitly.
func writeAlias(b *strings.Builder, m *check.Member) {
	b.WriteString("    [IgnoreMember]\n")
	writeAnnotations(b, "    ", m.Annotations)
	b.WriteString("    public ")
	b.WriteString(TypeName(m.Type))
	b.WriteString(" ")
	b.WriteString(pascal(m.Name))
	b.WriteString("\n    {\n")
	target := pascal(aliasTarget(m))
	b.WriteString("        get => ")
	b.WriteString(target)
	b.WriteString(";\n        set => ")
	b.WriteString(target)
	b.WriteString(" = value;\n    }\n")
}

// aliasTarget is the name the passthrough reads and writes. In a view it is the
// source's member name for the same reason the composite key's parts are: what
// the alias passes through to is a member of the declaration the alias was
// written in.
func aliasTarget(m *check.Member) string {
	if m.Aliased == nil {
		return ""
	}
	return m.Aliased.Name
}

func writeKey(b *strings.Builder, m *check.Member) {
	b.WriteString("    [Key(")
	b.WriteString(strconv.FormatUint(m.Tag-1, 10))
	b.WriteString(")]\n")
}

// writeAnnotations copies each annotation's contents out between brackets, in
// source order, neither sorted nor deduplicated.
//
// The whole of section 3's passthrough rule is the absence of anything else in
// this function. protopc does not parse the text, does not know what attribute
// it names, and has no opinion about whether it compiles -- the check that
// matters happens when the generated code does. protop's own annotations carry
// no prefix and are read by the checker, so they reach the backend as resolved
// relations and emit nothing here: `[references(SportGame)]` is a fact about
// the schema and not a C# attribute.
func writeAnnotations(b *strings.Builder, indent string, anns []*syntax.Annotation) {
	for _, a := range anns {
		if a.Prefix == nil {
			continue
		}
		b.WriteString(indent)
		b.WriteString("[")
		b.WriteString(a.Text)
		b.WriteString("]\n")
	}
}
