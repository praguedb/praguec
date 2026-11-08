package check

import (
	"strings"

	"github.com/praguedb/praguec/internal/diag"
	"github.com/praguedb/praguec/internal/syntax"
)

// resolveEnrichment reads protop's `header` and `from_timestamp`, which say a
// cached member is written from the Kafka message as well as from its value.
// With views false it resolves every other declaration; with views true it
// resolves the views, each inheriting what its source's members resolved to.
func (c *checker) resolveEnrichment(w *fileWork, views bool) {
	for _, dw := range w.decls {
		d := dw.decl
		if (d.Kind == DeclView) != views {
			continue
		}
		for _, a := range d.Annotations {
			if enrichmentWord(a) != "" {
				w.sink.Raise(diag.EnrichmentOutsideTable, a.Span,
					"`%s` belongs on a member, which it says is written from the Kafka message; on a declaration it names no member to write",
					enrichmentWord(a))
			}
		}
		byHeader := map[string]*Member{}
		for _, m := range d.Members {
			if m.From != nil {
				m.Header, m.FromTimestamp = m.From.Header, m.From.FromTimestamp
			}
			for _, a := range m.Annotations {
				c.enrichMember(w, d, m, a)
			}
			if d.Kind != DeclView && m.Kind == MemberField && !m.HasTag() && !m.Enriched() {
				w.sink.Raise(diag.FieldWithoutTag, m.Span,
					"`%s` has no tag, so nothing on the wire holds it, and nothing on the Kafka message writes it either; give it a tag, or say which header it comes from with `[header]`",
					m.Name)
			}
			if m.Header == nil {
				continue
			}
			if prev, dup := byHeader[m.Header.Name]; dup {
				w.sink.Raise(diag.ConflictingEnrichment, m.Header.Span,
					"`%s` and `%s` both read the header `%s`; one header writes one member",
					prev.Name, m.Name, m.Header.Name)
				continue
			}
			byHeader[m.Header.Name] = m
		}
	}
}

func (c *checker) enrichMember(w *fileWork, d *Decl, m *Member, a *syntax.Annotation) {
	word := enrichmentWord(a)
	if word == "" {
		return
	}
	if d.Kind != DeclTable && d.Kind != DeclView {
		w.sink.Raise(diag.EnrichmentOutsideTable, a.Span,
			"`%s` is on a member of the %s `%s`; only a table or a view is cached from a Kafka message, and a %s is decoded inside whatever holds it",
			word, d.Kind, d.Name, d.Kind)
		return
	}
	if m.Kind != MemberField {
		w.sink.Raise(diag.EnrichmentOutsideTable, a.Span,
			"`%s` is on the %s `%s`, which has no field of its own to write; put it on the member it names",
			word, m.Kind, m.Name)
		return
	}
	if m.Enriched() {
		w.sink.Raise(diag.ConflictingEnrichment, a.Span,
			"`%s` is already written from the Kafka message; a member has one source besides its value", m.Name)
		return
	}

	if word == syntax.FromTimestampAnnotation {
		if strings.TrimSpace(a.Text) != syntax.FromTimestampAnnotation {
			w.sink.Raise(diag.MalformedFromTimestamp, a.Span,
				"`%s` takes no arguments; a Kafka message has one timestamp", syntax.FromTimestampAnnotation)
			return
		}
		if t := m.Type; t.Kind != TypeScalar || t.Name != "int64" || t.Optional {
			w.sink.Raise(diag.TimestampTypeUnsupported, a.Span,
				"`%s` is `%s`; the message timestamp is Unix milliseconds and is written to an `int64`",
				m.Name, typeText(t))
			return
		}
		m.FromTimestamp = true
		return
	}

	name, ok := headerName(a)
	if !ok {
		w.sink.Raise(diag.MalformedHeader, a.Span,
			"`%s` reads the header named as the member is, or the one it names, as in `[%s(\"tenant-id\")]`",
			syntax.HeaderAnnotation, syntax.HeaderAnnotation)
		return
	}
	if !headerType(m.Type) {
		w.sink.Raise(diag.HeaderTypeUnsupported, a.Span,
			"`%s` is `%s`; a header carries one scalar or one enum value", m.Name, typeText(m.Type))
		return
	}
	if name == "" {
		name = Pascal(m.Name)
	}
	m.Header = &Header{Name: name, Span: a.Span}
}

// enrichmentWord is the annotation's word if it is protop's `header` or
// `from_timestamp`, and empty otherwise.
func enrichmentWord(a *syntax.Annotation) string {
	if a.Prefix != nil {
		return ""
	}
	word, _ := splitWord(strings.TrimSpace(a.Text))
	if word == syntax.HeaderAnnotation || word == syntax.FromTimestampAnnotation {
		return word
	}
	return ""
}

// headerName reads `header` or `header("name")`. An empty name with ok means
// the member's own. The name may not hold a quote or a backslash: protop has
// no escapes, and a header key that needs one is not worth inventing them for.
func headerName(a *syntax.Annotation) (string, bool) {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(a.Text), syntax.HeaderAnnotation))
	if rest == "" {
		return "", true
	}
	if !strings.HasPrefix(rest, "(") || !strings.HasSuffix(rest, ")") {
		return "", false
	}
	lit := strings.TrimSpace(rest[1 : len(rest)-1])
	if len(lit) < 3 || lit[0] != '"' || lit[len(lit)-1] != '"' {
		return "", false
	}
	name := lit[1 : len(lit)-1]
	if strings.ContainsAny(name, "\"\\") {
		return "", false
	}
	return name, true
}

func headerType(t Type) bool {
	switch t.Kind {
	case TypeScalar:
		return true
	case TypeDecl:
		return t.Decl != nil && t.Decl.Kind == DeclEnum
	}
	return false
}

func typeText(t Type) string {
	s := t.Name
	if len(t.Args) > 0 {
		parts := make([]string, len(t.Args))
		for i, a := range t.Args {
			parts[i] = typeText(a)
		}
		s += "<" + strings.Join(parts, ", ") + ">"
	}
	if t.Optional {
		s += "?"
	}
	return s
}
