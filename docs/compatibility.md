# Bounded consumer conformance

`go run ./cmd/calx-check-consumers` runs compatibility checks using Go only.
The four formal consumers remain unchanged. The command accepts explicit source
roots and a new output directory; it never copies, imports or executes Python.

```sh
go run ./cmd/calx-check-consumers \
  --core "$AHE_CORE_ROOT" --mcp "$AHE_MCP_ROOT" \
  --pouch "$POUCH_PRODUCT_ROOT" --lab "$POUCH_LAB_ROOT" \
  --out artifacts/consumer-check-NEW
```

Set `CALX_CADICAL` and `CALX_DRAT` to the solver and proof-checker executable
paths. Set `CALX_CADICAL_DIGEST` and `CALX_DRAT_DIGEST` to their expected file
content digests: currently SHA-256 as 64 lowercase hexadecimal characters.
The compatibility command reads these environment variables; its CLI subprocesses
receive explicit tool flags. `--calx` defaults to the current
checkout, and `--timeout` bounds the entire check (default ten minutes).

The command creates a CLI binary and isolated external Go modules in the output.
It uses the Go parser to select AHE compiler declarations, records source hashes,
and supplies a local replace to the Calx checkout. Logs, temporary replacements,
source snapshots and tool artifacts stay in the ignored output. The output must
be outside all four consumer roots. Source identities are rechecked before return.

| Boundary | What runs now | Excluded scope |
| --- | --- | --- |
| AHE Core and MCP | Actual compileConsistency/consistencyText bodies and condition types, minimal in-memory reader stubs; compatible/conflicting formulas, incomplete-input rejection and real checked solver | DB scope reader, currentness, RLS, persistence, admission and deployed MCP |
| Pouch Product | Source planning/validation/projection in an external Go module; exact DIMACS at horizons 0–3; native Search through Calx; forward/return original-rule replay; topology comparison | Full CLI/governance deployment and real repair execution |
| Pouch Lab | 13 frozen H2/H6 DIMACS inputs with per-file digests, expected status and matching original source hashes; real CLI SAT/proof checks | Fresh Lab encoder execution, fresh Lab semantic replay, all Lab profiles and typed/copy lowering into finite v1 |

The Lab fixtures are the exact 8 H2 and 5 H6 inputs from the prior accepted
conformance run. Their source identities and provenance are retained in
[testdata/lab](../internal/consumercheck/testdata/lab/README.md). Source files in
the Lab checkout are only read to check their hashes. A mismatch stops the check;
it does not silently regenerate or replace the frozen formulas.

This Go-only boundary is intentionally reported as **frozen CNF compatibility**.
The earlier Python-based run executed the original Lab encoder and reference
replay. That historical evidence is retained, but those operations are not
claimed for the new Go command. Calx does not reimplement the Lab's semantic
interpreter merely to remove a tooling dependency.

The standalone finite CLI and rejection of unsupported typed input are also
checked. A successful command produces results.json, source-manifest.json and
per-command stdout/stderr/exit records. Failure returns an error and retains
partial check results. Passing these selected checks is not native adoption.

The checker expects `calx-cli/v2` and records newly generated source identities
under `digest` keys. The frozen Lab manifest keeps its original `sha256` keys;
that input schema is mapped to `digest.Digest` at the adapter boundary. The
Pouch adapter similarly maps Calx artifact digests into the external snapshot's
existing artifact fields. These mappings preserve the original digest values.
