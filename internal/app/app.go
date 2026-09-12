package app

import (
	"context"
	"fmt"
	"io"

	"github.com/AIAI-Laboratory/aiai-cli/internal/command"
)

func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer, version, apiURL, githubClientID string) int {
	deps, err := dependencies(in, out, errOut, version, apiURL, githubClientID)
	if err != nil {
		fmt.Fprintln(errOut, "Cannot initialize AIAI CLI:", err)
		return 4
	}
	return command.Run(ctx, args, deps)
}
