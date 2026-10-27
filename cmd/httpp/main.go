package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/stuft2/httpp/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit((cli.Runner{}).Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
