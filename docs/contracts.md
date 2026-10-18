# Public and CLI contracts

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
Variables accessors return copies. Decode requires a total assignment checked
against the owned original formula. Its trace is not independent original-rule
validation or an execution receipt. Inputs must not be mutated concurrently.

QuerySHA256 hashes compact Go JSON of `{model,query}`, including profile,
horizon and blocking. Order-sensitive lists remain order-sensitive. It binds
this serialization, not mathematical equivalence or cross-language canonical
JSON. Native textual names are UTF-8-validated to prevent lossy JSON conversion.

## SAT and artifacts

CNF variables have contiguous one-based IDs. Signed clause literals are nonzero.
No clauses means true; one empty clause means false. Origins and variable
metadata are opaque mappings and add no constraints. Metadata and origin
strings must be valid UTF-8. Limits are checked before encoding.

SAT requires a total assignment checked against every original clause. UNSAT
requires a DRAT proof accepted by the pinned DRAT-trim process for strict
consumers. UNKNOWN cannot meet Require. Always inspect Solve's error first:
partial answers/artifacts can survive a failed check. Require is a policy check
on an actual result, not authentication of an arbitrary deserialized struct.

InputSHA256 binds exact generated input.json, FormulaSHA256 exact generated
input.cnf. ReadDIMACS parses comments/whitespace; re-emitted formula bytes can
therefore differ from original transport bytes. The CLI separately returns the
original input SHA256 and preserves request.input at the artifact root.

Runner initialization checks both binary pins and creates private copies in a
new directory. Query IDs reserve new subdirectories atomically; callers own
retention and total/concurrent query budgets. No download, fallback or retry is
implicit. Timeout defaults to 30 seconds shared by solver and checker. Defaults
also bound input at 16 MiB, each output stream at 16 MiB, and accepted proof at
64 MiB. The proof limit is checked after exit, not a disk quota. Unix cancellation
terminates the process group; descendant termination is not guaranteed on other
platforms. Native Windows execution has not been validated.

## JSON CLI

Commands: project, reconstruct, encode, decode, solve, solve-finite.

Inputs are at most 16 MiB. Exact JSON field names are required. Duplicate keys,
unknown/case-aliased fields, null, non-UTF-8 bytes, noncanonical integer map keys,
trailing values and nesting beyond 64 levels are rejected. Omitted fields are
allowed only when their native zero semantics or subsequent validation permits
it. Horizon presence is mandatory for finite requests. CNF input must explicitly
contain variables and clauses; use empty arrays for an empty formula/clause.

- project accepts incidence.Graph; reconstruct accepts incidence.Projection.
- encode/solve-finite accept `{model,query,options?}`. Options has optional
  max_variables and max_clauses. Query includes explicit profile/start/goal/horizon
  and optional blocked sequences. The examples show the exact field spelling.
- decode accepts `{request:{model,query,options?},assignment:{"1":true,...}}`;
  it recompiles the request instead of trusting a supplied decoding map.
- solve accepts sat.CNF JSON, or DIMACS with `--format dimacs`.

Every response has schema_version `calx-cli/v1`, outcome OK/ERROR, the transport
input_sha256, optional data and error. OK on project/encode means those operations
succeeded, not that SAT was checked. Successful solve commands additionally
require checked SAT/UNSAT. Error is separate from any retained backend answer.

CLI flags provide binary paths/pins, a fresh artifact directory, query ID and
timeout. No result received from another process is implicitly authenticated by
this format. Trust configuration or independent rechecking belongs to consumers.
