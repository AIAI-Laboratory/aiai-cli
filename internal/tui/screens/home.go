package screens

import "strings"

var HomeItems = []string{"Initialize Python Project", "Keyboard Shortcuts", "About", "Quit"}

func Home(cursor int) string {
	var s strings.Builder
	for i, item := range HomeItems {
		switch i {
		case 0:
			s.WriteString("PROJECT\n")
		case 1:
			s.WriteString("\nHELP\n")
		case 3:
			s.WriteString("\n")
		}
		prefix := "  "
		if i == cursor {
			prefix = "> "
		}
		s.WriteString(prefix + item + "\n")
	}
	return s.String()
}

const Help = "KEYBOARD SHORTCUTS\n\n↑/↓ or j/k  Navigate menus and scroll previews\nEnter       Select or continue\nTab         Next form field\nShift+Tab   Previous form field\nSpace       Toggle a form option\nEsc         Go back\nCtrl+C      Cancel safely\n?           Toggle help\n\nIn text fields, j/k are entered as text. Use arrows or Tab to change fields.\n\nEsc to return."
