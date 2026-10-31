# Validation

## Finite field limit — 2026-10-31

Observed locally on macOS arm64 with Go 1.27.3 and the pinned CaDiCaL and
DRAT-trim, under default budgets and `sat.Limits` unless stated. `maxFields`
goes from 16 to 256. A new `maxCoordinates` of 4,096, the former bound of
16×256, replaces `maxFields*maxValues`. `Profile` stays v1. The models are
synthetic; each figure is one run, so read small differences as noise.

The field count does not set the cost of `Project`: its matrices have a row per
action and a column per coordinate. One 1,024-action model at each shape, with
horizon 1 unless stated, allocated and refused as follows. All are budget
refusals, which is why the time is for the whole `Compile`. The "without" rows
ran on a copy with `maxFields` raised and no coordinate limit. The "before"
column used the former `checkEncodingBudget`, which scanned every coordinate for
each framed field of each action.

| Fields × values | Coordinates | Status | `Project` allocation | Refusal, before | Refusal, final |
| --- | ---: | --- | ---: | ---: | ---: |
| 16×256 | 4,096 | Within the limits | 66 MiB | 182 ms | 61 ms |
| 64×64 | 4,096 | Within the limits | 71 MiB | 1.2 s | 66 ms |
| 256×16 | 4,096 | Within the limits, the largest allocation they allow | 93 MiB | 2.2 s | 122 ms |
| 256×2, horizon 8 | 512 | Within the limits | 36 MiB | not measured | 71 ms |
| 64×256 | 16,384 | Without the coordinate limit | 265 MiB | 2.2 s | not reachable |
| 128×256 | 32,768 | Without the coordinate limit | 531 MiB | 9.0 s | not reachable |

Peak resident memory of `calx encode` refusing 256×16, including reading its
JSON input, was 82 MiB, in 0.13 s. These worst-case shapes stop at the budget,
so they have no solve or DRAT figures.

The coordinate limit costs little. A field of 256 values needs 32,641 clauses at
each time, so under the default budget at most 15 such fields fit at horizon 1
or more. This count comes from the formula and was not run.

Each action flips one two-valued field, with 100 actions at horizon 8. "SAT"
reaches 8 fields set and "UNSAT" asks for 9, which 8 steps cannot do, so UNSAT
hardness comes from counting and may differ on other structures. The actions
cover all fields, or the first 16 with the rest framed only, as on/off options
are; each row gives the range over the two. Solve includes writing inputs and
checking the model or proof.

| Fields | Clauses | Compile | `input.json` | SAT solve | UNSAT solve | Largest proof |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 16 | 90,717 | 4–9 ms | 1.47 MB | 0.15–0.37 s | 1.2–1.4 s | 11.8 MB |
| 17 | 94,008 | 4–5 ms | 1.53 MB | 0.15 s | 1.2–1.4 s | 14.7 MB |
| 32 | 143,373 | 6–7 ms | 2.50 MB | 0.16 s | 1.2–1.3 s | 13.7 MB |
| 64 | 248,685 | 10–11 ms | 4.66 MB | 0.19 s | 1.3–1.4 s | 13.7 MB |
| 128 | 459,309 | 17 ms | 9.13 MB | 0.24 s | 1.2–2.7 s | 26.9 MB |
| 224 | 775,245 | 29–30 ms | 15.87 MB | 0.31 s | 1.9–2.6 s | 24.2 MB |
| 232 | 801,573 | 30–32 ms | 16.43 MB | 0.32 s | 1.9–3.1 s | 27.7 MB |
| 240 | 827,901 | 26–36 ms | 16.99 MB | `sat.ErrLimit` | `sat.ErrLimit` | — |
| 256 | 880,557 | 32–36 ms | 18.12 MB | `sat.ErrLimit` | `sat.ErrLimit` | — |

240 and 256 fields fit the encoding budget, and stop at `MaxInputBytes`. With
that limit raised they solve in about 0.33 s for SAT and 1.9–3.2 s for UNSAT,
with proofs up to 31.8 MB. Every proof is under the default 64 MiB limit.

Blocking each found sequence until UNSAT, with 103 actions and 256 forward paths
over a 5-valued stage field and on/off fields:

| Fields | Compile total | Solve total | Largest proof |
| ---: | ---: | ---: | ---: |
| 17 | 0.8 s | 4.0 s | 0.13 MB |
| 32 | 1.2 s | 5.6 s | 0.24 MB |
| 64 | 2.0 s | 9.2 s | 2.2 MB |
| 128 | 3.8 s | 18.2 s | 4.4 MB |

