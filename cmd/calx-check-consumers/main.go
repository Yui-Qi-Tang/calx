// Command calx-check-consumers runs isolated, bounded Go-only compatibility checks.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/Yui-Qi-Tang/calx/internal/consumercheck"
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
	timeout := flag.Duration("timeout", 10*time.Minute, "overall compatibility-check deadline")
	flag.Parse()
	config.Tools = cadical.Config{
		SolverPath: os.Getenv("CALX_CADICAL"), SolverSHA256: os.Getenv("CALX_CADICAL_SHA256"),
		CheckerPath: os.Getenv("CALX_DRAT"), CheckerSHA256: os.Getenv("CALX_DRAT_SHA256"),
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	summary, checkErr := consumercheck.Run(ctx, config)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(summary); err != nil {
		return errors.Join(checkErr, err)
	}
	return checkErr
}
