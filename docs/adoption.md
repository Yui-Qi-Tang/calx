# Adopting Calx in AHE and Pouch repositories

Calx is meant to replace the solver, encoding and projection code that AHE and
Pouch each still carry. As of 2026-10-26 none of the four consumer repositories
imports it. This page lists what stands in the way, with the evidence behind
each item, and what each consumer would change. It complements the bounded
[consumer conformance check](compatibility.md), which already runs the
consumers' own compiler and planning sources against Calx through adapters.

| Repository | Go directive | Calx code it would replace |
| --- | --- | --- |
| `pouch` (`github.com/Yui-Qi-Tang/pouch`) | 1.27.1 | `internal/sat` runner, `internal/planning` encoder and matrix, `projection` |
| `ahe-mcp` (`github.com/Yui-Qi-Tang/ahe-mcp`) | 1.27.0 | `logicresolver` and `logicresolver/cadical`, the ancestor of `sat` and `sat/cadical` |
| `wdd/ahe-wrap` (`ahe-wrap`) | 1.26.5 | Same `logicresolver` code as ahe-mcp, apart from import paths |
| `wdd/pouch` | Python, plus one Go module under `experiments/h2/native` | Python CNF builders and solver wrappers under `experiments/` |

## Blockers shared by every Go consumer

**Module availability.** The Calx checkout has no git remote, so
`go get github.com/Yui-Qi-Tang/calx` cannot resolve it. Until the module is
published, each consumer needs a `replace` directive or a `go.work` file that
points at a local checkout. The conformance check already works this way.

**Go version.** `go.mod` declares `go 1.27.3`, matching the development
toolchain, so each consumer must declare at least `go 1.27.3` to import Calx.
That means pouch upgrades from 1.27.1, ahe-mcp from 1.27.0 and ahe-wrap from
1.26.5. With `GOTOOLCHAIN=auto`, adding the requirement bumps the consumer's
directive and fetches the toolchain. This was chosen over lowering the
directive. For reference, a copy of the module declaring `go 1.24.0` or
`go 1.26.5` passed `go build`, `go vet` and the full `go test` under those
toolchains on 2026-10-26, so the requirement reflects policy, not a language or
API need.

## ahe-mcp and ahe-wrap

The `CNF` type is identical to `sat.CNF`, including `Origins`, and the
conformance check compiles the real `compileConsistency` against Calx. The
friction is in names, records and one behavior:

- **Digest names.** `Result.InputSHA256`, `SolverSHA256` and the other digest
  fields, `Artifact.SHA256` and `Validation.TargetSHA256` are plain strings with
  `*_sha256` JSON keys. Calx uses `digest.Digest` with `*_digest` keys.
  `ConsistencyResult` embeds `cadical.Result` and is stored in
  `consistency-run/v1` records. Adoption therefore needs either a record version
  bump or an AHE-owned record type that maps from the Calx result.
- **Engine identity.** The worker compares `config.EngineID` with
  `Runner.Identity()`, which returns `string` in AHE and
  `(digest.Digest, error)` in Calx. The serialized identity also changes, so
  existing watches would need new EngineIDs. That migration should be planned
  explicitly, not discovered.
- **UNKNOWN.** AHE's runner returns a nil error for an explicit UNKNOWN and
  leaves rejection to `Require`. Calx returns `sat.ErrInconclusive` from
  `Solve`. Callers that treat any error as an execution failure must check
  `errors.Is(err, sat.ErrInconclusive)` first.
- **Configuration.** The `AHE_CONSISTENCY_*` environment variables stay
  AHE-owned. Digest strings need `digest.Parse` before they go into
  `cadical.Config`. The per-session artifact directory already matches Calx's
  rule that the artifact directory must be new.
- **Not in Calx.** `logicresolver/z3` and `linear` (QF_LIA). No product code
  imports them today. Removing `logicresolver` would drop them unless they move
  elsewhere.
- **Graph code** comes from `github.com/Yui-Qi-Tang/graph` (v0.1.2 in ahe-mcp,
  v0.1.1 in ahe-wrap). It is not incidence-matrix code and is unaffected.

## pouch

The conformance check already shows that Calx's `finite` encoding produces the
same DIMACS bytes and traces as Pouch at horizons 0–3, and that the
guard/effect/frame matrices and topology projection agree. The model limits
match: 16 fields, 64 values, 64 actions, 256 forbidden patterns and 4,096 states.

- **Search stays in Pouch.** Pouch's `planning.Search` sweeps horizons,
  enumerates paths with blocking clauses and runs return searches to the
  baseline. Calx compiles one exact-length query with `Blocked` sequences and
  leaves enumeration and stopping policy to the caller by design (see
  [architecture](architecture.md)). Pouch keeps `search.go` and calls
  `finite.Compile` for each query.
- **Variable mapping is mechanical.** `finite.Encoding.Variables()` returns the
  same fields and JSON keys as `planning.Variable`. The Calx fields use named
  string types, and `OtherTime` is `*int` instead of `int`.
- **Result mapping.** Calx `UNKNOWN` maps to Pouch `INCONCLUSIVE`. Calx
  `Artifact.Digest` maps to `SHA256`, and `Require` replaces the `Verified`
  flag.
- **Evidence layout.** Pouch writes `tools.json` with schema
  `pouch-sat-tools/v1` and a `variables.json`. Calx writes `tools.json` as a
  name-to-digest map, plus `limits.json`, `input.json` and `outcome.json`. Any
  Pouch bundle or inspection code that reads these files must change.
- **Configuration.** Pouch has one `Timeout`. Calx uses `sat.Limits` plus
  `MaxProofBytes`, and `--solver-sha256` values need `digest.Parse`.
- **Concurrency.** Pouch serializes solves with a mutex. Calx allows concurrent
  `Solve` calls with distinct query IDs. Pouch can keep its mutex.

## wdd/pouch experiments

- **Python cannot import Go.** The experiments would call the `calx` CLI
  (`encode`, `solve`, `solve-finite`) and rewrite their artifact readers.
- **Model gaps in `finite`.** The h7 encoder uses copy effects (a field takes
  another field's value), and h17 models use non-string values, forbidden
  patterns with value lists, and per-action `cost` and `basis`. `finite`
  supports only constant string effects and single-value patterns. Value lists
  can be expanded into several patterns; copy effects cannot be expressed
  today.
- **Patched DRAT-trim.** The pinned "drat-trim-exit-fix-v2" is acceptable if it
  exits 0 and prints `s VERIFIED` for a valid proof, which is Calx's checker
  contract.
- **Module path collision.** `experiments/h2/native/go.mod` declares
  `github.com/Yui-Qi-Tang/ahe-mcp`, the real ahe-mcp's path. That is a separate
  cleanup.

## Not planned in Calx

Calx keeps these out of scope: a mode without the proof checker, incremental
solving, and search or enumeration drivers. Copy effects in `finite` and an SMT
path are candidates only if a consumer commits to using them.
