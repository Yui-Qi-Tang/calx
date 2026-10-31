// Command digest demonstrates content identifiers: creating them, validating
// external text, and what a direct cast does not check.
package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"log"
	"os"
	"strings"

	"github.com/Yui-Qi-Tang/calx/digest"
)

func main() {
	data := []byte("exact bytes\n")

	// 1. Sum hashes bytes in memory; Read hashes a stream to the same value.
	sum := digest.Sum(data)
	read, size, err := digest.Read(strings.NewReader(string(data)))
	if err != nil {
		log.Fatal(err)
	}

	// 2. Parse validates external text before it becomes a Digest.
	parsed, err := digest.Parse(sum.String())
	if err != nil {
		log.Fatal(err)
	}
	_, uppercase := digest.Parse(strings.ToUpper(sum.String()))

	// 3. A cast checks nothing, so a boundary must call Validate.
	cast := digest.Digest("not-a-digest")

	// 4. The zero value means absent, not the digest of empty content.
	var absent digest.Digest

	if err := json.MarshalEncode(jsontext.NewEncoder(os.Stdout, jsontext.WithIndent("  ")), struct {
		Sum             digest.Digest `json:"sum"`
		Bytes           int64         `json:"bytes"`
		ReadMatches     bool          `json:"read_matches"`
		ParseMatches    bool          `json:"parse_matches"`
		UppercaseFails  bool          `json:"uppercase_fails"`
		CastFailsCheck  bool          `json:"cast_fails_validate"`
		AbsentFails     bool          `json:"absent_fails_validate"`
		EmptyContentSum digest.Digest `json:"empty_content"`
	}{
		Sum:             sum,
		Bytes:           size,
		ReadMatches:     read == sum,
		ParseMatches:    parsed == sum,
		UppercaseFails:  errors.Is(uppercase, digest.ErrInvalid),
		CastFailsCheck:  errors.Is(cast.Validate(), digest.ErrInvalid),
		AbsentFails:     errors.Is(absent.Validate(), digest.ErrInvalid),
		EmptyContentSum: digest.Sum(nil),
	}); err != nil {
		log.Fatal(err)
	}
}
