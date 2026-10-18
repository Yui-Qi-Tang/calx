# Calx

**Experimental v0, unreleased. Requires Go 1.27.2.**

Calx is a domain-neutral Go module for graph incidence projection, bounded
categorical-model encoding and checked Boolean SAT solving. Packages can be
imported independently. The command-line program exposes the same implementation
to Python and other consumers. No evidence service or database is required.

| Package | Responsibility |
| --- | --- |
| `incidence` | Directed multigraph topology ↔ source/target incidence matrices, IDs and opaque relation masks |
| `finite` | Validated categorical model → operation matrix → exact-length CNF; assignment → trace |
| `sat` | CNF, DIMACS, total-assignment checking, limits and explicit validation records |
| `sat/cadical` | Pinned CaDiCaL and DRAT-trim processes, checked results and retained artifacts |

The module currently uses only the Go standard library. Consumers can adapt
`graph` snapshots into `incidence.Graph`; graph traversal remains the graph
package's responsibility. Calx never interprets evidence relations as action
rules or infers a model from source text. See [architecture](docs/architecture.md)
and [contracts](docs/contracts.md).

## Library

Import only the required packages, for example:

```go
import (
    "github.com/Yui-Qi-Tang/calx/finite"
    "github.com/Yui-Qi-Tang/calx/sat/cadical"
)
```

`finite.Compile(ctx, model, query, options)` validates and compiles one query.
Its `CNF()` returns an independently owned formula for `cadical.Runner.Solve`.
Always check the returned error, then call `result.Require(...)` with the
consumer's guarantee requirements. On checked SAT, `Encoding.Decode` checks the
assignment against the original compiled formula and returns a trace. The
consumer still independently replays its original rules.

The runnable example in `finite/finite_test.go` needs no external solver.
Consumers own their narrow interfaces; Calx exports concrete implementations,
not a universal solver registry or fallback mechanism.

## Standalone command

```sh
go build -o bin/calx ./cmd/calx
bin/calx project --in examples/graph.json
bin/calx encode --in examples/finite.json
```

For actual solving, supply trusted executables and their lowercase SHA-256 pins.
Calx does not download or bundle them. The artifact parent must exist and the
specific run directory must be new:

```sh
mkdir -p artifacts
bin/calx solve-finite --in examples/finite.json \
  --solver "$CALX_CADICAL" --solver-sha256 "$CALX_CADICAL_SHA256" \
  --checker "$CALX_DRAT" --checker-sha256 "$CALX_DRAT_SHA256" \
  --artifacts artifacts/example
```

Other commands: `reconstruct`, `decode`, and `solve --format dimacs`.
`--in -` reads stdin. All command output is a versioned JSON envelope; exit 0
means that command met its checks, exit 1 is failure, and exit 2 is a flag/usage
error. An error response can retain a backend SAT/UNSAT label for diagnosis;
inspect `outcome`, `error` and validation records together.

## Verification

```sh
make verify
make race
```

Opt-in actual solver acceptance, with the four tool variables above and
`CALX_ARTIFACTS` naming an existing fresh acceptance parent:

```sh
go test -tags resolverintegration -count=1 -run TestRealTools ./sat/cadical
```

[Consumer conformance](docs/compatibility.md) exercises selected AHE Core/MCP,
Pouch Product and Python Lab source snapshots in fresh ignored directories.
It does not switch the formal consumers. [Validation](docs/validation.md)
records the observed scope and limitations.

Codebase-memory is enabled for this repository. `.codebase-memory/`, `bin/`,
`artifacts/` and `work/` are local ignored outputs. Source, small examples and
regression fixtures remain tracked candidates.

License and source attribution: [MIT](LICENSE), [NOTICE](NOTICE).
