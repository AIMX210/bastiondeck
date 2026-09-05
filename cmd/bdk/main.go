// Command bdk is the BastionDeck command-line client.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"bastiondeck/internal/cli"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	app := cli.New()
	os.Exit(app.Run(ctx, os.Args[1:]))
}
