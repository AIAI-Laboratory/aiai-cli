package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/AIAI-Laboratory/aiai-cli/internal/app"
	"github.com/AIAI-Laboratory/aiai-cli/internal/platform"
)

var (
	version        = "dev"
	apiURL         string
	githubClientID string
)

func main() {
	ctx, stop := context.WithCancel(context.Background())
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-interrupts:
				if !platform.TerminalRunning.Load() {
					stop()
				}
			}
		}
	}()
	code := app.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, version, apiURL, githubClientID)
	stop()
	signal.Stop(interrupts)
	os.Exit(code)
}
