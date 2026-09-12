// Package diag collects the diagnostics protopc reports and renders them
// against their source.
//
// Diagnostics are collected, never thrown. A compiler that stops at the first
// error makes the user drive it one mistake at a time, so every stage takes a
// Bag, appends to it, and carries on with whatever it can still do. The Bag,
// not a returned error, is what says whether the compile succeeded.
package diag

import (
	"fmt"
	"strings"

	"github.com/nanov/protobuf-to-messagepack-sharp/internal/source"
)

// Severity says whether a diagnostic stops the compile.
type Severity int

const (
	// Error fails the compile.
	Error Severity = iota
	// Warning is reported and does not fail the compile.
	Warning
)

func (s Severity) String() string {
	switch s {
	case Warning:
		return "warning"
	default:
		return "error"
	}
}

// Diagnostic is one thing protopc has to say about one place in the source.
//
// Every diagnostic carries a span. A message without a location is a message
// the user has to go looking for, and the span is free at the point it is
// raised -- the token is right there.
type Diagnostic struct {
	Code     Code
	Severity Severity
	Message  string
	Span     source.Span
}

// Bag accumulates diagnostics in the order they were raised.
type Bag struct {
	diags []Diagnostic
}

// Errorf records an error at span.
func (b *Bag) Errorf(code Code, span source.Span, format string, args ...any) {
	b.add(Error, code, span, format, args...)
}

// Warnf records a warning at span.
func (b *Bag) Warnf(code Code, span source.Span, format string, args ...any) {
	b.add(Warning, code, span, format, args...)
}

func (b *Bag) add(sev Severity, code Code, span source.Span, format string, args ...any) {
	b.diags = append(b.diags, Diagnostic{
		Code:     code,
		Severity: sev,
		Message:  fmt.Sprintf(format, args...),
		Span:     span,
	})
}

// All returns the diagnostics in the order they were raised, which is source
// order for everything a single-pass frontend produces.
func (b *Bag) All() []Diagnostic { return b.diags }

// Len is the number of diagnostics of any severity.
func (b *Bag) Len() int { return len(b.diags) }

// HasErrors reports whether the compile failed.
func (b *Bag) HasErrors() bool {
	for _, d := range b.diags {
		if d.Severity == Error {
			return true
		}
	}
	return false
}

// Render formats one diagnostic against the file its span indexes into:
//
//	prague.protop:3:14: error PROTOP0300: field tag must be greater than 0
//	  |
//	3 |   int64 id = 0;
//	  |              ^
//
// The snippet is the reason spans are carried rather than positions: an offset
// pair is what lets the caret have a width.
func Render(f *source.File, d Diagnostic) string {
	pos := f.Position(d.Span.Start)
	head := fmt.Sprintf("%s: %s %s: %s", pos, d.Severity, d.Code, d.Message)

	line := f.Line(pos.Line)
	if strings.TrimSpace(line) == "" {
		return head
	}
	gutter := fmt.Sprintf("%d", pos.Line)
	pad := strings.Repeat(" ", len(gutter))

	// The caret run is clamped to the first line of the span: a span crossing a
	// newline -- an unterminated block comment, say -- underlines its opening
	// rather than painting the rest of the file.
	width := len([]rune(line)) - (pos.Column - 1)
	if end := f.Position(d.Span.End); end.Line == pos.Line {
		width = end.Column - pos.Column
	}
	if width < 1 {
		width = 1
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", head)
	fmt.Fprintf(&b, "%s |\n", pad)
	fmt.Fprintf(&b, "%s | %s\n", gutter, line)
	fmt.Fprintf(&b, "%s | %s%s", pad, strings.Repeat(" ", pos.Column-1), strings.Repeat("^", width))
	return b.String()
}

// RenderAll formats every diagnostic in the bag, one per paragraph.
func RenderAll(f *source.File, b *Bag) string {
	out := make([]string, 0, b.Len())
	for _, d := range b.All() {
		out = append(out, Render(f, d))
	}
	return strings.Join(out, "\n")
}
