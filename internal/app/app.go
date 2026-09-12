package app

import (
	"context"
	"fmt"
	"io"

	"github.com/AIAI-Laboratory/aiai-cli/internal/command"
)

func Run(ctx context.Context, args []string, in io.Reader, out, errOut io.Writer, version string) int {
	deps, err := dependencies(in, out, errOut, version)
	if err != nil {
		fmt.Fprintln(errOut, "Cannot load embedded templates:", err)
		return 4
	}
	return command.Run(ctx, args, deps)
}
