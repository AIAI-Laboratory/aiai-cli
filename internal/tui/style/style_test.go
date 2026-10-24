package style

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestFrameAndKeysRemainReadableWithoutColor(t *testing.T) {
	keys := Keys("↑↓ select · enter open · q back", true)
	if keys != "↑↓ select  enter open  q back" || strings.Contains(keys, "\x1b") {
		t.Fatalf("keys %q", keys)
	}
	for _, width := range []int{1, 16, 36} {
		frame := Fit("服务器 project\nsecond\nthird", width, 2)
		if lipgloss.Width(frame) > width || lipgloss.Height(frame) != 2 {
			t.Fatalf("bad frame %q", frame)
		}
	}
	if got := Fit("one", 36, 5); lipgloss.Height(got) != 5 {
		t.Fatalf("not padded: %q", got)
	}
}

func TestSearchKeepsLabelAndCursorWhenResized(t *testing.T) {
	input := textinput.New()
	input.Prompt = ""
	input.SetStyles(textinput.Styles{})
	ResizeInput(&input, 100)
	query := strings.Repeat("目录/project/", 20)
	input.SetValue(query)
	for _, pos := range []int{0, 18, len([]rune(query))} {
		input.SetCursor(pos)
		for _, width := range []int{20, 36, 76, 20} {
			ResizeInput(&input, width-len(SearchPrefix)-1)
			view := ansi.Strip(Search(input, width, true))
			if !strings.HasPrefix(view, SearchPrefix) || lipgloss.Width(view) > width || lipgloss.Height(view) != 1 || input.Position() != pos || input.Value() != query {
				t.Fatalf("search lost its label, cursor or text at width %d: %q", width, view)
			}
		}
	}
}
