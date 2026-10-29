// Package source holds the text praguec reads and the positions it reports
// against.
//
// Everything downstream refers to source by byte offset -- a Pos, or a Span of
// two -- and never by line and column. Offsets are what a lexer produces for
// free and what an AST can carry cheaply; line and column are a rendering
// concern, computed here only when a diagnostic is printed.
package source

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

// Pos is a byte offset into a File's text. Pos(0) is the first byte, and a Pos
// equal to the text length is the end of the file -- a valid position to report
// against, which is what lets an unexpected end of file carry a span.
type Pos int

// Span is a half-open byte range [Start, End) within one File. An empty span
// (Start == End) points between two bytes; a diagnostic rendered from one
// underlines a single caret.
type Span struct {
	Start Pos
	End   Pos
}

// SpanAt returns the empty span at p.
func SpanAt(p Pos) Span { return Span{Start: p, End: p} }

// To returns the span covering both s and other, which is how a parser builds a
// declaration's span out of its first and last token.
func (s Span) To(other Span) Span { return Span{Start: s.Start, End: other.End} }

// Len is the span's width in bytes.
func (s Span) Len() int { return int(s.End - s.Start) }

// File is one source file: a name to report, its whole text, and an index of
// where its lines begin.
type File struct {
	// Name is what diagnostics print. It is the path as the user gave it, not
	// a cleaned or absolute one -- an error message should echo the argument.
	Name string

	// Text is the entire file contents. Spans index into it.
	Text string

	// lineStarts[i] is the offset of the first byte of line i+1. It always
	// begins with 0, so a file with no newline at all still has one line.
	lineStarts []Pos
}

// NewFile indexes text's lines and returns the File spans resolve against.
func NewFile(name, text string) *File {
	f := &File{Name: name, Text: text, lineStarts: make([]Pos, 1, strings.Count(text, "\n")+1)}
	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			f.lineStarts = append(f.lineStarts, Pos(i+1))
		}
	}
	return f
}

// Lines is the number of lines in the file, counting from 1.
func (f *File) Lines() int { return len(f.lineStarts) }

// Slice returns the text a span covers.
func (f *File) Slice(s Span) string {
	start, end := f.clamp(s.Start), f.clamp(s.End)
	if end < start {
		end = start
	}
	return f.Text[start:end]
}

// Position is a human-facing location: 1-based line, and 1-based column counted
// in runes so that a caret lands under the right glyph rather than the right
// byte.
type Position struct {
	File   string
	Line   int
	Column int
}

func (p Position) String() string { return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Column) }

// Position resolves a byte offset to a line and column. An offset outside the
// file is clamped rather than rejected; a position is only ever used to print a
// message, and a message that panics is worse than one that is a byte off.
func (f *File) Position(p Pos) Position {
	p = f.clamp(p)
	line := sort.Search(len(f.lineStarts), func(i int) bool { return f.lineStarts[i] > p }) - 1
	return Position{
		File:   f.Name,
		Line:   line + 1,
		Column: utf8.RuneCountInString(f.Text[f.lineStarts[line]:p]) + 1,
	}
}

// Line returns the text of the 1-based line n, without its terminator. An n
// outside the file returns "".
func (f *File) Line(n int) string {
	if n < 1 || n > len(f.lineStarts) {
		return ""
	}
	start := f.lineStarts[n-1]
	end := Pos(len(f.Text))
	if n < len(f.lineStarts) {
		end = f.lineStarts[n] - 1 // drop the '\n'
	}
	return strings.TrimSuffix(f.Text[start:end], "\r")
}

func (f *File) clamp(p Pos) Pos {
	if p < 0 {
		return 0
	}
	if max := Pos(len(f.Text)); p > max {
		return max
	}
	return p
}
