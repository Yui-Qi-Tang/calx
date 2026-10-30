package cadical

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/internal/jsoncompat"
	"github.com/Yui-Qi-Tang/calx/sat"
)

// Identity binds tool binaries and effective limits, excluding storage paths.
// It returns an error for an uninitialized runner or failed serialization.
func (r *Runner) Identity() (digest.Digest, error) {
	if r == nil || r.root == "" {
		return "", fmt.Errorf("%w: uninitialized runner", sat.ErrInput)
	}
	raw, err := jsoncompat.Marshal(struct {
		Solver, Checker digest.Digest
		Limits          sat.Limits
		ProofLimit      int64
	}{
		Solver: r.config.SolverDigest, Checker: r.config.CheckerDigest,
		Limits: r.config.Limits, ProofLimit: r.config.MaxProofBytes,
	})
	if err != nil {
		return "", fmt.Errorf("encoding runner identity: %w", err)
	}
	return digest.Sum(raw), nil
}

// ReadArtifact reads retained bytes and verifies size and digest. Close errors
// are joined with any read/validation error before returning to the consumer.
func (r *Runner) ReadArtifact(a sat.Artifact) (data []byte, err error) {
	if r == nil || r.root == "" {
		return nil, fmt.Errorf("%w: uninitialized runner", sat.ErrInput)
	}
	limit := max(r.config.MaxProofBytes, int64(r.config.Limits.MaxInputBytes), int64(r.config.Limits.MaxOutputBytes))
	if a.Bytes < 0 || a.Bytes > limit {
		return nil, sat.ErrLimit
	}
	f, err := os.OpenInRoot(r.root, a.ID)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() != a.Bytes {
		return nil, fmt.Errorf("%w: artifact size", sat.ErrValidation)
	}
	data, err = io.ReadAll(io.LimitReader(f, a.Bytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != a.Bytes || digest.Sum(data) != a.Digest {
		return nil, fmt.Errorf("%w: artifact digest", sat.ErrValidation)
	}
	return data, nil
}

// cnfJSON and variableJSON fix the form of input.json, the target of the model
// check, so it does not depend on sat's types. Variables and Clauses keep nil
// and empty distinct; a nil or empty Origins or Metadata is omitted.
type cnfJSON struct {
	Variables []variableJSON `json:"variables"`
	Clauses   [][]int        `json:"clauses"`
	Origins   [][]string     `json:"origins,omitempty"`
}

type variableJSON struct {
	ID       int               `json:"id"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

func inputJSON(c sat.CNF) cnfJSON {
	var variables []variableJSON
	if c.Variables != nil {
		variables = make([]variableJSON, len(c.Variables))
		for i, v := range c.Variables {
			variables[i] = variableJSON(v)
		}
	}
	return cnfJSON{Variables: variables, Clauses: c.Clauses, Origins: c.Origins}
}
