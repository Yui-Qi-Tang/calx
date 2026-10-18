# Validation — 2026-10-18

Observed locally on darwin/arm64 with Go 1.27.2 and Python 3.14.8.
This is an experimental library delivery, not a release or consumer migration.

| Check | Observed result |
| --- | --- |
| gofmt, ordinary uncached tests, go vet, go build | PASS (`make verify`) |
| Full race selection, serialized packages | PASS (`make race`, `-race -p 1 -count=1`) |
| Real pinned CaDiCaL/DRAT-trim | 522/522 subtests PASS on final SAT source |
| Independent finite-model path-set comparison | 511 three-state transition models × horizons 0–3; complete action-sequence sets compared with direct DFS, including blocking |
| Finite boundary cases | Zero-step SAT, empty-sequence blocking, forbidden goal, frame preservation, rejected malformed models/profiles, cancellation, limits and input/getter ownership |
| Topology and CLI | Empty graph, isolated node, loops, parallel edges, reconstruction, exact field names, malformed JSON, preserved input bytes and answer/error separation |
| AHE Core source compiler boundary | PASS, checked compatible/conflicting formulas and incomplete coverage rejection |
| AHE MCP source compiler boundary | PASS, same bounded checks in a separate source snapshot module |
| Pouch Product source boundary | PASS, exact DIMACS at horizons 0–3, equivalent topology matrices, native Search through Calx, forward and return replay by original authority |
| Python Lab selected profiles | PASS: 8 H2 queries, 5 H6 horizons, 1 SAT trace independently replayed; standalone finite CLI and rejection of typed input also passed |

The two AHE conformance tests use minimal in-memory scope structs and the actual
pure compiler body. They do not exercise database scope/currentness/RLS readers.
Pouch conformance does not establish full product CLI/governance deployment.
Lab H6 retains its existing signed/copy encoder; only its CNF is sent to Calx.
No claim is made about all Lab profiles or new experimental outcomes.

Pinned tools used in the actual-tool checks:

- CaDiCaL SHA256: `338a9bec6d24cdd04b117b63504e804f7ce4c0605370fe21cc5b8a98c25073df`
- DRAT-trim SHA256: `6dfac4c795691b620c0d8adf359f67232e65e27782c9cd88e4d4d4d28342dc62`

The binaries were existing local validation tools. Calx copied and checked their
bytes; no binaries were downloaded or added to source. Bounded correctness
checks do not establish arbitrary-scale performance or general solver reliability.

## Failures retained and resolved

- A CLI round-trip test detected Go's base64 JSON encoding of byte slices in the
  initial incidence representation. Binary cells now use integer slices; the
  same empty/nonempty round-trip checks pass.
- A reasoning counterexample identified lossy JSON hashing of invalid UTF-8
  native strings. Public textual inputs now validate UTF-8; ownership and invalid
  native-name regressions pass. The text-only critique was not independent code
  execution; the main agent verified and fixed the issue.
- Consumer attempt 01 used system Python 3.9.6. Core/MCP/Product passed, then the
  H6 encoder stopped because `zip(strict=True)` requires newer Python. Attempt 02
  uses Python 3.14.8 and all selected boundaries pass. The harness now checks its
  Python prerequisite before creating output. Original attempt 01 is retained.
- The initial parallel full race run hit the inherited one-second shell fixture
  deadline in TestArtifactArchivalBoundary. The full run passed with package
  serialization. No assertion, solver timeout or fixture timeout was relaxed.

## Local evidence

These paths are intentionally ignored and are not included in a source clone:

- `artifacts/verify.log`
- `artifacts/race.log` (initial failure) and `artifacts/race-serial.log` (PASS)
- `artifacts/acceptance-20261018/` (earlier 522-case run)
- `artifacts/acceptance-final-20261018/` (final 522-case run)
- `artifacts/consumers-20261018-01/` and `artifacts/consumers-20261018-02/`
- `artifacts/source-manifest.json` and `artifacts/summary.json`

Consumer source checkpoints: Core and Lab share the WDD repository checkpoint
`c28c074ce`; MCP `9d301f8`; Pouch Product `6d9c7dc`. Exact selected source hashes
are retained per conformance run. Formal consumers were not edited or switched.

Codebase-memory is indexed with a persisted local graph; its entire
`.codebase-memory/` directory is ignored. Coverage checks are best-effort and do
not prove completeness. Protobuf, SMT and expanded model profiles remain outside
this implemented v0 scope.
