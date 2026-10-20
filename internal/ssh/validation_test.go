package ssh

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestValidationAndNormalization(t *testing.T) {
	alias, err := NormalizeAlias("  Production_1 ")
	if err != nil || alias != "production_1" {
		t.Fatalf("NormalizeAlias() = %q, %v", alias, err)
	}
	host, err := NormalizeHost("[2001:0db8::1]")
	if err != nil || host != "2001:db8::1" {
		t.Fatalf("NormalizeHost(IPv6) = %q, %v", host, err)
	}
	for _, value := range []string{"-oProxyCommand=bad", "user@host", "host name", "bad/host", "bad..host"} {
		if _, err := NormalizeHost(value); err == nil {
			t.Errorf("NormalizeHost(%q) unexpectedly succeeded", value)
		}
	}
	for _, value := range []string{"-root", "user name", "user@host"} {
		if _, err := ValidateUsername(value); err == nil {
			t.Errorf("ValidateUsername(%q) unexpectedly succeeded", value)
		}
	}
	for _, port := range []int{-1, 0, 65536} {
		if err := ValidatePort(port); err == nil {
			t.Errorf("ValidatePort(%d) unexpectedly succeeded", port)
		}
	}
}

func TestNormalizeKeyPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_test")
	if err := os.WriteFile(path, []byte("not-real-key-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := NormalizeKeyPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("NormalizeKeyPath() = %q, want %q", got, path)
	}
	if _, err := NormalizeKeyPath(path + ".missing"); err == nil {
		t.Fatal("missing key unexpectedly accepted")
	}
	if _, err := NormalizeKeyPath(filepath.Dir(path)); err == nil {
		t.Fatal("directory key unexpectedly accepted")
	}
}

func TestBuildArgsPreventsOptionInjection(t *testing.T) {
	server := Server{Host: "2001:db8::1", Port: 2222, Username: "deploy", IdentityFile: "/keys/id"}
	got := BuildArgs(server)
	want := []string{"-p", "2222", "-i", "/keys/id", "-l", "deploy", "--", "2001:db8::1"}
	if len(got) != len(want) {
		t.Fatalf("BuildArgs() = %#v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("BuildArgs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestOpenSSHMissing(t *testing.T) {
	runner := OpenSSHRunner{LookPath: func(string) (string, error) { return "", os.ErrNotExist }}
	_, err := runner.Run(context.Background(), nil, Streams{})
	if !errors.Is(err, ErrSSHMissing) {
		t.Fatalf("Run() error = %v", err)
	}
}
