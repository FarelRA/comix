package main

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/FarelRA/comix/internal/cli"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	cli.SetRootContext(ctx)
	cli.Execute()
}
