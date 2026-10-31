# Public and CLI contracts

See the [data-structure reference](data-structures.md) for complete fields and
[graph-to-matrix walkthrough](graph-to-matrix.md) for a worked projection.

## Topology

`incidence.Graph` uses nonempty UTF-8 string node/edge IDs and relation labels.
Project sorts axes lexicographically by ID and owns its new slices. Reconstruct
validates unique axes, binary cells and exactly one source, target and relation
per edge. Two incidence matrices preserve self-loops; parallel edges retain
separate columns. Payloads are not part of this topology contract. Callers retain
original bytes, node/edge sidecars and bounded-view metadata themselves.

Defaults: 4,096 nodes, 8,192 edges, 4,000,000 total cells across both matrices and
relation masks. Negative limits fail. A consumer retaining a stricter prior cap
must supply it explicitly. Bounds precede matrix allocation. Caller-owned input
allocation and string storage are not a memory sandbox.

## Finite categorical profile

`categorical-simple-first-hit/v1` supports 1–256 fields, 1–256 nonempty UTF-8
categorical values per field, at most 4,096 coordinates (field/value pairs) in
total, 1–1,024 actions and at most 256 forbidden partial patterns. The number of
Cartesian states is not limited: encoding does not enumerate states, and the
encoding budget below bounds its size. The coordinate limit bounds the dense
matrices built before that budget is checked. Field/value/action names are at
most 200 bytes. Guards are conjunctions, effects are nonempty
constant assignments, and effect/frame sets exactly partition the fields. No
copy effect or typed numeric arithmetic is inferred. Invalid models fail before
solving.

The profile names query semantics, not limits. Both raises of the limits on
2026-10-31 kept the profile, because models accepted before keep their CNF and
digest. Older binaries still reject the newly accepted models with `ErrInput`.

Start is complete and domain-valid; goal is a nonempty partial pattern. Horizon
is an exact count from 0 through 4,095. Each transition chooses exactly one
action; there is no implicit idle action. All states differ, and no proper
prefix may satisfy the goal. Blocked sequences must contain known actions and
have exactly the query's horizon, including the empty sequence at horizon zero.
A blocked query's UNSAT does not claim the unblocked problem is unsatisfiable.