These runs found the 256 paths the model was built to have. Only the count was
compared, not the set; the set comparison below is on a smaller model.

| Change | Evidence |
| --- | --- |
| Accepted models keep their encoding | `TestAcceptedEncodingsAreUnchanged` gains 16 fields of 2, 16 fields of 256 and a mixed model, with digest and DIMACS hash captured before any code changed. Encoding `examples/finite.json` is byte-identical on the previous and new binary. |
| New limits | `TestFieldLimit` covers 17, 64 and 256 fields with the `Project` and `Matrix.Model` round trip and rejection of 257. `TestCoordinateLimit` covers 4,096 and rejection of 4,097. Both check `Validate`, `Project` and `Matrix.Model`. |
| Over budget is `ErrEncodingLimit` | `TestOverBudgetIsAnEncodingLimit` gains 256 fields of 2 at 1,024 actions. |
| `checkEncodingBudget` counts per field | `TestBudgetCountsMixedFields`: a formula that exactly fills the budget on mixed field sizes compiles, and one clause less does not. |
| Wide paths are right | `TestWideModelPathsMatchEnumeration` compares the path set on 20 fields, horizons 0–4, against a direct enumeration of states from the action rules, with the test DPLL oracle. |
| Repository checks | `make verify` and `make race` pass. With the pinned tools, the real-tool tests in `sat/cadical`, `lga`, `internal/cli`, `mus` and `linear/z3` pass. |
| Test strength | 10 mutants on a copy: the limit one above and below, the checks removed and the counts changed. 8 were killed. Removing the field or coordinate bound in `Matrix.actions` survives, because `Matrix.Model` then validates the model and returns the same `ErrInput`. |
| The CLI accepts wide models | `solve-finite` on the previous and new binary. The previous one fails all seven with `INPUT_OR_EXECUTION`. The new one returns SAT for 17 fields at 20 and 103 actions, 64 and 256 fields, and 4,096 coordinates, with traces replayed independently in Python; 257 fields and 4,097 coordinates keep `INPUT_OR_EXECUTION`. |

Not covered: models from real use, Linux, horizons other than those listed,
solving with many-valued wide fields, enumeration above 128 fields or 103
actions, and the CLI's own 16 MiB input limit, which frame lists reach sooner on
wide models with long names.

## Conflict localization and linear integers — 2026-10-31

`mus`, `linear` and `linear/z3` were added on Go 1.27.3, on macOS arm64, with Z3
5.3.3 from Homebrew and the pinned CaDiCaL and DRAT-trim.

| Check | Method | Result |
| --- | --- | --- |
| `make verify`, `make race` | Run locally | Exit 0 |
| `mus` against every acceptance formula | The core must be among the irreducible subsets found by enumerating all subsets with the independent oracle | Pass, 520 formulas |
| `mus` on real tools | One formula with two disjoint conflicts, checked SAT and UNSAT | Pass |
| `linear/z3` on real tools | 512 Boolean formulas against the independent oracle, 10 linear cases with recorded statuses, including values beyond 64 bits | Pass |
| `mus` over `linear/z3` | Groups are constraints. Requiring a checked proof fails with `sat.ErrGuarantee`; accepting unchecked UNSAT finds the conflict | Pass |
| `linear.WriteSMTLIB` and `input.json` | Exact byte comparison | Pass |
| Test strength | 14 hand-written mutants of `mus`, `linear` and `linear/z3` on a copy of the repository | All killed |
| README examples | Source of all 10 example programs compared byte for byte with the README, without their package comments. All 10 run, with real tools where needed, and output compared with the README where it is stable | Pass |
| Primer snippets | Go snippets compiled in a throwaway module, not run | Pass |
| `docs/mus-primer.md`, `docs/linear-primer.md` | Tables taken from real solver runs; `mus/primer_test.go` and `linear/primer_test.go` recheck them with an enumerator, and the Z3 acceptance rechecks the unique model | Pass |
| Primer images | SVG parsed by xmllint, rendered with rsvg-convert and inspected by eye | Pass |
| Links in README and docs | Script resolving relative paths and heading anchors | Pass, 158 links |
| Docs against the new packages | Script checks that exported names appear in the docs and that package-qualified references exist. `go list` imports compared with the architecture text. Prose compared by reading | Differences fixed |
| Real tools on Linux amd64 | CI run on merge commit `0f01d6b`: Z3 5.3.3 release checksum verified, and `sat/cadical`, `lga`, `internal/cli`, `mus` and `linear/z3` ran with the tools. The release binary runs from a private copy within the 64 MiB limit | Pass |
| New Mermaid diagrams | Parsed and rendered with Mermaid 11.4.1 in a browser; output not inspected by eye | Pass, 10 diagrams |

