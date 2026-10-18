// Command calx exposes the Calx library to independent and non-Go consumers.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/Yui-Qi-Tang/calx/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	status := cli.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop()
	os.Exit(status)
}
