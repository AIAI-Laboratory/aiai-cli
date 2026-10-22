package platform

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestShellCommandsKeepDirectoryAndExecuteNativeSyntax(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Setenv("SHELL", "/bin/sh")
	}
	root := t.TempDir()
	destination := filepath.Join(root, "folder with spaces")
	if err := os.Mkdir(destination, 0o700); err != nil {
		t.Fatal(err)
	}
	session := &ShellSession{Dir: root}
	run := func(line string, wantError bool) string {
		t.Helper()
		var output bytes.Buffer
		command := &ShellCommand{Context: context.Background(), Session: session, Line: line, In: strings.NewReader("\n"), Out: &output, Err: &output}
		err := command.Run()
		if (err != nil) != wantError || TerminalRunning.Load() || !strings.Contains(output.String(), "Press Enter to return") {
			t.Fatalf("%s: %v %s", line, err, output.String())
		}
		return output.String()
	}
	run(`cd "folder with spaces"`, false)
	canonical, err := filepath.EvalSymlinks(destination)
	if err != nil {
		t.Fatal(err)
	}
	if session.Dir != canonical {
		t.Fatalf("cd did not update cwd: %q != %q", session.Dir, canonical)
	}
	line := `printf 'hello world\n' | tr a-z A-Z > result.txt`
	if runtime.GOOS == "windows" {
		line = `'HELLO WORLD' | Out-File -Encoding ascii result.txt`
	}
	run(line, false)
	content, err := os.ReadFile(filepath.Join(destination, "result.txt"))
	if err != nil || strings.TrimSpace(string(content)) != "HELLO WORLD" {
		t.Fatalf("pipeline or redirection did not execute: %q %v", content, err)
	}
	run(`cd missing-directory-that-does-not-exist`, true)
	if session.Dir != canonical {
		t.Fatal("failed cd changed session directory")
	}
	if runtime.GOOS != "windows" {
		run("cd -", false)
		canonicalRoot, _ := filepath.EvalSymlinks(root)
		if session.Dir != canonicalRoot {
			t.Fatal("cd - did not restore previous directory")
		}
		run(`cd "folder with spaces"; false`, true)
		if session.Dir != canonical {
			t.Fatal("command failure lost a successful cd")
		}
	}
	run("aiai-command-that-does-not-exist", true)
}

func TestCapturedShellStreamsOutputAndCancelsChildProcesses(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process-group cancellation")
	}
	t.Setenv("SHELL", "/bin/sh")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	command := &ShellCommand{
		Context: ctx, Session: &ShellSession{Dir: t.TempDir()}, Capture: true,
		Line: "printf 'stdout ready\\n'; printf 'stderr ready\\n' >&2; sleep 30",
		Out:  writer, Err: writer,
	}
	done := make(chan error, 1)
	go func() { done <- command.Run(); _ = writer.Close() }()
	scanner := bufio.NewScanner(reader)
	for _, want := range []string{"stdout ready", "stderr ready"} {
		if !scanner.Scan() || scanner.Text() != want {
			t.Fatalf("output did not stream before completion: %q", scanner.Text())
		}
	}
	cancel()
	select {
	case err := <-done:
		if err == nil || scanner.Scan() || TerminalRunning.Load() {
			t.Fatal("captured command did not stop cleanly")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("child process survived cancellation")
	}
}

func TestCapturedShellKeepsDirectoryWithoutReturnPrompt(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Setenv("SHELL", "/bin/sh")
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "child"), 0o700); err != nil {
		t.Fatal(err)
	}
	session := &ShellSession{Dir: root}
	var output bytes.Buffer
	command := &ShellCommand{Context: context.Background(), Session: session, Line: "cd child", Capture: true, Out: &output, Err: &output}
	if err := command.Run(); err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(filepath.Join(root, "child"))
	if session.Dir != want || strings.Contains(output.String(), "Press Enter") {
		t.Fatalf("inline cd failed or requested native input: %q %s", session.Dir, output.String())
	}
}