Statement coverage is 100% for `mus`, 94.9% for `linear` and 89.7% for
`linear/z3`, without the integration tests.

Not covered: the rendered Mermaid output was not viewed, and GitHub's Mermaid version
may differ. The architecture dependency diagram was rendered from its changed lines
only. Hand-written mutants are a sample, not a mutation score. The linear cases
are a fixed set, not a general reliability claim, and Z3 UNSAT answers were
compared with recorded statuses and the enumerating oracle, not independently
proved. Monotonicity of a caller's Oracle is assumed, not checked.

## Consumer check removed — 2026-10-31

`calx-check-consumers` was a pre-adoption tool. It read external consumer
checkouts, some of which CI cannot reach, and it broke when a consumer moved on.
The consumer that imports Calx now verifies each new version with its own tests
when it updates its pin. Records below still describe the check as it was.

| Change | Evidence |
| --- | --- |
| The 13 frozen formulas move to `sat/cadical/testdata/formulas`, without comments, and `TestRealTools` solves them against their recorded statuses | The expected statuses are copied from the removed manifest. Flipping one expected status fails the test. |
| The check's finite CLI and typed-input cases move to `internal/cli` | `TestSolveFiniteWithRealTools` runs `solve-finite` with real tools; `TestEncodeRejectsTypedValues` needs no solver. |

## Finite encoding limits — 2026-10-31

Observed locally on darwin/arm64 (Apple M5 Max) with Go 1.27.3, on a temporary
copy with the action limit at 8,192, so the budget could be measured past 1,024.
These models are synthetic, with assumed field and action structure, so their
solve times are indicative only; the composed models below are from real use.
Compile times are the median of five runs, in one process; solving ran one query
at a time with the pinned CaDiCaL and DRAT-trim under default `sat.Limits`. A
full rerun with the 1,024 action limit, which still had 64 values per field,
gave the same variable, clause and literal counts, sequence counts and statuses.
The only differences are the ones that limit causes: 1,414 or more actions fail
with `ErrInput`, and horizon 1 stops at 1,024. Its timings are not shown, because
other work ran beside it.

One compile at horizon 8 with 243 actions and 4,140 Cartesian states:

| Fields | Clauses | Literals | Compile | DIMACS |
| --- | ---: | ---: | ---: | ---: |
| 23×20×9 | 308,914 | 682,754 | 9.1 ms | 4.13 MiB |
| 23×5×3×3×2×2 | 295,888 | 642,315 | 9.9 ms | 3.88 MiB |

Blocking each found action sequence until UNSAT, as a path-enumerating consumer
does, recompiles every query. Compile took 5–11% of the total, so solving and
proof checking set the real cost. Each set of sequences equals that of a direct
depth-first enumeration, and each final UNSAT passed DRAT-trim.

| Model | Cartesian states | Actions | Sequences | Compile total | Solve total |
| --- | ---: | ---: | ---: | ---: | ---: |
| Forward only, 23×20×9 | 4,140 | 243 | 416 | 4.0 s | 37.7 s |
| Forward only, 23×5×3×3×2×2 | 4,140 | 243 | 452 | 4.3 s | 43.5 s |
| Explicit graph with cycles, 23×20×9 | 4,140 | 243 | 406 | 3.2 s | 56.1 s |
| Explicit graph with cycles, 23×5×3×3×2×2 | 4,140 | 243 | 374 | 3.5 s | 70.6 s |
| Forward only, 60×44×40 | 105,600 | 88 | 922 | 4.7 s | 38.8 s |
| Forward only, 11×8×5×5×4×4×3 | 105,600 | 88 | 1,021 | 3.1 s | 24.2 s |
| Forward only, 10×9×8×7×4×4×2 | 161,280 | 41 | 628 | 1.1 s | 8.9 s |

Largest action count that `Compile` accepts under the default budget:

| Horizon | 23×20×9 | 23×5×3×3×2×2 | 16 fields of 64 |
| ---: | ---: | ---: | ---: |
| 1 | 1,381 | 1,385 | 459 |
| 2 | 966 | 971 | 234 |
| 4 | 673 | 678 | 111 |
| 8 | 466 | 471 | 46 |
| 16 | 318 | 324 | 10 |
| 32 | 212 | 219 | 0 |

