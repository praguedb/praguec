package check

import (
	"github.com/praguedb/praguec/internal/diag"
)

// resolveUnion resolves a union's enum and holds each arm to it: an arm is one
// of the enum's, named once, and its payload type resolves like a member's.
// An arm of the enum the union does not list is not an error -- a reader keeps
// it, like an arm it has never heard of, as raw MessagePack.
func (c *checker) resolveUnion(w *fileWork, dw *declWork) {
	u, d := dw.union, dw.decl
	disc, found := w.file.scope[u.Discriminant.Text]
	switch {
	case !found:
		w.sink.Raise(diag.UnknownType, u.Discriminant.Span,
			"unknown type `%s`; a file sees its own declarations and those of the files it imports, and nothing else",
			u.Discriminant.Text)
		return
	case disc.Kind != DeclEnum:
		w.sink.Raise(diag.UnionOfNotAnEnum, u.Discriminant.Span,
			"`%s` is a %s; a union's arm is an enum's number on the wire, so `of` names an enum", disc.Name, disc.Kind)
		return
	}
	d.Discriminant = disc

	seen := map[string]bool{}
	for _, a := range u.Arms {
		var number int64
		ok := false
		for _, m := range disc.EnumMembers {
			if m.Name == a.Arm.Text {
				number, ok = int64(m.Number), true
			}
		}
		if !ok {
			w.sink.Raise(diag.NoSuchArm, a.Arm.Span, "`%s` has no arm `%s`", disc.Name, a.Arm.Text)
			continue
		}
		if seen[a.Arm.Text] {
			w.sink.Raise(diag.DuplicateArm, a.Arm.Span, "`%s` is given a payload twice; an arm has one", a.Arm.Text)
			continue
		}
		seen[a.Arm.Text] = true
		arm := &UnionArm{Name: a.Arm.Text, Number: number, Span: a.Span}
		if a.Type != nil {
			t := c.resolveType(w, *a.Type)
			arm.Type = &t
		}
		d.Arms = append(d.Arms, arm)
	}
}

// rejectRaw raises PROTOP0327 for a `raw` anywhere in t: everywhere but a union
// arm's own payload, which does not come through here.
func rejectRaw(w *fileWork, t Type) {
	if t.Kind == TypeScalar && t.Name == "raw" {
		w.sink.Raise(diag.RawOutsideUnion, t.Span,
			"`raw` is a union arm's payload, kept as the MessagePack it was written as; anywhere else, give the member its type")
	}
	for _, a := range t.Args {
		rejectRaw(w, a)
	}
}
