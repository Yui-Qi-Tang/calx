package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

const usage = `usage: calx <command> [flags]

Commands:
  project       Convert graph topology to incidence matrices.
  reconstruct   Rebuild topology from incidence matrices.
  encode        Compile a finite model and exact-length query to CNF.
  decode        Check an assignment and decode its finite-model trace.
  solve         Solve CNF supplied as JSON or DIMACS.
  solve-finite  Compile, solve, check and decode a finite-model query.

Use calx <command> --help for command-specific flags.
The CLI reads tool configuration from flags, not CALX_* environment variables.`

type options struct {
	command string
	input   string
	format  string
	id      string
	tools   cadical.Config
}

func parseOptions(args []string, output io.Writer) (options, error) {
	var opts options
	if len(args) == 0 {
		return opts, errors.New(usage)
	}
	if args[0] == "-h" || args[0] == "--help" {
		if _, err := fmt.Fprintln(output, usage); err != nil {
			return opts, err
		}
		return opts, flag.ErrHelp
	}
	opts.command, opts.format = args[0], "json"
	solves := false
	switch opts.command {
	case "project", "reconstruct", "encode", "decode":
	case "solve", "solve-finite":
		solves = true
	default:
		return opts, fmt.Errorf("unknown command %q\n%s", opts.command, usage)
	}

	fs := flag.NewFlagSet("calx "+opts.command, flag.ContinueOnError)
	fs.SetOutput(output)
	fs.StringVar(&opts.input, "in", "-", "input `file`, or - for stdin; at most 16 MiB")
	if opts.command == "solve" {
		fs.StringVar(&opts.format, "format", "json", "CNF input `format`: json or dimacs")
	}
	if solves {
		fs.StringVar(&opts.tools.SolverPath, "solver", "", "required CaDiCaL executable `path`; not a shell command")
		fs.Var(&opts.tools.SolverDigest, "solver-digest", "required expected `digest` of solver executable bytes: "+digest.Format)
		fs.StringVar(&opts.tools.CheckerPath, "checker", "", "required DRAT-trim executable `path`, including runs that return SAT")
		fs.Var(&opts.tools.CheckerDigest, "checker-digest", "required expected `digest` of checker executable bytes: "+digest.Format)
		fs.StringVar(&opts.tools.ArtifactDir, "artifacts", "", "required new run `directory` for tool copies, inputs and checks; parent must exist")
		fs.StringVar(&opts.id, "id", "query", "query subdirectory `name`: 1-80 ASCII letters, digits, _ or -; starts with a letter or digit")
		fs.DurationVar(&opts.tools.Limits.Timeout, "timeout", 30*time.Second, "solve `duration` shared by solver and checker; excludes compilation and tool setup; 0 uses 30s")
	}
	if err := fs.Parse(args[1:]); err != nil {
		return opts, err
	}
	if fs.NArg() != 0 {
		return opts, fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	if opts.format != "json" && opts.format != "dimacs" {
		return opts, fmt.Errorf("unsupported input format %q: use json or dimacs", opts.format)
	}
	if solves {
		for _, required := range []struct{ name, value string }{
			{"solver", opts.tools.SolverPath},
			{"solver-digest", opts.tools.SolverDigest.String()},
			{"checker", opts.tools.CheckerPath},
			{"checker-digest", opts.tools.CheckerDigest.String()},
			{"artifacts", opts.tools.ArtifactDir},
		} {
			if required.value == "" {
				return opts, fmt.Errorf("--%s is required for %s", required.name, opts.command)
			}
		}
		if opts.tools.Limits.Timeout < 0 {
			return opts, errors.New("--timeout must not be negative")
		}
	}
	return opts, nil
}
