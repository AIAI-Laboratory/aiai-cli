package command

import (
	"errors"

	"github.com/spf13/cobra"

	sshmanager "github.com/AIAI-Laboratory/aiai-cli/internal/ssh"
)

func (s *state) sshCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "ssh <alias>",
		Short: "Connect using the system OpenSSH client",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if s.json {
				return errors.New("--json is not supported for interactive SSH sessions")
			}
			if err := s.requireSSH(); err != nil {
				return err
			}
			return s.deps.SSH.Connect(cmd.Context(), args[0], sshmanager.Streams{In: s.deps.In, Out: s.deps.Out, Err: s.deps.Err})
		},
	}
}
