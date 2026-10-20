package command

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/AIAI-Laboratory/aiai-cli/internal/output"
	sshmanager "github.com/AIAI-Laboratory/aiai-cli/internal/ssh"
)

func (s *state) keyCommand() *cobra.Command {
	key := &cobra.Command{Use: "key", Short: "Generate, store, and find SSH keys", Args: cobra.NoArgs}
	key.AddCommand(s.keyGenerateCommand(), s.keyListCommand(), s.keyFindCommand(), s.keyShowCommand())
	return key
}

func (s *state) keyGenerateCommand() *cobra.Command {
	var keyType, comment string
	var bits int
	var noPassphrase bool
	cmd := &cobra.Command{
		Use:   "generate <name>",
		Short: "Generate a key pair in AIAI's managed key directory",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := s.requireSSH(); err != nil {
				return err
			}
			if !s.interactive() && !noPassphrase {
				return errors.New("non-interactive key generation requires --no-passphrase; otherwise run in a terminal to enter a passphrase")
			}
			key, err := s.deps.SSH.GenerateKey(cmd.Context(), sshmanager.KeyGenerateRequest{
				Name: args[0], Type: keyType, Bits: bits, Comment: comment, NoPassphrase: noPassphrase,
			}, sshmanager.Streams{In: s.deps.In, Out: s.human(), Err: s.deps.Err})
			if err != nil {
				return err
			}
			s.ssh = &output.SSH{Action: "key-generate", Key: &key}
			if !s.json {
				printKey(s.human(), key)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&keyType, "type", "ed25519", "Key type: ed25519 or rsa")
	cmd.Flags().IntVar(&bits, "bits", 4096, "RSA key size (3072-16384)")
	cmd.Flags().StringVar(&comment, "comment", "", "Public key comment")
	cmd.Flags().BoolVar(&noPassphrase, "no-passphrase", false, "Generate without a passphrase (explicitly less secure)")
	return cmd
}

func (s *state) keyListCommand() *cobra.Command {
	return &cobra.Command{Use: "list", Short: "List managed and standard SSH key pairs", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		return s.findKeys(cmd, "", "key-list")
	}}
}

func (s *state) keyFindCommand() *cobra.Command {
	return &cobra.Command{Use: "find [query]", Short: "Find SSH keys by name, type, comment, fingerprint, or path", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		query := ""
		if len(args) == 1 {
			query = args[0]
		}
		return s.findKeys(cmd, query, "key-find")
	}}
}

func (s *state) keyShowCommand() *cobra.Command {
	return &cobra.Command{Use: "show <name-or-path>", Short: "Show SSH key metadata without private key content", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if err := s.requireSSH(); err != nil {
			return err
		}
		key, err := s.deps.SSH.FindKey(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		s.ssh = &output.SSH{Action: "key-show", Key: &key}
		if !s.json {
			printKey(s.human(), key)
		}
		return nil
	}}
}

func (s *state) findKeys(cmd *cobra.Command, query, action string) error {
	if err := s.requireSSH(); err != nil {
		return err
	}
	keys, err := s.deps.SSH.FindKeys(cmd.Context(), query)
	if err != nil {
		return err
	}
	s.ssh = &output.SSH{Action: action, Keys: &keys}
	if !s.json {
		printKeys(s.human(), keys)
	}
	return nil
}

func printKey(w io.Writer, key sshmanager.Key) {
	fmt.Fprintf(w, "Name: %s\nType: %s\nFingerprint: %s\nPrivate key: %s\nPublic key: %s\nManaged: %t\n", key.Name, key.Type, key.Fingerprint, key.PrivatePath, key.PublicPath, key.Managed)
	if key.Comment != "" {
		fmt.Fprintf(w, "Comment: %s\n", key.Comment)
	}
}

func printKeys(w io.Writer, keys []sshmanager.Key) {
	if len(keys) == 0 {
		fmt.Fprintln(w, "No SSH key pairs found.")
		return
	}
	fmt.Fprintf(w, "%-20s %-18s %-8s %s\n", "NAME", "TYPE", "SOURCE", "FINGERPRINT")
	for _, key := range keys {
		source := "standard"
		if key.Managed {
			source = "managed"
		}
		keyType := strings.TrimPrefix(key.Type, "ssh-")
		fmt.Fprintf(w, "%-20s %-18s %-8s %s\n", key.Name, keyType, source, key.Fingerprint)
	}
	fmt.Fprintln(w, "\n"+strconv.Itoa(len(keys))+" key pair(s). Private key contents are never displayed.")
}
