package toolrun

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/sat"
)

// Tool identifies an executable by its expected content digest.
// Digest currently accepts SHA-256 as 64 lowercase hexadecimal characters.
type Tool struct {
	Name, Path string
	Digest     digest.Digest
}

const maxExecutableBytes = 64 << 20

func readTool(path string, expected digest.Digest) (raw []byte, err error) {
	if path == "" {
		return nil, fmt.Errorf("%w: executable path is required", sat.ErrInput)
	}
	if expected == "" {
		return nil, fmt.Errorf("%w: executable digest is required", sat.ErrInput)
	}
	if err := expected.Validate(); err != nil {
		return nil, fmt.Errorf("%w: executable digest: %w", sat.ErrInput, err)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening executable: %w", err)
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("inspecting executable: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return nil, errors.New("not an executable regular file")
	}
	if info.Size() > maxExecutableBytes {
		return nil, fmt.Errorf("%w: executable size", sat.ErrLimit)
	}
	raw, err = io.ReadAll(io.LimitReader(f, maxExecutableBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading executable: %w", err)
	}
	if len(raw) > maxExecutableBytes {
		return nil, fmt.Errorf("%w: executable size", sat.ErrLimit)
	}
	if digest.Sum(raw) != expected {
		return nil, fmt.Errorf("%w: executable digest mismatch", sat.ErrValidation)
	}
	return raw, nil
}
