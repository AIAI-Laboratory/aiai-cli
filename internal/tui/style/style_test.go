package style

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
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
