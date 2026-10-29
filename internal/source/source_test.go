package source

import "testing"

func TestPositionAndLine(t *testing.T) {
	f := NewFile("prague.prague", "revision 1;\n\nstruct Venue {\n  int64 id = 2;\n}\n")

	tests := []struct {
		name     string
		at       Pos
		wantLine int
		wantCol  int
		wantText string
	}{
		{"first byte", 0, 1, 1, "revision 1;"},
		{"mid line", 9, 1, 10, "revision 1;"},
		{"blank line", 12, 2, 1, ""},
		{"after newline", 13, 3, 1, "struct Venue {"},
		{"indented", 30, 4, 3, "  int64 id = 2;"},
		{"end of file", Pos(len(f.Text)), 6, 1, ""},
		{"past end is clamped", 9999, 6, 1, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := f.Position(tt.at)
			if got.Line != tt.wantLine || got.Column != tt.wantCol {
				t.Errorf("Position(%d) = %d:%d, want %d:%d", tt.at, got.Line, got.Column, tt.wantLine, tt.wantCol)
			}
			if text := f.Line(got.Line); text != tt.wantText {
				t.Errorf("Line(%d) = %q, want %q", got.Line, text, tt.wantText)
			}
		})
	}
}

// Columns count runes, not bytes: a caret has to land under the glyph the
// message is about, and protop source is UTF-8 -- comments above all.
func TestPositionCountsRunesNotBytes(t *testing.T) {
	f := NewFile("x.prague", "// naïve — dashes\nstruct S {}\n")
	at := Pos(len("// naïve — dashes"))
	if got := f.Position(at); got.Line != 1 || got.Column != 18 {
		t.Errorf("Position(%d) = %d:%d, want 1:18", at, got.Line, got.Column)
	}
}

func TestFileWithoutTrailingNewlineHasOneLine(t *testing.T) {
	f := NewFile("x.prague", "revision 1;")
	if got := f.Lines(); got != 1 {
		t.Errorf("Lines() = %d, want 1", got)
	}
	if got := f.Line(1); got != "revision 1;" {
		t.Errorf("Line(1) = %q, want %q", got, "revision 1;")
	}
}

func TestSliceAndSpan(t *testing.T) {
	f := NewFile("x.prague", "struct Venue {}")
	name := Span{Start: 7, End: 12}
	if got := f.Slice(name); got != "Venue" {
		t.Errorf("Slice = %q, want %q", got, "Venue")
	}
	if got := f.Slice(Span{Start: 0, End: 6}.To(name)); got != "struct Venue" {
		t.Errorf("Slice of joined span = %q, want %q", got, "struct Venue")
	}
	if got := f.Slice(Span{Start: 12, End: 3}); got != "" {
		t.Errorf("Slice of inverted span = %q, want %q", got, "")
	}
	if got := (Span{Start: 7, End: 12}).Len(); got != 5 {
		t.Errorf("Len = %d, want 5", got)
	}
}
