# Validation

## Functional budget options — 2026-10-30

Observed locally on darwin/arm64 with Go 1.27.3.

| Change | Evidence |
| --- | --- |
| `automaton.Compile` and `finite.Compile` take `...Option`; each option checks its own value and returns `ErrInput` below 1 | `TestBudgetOptions` in both packages: defaults equal the `sat.Limits` defaults, a later option wins, zero and negative values fail, and the first invalid option's error is returned |
| `automaton.Compile` cancellation | `TestCompileStopsWhenCanceledMidway` cancels during rule tables and during clause generation; with an already canceled context, cancellation is reported before validation. Removing any one of the three context checks fails a test. |
| CLI `options` decoded into its own request struct and converted to finite options | `TestEncodeOptions`; treating zero as an explicit value fails it. Of 24 encode/decode option cases compared with the previous binary, 20 are identical and the four negative-value cases keep code `INPUT_OR_EXECUTION` with a new message (for example `invalid finite input: max variables -1, want at least 1`). The v2 JSON type-error wording ("cannot" or "unable to") varies between runs in both binaries and was normalized. |
| Consumer conformance and real tools | Consumer check passed every check (`artifacts/consumer-check-20261030-01/`); real-tool tests in `sat/cadical` and `lga` pass |
| `automaton` steps bounded only by the budget | `TestStepsAreBoundedOnlyByBudget`: 5,000 steps compile and simulate; `Steps: math.MaxInt` returns `ErrEncodingLimit` under the default and the maximal budget, because the check divides by the per-step size instead of multiplying. `TestEncodingSizeMatchesFormula` still matches the built formula. The overflow mutant (multiplying again) was not run, since it could allocate without bound. |
| JSON tags removed from `Bit`, `Term` and `Trajectory`; the identity uses private structs | Six HPP, FHP and wall-lattice digests captured before the change are identical after it. A temporary test compared the digest with a copy of the old identity, including a cell with nil inputs and an empty `Violates`, and caught a mutant that turned nil inputs into `[]`. |
| JSON tags removed from all `finite` types; the CLI decodes requests into its own `modelInput`/`actionInput` and writes `variableOutput`/`traceOutput`; the digest uses private structs | 24 CLI cases (encode, decode, solve-finite with the real tools; nil and empty `forbidden`, `frame` and `blocked`; horizon 0; UNSAT; four malformed inputs) are identical to the previous binary after normalizing the v2 JSON wording, as is the decode error output. `examples/matrix`, `compile`, `incidence`, `sat` and `solve` print identical output; `matrix` and `solve` now define their own JSON structs. The CNF metadata format is pinned by `TestMetadataMatchesMarshal` against a test-local tagged struct. The consumer check first failed because it read CLI output into `finite.Trace`; with its own struct it passes every check (`artifacts/consumer-check-20261030-03/`). |
| `finite.Query.Profile` removed (the CLI checks `profile` itself; the digest still records `finite.Profile`); `sat.Result.Profile` removed; JSON tags removed from `incidence` and `sat` types, with the CLI, `input.json`, `outcome.json`, `examples/incidence` and the consumer check each using their own structs | 35 CLI cases compared with the previous binary: project/reconstruct (including null, missing, unknown and duplicate inputs), solve with JSON and DIMACS input, encode, solve-finite and two profile errors. With timing-dependent artifacts masked, the only difference is the removed `"profile": "cnf/v0"` in 10 solve outputs and `outcome.json` files; encode output, query digests and `input.json` bytes are identical. `examples/compile`, `incidence`, `matrix`, `sat` and `solve` print identical output. `make verify`, `make race`, the real-tool tests in `sat/cadical` and `lga`, and the consumer check (`artifacts/consumer-check-20261030-04/`) pass. |
| Failed `project`, `reconstruct` and `decode` report empty arrays (and `{}` for `relation_masks`) in `data` instead of nulls | `TestFailedCommandsReportEmptyData` covers all three; returning nulls from decode again fails it. Of the 35 CLI cases above, only the failed project and reconstruct outputs changed, plus one case that feeds the failed project output to reconstruct and now reconstructs an empty graph. |
| Slices of known length are preallocated: `finite.Project` axes and rows, finite action and difference variables, quantity sides, packed planes, result artifact conversions; `automaton` preallocates its formula from `encodingSize` | `TestEncodingSizeMatchesFormula` now also requires capacity to equal length, so a wrong size estimate fails it. A temporary benchmark (rule 90, 200 cells, 200 steps, five runs each) went from about 8.3 ms and 52.3 MB to 4.2 ms and 18.7 MB per Compile, with allocations unchanged at about 321,000. The 35 CLI cases, `examples/compile`, `incidence`, `matrix`, `sat` and `lga` (with and without `-check`, compared with HEAD) are unchanged; `make verify`, `make race` and the real-tool tests pass. `Simulate` keeps growing its states one step at a time: preallocating `steps+1` would panic for very large or overflowing step counts. |
| `Violates` compares every pair of consecutive times (`synchronous-boolean/v2`), so a quantity that changes and changes back is found; v1 compared only times 0 and Steps | Before the change, endpoints, time 0 against every time, and consecutive times were measured on 12 cases with the pinned tools and a 90 s limit (table in the [v0.1 design](v0.1-design.md#per-step-comparison-2026-10-30)). `TestViolationOracle` now requires a change at some step and adds rule 51 (every cell negated), where all 8 starts change and return; reverting to the endpoint comparison fails it. `TestEncodingSizeMatchesFormula` passes with the new counts (HPP 2×2, 3 steps, mass: 3,076 clauses). The real-tool test `hpp-walls-change-momentum` failed under v2 at first because it compared only the endpoints of the counterexample; its three counterexample checks now require a change at some step, as does `examples/lga`, which prints the mass at every time. All automaton digests change with the profile. `make verify`, `make race` and the real-tool tests pass. |
| The four primers' examples are pinned by tests that need no solver: `automaton/primer_test.go` (rule 90 run, variable layout, cell and unit clauses, 12/72 counts, Garden of Eden graph and predecessor answers), `finite/primer_test.go` (transition graph, Guard/Effect/Frame and the Model round trip, allowed states, horizon table with decoded states, the full variable table, via the test DPLL), `sat/primer_test.go` (DIMACS, assignment checks, RUP check of the proof lines) and `lga/primer_test.go` (collision and wall frames, flow averages, exhaustive 2×2 mass, a hand-built leaky counterexample, FHP collisions and wall choice bits) | Each test fails under a matching mutation: HPP head-on pairs passing through, inverted rule-table outputs, and finite runs allowed to repeat states. Solver-chosen values (returned predecessors, the leaky counterexample's masses, the exact proof) are marked as such in the primers. `make verify` and `make race` pass. |

## JSON v2, tool manifest reuse and performance — 2026-10-27

Observed locally on darwin/arm64 with Go 1.27.3.

| Change | Evidence |
| --- | --- |
| `encoding/json/v2` everywhere, through `internal/jsoncompat` for output | Byte-for-byte equality with `encoding/json` for every Calx value kind (map key order, nil collections, escaping, floats, durations, indent, trailing newline). Each of the six compatibility options fails the test when removed. Finite, automaton and CNF-JSON digests, every CLI output in the README and docs (including error messages) and every example output match pre-migration captures. |
| Strict CLI decoding with v2 (`internal/jsoninput`) | Every rejection keeps its reason: duplicates, case aliases, null, trailing data, noncanonical integer keys, unknown fields, 65 nesting levels accepted and 66 rejected. Decoding a 1.9 MB CNF went from 145 ms and 3.4 M allocations to 41 ms and 0.52 M, the cost of a plain non-strict v1 Unmarshal. |
| JSON marshal of the same CNF | 8.99 ms with `jsoncompat` vs 9.09 ms with `encoding/json` (mean of 6 runs) |
| `finite.Compile` metadata | Quoted names are cached; the output stays identical to `jsoncompat.Marshal` (`TestMetadataMatchesMarshal`). A/B on 6 fields × 4 values, 12 actions, horizon 8: 389 µs → 275 µs. |
| `IsZero()` on `sat.Result`, `sat.Validation`, `cadical.Result`, `finite.Trace`, `automaton.Trajectory` | Zero values report true; each field alone makes them non-zero; Solve never returns a zero Result |
| `internal/toolmanifest`; `calx-check-consumers --tools` | CLI manifest tests unchanged and passing; the consumer check rejects a missing or unreadable manifest. A full conformance run with `--tools` passed every check: two declaration-compiler consumers, the planning consumer (3 tests), 13 frozen experiment CNFs, the finite CLI and typed-input rejection (`artifacts/consumer-check-20261027-01/`). |
| `go fix` (Go 1.27) | range-over-int, `errors.AsType`, `slices.Contains`, `strings.SplitSeq`, struct literals without the embedded field name; no fixes remain |
| NOTICE removed | No references remain |
| `sat.CNF.WriteDIMACS` uses `strconv.AppendInt` and a buffered writer | `TestWriteDIMACSMatchesFmt` compares the bytes with the original `fmt` writer on the true and false formulas, mixed signs, six-digit IDs and HPP 16×16×8; changing the line format fails it. Writer errors are still returned. The HPP DIMACS (3.8 MB) and `examples/compile` output match pre-change captures. Real CaDiCaL and DRAT-trim tests (`sat/cadical`, `lga`) pass. HPP 16×16×8 (131,072 clauses), old vs new in one run: to `io.Discard` 22.4 ms and 647 k allocations → 5.7 ms and 2; to a `bytes.Buffer` 23.9 ms → 6.2 ms. |
| `sat.ReadDIMACS` reads lines as bytes and stores literals in one shared backing slice | `TestReadDIMACSMatchesReference` compares the result (`reflect.DeepEqual`, so nil and empty clauses differ) and the error text with the original implementation on HPP 16×16×8 and 33 accepted and rejected inputs (tabs, CRLF, `+1`, `-0`, U+00A0, `pcnf`, `px cnf`, extra fields, overflow), under five limit sets. `TestReadDIMACSClausesDoNotAlias` checks that appending to one clause leaves the next unchanged. Each of three mutations (no capacity cap, nil backing slice, prefix-only header test) fails a test. Real-tool tests pass; a second consumer conformance run, whose 13 frozen experiment CNFs enter through the CLI DIMACS reader, passed every check (`artifacts/consumer-check-20261027-02/`). HPP 16×16×8, old vs new in one run: 18.8 ms and 786 k allocations → 13.5 ms and 65. |

Linux amd64: in Docker with `golang:1.27.3-bookworm` (image digest
`sha256:972b4110ba0f…`, emulated on an arm64 Mac), `make verify` and
`make race` pass, and `go test -v ./...` reports the same 105 passing
top-level tests as macOS, none skipped.

Linux amd64 real tools, built in the same image from upstream sources: CaDiCaL
`rel-3.0.1` (commit `c607304`, SHA-256 `6e1a06e5…797ab5`) and DRAT-trim
master (commit `2e3b2dc`, 2024-11-25, SHA-256 `9c09fe81…18412a`).

| Test | Result |
| --- | --- |
| `lga` `TestRealToolsHPP`, `TestRealToolsFHPAndWalls` | PASS |
| `sat/cadical` `TestRealTools`, first run | FAIL: 100 SAT and 165 UNSAT queries checked, 257 UNSAT queries rejected by the proof checker |
| `sat/cadical` `TestRealTools` and both `lga` tests, after the trivial-UNSAT exception | PASS; no subtest fails |

Every rejected query was a formula that already contains an empty clause.
DRAT-trim prints `c trivial UNSAT` and `s VERIFIED` and exits 1: on that path
upstream `main` leaves its status at `ERROR` and returns `sts != UNSAT`, which
has been so since the first commit (2016). Calx required exit 0 together with
`s VERIFIED`, so it rejected the proof. The pinned macOS DRAT-trim prints the
same lines and exits 0 for `p cnf 0 1` / `0`; its source revision is not
recorded.

The runner now also accepts exit 1 when the formula contains an empty clause,
the output has `c trivial UNSAT`, and `s VERIFIED` is the only status line
(see [contracts](contracts.md#sat-and-artifacts)). `TestTrivialUNSATCheckerExitOne`
covers the accepted case, exit 0, and six rejections: no empty clause, no
trivial line, exit 2, an extra `s NOT VERIFIED`, an extra `s DERIVATION`, and a
trivial line without `s VERIFIED`. Removing any one condition (empty clause,
trivial output, exit 1, sole status line) fails a subtest. After the change,
Linux proofs record 178 `drat-trim/exit-zero-exact-verified` and 257
`drat-trim/trivial-unsat-empty-clause`; the macOS run records 435
`drat-trim/exit-zero-exact-verified`. `make verify`, `make race` and the macOS
real-tool tests pass.

Comparison with two related external codebases, A and B, on source snapshots
built in a scratch module (their repositories were not modified):

| Workload | A | B | Calx |
| --- | ---: | ---: | ---: |
| Finite encoding, same model (project + encode) | about 340 µs | — | 275 µs (also builds metadata, digest and validation) |
| `WriteDIMACS`, 131,072 clauses, to a `bytes.Buffer` | same code as B | 23.9 ms, 647 k allocations | 6.2 ms, 10 allocations |

A and B write DIMACS with one `fmt.Fprintf` per literal, as Calx did before
this change; the same rewrite would apply to them.

## v0.1 automaton and lattice gas — 2026-10-26

Phases 1–3 of the [v0.1 design](v0.1-design.md) add `automaton` and `lga`; the
phase 4 and 5 checks follow below. v0.1 leaves `sat` and `sat/cadical`
unchanged apart from one test timeout: the fixture in
`TestArtifactArchivalBoundary` went from 1 s to 10 s after it failed under
parallel load. The separate refactor commit restructured `Runner.Solve`
without changing behavior.

Observed locally on darwin/arm64 with Go 1.27.3. The checks ran while `go.mod`
still declared `go 1.27.2`; the directive was then raised to `go 1.27.3` to
match the toolchain, together with the external module generated by the
conformance check, and `make verify` and `make race` passed again.

| Check | Observed result |
| --- | --- |
| `make verify`, `make race` | PASS, with `automaton` and `lga` added to the gofmt and errcheck lists |
| Encoding oracle | Every assignment of 7 small systems (up to 16 variables): satisfying assignments equal the oracle's valid runs |
| Totalizer | Outputs forced to the exact count for every input of 2–8 terms |
| Violation oracle | 8 systems, every start and choice sequence: formula satisfiable exactly when the oracle sees the quantity change |
| Planted mutations | 1 rule-table entry and 4 totalizer or weight faults; every one fails the tests |
| Budget | Predicted variables, clauses and literals equal the built formula in 12 cases; 32×32 HPP fits 12 steps, not 13 |
| HPP | Collision table, independent grid implementation, time reversal, exhaustive 2×2 conservation |
| Real tools | 13 checked queries, three consecutive runs: whole-lattice sums on 2×2 and row or column sums on 4×4 UNSAT with checked proofs; mutated rules SAT with replayed counterexamples; head-on reachability |

Phase 4 (FHP, walls, bit-packed runs) was checked the same day.
- The FHP table was checked for all states and both choices, and axial
  geometry for every direction.
- Exhaustive one-step conservation ran on small FHP lattices, with and without
  a wall, and on a walled HPP lattice.
- `Run` equals `Simulate` on widths 1, 2, 3, 63, 64, 65 and 130, with and
  without walls, and for 150 steps.
- Planted faults in the packed path were caught: wrap bits, chirality, the
  triple rule and walls. The one surviving mutant, which removes the padding
  mask, has no observable effect, because stray padding bits only occur in
  planes that never rotate back. The mask stays as an invariant guard.
- Nine real-tool queries passed in three consecutive runs
  (`artifacts/lga-phase4-20261026-02/`).

Phase 5 (CLI tool manifest) was checked the same day: `internal/cli` tests
cover pin-tools then solve, refusal to overwrite, non-executable and missing
tools, nine malformed or conflicting manifests rejected before input, and a
tool changed after pinning failing with a digest mismatch. With the real tools,
`pin-tools` reproduced the pinned digests below, and `solve-finite --tools`
returned a checked SAT with a trace (`artifacts/cli-tools-20261026-01/`).

The real-tool runs used the pinned CaDiCaL and DRAT-trim listed under
historical evidence below. Artifacts: `artifacts/lga-phase3-20261026-02/` and
the scaling probe `artifacts/lga-probe-20261026-01/`. The probe showed
whole-lattice proofs with 36 or more terms timing out at 90 s; see the
[measurements](v0.1-design.md#measured-conservation-proofs-are-counting-arguments).

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
| Planning-consumer adapter | UNKNOWN propagates ErrInconclusive without marking the result verified |
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
| Consumer checks | 1 + 1 declaration-compiler and 3 planning-consumer native-source tests and 13 frozen experiment CNFs PASS; generated source digest records independently rehashed |
| Examples and documentation | Checked SAT/UNSAT examples PASS; 78 links/anchors and five embedded Go examples checked; both changed SVGs parsed, rendered and visually inspected |

Evidence is retained in the ignored `artifacts/digest-type-20261020-zec90rod/`
directory, including `summary.json`, real-tool events, CLI results, example
outcomes and consumer logs. The frozen fixture manifest and external consumer
fields are translated at adapter boundaries; their original formats are
preserved. This remains bounded consumer compatibility, not formal consumer
migration or a fresh encoder/semantic-replay experiment.

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
| Consumer checks | 1 + 1 declaration-compiler and 3 planning-consumer native-source tests and 13 frozen experiment CNFs PASS; source identities rechecked |
| Documentation | 76 local links/anchors and all five embedded Go examples match their sources |

Local evidence is retained in the ignored
`artifacts/digest-refactor-20261019-z_a6r7e3/` directory, including `summary.json`,
real-tool events, example outcomes, command help and consumer results.
The experiment-consumer scope remains frozen-input compatibility; this run
does not execute its encoder or semantic replay. Formal consumer checkouts were not migrated.

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
| Go consumer command | 1 + 1 declaration-compiler and 3 planning-consumer native-source tests PASS; source hashes rechecked |
| Experiment frozen-input compatibility | 13 CNFs from two experiment series PASS with checked SAT/UNSAT and matching source hashes; no fresh encoder or semantic replay |
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
It preserves historical experiment inputs without copying or executing Python.
It does not claim that frozen formulas validate a newly executed lowering or
replay.
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
| First declaration-compiler source boundary | PASS, checked compatible/conflicting formulas and incomplete coverage rejection |
| Second declaration-compiler source boundary | PASS, same bounded checks in a separate source snapshot module |
| Planning-consumer source boundary | PASS, exact DIMACS at horizons 0–3, equivalent topology matrices, the consumer's own search through Calx, forward and return replay by original authority |
| Python experiment profiles | PASS: 8 queries from one series, 5 horizons from the other, 1 SAT trace independently replayed; standalone finite CLI and rejection of typed input also passed |

The two declaration-compiler conformance tests use minimal in-memory scope
structs and the actual pure compiler body. They do not exercise database
scope/currentness/row-level-security readers. Planning-consumer conformance
does not establish full product CLI/governance deployment. The second
experiment series retains its existing signed/copy encoder; only its CNF is
sent to Calx. No claim is made about all experiment profiles or new
experimental outcomes.

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
- Consumer attempt 01 used system Python 3.9.6. The declaration-compiler and
  planning boundaries passed, then the second experiment series' encoder
  stopped because `zip(strict=True)` requires newer Python. Attempt 02
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

Exact selected consumer source hashes are retained per conformance run.
Consumers were not edited or switched.

Codebase-memory is indexed with a persisted local graph; its entire
`.codebase-memory/` directory is ignored. Coverage checks are best-effort and do
not prove completeness. Protobuf, SMT and expanded model profiles remain outside
this implemented v0 scope.
