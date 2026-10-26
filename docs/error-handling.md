# Explicit errors and variable contracts

The compiler returns errors directly from variable allocation, clause insertion
and every encoding stage. It no longer stores a delayed error in compiler state
or returns a bare invalid variable ID. No failed metadata encoding is appended to
the variable map. Cancellation, budget and input errors remain inspectable.

## Inconclusive solver responses

An explicit `s UNKNOWN` with the matching exit code returns
`sat.ErrInconclusive` directly from Solve. It retains `Status: UNKNOWN`, exact
inputs, solver output, process records and the artifact inventory. The saved
`outcome.json` also records the inconclusive error. Require returns the same
sentinel for an UNKNOWN result.

| Condition | Go error | CLI code |
| --- | --- | --- |
| Explicit UNKNOWN response | `sat.ErrInconclusive` | `INCONCLUSIVE` |
| Cancellation | `context.Canceled` | `CANCELED` |
| Deadline exceeded | `context.DeadlineExceeded` | `TIMEOUT` |
| Contradictory or malformed solver response | `sat.ErrProtocol` | `PROTOCOL` |

Use `errors.Is`: context and artifact errors may be joined with the inconclusive
error. CLI cancellation/deadline codes take precedence over INCONCLUSIVE. The
UNKNOWN status is also the initial state before any backend answer, so status
alone does not identify the error cause. None of these errors imply UNSAT.

## Variable roles

VariableKind has the constants StateVariable, ActionVariable and
DifferenceVariable. FieldName, Category and ActionID are distinct Go string types
for caller-defined names; they are not a fixed list of application names.
`Variable.Validate(model, horizon)` checks both role and model membership, and
the compiler applies the same validation before adding each variable.

Validate checks one record, not uniqueness of a whole mapping. The compiler
privately allocates unique contiguous IDs. An Encoding does not accept a
replacement variable mapping from a caller or a deserialized response.

| Kind | Required | Must be absent | Time bounds |
| --- | --- | --- | --- |
| state | Field, Value | Action, OtherTime | 0 ≤ Time ≤ horizon |
| action | Action, matching Model.Actions | Field, Value, OtherTime | 0 ≤ Time < horizon |
| different | Field, OtherTime | Value, Action | 0 ≤ Time < OtherTime ≤ horizon |

OtherTime is `*int`: nil means absent. A difference variable cannot omit it, and
an explicitly supplied zero is invalid for its later-state role. ID must be
positive. Field/category pairs must be declared in Model.Fields. Unknown kinds
and unrelated populated fields are rejected.

Callers choose action names when declaring a Model, subject to its uniqueness,
UTF-8, size and rule checks. They cannot invent a new action in a Variable map:
an action variable must refer to an existing declared action. The `omitempty`
JSON tags omit only fields that do not apply to the validated role.

## API changes in this v0 revision

| Method | Current return values |
| --- | --- |
| Encoding.CNF | `(sat.CNF, error)` |
| Encoding.Variables | `([]Variable, error)` |
| Encoding.Digest | `(digest.Digest, error)` |
| cadical.Runner.Identity | `(digest.Digest, error)` |

These reject nil/uninitialized receivers. An uninitialized Encoding must not
produce an apparently valid empty CNF, since an empty formula means true.
Encoding.Variables deep-copies OtherTime pointers as well as the surrounding
records. The README examples and consumer adapters check every new error result.

File read/write/close failures are wrapped or joined rather than discarded.
Process exit codes 10 and 20 are explicit SAT protocol data, checked by the
backend parser; they are not silently ignored process failures. Failed process
startup, cancellation, output limits and record-writing errors return errors.
Output-copy errors are retained separately because os/exec can prioritize a
nonzero process exit over a pipe-copy failure. A valid solver exit cannot hide a
stdout/stderr write failure.

`make verify` includes pinned errcheck with blank assignments, type assertions,
standard-library exclusions disabled and fmt included. The checker complements
unit tests: it detects discarded errors, not the correctness of every branch.
Application/library code does not depend on errcheck; it is a development tool.

`digest.Parse` and `Digest.Validate` return `digest.ErrInvalid` for unsupported
representations. `digest.Read` returns no value after a read failure and preserves
the underlying I/O error. `Digest.Set` leaves the receiver unchanged on failure.
The CLI parses digest flags before reading input; the runner revalidates native
Config values, and Require rejects malformed check-target identifiers.
