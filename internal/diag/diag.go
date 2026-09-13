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

	// File is the source the span indexes into, and is nil for a diagnostic
	// raised by a stage that sees one file at a time. The frontend is such a
	// stage: the caller handed Parse a file and knows which one it got back.
	//
	// The checking stage is the first that sees several at once, and a span
	// alone cannot say which of them an offset belongs to. A stage that works
	// across files fills this in through Bag.In, and Render prefers it over the
	// file it is passed.
	File *source.File
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

// In returns a view of the bag that stamps every diagnostic it records with f,
// for a stage that reports against more than one file. The diagnostics go into
// the same bag in the same order; all that is added is which file each span
// indexes into.
func (b *Bag) In(f *source.File) *Sink { return &Sink{bag: b, file: f} }

// Within runs fn against the bag and stamps every diagnostic fn adds that does
// not already name a file.
//
// It exists for the stages that see one file at a time -- the lexer and the
// parser -- reporting into a bag that a driver is filling from several. Those
// stages cannot name the file they are reporting against and should not have
// to; the driver opened it and is the only thing that knows. Without this, a
// parse error in the fourth file of a compile renders its snippet against the
// first.
func (b *Bag) Within(f *source.File, fn func(*Bag)) {
	start := len(b.diags)
	fn(b)
	for i := start; i < len(b.diags); i++ {
		if b.diags[i].File == nil {
			b.diags[i].File = f
		}
	}
}

// Sink is a Bag as seen from inside one file. It exists so that a stage holding
// several files does not repeat which one it is reporting against on every
// call, and so that it cannot pass the wrong one.
type Sink struct {
	bag  *Bag
	file *source.File
}

// Errorf records an error at span in the sink's file.
func (s *Sink) Errorf(code Code, span source.Span, format string, args ...any) {
	s.bag.add(Error, code, span, format, args...)
	s.stamp()
}

// Warnf records a warning at span in the sink's file.
func (s *Sink) Warnf(code Code, span source.Span, format string, args ...any) {
	s.bag.add(Warning, code, span, format, args...)
	s.stamp()
}

// File is the file the sink reports against, which a caller needs to render a
// position of its own into a message.
func (s *Sink) File() *source.File { return s.file }

func (s *Sink) stamp() { s.bag.diags[len(s.bag.diags)-1].File = s.file }

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
	// A diagnostic that names its own file renders against that one. Only a
	// stage that saw several files names one, and it is the only thing that
	// could know: f is whatever the caller had to hand.
	if d.File != nil {
		f = d.File
	}
	if f == nil {
		// No file, so no position and no snippet: the headline alone, under
		// the tool's name. See renderMSVS on why the branch exists at all.
		return fmt.Sprintf("%s: %s %s: %s", toolName, d.Severity, d.Code, d.Message)
	}
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

// Format selects how a diagnostic is written out.
//
// Two consumers want different things from the same fact. A terminal wants the
// caret snippet above: the line, and a run of ^ under the span, which is why a
// span is carried rather than a position. MSBuild wants
// `path(line,col): error CODE: text`, which is the one shape its canonical
// error parser recognises -- and recognising it is what lights up Visual
// Studio's error list, `dotnet build`'s summary and GitHub Actions'
// annotations, none of which read the GCC form at all.
//
// The GCC form is for a terminal and for nothing else. Verified: MSBuild's GCC
// fallback pattern wants the colon immediately after the severity, so protopc's
// code between the two kills the match and the line is logged as a plain
// message -- the build passes with an error in it. Anything reading protopc's
// output mechanically wants MSVS.
//
// The default is GCC, and it is provisional. docs/research/
// proto-to-messagepack-csharp.md:924 and :1292 both say the MSVS-shaped form
// should be the default, on the grounds that MSBuild parses it natively; a
// terminal-first tool makes the caret renderer the default and the MSVS form a
// flag, which is what protopc does here. The contradiction is in the record and
// is filed rather than resolved -- nanov/protobuf-to-messagepack-sharp#49 says
// to pick terminal-first provisionally and say so, and this is the saying.
type Format int

const (
	// GCC is the caret form: `path:line:col: error CODE: text`, followed by
	// the source line and a caret run under the span.
	GCC Format = iota
	// MSVS is MSBuild's canonical form: `path(line,col): error CODE: text`,
	// one diagnostic per line and no snippet.
	MSVS
)

// String is the spelling --error-format accepts, which is also buf's: `gcc` and
// `msvs` are two of the six names buf gives the same choice.
func (f Format) String() string {
	if f == MSVS {
		return "msvs"
	}
	return "gcc"
}

// ParseFormat reads a --error-format value. It reports whether the name is one
// protopc has, so that the caller can list the ones it does.
func ParseFormat(name string) (Format, bool) {
	switch name {
	case "gcc":
		return GCC, true
	case "msvs":
		return MSVS, true
	}
	return GCC, false
}

// RenderAs formats one diagnostic in the given format.
func RenderAs(format Format, f *source.File, d Diagnostic) string {
	if format == MSVS {
		return renderMSVS(f, d)
	}
	return Render(f, d)
}

// RenderAllAs formats every diagnostic in the bag in the given format, one per
// paragraph.
func RenderAllAs(format Format, f *source.File, b *Bag) string {
	out := make([]string, 0, b.Len())
	for _, d := range b.All() {
		out = append(out, RenderAs(format, f, d))
	}
	return strings.Join(out, "\n")
}

// renderMSVS writes the one shape MSBuild's canonical error parser reads:
//
//	prague.protop(3,14): error PROTOP0300: field tag must be greater than 0
//
// The authority is dotnet/msbuild, src/Shared/CanonicalError.cs, and its
// OriginCategoryCodeTextExpressionPattern. The grammar is
// `Origin : Subcategory Category Code : Text` in that order; the location is
// `(line)`, `(line,col)`, `(line,col,line,col)` or `(line,col-col)`, one-based;
// the category is `error` or `warning`; and the code may contain neither a
// space nor a colon. The **comma** between line and column is load-bearing --
// the `(line-col)` that appears in Microsoft's own documentation is a typo with
// no matching regex behind it.
//
// This is not protoc's `--error_format=msvs`, which is `f.proto(14) : error in
// column=10: msg` -- line only, column in prose, and no code at all. MSBuild
// logs that as a plain message rather than as an error, which is why Grpc.Tools
// passes the flag and then overrides LogEventsFromTextOutput with six bespoke
// regexes. Copying protoc here would be copying a shape that does not work.
//
// A diagnostic with no file to report against takes the tool name as its
// origin, which is the spelling CanonicalError accepts for a message about no
// particular place. protopc raises none today -- every diagnostic carries a
// span, deliberately -- and the branch is here so that the first one to be
// raised is still parseable rather than a crash.
//
// The message is flattened to one line because that parser is line-based: a
// diagnostic whose text wrapped would have its tail read as an unrelated line
// of build output, and protopc's messages are long enough that this is not
// hypothetical.
func renderMSVS(f *source.File, d Diagnostic) string {
	if d.File != nil {
		f = d.File
	}
	if f == nil {
		return fmt.Sprintf("%s : %s %s: %s", toolName, d.Severity, d.Code, oneLine(d.Message))
	}
	pos := f.Position(d.Span.Start)
	return fmt.Sprintf("%s(%d,%d): %s %s: %s",
		pos.File, pos.Line, pos.Column, d.Severity, d.Code, oneLine(d.Message))
}

// toolName is the origin a diagnostic about no particular file reports under.
const toolName = "protopc"

// oneLine collapses every line break in a message to a single space.
func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\n", " ")
}
