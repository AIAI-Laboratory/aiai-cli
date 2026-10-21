package git

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

// ParseCommand accepts Git arguments with quotes and escapes. It never expands
// variables, command substitutions, glob patterns, pipes or shell operators.
func ParseCommand(line string) ([]string, error) {
	if strings.ContainsAny(line, "\x00\r\n") || len(line) > 8192 {
		return nil, fmt.Errorf("enter one Git command, at most 8192 bytes")
	}
	args := []string{}
	var word strings.Builder
	var quote rune
	escaped, started := false, false
	for _, char := range line {
		switch {
		case escaped:
			word.WriteRune(char)
			escaped = false
		case char == '\\' && quote != '\'':
			escaped, started = true, true
		case quote != 0:
			if char == quote {
				quote = 0
			} else {
				word.WriteRune(char)
			}
		case char == '\'' || char == '"':
			quote, started = char, true
		case unicode.IsSpace(char):
			if started {
				args = append(args, word.String())
				word.Reset()
				started = false
			}
		default:
			word.WriteRune(char)
			started = true
		}
	}
	if escaped || quote != 0 {
		return nil, fmt.Errorf("close the quote or finish the escape before running Git")
	}
	if started {
		args = append(args, word.String())
	}
	if len(args) > 0 && args[0] == "git" {
		args = args[1:]
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("enter a Git command")
	}
	return args, nil
}

func FormatArguments(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		if arg != "" && strings.IndexFunc(arg, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("-._/@:{}=+,", r)
		}) < 0 {
			quoted[i] = arg
		} else {
			quoted[i] = "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
		}
	}
	return strings.Join(quoted, " ")
}

func gitEnvironment() []string {
	env := []string{}
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		if key == "GIT_DIR" || key == "GIT_WORK_TREE" || key == "GIT_INDEX_FILE" || key == "GIT_COMMON_DIR" || key == "GIT_CONFIG_COUNT" || strings.HasPrefix(key, "GIT_CONFIG_KEY_") || strings.HasPrefix(key, "GIT_CONFIG_VALUE_") {
			continue
		}
		env = append(env, item)
	}
	return env
}

// InteractiveCommand prepares a native Git process for terminal handoff. Git
// retains its credential helpers, SSH, hooks, pager, editor and signing settings.
// Execution belongs to the TUI so it can release and restore terminal ownership.
func (s *Service) InteractiveCommand(ctx context.Context, path string, args []string) (*exec.Cmd, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("enter a Git command")
	}
	for _, arg := range args {
		if strings.ContainsRune(arg, 0) {
			return nil, fmt.Errorf("git arguments cannot contain NUL")
		}
	}
	root, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	exe := s.Executable
	if exe == "" {
		exe = "git"
	}
	cmd := exec.CommandContext(ctx, exe, append([]string{"-C", root}, args...)...)
	cmd.Dir = root
	cmd.Env = append(gitEnvironment(), "GIT_OPTIONAL_LOCKS=1", "GIT_TERMINAL_PROMPT=1")
	return cmd, nil
}
