// Package digest provides content identifiers with one shared representation.
// Callers choose the exact bytes to identify; this package does not serialize
// values or establish that the identified content is trustworthy.
package digest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
)

// format describes the currently supported textual representation.
const format = "SHA-256, 64 lowercase hex characters"

// ErrInvalid identifies an unsupported or malformed digest representation.
var ErrInvalid = errors.New("invalid digest")

// Digest identifies exact bytes. The zero value means absent, not the digest
// of empty content. Direct casts and JSON decoding still require Validate at
// acceptance boundaries; use Sum, Read or Parse to construct a valid value.
type Digest string

// Sum returns the digest of data without modifying it.
func Sum(data []byte) Digest {
	sum := sha256.Sum256(data)
	return Digest(hex.EncodeToString(sum[:]))
}

// Read hashes a stream and returns the number of bytes read. It does not close
// the reader. Read errors return no digest, even when some bytes were consumed.
func Read(r io.Reader) (Digest, int64, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return "", n, err
	}
	return Digest(hex.EncodeToString(h.Sum(nil))), n, nil
}

// Parse accepts the current format without a prefix or surrounding whitespace.
func Parse(text string) (Digest, error) {
	if len(text)%2 != 0 {
		return "", fmt.Errorf("%w: %w", ErrInvalid, hex.ErrLength)
	}
	if len(text) != hex.EncodedLen(sha256.Size) {
		return "", fmt.Errorf("%w: expected %s", ErrInvalid, format)
	}
	decoded, err := hex.DecodeString(text)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	if text != hex.EncodeToString(decoded) {
		return "", fmt.Errorf("%w: expected %s", ErrInvalid, format)
	}
	return Digest(text), nil
}

// String returns the encoded value, or an empty string for an absent digest.
func (d Digest) String() string { return string(d) }

// Validate rejects absent or malformed values, including unchecked casts.
func (d Digest) Validate() error {
	_, err := Parse(string(d))
	return err
}

// Set parses text before changing d. It implements flag.Value; failed parsing
// leaves the previous value intact.
func (d *Digest) Set(text string) error {
	if d == nil {
		return fmt.Errorf("%w: nil destination", ErrInvalid)
	}
	parsed, err := Parse(text)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
