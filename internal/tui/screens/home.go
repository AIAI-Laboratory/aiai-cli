package screens

import (
	"strconv"
	"strings"
)

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
	Section(title string) string
	Truncate(text string, width int) string
}

type Command struct {
	Name        string
	Description string
	Category    string
}

var HomeItems = []Command{
	{"/init", "Initialize a Python project", "Project"},
	{"/workspace", "Your projects and development timeline", "Project"},
	{"/git", "Read-only Git workspace", "Project"},
	{"/servers", "Browse local SSH server profiles", "SSH"},
	{"/ssh", "Connect to a configured SSH server", "SSH"},
	{"/logs", "View SSH access history", "SSH"},
	{"/keys", "Find managed and standard SSH keys", "SSH"},
	{"/keygen", "Generate a managed key: /keygen <name>", "SSH"},
	{"/login", "Sign in with GitHub", "Account"},
	{"/whoami", "Show the signed-in account", "Account"},
	{"/logout", "Remove the current session", "Account"},
	{"/update", "Check for updates and upgrade AIAI", "AIAI"},
	{"/zen", "A little peace inside your terminal", "AIAI"},
	{"/monitor", "Live CPU, memory, disks, network, and processes", "AIAI"},
	{"/about", "About AIAI CLI", "AIAI"},
	{"/help", "Explore keyboard shortcuts", ""},
	{"/quit", "Return to your terminal", ""},
}

func Categories() []string {
	var categories []string
	for _, item := range HomeItems {
		if item.Category == "" {
			continue
		}
		if len(categories) == 0 || categories[len(categories)-1] != item.Category {
			categories = append(categories, item.Category)
		}
	}
	return categories
}

func RootCommands() []int {
	var commands []int
	for index, item := range HomeItems {
		if item.Category == "" {
			commands = append(commands, index)
		}
	}
	return commands
}

func MatchingCommands(query string) []int {
	query = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(query)), "/")
	commandQuery := query
	if fields := strings.Fields(query); len(fields) > 0 {
		commandQuery = fields[0]
	}
	for i, item := range HomeItems {
		if strings.TrimPrefix(item.Name, "/") == commandQuery && commandQuery != "" {
			return []int{i}
		}
	}
	var matches []int
	for i, item := range HomeItems {
		if strings.Contains(strings.TrimPrefix(item.Name, "/"), query) || strings.Contains(strings.ToLower(item.Description), query) {
			matches = append(matches, i)
		}
	}
	return matches
}

func VisibleCommands(query, category string) []int {
	matches := MatchingCommands(query)
	if strings.TrimSpace(query) != "" || category == "" {
		return matches
	}
	visible := matches[:0]
	for _, index := range matches {
		if HomeItems[index].Category == category {
			visible = append(visible, index)
		}
	}
	return visible
}

func Home(cursor int, query, category string, t Theme) string {
	var s strings.Builder
	if strings.TrimSpace(query) == "" && category == "" {
		descriptions := map[string]string{
			"Project": "Workspace, Git, and scaffolding",
			"SSH":     "Servers, keys, and access logs",
			"Account": "Authentication and profile",
			"AIAI":    "Zen, monitor, and updates",
		}
		categories := Categories()
		for row, name := range categories {
			s.WriteString(t.Choice(name, descriptions[name], row == cursor))
		}
		for row, index := range RootCommands() {
			item := HomeItems[index]
			s.WriteString(t.Choice(item.Name, item.Description, len(categories)+row == cursor))
		}
		return strings.TrimSuffix(s.String(), "\n")
	}
	matches := VisibleCommands(query, category)
	if len(matches) == 0 {
		return t.Heading("No matching commands", "Nothing matches "+strconv.Quote(strings.TrimSpace(query))+".") +
			"Try a shorter command name, or press " + t.Accent("Esc") + " to clear the filter."
	}
	if category != "" && strings.TrimSpace(query) == "" {
		s.WriteString(t.Muted("Esc returns to workspace") + "\n\n")
	}
	for row, index := range matches {
		item := HomeItems[index]
		s.WriteString(t.Choice(item.Name, item.Description, row == cursor))
	}
	return strings.TrimSuffix(s.String(), "\n")
}

func Help(t Theme) string {
	return t.Heading("Help & shortcuts", "Keyboard-first, predictable everywhere.") +
		t.Section("Navigate") + "\n" +
		"  ↑/↓  j/k      Move selection\n" +
		"  Enter         Open or confirm\n" +
		"  Esc           Back or dismiss\n\n" +
		t.Section("Find") + "\n" +
		"  /             Search the current list\n" +
		"  Tab           Complete a command\n\n" +
		t.Section("Forms") + "\n" +
		"  Tab / Shift+Tab  Next / previous field\n" +
		"  Space            Toggle option\n\n" +
		t.Section("Global") + "\n" +
		"  ?             Toggle this help\n" +
		"  Ctrl+C        Cancel safely\n\n" +
		t.Muted("Printable shortcuts are entered as text while an input is focused.")
}
