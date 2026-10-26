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

`categorical-simple-first-hit/v1` supports 1–16 fields, 1–64 nonempty UTF-8
categorical values per field, at most 4,096 Cartesian states, 1–64 actions and at
most 256 forbidden partial patterns. Field/value/action names are at most 200
bytes. Guards are conjunctions, effects are nonempty constant assignments, and
effect/frame sets exactly partition the fields. No copy effect or typed numeric
arithmetic is inferred. Invalid models fail before solving.

Start is complete and domain-valid; goal is a nonempty partial pattern. Horizon
is an exact count from 0 through 4,095. Each transition chooses exactly one
action; there is no implicit idle action. All states differ, and no proper
prefix may satisfy the goal. Blocked sequences must contain known actions and
have exactly the query's horizon, including the empty sequence at horizon zero.
A blocked query's UNSAT does not claim the unblocked problem is unsatisfiable.

Encoding budgets default to 100,000 variables and 1,000,000 clauses. Budget
exhaustion and cancellation are errors, never UNSAT. Context is checked at
stages and during clause construction; this is not a hard CPU quota.

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

`synchronous-boolean/v1` encodes exactly `Steps` synchronous steps (0–4,095) of
a caller-supplied `automaton.System`: 1–1,048,576 cells and 1–16 bits per cell.
More cells fail with `sat.ErrLimit`; other structural violations fail with
`automaton.ErrInput`.
Each cell lists distinct, in-range input bits. Inputs plus `Chooser` choice bits
are at most 12 per cell. Choice bits are free variables, so a nondeterministic
rule allows every choice. Start constrains time zero, Goal constrains the last
state, and each nonempty Forbidden pattern is excluded at every time.

A nonempty `Violates` quantity lists distinct in-range bits with weight +1 or
-1 and requires its value at time zero to differ from its value at the last
state. Calx compares `P(0) + N(k)` with `P(k) + N(0)` through two exact
totalizer counters, so the clause count grows roughly with the square of the
number of terms. With zero steps the query is always UNSAT.

Proving a whole-lattice sum unchanged is a counting argument, which CDCL
solvers handle poorly. Measured with the pinned CaDiCaL on HPP (90 s limit):
16-term mass proofs finish in under a second, but 36- and 64-term mass proofs
time out even for one step, and 32-term momentum proofs range from 1.5 s to a
timeout. Counterexample search (SAT) and row or column sums stay fast. Treat
UNKNOWN on a large quantity as inconclusive, never as a violation.

`Next` must be deterministic. Compile evaluates it once per cell and input row
before encoding; no call is concurrent, and panics are not recovered. The
variable, clause and literal budget (defaults 100,000, 1,000,000 and 4,000,000)
is checked before any rule evaluation. Budget exhaustion and cancellation are
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
validate it themselves. For 0–4,095 steps, `Run` must return the same final
state as `automaton.Simulate` with the same choices; `Run` also accepts longer
runs, which `Simulate` rejects. FHP's `choose` is not called for wall cells.

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
consumers. An explicit UNKNOWN response returns `sat.ErrInconclusive` from
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

## JSON CLI

Commands: project, reconstruct, encode, decode, solve, solve-finite, pin-tools.

Inputs are at most 16 MiB. Exact JSON field names are required. Duplicate keys,
unknown/case-aliased fields, null, non-UTF-8 bytes, noncanonical integer map keys,
trailing values and more than 65 nested levels of arrays or objects are
rejected. Omitted fields are allowed only when their native zero semantics or
subsequent validation permits it. Horizon presence is mandatory for finite requests. CNF input must explicitly
contain variables and clauses; use empty arrays for an empty formula/clause.

- project accepts incidence.Graph; reconstruct accepts incidence.Projection.
- encode/solve-finite accept `{model,query,options?}`. Options has optional
  max_variables and max_clauses. Query includes explicit profile/start/goal/horizon
  and optional blocked sequences. The examples show the exact field spelling.
- decode accepts `{request:{model,query,options?},assignment:{"1":true,...}}`;
  it recompiles the request instead of trusting a supplied decoding map.
- solve accepts sat.CNF JSON, or DIMACS with `--format dimacs`.
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
variables. The Go examples and the consumer-check command read the four
`CALX_*` tool variables themselves.
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

Current CLI output is `calx-cli/v2`: digest JSON keys use `*_digest`, and artifact
records use `digest`. The v1 keys are not aliases. Consumers must check the schema
and update their field mappings. Old saved artifacts remain in their original
format. Frozen Lab inputs and external Pouch records are handled by explicit
consumer adapters; those external formats are not rewritten by this refactor.
