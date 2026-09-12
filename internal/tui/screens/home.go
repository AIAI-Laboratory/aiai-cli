package screens

import "strings"

// Theme interface defines the styling contract required by screens without
// coupling back to internal/tui, preventing circular import dependencies.
type Theme interface {
	Width() int
	NoColor() bool
	Accent(text string) string
	Brand(text string) string
	Muted(text string) string
	Good(text string) string
	Warn(text string) string
	Title(text string) string
	Rule() string
	Heading(title, subtitle string) string
	Keys(hint string) string
	Choice(label, description string, selected bool) string
	Step(current int) string
}

type Command struct {
	Name        string
	Description string
}

var HomeItems = []Command{
	{"/init", "Initialize a Python project"},
	{"/login", "Sign in with GitHub"},
	{"/whoami", "Show the signed-in account"},
	{"/logout", "Remove the current session"},
	{"/help", "Explore keyboard shortcuts"},
	{"/about", "About AIAI CLI"},
	{"/quit", "Return to your terminal"},
}

func MatchingCommands(query string) []int {
	query = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(query)), "/")
	var matches []int
	for i, item := range HomeItems {
		if strings.Contains(strings.TrimPrefix(item.Name, "/"), query) || strings.Contains(strings.ToLower(item.Description), query) {
			matches = append(matches, i)
		}
	}
	return matches
}

func Home(cursor int, query string, t Theme) string {
	var s strings.Builder
	matches := MatchingCommands(query)
	if len(matches) == 0 {
		return t.Muted("No matching commands. Clear the filter with Esc.")
	}
	for row, index := range matches {
		item := HomeItems[index]
		s.WriteString(t.Choice(item.Name, item.Description, row == cursor))
	}
	return strings.TrimSuffix(s.String(), "\n")
}

func Help(t Theme) string {
	return t.Heading("Keyboard shortcuts", "A few keys are all you need.") +
		"↑/↓ or j/k  Navigate menus and scroll previews\n" +
		"Enter       Select or continue\n" +
		"Tab         Complete a command / next form field\n" +
		"Shift+Tab   Previous form field\n" +
		"Space       Toggle a form option\n" +
		"Esc         Clear command filter / go back\n" +
		"Ctrl+C      Cancel safely\n" +
		"?           Toggle help\n\n" +
		t.Muted("Type / on the home screen to filter commands.\nIn text fields and command filters, j/k are entered as text.")
}
