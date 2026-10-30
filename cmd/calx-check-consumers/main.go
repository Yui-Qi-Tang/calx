// Command calx-check-consumers runs isolated, bounded Go-only compatibility checks.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/Yui-Qi-Tang/calx/internal/consumercheck"
	"github.com/Yui-Qi-Tang/calx/internal/jsoncompat"
	"github.com/Yui-Qi-Tang/calx/internal/toolmanifest"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	var config consumercheck.Config
	flag.StringVar(&config.CalxDir, "calx", ".", "Calx checkout")
	flag.StringVar(&config.CoreDir, "core", "", "AHE Core checkout")
	flag.StringVar(&config.MCPDir, "mcp", "", "AHE MCP checkout")
	flag.StringVar(&config.PouchDir, "pouch", "", "Pouch Product checkout")
	flag.StringVar(&config.LabDir, "lab", "", "Pouch Lab source root (hash verification only)")
	flag.StringVar(&config.OutputDir, "out", "", "new output directory outside consumer checkouts")
	tools := flag.String("tools", "", "required tool manifest written by calx pin-tools")
	timeout := flag.Duration("timeout", 10*time.Minute, "overall compatibility-check deadline")
	flag.Parse()
	if *tools == "" {
		return errors.New("--tools is required")
	}
	m, err := toolmanifest.Load(*tools)
	if err != nil {
		return err
	}
	config.Tools = cadical.Config{
		SolverPath: m.Solver.Path, SolverDigest: m.Solver.Digest,
		CheckerPath: m.Checker.Path, CheckerDigest: m.Checker.Digest,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	summary, checkErr := consumercheck.Run(ctx, config)
	if err := jsoncompat.Write(os.Stdout, summary, "  "); err != nil {
		return errors.Join(checkErr, err)
	}
	return checkErr
}
