package ssh

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type fakeKeyTool struct {
	args []string
	err  error
}

func (f *fakeKeyTool) Generate(_ context.Context, args []string, _ Streams) error {
	f.args = append([]string(nil), args...)
	if f.err != nil {
		return f.err
	}
	target := ""
	comment := ""
	keyType := "ed25519"
	for i := 0; i+1 < len(args); i++ {
		switch args[i] {
		case "-f":
			target = args[i+1]
		case "-C":
			comment = args[i+1]
		case "-t":
			keyType = args[i+1]
		}
	}
	if err := os.WriteFile(target, []byte("PRIVATE-MATERIAL"), 0o600); err != nil {
		return err
	}
	return os.WriteFile(target+".pub", []byte("ssh-"+keyType+" AQID "+comment+"\n"), 0o644)
}

func TestGenerateStoreAndFindKeys(t *testing.T) {
	service, _ := newTestService(t, &fakeRunner{})
	tool := &fakeKeyTool{}
	service.KeyTool = tool
	service.KeyDir = filepath.Join(t.TempDir(), "keys")
	key, err := service.GenerateKey(context.Background(), KeyGenerateRequest{Name: "Laptop", Type: "ed25519", Comment: "work laptop", NoPassphrase: true}, Streams{})
	if err != nil {
		t.Fatal(err)
	}
	if key.Name != "laptop" || key.Type != "ssh-ed25519" || key.Comment != "work laptop" || !key.Managed || !strings.HasPrefix(key.Fingerprint, "SHA256:") {
		t.Fatalf("generated key = %+v", key)
	}
	if !containsArgs(tool.args, "-N", "") {
		t.Fatalf("ssh-keygen args do not contain explicit empty passphrase: %#v", tool.args)
	}
	if runtime.GOOS != "windows" {
		if info, statErr := os.Stat(key.PrivatePath); statErr != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("private key mode = %v, %v", info, statErr)
		}
		if info, statErr := os.Stat(key.PublicPath); statErr != nil || info.Mode().Perm() != 0o644 {
			t.Fatalf("public key mode = %v, %v", info, statErr)
		}
	}
	keys, err := service.FindKeys(context.Background(), "work laptop")
	if err != nil || len(keys) != 1 || keys[0].PrivatePath != key.PrivatePath {
		t.Fatalf("FindKeys() = %+v, %v", keys, err)
	}
	found, err := service.FindKey(context.Background(), "laptop")
	if err != nil || found.Fingerprint != key.Fingerprint {
		t.Fatalf("FindKey() = %+v, %v", found, err)
	}
	publicKey, err := service.PublicKey(context.Background(), "laptop")
	if err != nil || publicKey != "ssh-ed25519 AQID work laptop" {
		t.Fatalf("PublicKey() = %q, %v", publicKey, err)
	}
	if _, err := service.GenerateKey(context.Background(), KeyGenerateRequest{Name: "laptop"}, Streams{}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate generation error = %v", err)
	}
	if err := service.RemoveKey(context.Background(), "laptop"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{key.PrivatePath, key.PublicPath} {
		if _, statErr := os.Stat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("removed key path still exists: %s (%v)", path, statErr)
		}
	}
}

func TestGenerateKeyValidationAndCleanup(t *testing.T) {
	service, _ := newTestService(t, &fakeRunner{})
	service.KeyDir = filepath.Join(t.TempDir(), "keys")
	for _, request := range []KeyGenerateRequest{
		{Name: "-bad"},
		{Name: "key", Type: "dsa"},
		{Name: "key", Type: "rsa", Bits: 1024},
	} {
		if _, err := service.GenerateKey(context.Background(), request, Streams{}); err == nil {
			t.Fatalf("GenerateKey(%+v) unexpectedly succeeded", request)
		}
	}
	tool := &fakeKeyTool{err: errors.New("generation failed")}
	service.KeyTool = tool
	_, err := service.GenerateKey(context.Background(), KeyGenerateRequest{Name: "cleanup"}, Streams{})
	if err == nil {
		t.Fatal("failed generator returned no error")
	}
	if _, statErr := os.Stat(filepath.Join(service.KeyDir, "cleanup")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("partial private key remains: %v", statErr)
	}
}

func TestOpenSSHKeyToolMissing(t *testing.T) {
	tool := OpenSSHKeyTool{LookPath: func(string) (string, error) { return "", os.ErrNotExist }}
	if err := tool.Generate(context.Background(), nil, Streams{}); !errors.Is(err, ErrSSHMissing) {
		t.Fatalf("Generate() error = %v", err)
	}
}

func containsArgs(args []string, pair ...string) bool {
	for i := 0; i+len(pair) <= len(args); i++ {
		matched := true
		for j := range pair {
			if args[i+j] != pair[j] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}