With the default budget, 1,024 actions can compile only at horizons 0 and 1: at
horizon 2 the action clauses alone reach 1,047,552. Among these shapes, the
limit decides at horizon 1 for the first two, and 16 fields of 64 stop at the
budget first. Over-budget rejection took about 1.5 ms at 1,024 actions on the
first two shapes. At the budget's edge, 998,650 clauses (466 actions, 23×20×9)
made a 13.82 MiB `input.json` and solved in 158 ms. On 16 fields of 64, 36
actions made 15.15 MiB and solved, while 40 actions compiled and `Solve`
returned `sat.ErrLimit`, so literal counts (at most about 2.7 million) are not
the first solver limit reached; `MaxInputBytes` is.

Peak resident memory of one `calx encode`, which includes reading the JSON
input: 165 MiB at 243 actions and horizon 8, 212 MiB at 1,024 actions and
horizon 1, and 26 MiB for a rejection at 1,024 actions on 16 fields of 64. The
largest model the limits allow, 1,024 actions on 16 fields of 256, is rejected
at horizon 1 in 0.56 s with 74 MiB. Those two rejections are budget errors: the
matrices are built first, and the budget check then refuses them.

Three composed models from real use, enumerated on the final code at every
horizon from 0 to 8, each until UNSAT, with default budgets and `sat.Limits`.
The largest field of the first has 115 values, which the former limit of 64
rejected, so `maxValues` went to 256. Each set of action sequences equals that
of a direct depth-first enumeration, and every answer was checked. Clauses are
for horizon 8 with nothing blocked:

| Fields | Coordinates | Cartesian states | Actions | Sequences | Clauses | Compile total | Solve total |
| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 3 | 135 | 4,140 | 243 | 414 | 525,044 | 3.9 s | 28.0 s |
| 8 | 43 | 105,600 | 91 | 969 | 76,249 | 1.9 s | 11.1 s |
| 10 | 39 | 161,280 | 41 | 590 | 28,335 | 0.7 s | 5.3 s |

