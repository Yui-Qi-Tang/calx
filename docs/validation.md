# Validation

## UNKNOWN error contract — 2026-10-20

Explicit UNKNOWN responses now return `sat.ErrInconclusive` from Solve and
Require. The result remains UNKNOWN, with no checked model/proof. The deferred
artifact finalizer records the error and preserves the diagnostic files.
Observed context errors are retained with `errors.Join`; CLI cancellation and
timeout codes take precedence over INCONCLUSIVE.

Observed locally on darwin/arm64 with Go 1.27.2:

| Check | Observed result |
| --- | --- |
| `make verify` | PASS: formatting, strict errcheck, uncached tests, vet and build |
| `make race` | PASS: full selection with serialized packages |
| UNKNOWN protocol fixture | PASS: ErrInconclusive, UNKNOWN status, no checked model/proof, saved error and readable digest-verified diagnostic artifacts; checker not invoked |
| CLI solve / solve-finite | PASS: exit 1, ERROR/INCONCLUSIVE, retained UNKNOWN and artifact inventory |
| Context errors | Existing cancellation/deadline regressions PASS; joined inconclusive/context errors retain CANCELED/TIMEOUT CLI precedence |
| Pouch adapter | UNKNOWN propagates ErrInconclusive without marking the result verified |
| Documentation | 78 links/anchors checked; updated resolver diagram parsed, rendered and visually inspected |

The UNKNOWN tests use explicit POSIX protocol fixtures. Earlier real-tool
acceptance records below remain separate evidence; they were not rerun for this
error-contract change.

## Shared Digest type and CLI v2 — 2026-10-20

`digest.Digest` now carries content identifiers throughout Calx. Generation and
parsing are centralized in the leaf `digest` package. Calx-owned Go fields and
new JSON records use Digest role names; the CLI emits `calx-cli/v2`.
The prior checkpoint below describes its own v1-era output contract.

Observed locally on darwin/arm64 with Go 1.27.2:

| Check | Observed result |
| --- | --- |
| `make verify` | PASS: formatting, strict errcheck including digest, uncached tests, vet and build |
| `make race` | PASS: full selection with serialized packages |
| Digest value contract | PASS: known empty/abc vectors, streamed bytes and counts, partial-read failure, invalid text, unchanged receiver after failed Set, absent versus empty-content identity |
| Typed acceptance and JSON | PASS: malformed or mismatched targets rejected; new role keys round-trip; legacy result keys cannot satisfy checked-result requirements |
| Real pinned CaDiCaL/DRAT-trim | 522/522 subtests PASS |
| CLI v2 | PASS: encode, finite SAT, DIMACS UNSAT; independent Python hashes match input, formula and every returned artifact digest and byte count |
| Consumer checks | Core 1, MCP 1, Pouch Product 3 selected native-source tests and 13 frozen Lab CNFs PASS; generated source digest records independently rehashed |
| Examples and documentation | Checked SAT/UNSAT examples PASS; 78 links/anchors and five embedded Go examples checked; both changed SVGs parsed, rendered and visually inspected |

Evidence is retained in the ignored `artifacts/digest-type-20261020-zec90rod/`
directory, including `summary.json`, real-tool events, CLI results, example
outcomes and consumer logs. The frozen Lab manifest and external Pouch fields
are translated at adapter boundaries; their original formats are preserved.
This remains bounded consumer compatibility, not formal consumer migration or
a fresh Lab encoder/semantic-replay experiment.

## Tool configuration refactor — 2026-10-19

The pre-refactor source checkpoint is `2c36e43`. Tool inputs now use Digest names;
result JSON and its existing SHA256 evidence bindings are preserved. CLI flags
are command-specific, help exits successfully, and required tool settings are
checked before reading input. Executable digest checks retain the same SHA-256
format and reject mismatches before creating run artifacts.

Observed locally on darwin/arm64 with Go 1.27.2:

