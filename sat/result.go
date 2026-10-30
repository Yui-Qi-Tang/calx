package sat

import (
	"errors"
	"fmt"

	"github.com/Yui-Qi-Tang/calx/digest"
)

// Status is a backend's satisfiability answer, separate from validation.
type Status string

const (
	// Unknown means the backend has not established SAT or UNSAT.
	Unknown Status = "UNKNOWN"
	// SAT means the backend reports a satisfying interpretation.
	SAT Status = "SAT"
	// UNSAT means the backend reports no satisfying interpretation.
	UNSAT Status = "UNSAT"
)

var (
	// ErrInput marks malformed input.
	ErrInput = errors.New("invalid resolver input")
	// ErrUnsupported marks a language construct outside the supported profile.
	ErrUnsupported = errors.New("unsupported logic")
	// ErrLimit marks a configured input, output or accepted proof limit.
	ErrLimit = errors.New("resolver limit exceeded")
	// ErrProtocol marks output inconsistent with the pinned tool protocol.
	ErrProtocol = errors.New("invalid tool response")
	// ErrValidation marks an invalid model or unverified proof.
	ErrValidation = errors.New("resolver validation failed")
	// ErrInconclusive means the backend reported no SAT or UNSAT conclusion.
	ErrInconclusive = errors.New("solver returned no conclusive answer")
	// ErrGuarantee marks an unmet consumer acceptance requirement.
	ErrGuarantee = errors.New("required guarantee unavailable")
)

// Artifact is a relative file identity within the configured artifact root.
type Artifact struct {
	ID     string        `json:"id"`
	Digest digest.Digest `json:"digest"`
	Bytes  int64         `json:"bytes"`
}

// Validation separates availability, decoding and successful checking.
// Decoded applies only to models; proof syntax is handled by the external checker.
// Method and TargetDigest are populated only after a successful check.
// For models the target is input.json; for proofs it is the encoded formula.
type Validation struct {
	Present      bool          `json:"present"`
	Decoded      bool          `json:"decoded"`
	Checked      bool          `json:"checked"`
	Method       string        `json:"method,omitempty"`
	TargetDigest digest.Digest `json:"target_digest,omitempty"`
}

// IsZero reports whether v is the zero Validation: nothing present, decoded or
// checked, and no method or target recorded.
func (v Validation) IsZero() bool {
	return !v.Present && !v.Decoded && !v.Checked && v.Method == "" && v.TargetDigest == ""
}

// Result binds one answer to the input, encoding, tools and check records.
// Callers must inspect the Solve error before using the result. Even on error,
// a backend answer and partial artifacts may be available for diagnosis.
type Result struct {
	Status        Status        `json:"status"`
	InputDigest   digest.Digest `json:"input_digest,omitempty"`
	FormulaDigest digest.Digest `json:"formula_digest,omitempty"`
	SolverDigest  digest.Digest `json:"solver_digest"`
	CheckerDigest digest.Digest `json:"checker_digest,omitempty"`
	Model         Validation    `json:"model"`
	Proof         Validation    `json:"proof"`
	Artifacts     []Artifact    `json:"artifacts,omitempty"`
}

// IsZero reports whether r is the zero Result. A runner never returns a zero
// Result, even on error: it sets at least Status, so IsZero distinguishes an
// unset Result from an UNKNOWN answer.
func (r Result) IsZero() bool {
	return r.Status == "" && r.InputDigest == "" && r.FormulaDigest == "" &&
		r.SolverDigest == "" && r.CheckerDigest == "" && r.Model.IsZero() && r.Proof.IsZero() &&
		len(r.Artifacts) == 0
}

// Requirements specifies the checks a consumer needs for each possible answer.
// It is application policy, not a promise that every backend can provide it.
type Requirements struct {
	SATModelChecked   bool
	UNSATProofChecked bool
}

// Require checks answer-specific guarantees after a successful Solve call.
// UNKNOWN returns ErrInconclusive. It does not authenticate an arbitrary Result.
func (r Result) Require(want Requirements) error {
	switch r.Status {
	case Unknown:
		return ErrInconclusive
	case SAT:
		if want.SATModelChecked && (!r.Model.Present || !r.Model.Decoded || !r.Model.Checked || r.Model.TargetDigest != r.InputDigest || r.InputDigest.Validate() != nil) {
			return fmt.Errorf("%w: checked SAT model", ErrGuarantee)
		}
	case UNSAT:
		if want.UNSATProofChecked && (!r.Proof.Present || !r.Proof.Checked || r.Proof.TargetDigest != r.FormulaDigest || r.FormulaDigest.Validate() != nil) {
			return fmt.Errorf("%w: checked UNSAT proof", ErrGuarantee)
		}
	default:
		return fmt.Errorf("%w: no conclusive answer", ErrGuarantee)
	}
	return nil
}
