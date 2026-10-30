package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/Yui-Qi-Tang/calx/internal/toolmanifest"
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
  pin-tools     Hash the solver and checker into a tool manifest for --tools.

Use calx <command> --help for command-specific flags.
The CLI reads tool configuration from flags or a --tools manifest,
not CALX_* environment variables.`

type options struct {
	command   string
	input     string
	format    string
	id        string
	toolsFile string // --tools manifest; replaces the four tool flags
	output    string // pin-tools manifest destination
	tools     cadical.Config
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
	case "pin-tools":
		return parsePinTools(opts, args[1:], output)
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
		fs.StringVar(&opts.toolsFile, "tools", "", "tool manifest `file` written by pin-tools; replaces --solver, --solver-digest, --checker and --checker-digest")
		fs.StringVar(&opts.tools.SolverPath, "solver", "", "CaDiCaL executable `path`, not a shell command; required without --tools")
		fs.Var(&opts.tools.SolverDigest, "solver-digest", "expected `digest` of solver executable bytes, required without --tools: SHA-256, 64 lowercase hex characters")
		fs.StringVar(&opts.tools.CheckerPath, "checker", "", "DRAT-trim executable `path`, also used for runs that return SAT; required without --tools")
		fs.Var(&opts.tools.CheckerDigest, "checker-digest", "expected `digest` of checker executable bytes, required without --tools: SHA-256, 64 lowercase hex characters")
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
		toolFlags := []struct{ name, value string }{
			{"solver", opts.tools.SolverPath},
			{"solver-digest", opts.tools.SolverDigest.String()},
			{"checker", opts.tools.CheckerPath},
			{"checker-digest", opts.tools.CheckerDigest.String()},
		}
		if opts.toolsFile != "" {
			for _, f := range toolFlags {
				if f.value != "" {
					return opts, fmt.Errorf("--tools cannot be combined with --%s", f.name)
				}
			}
			m, err := toolmanifest.Load(opts.toolsFile)
			if err != nil {
				return opts, err
			}
			opts.tools.SolverPath, opts.tools.SolverDigest = m.Solver.Path, m.Solver.Digest
			opts.tools.CheckerPath, opts.tools.CheckerDigest = m.Checker.Path, m.Checker.Digest
			toolFlags = nil
		}
		for _, required := range append(toolFlags, struct{ name, value string }{"artifacts", opts.tools.ArtifactDir}) {
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

func parsePinTools(opts options, args []string, output io.Writer) (options, error) {
	fs := flag.NewFlagSet("calx pin-tools", flag.ContinueOnError)
	fs.SetOutput(output)
	fs.StringVar(&opts.tools.SolverPath, "solver", "", "required CaDiCaL executable `path` to hash")
	fs.StringVar(&opts.tools.CheckerPath, "checker", "", "required DRAT-trim executable `path` to hash")
	fs.StringVar(&opts.output, "out", "", "required new manifest `file`; an existing file is never overwritten")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if fs.NArg() != 0 {
		return opts, fmt.Errorf("unexpected positional arguments: %v", fs.Args())
	}
	for _, required := range []struct{ name, value string }{
		{"solver", opts.tools.SolverPath},
		{"checker", opts.tools.CheckerPath},
		{"out", opts.output},
	} {
		if required.value == "" {
			return opts, fmt.Errorf("--%s is required for pin-tools", required.name)
		}
	}
	return opts, nil
}
