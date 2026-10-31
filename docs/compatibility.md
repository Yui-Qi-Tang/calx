# Bounded consumer conformance

`go run ./cmd/calx-check-consumers` runs compatibility checks against external
consumer source trees using Go only. The consumers themselves remain unchanged.
The command accepts explicit source roots and a new output directory; it never
copies, imports or executes Python.

```sh
go run ./cmd/calx-check-consumers -h   # lists the source-root flags
go run ./cmd/calx-check-consumers <source-root flags> \
  --tools tools.json --out artifacts/consumer-check-NEW
```

Each consumer source root is passed by its own flag. `--tools` is required and
names a manifest written by `calx pin-tools` (see the
[README](../README.md#standalone-command)). The command reads no tool
environment variables. Inside the check it passes the pinned tools on: CLI
subprocesses receive explicit tool flags, and the external Go test modules
receive them through their environment. `--calx` defaults to the current
checkout, and `--timeout` bounds the entire check (default ten minutes).

The command creates a CLI binary and isolated external Go modules in the output.
It uses the Go parser to select the consumers' compiler declarations, records
source hashes, and supplies a local replace to the Calx checkout. Logs,
temporary replacements, source snapshots and tool artifacts stay in the ignored
output. The output must be outside every consumer root. Source identities are
rechecked before return.

| Boundary | What runs now | Excluded scope |
| --- | --- | --- |
| Declaration compilers (two source trees) | The consumers' actual compiler functions and condition types, minimal in-memory reader stubs; compatible/conflicting formulas, incomplete-input rejection and real checked solver | Database scope reader, currentness, row-level security, persistence, admission and deployed services |
| Planning consumer | Source planning/validation/projection built against Calx in an external Go module; topology comparison | The consumer's own search and replay, which its own tests cover; exact DIMACS comparison; full CLI/governance deployment and real repair execution |
| Experiment consumer | 13 frozen DIMACS inputs from two experiment series, with per-file digests, expected status and matching original source hashes; real CLI SAT/proof checks | Fresh encoder execution, fresh semantic replay, all experiment profiles and typed/copy lowering into finite v1 |

The experiment fixtures are the exact 13 inputs from the prior accepted
conformance run. Their source identities and provenance are retained with the
[fixtures](../internal/consumercheck/testdata/lab/README.md). Source files in
the consumer checkout are only read to check their hashes. A mismatch stops the
check; it does not silently regenerate or replace the frozen formulas.

This Go-only boundary is intentionally reported as **frozen CNF compatibility**.
The earlier Python-based run executed the original experiment encoder and
reference replay. That historical evidence is retained, but those operations
are not claimed for the new Go command. Calx does not reimplement the
consumer's semantic interpreter merely to remove a tooling dependency.

The standalone finite CLI and rejection of unsupported typed input are also
checked. A successful command produces results.json, source-manifest.json and
per-command stdout/stderr/exit records. Failure returns an error and retains
partial check results. The planning consumer imports Calx directly. For the
declaration compilers and the experiment consumer, passing these selected checks
is not native adoption.

The checker expects `calx-cli/v2` and records newly generated source identities
under `digest` keys. The frozen fixture manifest keeps its original `sha256`
keys; that input schema is mapped to `digest.Digest` at the adapter boundary.
That mapping preserves the original digest values.
