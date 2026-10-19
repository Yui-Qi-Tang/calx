package cadical

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Yui-Qi-Tang/calx/sat"
)

// Identity binds tool binaries and effective limits, excluding storage paths.
// It returns an error for an uninitialized runner or failed serialization.
func (r *Runner) Identity() (string, error) {
	if r == nil || r.root == "" {
		return "", fmt.Errorf("%w: uninitialized runner", sat.ErrInput)
	}
	raw, err := json.Marshal(struct {
		Solver, Checker string
		Limits          sat.Limits
		ProofLimit      int64
	}{
		Solver: r.config.SolverSHA256, Checker: r.config.CheckerSHA256,
		Limits: r.config.Limits, ProofLimit: r.config.MaxProofBytes,
	})
	if err != nil {
		return "", fmt.Errorf("encoding runner identity: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
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
	sum := sha256.Sum256(data)
	if int64(len(data)) != a.Bytes || hex.EncodeToString(sum[:]) != a.SHA256 {
		return nil, fmt.Errorf("%w: artifact digest", sat.ErrValidation)
	}
	return data, nil
}
