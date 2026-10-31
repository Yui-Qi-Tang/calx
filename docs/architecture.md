# Architecture

Calx is one module with separately importable packages and a thin CLI. Public
packages depend on no consumer. The current implementation has no
third-party Go dependency. `sat/cadical` uses `internal/toolrun` and `sat`;
`finite`, `automaton`, `sat/cadical`, the internal tool packages and the CLI
package also use `internal/jsoncompat` for their JSON bytes;
`finite` and `automaton` use `sat`; `lga` uses only `automaton`; `incidence` is
independent of all of them. The leaf package `digest` owns content identifiers,
their current algorithm and textual format.
`finite`, `automaton`, `sat`, `sat/cadical`, tool execution and CLI code use
that shared type.
CLI wiring lives in `internal/cli` and `cmd/calx`.

JSON goes through the standard library's `encoding/json/v2`.
`internal/jsoncompat` sets the options that reproduce the bytes of the earlier
`encoding/json` output: sorted map keys, null for nil slices and maps, HTML and
JavaScript escaping, and durations as nanoseconds. Digests, retained artifacts
and CLI output therefore did not change with the engine. `internal/jsoninput`
decodes CLI input strictly with v2 in a single token scan plus one unmarshal;
where fields are required, a presence check unmarshals the top-level object
again.
`internal/toolmanifest` owns the `calx-tools/v1` manifest that `calx` reads and
writes.

## Package dependencies

Arrows mean “imports”; consumers choose the packages they need.

```mermaid
flowchart TD
    Consumer["Consumer Go program"] --> Incidence["incidence"]
    Consumer --> Finite["finite"]
    Consumer --> LGA["lga"]
    Consumer --> Automaton["automaton"]
    LGA --> Automaton
    Consumer --> Cadical["sat/cadical"]
    CLI["cmd/calx + internal/cli"] --> Incidence
    CLI --> Finite
    CLI --> Cadical
    Finite --> SAT["sat"]
    Automaton --> SAT
    Cadical --> SAT
    Cadical --> Toolrun["internal/toolrun"]
    Toolrun --> SAT
    SAT --> Digest["digest"]
    Finite --> Digest
    Automaton --> Digest
    Cadical --> Digest
    Toolrun --> Digest
    CLI --> Digest
    CLI --> SAT
    CLI --> Toolrun
    CLI --> JSONInput["internal/jsoninput"]
    CLI --> Manifest["internal/toolmanifest"]
    Manifest --> JSONInput
    Manifest --> Digest
```

`internal/jsoncompat`, imported by `finite`, `automaton`, `sat/cadical`,
`internal/toolrun`, `internal/toolmanifest` and the CLI, is left out of the
diagram.

## Data flow

The [illustrated internal execution flow](execution-flow.md) follows concrete
functions, an exact CNF example, solver verification and result decoding.

Arrows here mean “produces or supplies data.” The two matrix routes have
separate inputs and meanings.

```mermaid
flowchart LR
    Graph["Explicit graph topology"] --> Project["incidence.Project"]
    Project --> Projection["Source / Target / axes / relation masks"]
    Projection --> Reconstruct["Projection.Graph"] --> Rebuilt["Reconstructed topology"]
    Model["Categorical model + explicit query"] --> Compile["finite.Compile"]
    Compile --> Encoding["Owned Encoding: operation matrix + CNF + map"]
    Encoding --> CNF["CNF copy"]
    Declared["Consumer-supplied CNF"] --> CNF
    CNF --> Solve["Runner.Solve + model/proof checks"]
    Solve --> Result["Status + validation + artifacts"]
    Result -->|SAT assignment| Decode["Encoding.Decode"]
    Encoding --> Decode
    Decode --> Trace["Model trace"]
```

The finite Decode route requires its matching Encoding. A raw CNF consumer uses
its own interpretation map. Full structures and JSON fields are documented in
[data structures](data-structures.md); see [graph-to-matrix](graph-to-matrix.md)
for the topology projection steps.

## Matrix meanings

Two matrix meanings are deliberately explicit:

- `incidence.Projection`: node rows, independently identified edge columns,
  separate source/target matrices and opaque relation masks. It preserves
  topology, including isolated nodes, self-loops and parallel edges. Full source
  records, derivation manifests, provenance, scope and truncation stay in caller
  sidecars. A round trip is not a new snapshot identity or a completeness claim.
- `finite.Matrix`: action rows, categorical coordinates for guard/effect columns,
  and field columns for frames. Zero guard cells impose no requirement. The
  matrix needs its axes and forbidden patterns to reconstruct a finite model.

An incidence matrix alone cannot determine the semantics of a SAT formula.
A consumer with normalized declarations can keep its own compiler, a
categorical model can use `finite`, and typed or copy profiles can keep their
own encoders and submit their CNF through the CLI. No silent bool/int/string coercion is supported.

`finite` owns pure model validation and single-query encoding/decoding. Caller
policy decides horizons, enumeration, stopping conditions, recovery targets and
acceptance of a collection. `finite` encodes one fixed semantics,
`finite.Profile` (simple first-hit runs of exact length), and records it in
every encoding digest; a CLI request names it in `query.profile`, and the CLI
rejects any other value. UNSAT is relative to that formula, including its
blocking clauses. Independent original-rule receivers remain outside Calx.

The CLI is a transport/composition boundary, not a server. Its versioned JSON
and DIMACS interfaces are implemented now. Protobuf generation is deferred:
freeze semantic profiles and mappings before introducing a second wire format.
A future codec can depend on generated DTOs and public Calx types; algorithms
need not depend on generated types. Existing consumer formats and frozen
consumer artifacts are not rewritten by this module.

There is no SMT, arbitrary numerical matrix algebra, physics, automatic source
interpretation, execution, persistence, or evidence-admission API in v0.

## v0.1 additions

v0.1 adds the `automaton` and `lga` packages and the CLI tool manifest. It adds
no `sat` or `sat/cadical` capability. Since then their public types have lost
`Result.Profile` and their JSON tags; the CLI, `input.json` and `outcome.json`
define their JSON forms with private structs.

- Implemented: `automaton` encodes exact-length runs of a caller-supplied
  synchronous Boolean automaton. The caller implements `automaton.System`, so
  Calx sees cell and bit indices and the local rule, never their meaning. The
  consumer solves the CNF with its own `cadical.Runner`, decodes with
  `Encoding.Decode` and checks the result with `automaton.Replay`. A
  `Violates` query asks whether a signed sum of bits can change at any step of
  the run; UNSAT is a conservation proof bounded to that run.
- Implemented: `lga` provides the HPP and FHP-I lattice gases as
  `automaton.System` values, with bounce-back walls, conserved-quantity
  helpers, density and velocity averaging, and bit-packed `Run` methods for
  fast forward simulation. It imports only `automaton`. It does not claim to
  reproduce a real fluid.
- Implemented: the CLI's `pin-tools` command writes a tool manifest that solve
  commands accept through `--tools`, as an alternative to the four tool flags.
- The v0 exclusions above still apply to everything else, including SMT,
  arbitrary numerical matrix algebra and persistence.
