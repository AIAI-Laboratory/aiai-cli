package platform

import (
	"io"

	"golang.org/x/term"
)

func IsTerminal(stream any) bool {
	f, ok := stream.(interface{ Fd() uintptr })
	return ok && term.IsTerminal(int(f.Fd()))
}

func Interactive(in io.Reader, out io.Writer) bool { return IsTerminal(in) && IsTerminal(out) }