| Change | Evidence |
| --- | --- |
| The Cartesian state limit is removed, `maxActions` goes from 64 to 1,024 and `maxValues` from 64 to 256; `Profile` stays v1 | `TestAcceptedEncodingsAreUnchanged` pins the digest and DIMACS hash of three models, including 4,096 states with 64 actions, captured from the code before the change. `TestDomainProductIsNotALimit`, `TestActionLimit` (with the `Project` and `Matrix.Model` round trip at 65, 243 and 1,024 actions, and rejection of 1,025), `TestValueLimit` (65, 115 and 256 values, and rejection of 257) and `TestOverBudgetIsAnEncodingLimit` all failed before the change. Setting either limit one above or below and changing `Profile` each fail a test. `make verify` and `make race` pass, as do the real-tool tests in `sat/cadical` and `lga`. |
| The CLI accepts the wider models | `solve-finite` with the pinned tools, previous and new binary, on seven models. All past the old limits fail on the previous binary. On the new one, 243 actions over 4,140 states, 88 actions over 105,600, 16 fields of 64 and 1,024 actions at horizon 1 are SAT, with traces replayed independently in Python; 8,192 states is UNSAT; 1,025 actions keeps `INPUT_OR_EXECUTION`; 1,024 actions at horizon 8 now returns `LIMIT`. Encoding `examples/finite.json` is byte-identical on both binaries. |
| Frozen planning-consumer solver tests removed from `sat/cadical` | They exercised a test-local copy of a contract that consumer has replaced with `sat.CNF`. Removing them and their fixtures loses none of the package's 132 coverage blocks, measured without the `resolverintegration` tag: `contract_test.go` and `runner_test.go` cover the same errors, limits, artifacts and metadata with generic formulas, and `acceptance.Cases()` covers real SAT and UNSAT runs. |
| Consumer conformance | The planning-consumer step had stopped building at the commit before this change: its adapter fixture expected a `CNF` type and an older `encode` signature that the consumer has replaced. The fixture is removed, so this check no longer compares exact DIMACS or runs that consumer's search; the consumer's own tests cover its search. The step still builds the consumer's planning code against Calx and compares topology. The full check passes every step (`artifacts/consumer-check-20261031-02/`). |

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
| `Violates` compares every pair of consecutive times (`synchronous-boolean/v2`), so a quantity that changes and changes back is found; v1 compared only times 0 and Steps | Before the change, endpoints, time 0 against every time, and consecutive times were measured on 12 cases with the pinned tools and a 90 s limit ([table](#per-step-comparison-2026-10-30)). `TestViolationOracle` now requires a change at some step and adds rule 51 (every cell negated), where all 8 starts change and return; reverting to the endpoint comparison fails it. `TestEncodingSizeMatchesFormula` passes with the new counts (HPP 2×2, 3 steps, mass: 3,076 clauses). The real-tool test `hpp-walls-change-momentum` failed under v2 at first because it compared only the endpoints of the counterexample; its three counterexample checks now require a change at some step, as does `examples/lga`, which prints the mass at every time. All automaton digests change with the profile. `make verify`, `make race` and the real-tool tests pass. |
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

Phases 1–3 of v0.1 add `automaton` and `lga`; the
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
[measurements](#measured-conservation-proofs-are-counting-arguments).

## Measured: conservation proofs are counting arguments

Phase 3 ran `Violates` queries on HPP with the pinned CaDiCaL and DRAT-trim and
a 90-second limit. These runs used `synchronous-boolean/v1`, which compared
only times 0 and Steps; under v2 the clause counts are larger (for example
1,026 / 3,076 for the first row):

| Lattice | Quantity | Terms | Steps | Clauses | Result |
| --- | --- | ---: | ---: | ---: | --- |
| 2×2 | mass | 16 | 1 / 3 | 1,025 / 1,537 | UNSAT in 0.5 s |
| 3×3 | mass | 36 | 1 / 3 | 3,921 / 5,073 | timeout |
| 4×4 | mass | 64 | 1 | 10,753 | timeout |
| 4×4 | momentum y | 32 | 1 / 2 | 3,713 / 4,737 | timeout / 10.8 s |
| 4×4 | momentum x | 32 | 3 | 5,761 | UNSAT in 1.5 s |

On 2026-10-27, FHP 2×2 mass (24 terms, 2 steps, 7,745 clauses) was UNSAT in
28 s with the same CaDiCaL and no proof output. The pure-Go solvers gophersat
(27.7 s) and gini (timeout at 60 s) did no better.

The formulas are small; the difficulty is the proof. Showing that a sum is
unchanged requires counting, which resolution-based CDCL does not find
efficiently. Counterexamples for broken rules are found in about 0.01 s on 4×4,
and row and column sums are proved in about 0.1 s.

### Per-step comparison (2026-10-30)

`synchronous-boolean/v2` compares every pair of consecutive times instead of
the first and last time. Before the change, three encodings were measured on
the cases above with the same tools and limit: endpoints (v1), time 0 against
every later time, and consecutive times (v2). All answers were checked and
every SAT run was replayed. One run per case:

| Case | Steps | v1 endpoints | time 0 vs every time | v2 consecutive |
| --- | ---: | --- | --- | --- |
| One cell negated each step, mass | 2 | UNSAT, 0.65 s | SAT, 0.09 s | SAT, 0.09 s |
| Broken HPP rule, 4×4 mass | 3 | SAT, 0.10 s | SAT, 0.11 s | SAT, 0.12 s |
| HPP 2×2 mass | 1 | UNSAT, 0.37 s | UNSAT, 0.37 s | UNSAT, 0.37 s |
| HPP 2×2 mass | 3 | UNSAT, 0.65 s | UNSAT, 2.44 s | UNSAT, 2.07 s |
| HPP 4×4 momentum x | 3 | UNSAT, 1.66 s | UNSAT, 4.20 s | UNSAT, 0.63 s |
| HPP 4×4 row 1 momentum x | 3 | UNSAT, 0.63 s | UNSAT, 0.47 s | UNSAT, 0.29 s |
| HPP 4×4 momentum y | 2 | UNSAT, 9.66 s | timeout | timeout |
| FHP 2×2 mass | 2 | UNSAT, 61.9 s | timeout | timeout |
| HPP 4×4 momentum y; HPP 3×3 mass; HPP 4×4 mass | 1 or 3 | timeout | timeout | timeout |

Times include the proof check. The first row shows the semantic difference:
the mass goes 0, 1, 0, so only the per-step encodings find the change. With
one step all three encodings are logically equivalent; v2 adds one variable
and one clause. Counterexamples and
few-term quantities are as fast or faster with v2; multi-step whole-lattice
proofs are slower, and two that v1 finished now time out.

Consequence for users: use `Violates` to find counterexamples and to prove
quantities with few terms. Without Start, Forbidden or Goal patterns, prove a
whole-lattice quantity with `Steps: 1`, which covers every state and so every
step count. A whole-lattice conservation law is better
established by checking each cell's rule table locally, which is exact and
covers every step count. That local check is a candidate for a later phase; it
is not part of v0.1.

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

The Go-only compatibility check was removed on 2026-10-31. It preserved
historical experiment inputs without copying or executing Python, and it did not
claim that frozen formulas validate a newly executed lowering or replay.
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
