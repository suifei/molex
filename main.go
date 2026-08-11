package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/suifei/molex/cmd"
)

var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := cmd.Execute(ctx, version); err != nil {
		fmt.Fprintln(os.Stderr, "molex:", err)
		os.Exit(1)
	}
}
