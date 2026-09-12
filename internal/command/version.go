package command

import (
	"fmt"

	"github.com/spf13/cobra"
)

func (s *state) versionCommand() *cobra.Command {
	return &cobra.Command{Use: "version", Short: "Print the AIAI version", Args: cobra.NoArgs, Run: func(_ *cobra.Command, _ []string) {
		s.version = s.deps.Version
		if !s.json {
			fmt.Fprintln(s.deps.Out, "aiai", s.deps.Version)
		}
	}}
}