Encoding budgets default to 100,000 variables and 1,000,000 clauses; change
them with `WithMaxVariables` and `WithMaxClauses`, each at least 1. Budget
exhaustion and cancellation are errors, never UNSAT. Context is checked at
stages and during clause construction; this is not a hard CPU quota. At 1,024
actions, the clauses that allow one action per step alone exceed the default
clause budget at every horizon above 1, so the action limit decides only at
horizons 0 and 1 or under a larger `WithMaxClauses`. The
[validation record](validation.md#finite-encoding-limits--2026-10-31) has the
measurements.

A formula within the budget can still exceed the solver's `sat.Limits`.
Near the clause budget, 16 fields of 64 values produce more than the default
16 MiB `MaxInputBytes` of `input.json`, and `Solve` then fails with
`sat.ErrLimit`. Wide models reach it the same way: in the measured shape of 100
actions at horizon 8, two-valued fields fit the budget up to the limit of 256 but
pass `MaxInputBytes` between 232 and 240 fields.

Compile's private formula and decoding map are owned by the Encoding. CNF and
Variables accessors return copies and an error; nil/uninitialized encodings
are rejected. Digest also returns `(digest.Digest, error)`. Variable kinds,
model membership and role-specific optional fields are validated; see
[explicit error and variable contracts](error-handling.md). Decode requires a total assignment checked
against the owned original formula. Its trace is not independent original-rule
validation or an execution receipt. Inputs must not be mutated concurrently.

Digest identifies the serialized model and query, including profile,
horizon and blocking. Order-sensitive lists remain order-sensitive. It binds
this serialization, not mathematical equivalence or cross-language canonical
JSON. Native textual names are UTF-8-validated to prevent lossy JSON conversion.

## Synchronous Boolean automata (v0.1)

`synchronous-boolean/v2` encodes exactly `Steps` synchronous steps (at least
0, otherwise bounded only by the encoding budget) of a caller-supplied
`automaton.System`: 1–1,048,576 cells and 1–16 bits per cell.
More cells fail with `sat.ErrLimit`; other structural violations fail with
`automaton.ErrInput`.
Each cell lists distinct, in-range input bits. Inputs plus `Chooser` choice bits
are at most 12 per cell. Choice bits are free variables, so a nondeterministic
rule allows every choice. Start constrains time zero, Goal constrains the last
state, and each Forbidden pattern is excluded at every time. An empty
Forbidden pattern is rejected with `automaton.ErrInput`.

A nonempty `Violates` quantity lists distinct in-range bits with weight +1 or
-1 and requires some step to change its value: for some t, the value at time t
differs from the value at time t−1. A run that changes the quantity and changes
it back therefore counts, and UNSAT means the value is the same at every time
of every admitted run. For each step Calx compares `P(t−1) + N(t)` with
`P(t) + N(t−1)` through two exact totalizer counters, so the clause count grows
with the number of steps and roughly with the square of the number of terms.
With zero steps the query is always UNSAT. Version 1 compared only the first
and last time.

Proving a whole-lattice sum unchanged is a counting argument, which CDCL
solvers handle poorly. Measured with the pinned CaDiCaL on HPP (90 s limit):
16-term mass proofs finish in under a second for one step and in about 2 s
for three, the 24-term FHP 2×2 mass proof over two steps times out, 36- and
64-term mass proofs time out even for one step, and 32-term momentum proofs
range from 0.6 s to a timeout. Comparing every step makes multi-step
whole-lattice proofs slower than the version 1 endpoint comparison (see the
[validation record](validation.md#per-step-comparison-2026-10-30)).
Counterexample search (SAT) and row or column sums stay fast. Without Start,
Forbidden or Goal patterns, `Steps: 1` already proves a quantity for every
state, and so for every step count; it costs the same as before. Treat UNKNOWN
on a large quantity as inconclusive, never as a violation.

`Next` must be deterministic. Compile evaluates it once per cell and input row
before encoding; no call is concurrent, and panics are not recovered. The
variable, clause and literal budget (defaults 100,000, 1,000,000 and 4,000,000;
change them with `WithMaxVariables`, `WithMaxClauses` and `WithMaxLiterals`)
is checked before any rule evaluation. A budget option below 1 fails with
`automaton.ErrInput`. Context cancellation is checked on entry, before each
cell's rule table and before each cell's clauses at every step; a canceled
context is reported before any validation. Budget exhaustion and cancellation are
errors, never UNSAT.

Digest identifies the cell structure, the evaluated rule tables and the query,
with pattern bits in sorted order. It does not identify the caller's System
implementation. Decode requires a total assignment checked against the owned
CNF. Replay re-evaluates the caller's rule on the decoded trajectory, so it
checks the encoding, not whether the rule models anything real. UNSAT is
limited to the declared System, the exact step count and the given patterns.

## Lattice gases (v0.1)

`lga.HPP` is a periodic Width×Height square lattice with at least one cell in
each direction. `lga.FHP` is the FHP-I hexagonal lattice in axial coordinates
on a periodic Width×Height parallelogram, with one choice bit per fluid cell
that rotates a head-on pair counterclockwise when true. Both accept `Walls`,
which is nil or has one entry per cell. A wall cell reverses every arriving
particle and has no choice bit; walls conserve mass but not momentum. Call
`Validate` before using a value as a System; `Run`, `Density` and `Velocity`
validate it themselves. `Run` must return the same final state as
`automaton.Simulate` with the same choices; `Run` keeps only the current
state, while `Simulate` returns every state. FHP's `choose` is not called for wall cells.

In both models a state is the post-collision configuration, and one step
streams and then collides, so each HPP cell reads four arrival bits and each
FHP cell six. The HPP collision exchanges exact head-on east–west and
north–south pairs. Without walls it conserves mass and momentum, including
momentum along every row and column. Quantity helpers name
bits only. An out-of-range row or column yields a quantity that Compile rejects.
Density and Velocity require block to divide both dimensions and every state to
have `Bits()*Width*Height` bits (4 per cell for HPP, 6 for FHP). They average
over all given states, and a region without particles has zero velocity. HPP is
not an isotropic fluid model.

## SAT and artifacts

CNF variables have contiguous one-based IDs. Signed clause literals are nonzero.
No clauses means true; one empty clause means false. Origins and variable
metadata are opaque mappings and add no constraints. Metadata and origin
strings must be valid UTF-8. Limits are checked before encoding.

SAT requires a total assignment checked against every original clause. UNSAT
requires a DRAT proof accepted by the pinned DRAT-trim process for strict
consumers: exit 0 and an exact `s VERIFIED` line, without `s NOT VERIFIED`. One
exception covers upstream DRAT-trim, which exits 1 for a formula that already
contains an empty clause: exit 1 is accepted only when the formula has an empty
clause, the output has a `c trivial UNSAT` line, and `s VERIFIED` is its only
status line. Such a proof records method `drat-trim/trivial-unsat-empty-clause`
instead of `drat-trim/exit-zero-exact-verified`. An explicit UNKNOWN response returns `sat.ErrInconclusive` from
Solve while retaining `Status: UNKNOWN` and diagnostic artifacts. Require also
returns `sat.ErrInconclusive` for UNKNOWN. Always inspect Solve's error first:
partial answers/artifacts can survive a failed check. Require is a policy check
on an actual result, not authentication of an arbitrary deserialized struct.
Cancellation and deadline errors remain discoverable with `errors.Is`, including
when joined with ErrInconclusive. They take precedence in CLI error reporting.

InputDigest binds exact generated input.json, FormulaDigest exact generated
input.cnf. ReadDIMACS parses comments/whitespace; re-emitted formula bytes can
therefore differ from original transport bytes. The CLI separately returns the
original input digest and preserves request.input at the artifact root.

Runner initialization requires both tool paths and expected content digests,
checks both binaries and creates private copies in a new directory. The input
fields are `SolverDigest` and `CheckerDigest`; they currently accept SHA-256 as
64 lowercase hexadecimal characters without a prefix. Invalid digest syntax
returns `sat.ErrInput`; a byte mismatch returns `sat.ErrValidation`. Both errors
occur before artifact creation. Digest values are provided by the shared
`digest` package; fields use role names such as `InputDigest` and `TargetDigest`.
For checked results, Require validates the digest representation as well as
matching the check target to the appropriate input/formula identity. Query IDs
reserve new subdirectories atomically; callers own retention and
total/concurrent query budgets. No download, fallback or retry is
implicit. Timeout defaults to 30 seconds shared by solver and checker. Defaults
also bound input at 16 MiB, each output stream at 16 MiB, and accepted proof at
64 MiB. The proof limit is checked after exit, not a disk quota. Unix cancellation
terminates the process group; descendant termination is not guaranteed on other
platforms. Native Windows execution has not been validated.

## Conflict localization

`mus.Shrink` first checks every group together, then tries deleting each group
in index order: an UNSAT deletion removes the group, a SAT deletion keeps it.
A run that finishes makes exactly `Groups()+1` checks, and one that stops makes
fewer. The index order
decides which conflict is found. Minimal means every remaining group had a
satisfiable deletion, which assumes the caller's Oracle is monotone: removing a
group never turns a satisfiable subset unsatisfiable. A Core is one conflict. It
is not every conflict, not a smallest one, and not a statement about which group
is wrong or how to repair it.

`Shrink` never trusts an earlier answer: the first check is its own, so a Witness
always comes from a check of exactly `Groups`. Every answer must meet the caller's
`sat.Requirements`; the zero value accepts unchecked answers and has to be passed
deliberately. UNKNOWN, a failed check and an unmet requirement stop the run, and
none of them is read as SAT or UNSAT. A complete set that is satisfiable returns
`mus.ErrSatisfiable`, and an exhausted budget returns `mus.ErrBudget`.

On any error, if `Witness.Status` is `sat.UNSAT` then `Groups` still conflict, but
`Minimal` is false and a group may be redundant. Otherwise no conflict was
established and `Groups` is nil. `Steps` always holds every check made, including
the failed one. Group indices mean nothing to Calx, and a check that fails has
already retained whatever its backend keeps.

`mus.ByOrigin` uses the distinct `CNF.Origins` strings, sorted, as groups. It
requires Origins. A clause with no origins is in every check. A clause with
several origins stays while any of them is kept, so each origin is taken to assert
the clause alone; clauses that depend on origins jointly need their own Oracle.

## Linear integers

The `linear` profile is Boolean CNF plus a conjunction of comparisons `=`, `<=`
or `>=` between a sum of constant multiples of integer variables and a constant.
An empty sum is zero. Integers are canonical decimal strings with no machine-size
limit. Boolean and integer IDs are separate, contiguous and one-based. This is a
subset of QF_LIA: no SMT-LIB input, quantifiers, nonlinear multiplication, arrays
or disjunctions that mix Booleans with arithmetic. An unknown relation returns
`linear.ErrUnsupported`, and other malformed input returns `sat.ErrInput`. Origins
and metadata add no constraints.

`Problem.CheckLimits` shares the `sat.Limits` budgets: Boolean plus integer
variables against MaxVariables, clauses plus constraints against MaxClauses, and
literals plus terms against MaxLiterals. Digit counts are bounded only by
MaxInputBytes.

`linear/z3` makes one process for the answer and, for SAT only, a second for the
values. Both share one deadline. The second may return a different model than the
first would; it must still be SAT and total, and it is evaluated against the
original typed problem with exact arithmetic. InputDigest binds exact generated
`input.json` and FormulaDigest exact generated `input.smt2`. An explicit `unknown`
returns `sat.ErrInconclusive`, as in `sat/cadical`.

Z3 UNSAT is a backend report. No proof is present, so `Require` with
`UNSATProofChecked` returns `sat.ErrGuarantee`. Runner initialization, pinning,
query directories, artifact retention, the default 30-second timeout and process
control otherwise match the SAT runner above, with no checker. The `Runner` has no
`Identity` or `ReadArtifact`. A pinned Z3 must run from a private copy, so a build
that loads a shared library relative to its own path does not work.

## JSON CLI

Commands: project, reconstruct, encode, decode, solve, solve-finite, pin-tools.

Inputs are at most 16 MiB. Exact JSON field names are required. Duplicate keys,
unknown/case-aliased fields, null, non-UTF-8 bytes, noncanonical integer map keys,
trailing values and more than 65 nested levels of arrays or objects are
rejected. Omitted fields are allowed only when their native zero semantics or
subsequent validation permits it. Horizon presence is mandatory for finite requests. CNF input must explicitly
contain variables and clauses; use empty arrays for an empty formula/clause.

- project accepts `{node_ids,edges}`; reconstruct accepts
  `{node_ids,edge_ids,source,target,relation_masks}`.
- encode/solve-finite accept `{model,query,options?}`. Options has optional
  max_variables and max_clauses; zero or omitted keeps the default, and a
  negative value is an input error. Query includes explicit profile/start/goal/horizon
  and optional blocked sequences. The CLI accepts only the profile
  `categorical-simple-first-hit/v1` and returns UNSUPPORTED otherwise. The examples show the exact field spelling.
- decode accepts `{request:{model,query,options?},assignment:{"1":true,...}}`;
  it recompiles the request instead of trusting a supplied decoding map.
- solve accepts CNF JSON `{variables,clauses,origins?}`, or DIMACS with `--format dimacs`.
- pin-tools reads no input. It hashes `--solver` and `--checker`, which must be
  executable regular files, and writes a new `--out` manifest. The manifest
  has schema `calx-tools/v1`, absolute paths and digests. The envelope's data
  is the same manifest, without an input digest.

Executed operations return schema_version `calx-cli/v2`, outcome OK/ERROR, the transport
input_digest, optional data and error. OK on project/encode means those operations
succeeded, not that SAT was checked. Successful solve commands additionally
require checked SAT/UNSAT. Error is separate from any retained backend answer.

CLI flags provide a fresh artifact directory, query ID and timeout, and either
the four tool flags or a `--tools` manifest provides the binary paths and pins. No result received from another process is implicitly authenticated by
this format. Trust configuration or independent rechecking belongs to consumers.

Solve commands take tools either from `--tools FILE` or from the four explicit
tool flags, never both. The manifest is decoded with the same strict JSON rules,
is at most 64 KiB, must name schema `calx-tools/v1`, and must contain absolute
paths and valid digests. It is validated before input is read; problems exit 2.
The manifest is not authenticated. Executable bytes are still checked against
its digests during tool preparation. `pin-tools` does not apply the runner's
64 MiB executable limit, so an oversized tool pins successfully and fails at
solve time with LIMIT. The standalone CLI does not read tool environment
variables. The Go examples and the `resolverintegration` tests read the four `CALX_*` tool
variables themselves; the `linear/z3` tests read `CALX_Z3` and `CALX_Z3_DIGEST`.
Old tool-input names are not aliases. Help exits 0 without reading input;
unknown flags, malformed flag syntax and missing required flags exit 2 on stderr
before input is read. CLI digest flags are parsed before input is read. Native
Config values are revalidated, and executable bytes are checked, during tool
preparation.
Only `solve` accepts `--format`; the other commands always consume JSON.

## Digest representation and wire version

`digest.Digest` is a defined string value. `Sum` and `Read` produce content
identifiers; `Parse` and `Validate` enforce the current representation. String
casts and JSON decoding are not validation. The empty value means absent, while
the digest of empty input is a valid nonempty value. Each caller still owns the
exact byte serialization being identified; this type adds no canonicalization.
Calx's own serializations (finite and automaton digests, `input.json`, runner
identity and `limits.json`) keep the same bytes since the move to
`encoding/json/v2`; automaton digests changed separately when the profile
became `synchronous-boolean/v2`. `outcome.json` and the CLI's `data.result` also keep their
bytes, except that the result's `profile` field (`"cnf/v0"`) was removed; the
CLI schema stays `calx-cli/v2`. `internal/jsoncompat` tests check that it
matches `encoding/json` on representative values, not on each serialization.

Current CLI output is `calx-cli/v2`: digest JSON keys use `*_digest`, and artifact
records use `digest`. The v1 keys are not aliases. Consumers must check the schema
and update their field mappings. Old saved artifacts remain in their original
format. Frozen consumer inputs and external consumer records are handled by
explicit consumer adapters; those external formats are not rewritten by this refactor.