| Check | Observed result |
| --- | --- |
| `make verify` | PASS: formatting, strict errcheck, uncached tests, vet and build |
| `make race` | PASS: full selection with serialized packages |
| CLI and tool-input regressions | PASS: applicable help, missing/obsolete flags, explicit environment boundary, digest syntax/mismatch, error identity and no artifacts after rejected tool input |
| Real pinned CaDiCaL/DRAT-trim | 522/522 subtests PASS using the new environment names |
| Solve examples | Checked SAT trace and checked UNSAT proof PASS; existing result digest fields verified |
| Consumer checks | Core 1, MCP 1, Pouch Product 3 selected native-source tests and 13 frozen Lab CNFs PASS; source identities rechecked |
| Documentation | 76 local links/anchors and all five embedded Go examples match their sources |

Local evidence is retained in the ignored
`artifacts/digest-refactor-20261019-z_a6r7e3/` directory, including `summary.json`,
real-tool events, example outcomes, command help and consumer results.
The Lab scope remains frozen-input compatibility; this run does not execute
its encoder or semantic replay. Formal consumer checkouts were not migrated.

## Validation checkpoint — 2026-10-19

Observed locally on darwin/arm64 with Go 1.27.2. This revision replaces the
maintained Python compatibility script with Go, validates variable roles and
returns compiler, serialization and artifact I/O errors explicitly.
This checkpoint predates removal of the external graph integration example;
the example and diagram results below describe the files at that time.

| Check | Observed result |
| --- | --- |
| `make verify` | PASS: formatting, pinned strict errcheck, uncached tests, vet and build |
| `make race` | PASS: full selection with `-race -p 1 -count=1` |
| Real pinned CaDiCaL/DRAT-trim | 522/522 subtests PASS after the error-handling changes |
| Finite semantics | Existing independent DFS comparison over 511 models × horizons 0–3 still passes |
| Error and role regressions | Invalid variable roles/references/times, cancellation, limits, nil/zero receivers, no partial Encoding on failure, deep-copy ownership, inspectable I/O/JSON errors and failed output writes pass |
| Go consumer command | Core 1, MCP 1 and Pouch Product 3 selected native-source tests PASS; source hashes rechecked |
| Lab frozen-input compatibility | 13 H2/H6 CNFs PASS with checked SAT/UNSAT and matching source hashes; no fresh Lab encoder or semantic replay |
| CLI checks | Finite solve and unsupported typed-input rejection PASS |
| README examples | Graph projection runs; H1 SAT returns off → enable → on; H0 UNSAT passes proof checking |
| Documentation | Three Go examples match runnable source; all five SVGs parse/render; 24 topology cells, five operation-matrix cells, six variables and 13 clauses match live computation |

The public accessor signatures now include errors; see
[error-handling contracts](error-handling.md). The strict checker covers the
maintained root packages and examples, including resolver-integration code.
At this checkpoint it also checked the now-removed external graph example module.
Source snapshots from external
consumers and ignored historical artifacts are not part of that source lint scope.

The Go-only compatibility scope is documented in [compatibility](compatibility.md).
It preserves historical Lab inputs without copying or executing Python. It does
not claim that frozen formulas validate a newly executed Lab lowering or replay.
The four formal consumers remain unchanged.

Current local evidence (ignored):

- `artifacts/go-quality/verify-final.log`, `race-final.log` and `docs.json`
- `artifacts/acceptance-go-errors-20261019/real-tools.jsonl`
- `artifacts/go-consumers-final/results.json` and `source-manifest.json`
- `artifacts/go-quality/example-sat.json`, `example-unsat.json` and `graph-example.json`
- `artifacts/go-example-sat-final/` and `artifacts/go-example-unsat-final/`

The tool pins below are unchanged. Codebase-memory was refreshed; its artifacts
remain ignored. The SVGs are deliberately excluded by the indexer and were
checked directly rather than treated as graph-covered source.

## Historical foundation checkpoint — 2026-10-18

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
  used Python 3.14.8 and all selected boundaries passed. At that checkpoint the
  legacy harness checked its Python prerequisite before creating output. The
  current command is Go-only; original attempt 01 is retained.
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
