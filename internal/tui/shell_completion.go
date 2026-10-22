package tui

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

type shellCompletion struct {
	before, after string
	line          string
	cursor        int
	quote         rune
	tilde         bool
	matches       []string
	selected      int
}

type completionWord struct {
	start, end int
	prefix     string
	quote      rune
	literal    bool
	command    bool
	dirsOnly   bool
}

// Inspect shell words without evaluating any shell code. Offsets are runes,
// matching textinput's cursor, including when editing in the middle of a line.
func shellWord(line string, cursor int) completionWord {
	runes := []rune(line)
	cursor = max(0, min(cursor, len(runes)))
	var words []string
	var word strings.Builder
	var quote rune
	escaped, started, redirect := false, false, false
	literal := false
	start := 0
	escape := '\\'
	if runtime.GOOS == "windows" {
		escape = '`'
	}
	for i, char := range runes[:cursor] {
		switch {
		case escaped:
			word.WriteRune(char)
			literal = literal || char == '$' || char == '~'
			escaped = false
		case char == escape && quote != '\'':
			escaped, started = true, true
		case quote != 0:
			if char == quote {
				if runtime.GOOS == "windows" && i+1 < cursor && runes[i+1] == quote {
					escaped = true
				} else {
					quote = 0
				}
			} else {
				word.WriteRune(char)
			}
		case char == '\'' || char == '"':
			quote, started = char, true
			literal = literal || char == '\''
		case unicode.IsSpace(char) || strings.ContainsRune("|&;<>()", char):
			if started {
				words = append(words, word.String())
			}
			word.Reset()
			literal = false
			started, start = false, i+1
			if strings.ContainsRune("|&;()", char) {
				words, redirect = nil, false
			} else if char == '<' || char == '>' {
				redirect = true
			}
		default:
			word.WriteRune(char)
			started = true
		}
	}
	end := cursor
	for i, char := range runes[cursor:] {
		if !escaped && quote == 0 && (unicode.IsSpace(char) || strings.ContainsRune("|&;<>()", char)) {
			break
		}
		switch {
		case escaped:
			escaped = false
		case char == escape && quote != '\'':
			escaped = true
		case quote != 0:
			if char == quote {
				if runtime.GOOS == "windows" && cursor+i+1 < len(runes) && runes[cursor+i+1] == quote {
					escaped = true
				} else {
					quote = 0
				}
			}
		case char == '\'' || char == '"':
			quote = char
		}
		end++
	}
	quote = 0
	if start < cursor && (runes[start] == '\'' || runes[start] == '"') {
		quote = runes[start]
	}
	for len(words) > 0 && strings.Contains(words[0], "=") {
		words = words[1:]
	}
	dirsOnly := len(words) == 1 && (words[0] == "cd" || words[0] == "pushd" || words[0] == "Set-Location")
	return completionWord{start: start, end: end, prefix: word.String(), quote: quote, literal: literal, command: len(words) == 0 && !redirect, dirsOnly: dirsOnly}
}

func shellMatches(word completionWord, cwd string) []string {
	prefix := word.prefix
	if !word.literal && word.quote != '\'' {
		prefix = os.ExpandEnv(prefix)
	}
	lookup := prefix
	if !word.literal && word.quote == 0 && lookup == "~" {
		return []string{"~/"}
	}
	if !word.literal && word.quote == 0 && strings.HasPrefix(lookup, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			lookup = filepath.Join(home, lookup[2:])
		}
	}
	seen := map[string]bool{}
	hasPrefix := func(name, prefix string) bool {
		if runtime.GOOS == "windows" {
			return strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix))
		}
		return strings.HasPrefix(name, prefix)
	}
	hasPath := strings.ContainsAny(prefix, "/\\")
	if word.command && !hasPath {
		builtins := "alias cd command echo exec exit export false history jobs kill printf pwd read set shift source test true type ulimit umask unalias unset wait"
		if runtime.GOOS == "windows" {
			builtins = "cd clear echo exit Get-ChildItem Get-Content Get-Location Set-Location Write-Output"
		}
		for _, name := range strings.Fields(builtins) {
			if hasPrefix(name, prefix) {
				seen[name] = true
			}
		}
		for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(cwd, dir)
			}
			entries, _ := os.ReadDir(dir)
			for _, entry := range entries {
				if hasPrefix(entry.Name(), prefix) && shellExecutable(filepath.Join(dir, entry.Name())) {
					seen[entry.Name()] = true
				}
			}
		}
	}
	dir, base := filepath.Split(lookup)
	displayDir, _ := filepath.Split(prefix)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(cwd, dir)
	}
	entries, _ := os.ReadDir(dir)
	for _, entry := range entries {
		if !hasPrefix(entry.Name(), base) || strings.HasPrefix(entry.Name(), ".") && !strings.HasPrefix(base, ".") {
			continue
		}
		isDir := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(filepath.Join(dir, entry.Name())); err == nil {
				isDir = info.IsDir()
			}
		}
		if !isDir && (word.dirsOnly || word.command && (!hasPath || !shellExecutable(filepath.Join(dir, entry.Name())))) {
			continue
		}
		name := displayDir + entry.Name()
		if isDir {
			name += "/"
		}
		seen[name] = true
	}
	matches := make([]string, 0, len(seen))
	for name := range seen {
		matches = append(matches, name)
	}
	sort.Strings(matches)
	return matches
}

func shellExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return false
	}
	if runtime.GOOS == "windows" {
		extensions := os.Getenv("PATHEXT")
		if extensions == "" {
			extensions = ".COM;.EXE;.BAT;.CMD"
		}
		for _, ext := range strings.Split(extensions, ";") {
			if strings.EqualFold(filepath.Ext(path), ext) {
				return true
			}
		}
		return false
	}
	return info.Mode()&0o111 != 0
}

func quoteCompletion(value string, quote rune) string {
	if quote == 0 && strings.IndexFunc(value, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("-._/:", r) && (runtime.GOOS != "windows" || r != '\\')
	}) < 0 {
		return value
	}
	if runtime.GOOS == "windows" {
		return "'" + strings.ReplaceAll(value, "'", "''") + "'"
	}
	if quote == '"' {
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`, "`", "\\`").Replace(value) + `"`
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (m Model) completeShell(reverse bool) (tea.Model, tea.Cmd) {
	line, cursor := m.command.Value(), m.command.Position()
	c := m.completion
	if c != nil && c.line == line && c.cursor == cursor && len(c.matches) > 1 {
		delta := 1
		if reverse {
			delta = -1
			if c.selected < 0 {
				c.selected = 0
			}
		}
		c.selected = (c.selected + delta + len(c.matches)) % len(c.matches)
		return m.applyCompletion(c.matches[c.selected], true), nil
	}
	word := shellWord(line, cursor)
	// ponytail: scan local PATH/files on Tab; use a tea.Cmd if network mounts
	// make these reads slow enough to block input.
	matches := shellMatches(word, m.opts.Terminal.Dir)
	runes := []rune(line)
	m.completion = &shellCompletion{
		before: string(runes[:word.start]), after: string(runes[word.end:]), quote: word.quote,
		tilde: !word.literal && word.quote == 0 && (word.prefix == "~" || strings.HasPrefix(word.prefix, "~/")), matches: matches, selected: -1,
	}
	if len(matches) == 1 {
		m = m.applyCompletion(matches[0], true)
		m.completion = nil
	} else if len(matches) > 1 {
		if reverse {
			m.completion.selected = len(matches) - 1
			m = m.applyCompletion(matches[len(matches)-1], true)
		} else {
			common := []rune(matches[0])
			for _, match := range matches[1:] {
				other := []rune(match)
				i := 0
				for i < min(len(common), len(other)) && common[i] == other[i] {
					i++
				}
				common = common[:i]
			}
			if len(common) > len([]rune(word.prefix)) {
				m = m.applyCompletion(string(common), false)
			}
		}
	}
	if m.completion != nil {
		m.completion.line, m.completion.cursor = m.command.Value(), m.command.Position()
	}
	return m, nil
}

func (m Model) applyCompletion(value string, complete bool) Model {
	c := m.completion
	replacement := quoteCompletion(value, c.quote)
	if c.tilde && strings.HasPrefix(value, "~/") {
		replacement = "~/" + quoteCompletion(value[2:], 0)
	}
	if complete && !strings.HasSuffix(value, "/") && c.after == "" {
		replacement += " "
	}
	m.command.SetValue(c.before + replacement + c.after)
	m.command.SetCursor(len([]rune(c.before + replacement)))
	c.line, c.cursor = m.command.Value(), m.command.Position()
	return m
}

func (m Model) completionView(height int) string {
	if m.completion == nil || height < 1 {
		return ""
	}
	t, c := m.theme(), m.completion
	if len(c.matches) == 0 {
		return t.Muted("No matching commands or paths")
	}
	rows := []string{t.Muted(t.Truncate("Tab next · Shift+Tab previous · Esc dismiss", t.Width()))}
	count := min(5, height-1, len(c.matches))
	start := max(0, min(c.selected-2, len(c.matches)-count))
	for i := start; i < start+count; i++ {
		label := strings.Join(strings.Fields(ansi.Strip(c.matches[i])), " ")
		label = t.Truncate(label, max(1, t.Width()-2))
		if i == c.selected {
			rows = append(rows, t.Accent("> "+label))
		} else {
			rows = append(rows, "  "+label)
		}
	}
	return lipgloss.NewStyle().Width(t.Width()).Render(strings.Join(rows, "\n"))
}
