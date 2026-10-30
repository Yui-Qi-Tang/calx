// Package jsoncompat marshals with encoding/json/v2 using the options that
// reproduce encoding/json v1 bytes for Calx values. Digests, retained artifacts
// and CLI output identify these exact bytes, so switching the JSON engine must
// not change them.
package jsoncompat

import (
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"
)

// options are the v1 output behaviors that differ from v2 defaults and that
// Calx output depends on: sorted map keys, null for nil slices and maps,
// HTML/JavaScript-safe string escaping, and time.Duration as integer
// nanoseconds (sat.Limits.Timeout). The duration option is defined in the v1
// package; it is a format flag, which is cheaper than a custom marshaler.
// Fields that v1 omitted with omitempty but v2 would not (numbers, booleans)
// use omitzero instead.
var options = json.JoinOptions(
	json.Deterministic(true),
	json.FormatNilSliceAsNull(true),
	json.FormatNilMapAsNull(true),
	jsontext.EscapeForHTML(true),
	jsontext.EscapeForJS(true),
	jsonv1.FormatDurationAsNano(true),
)

// Marshal returns the same bytes as encoding/json v1 Marshal.
func Marshal(v any) ([]byte, error) { return json.Marshal(v, options) }

// MarshalIndent returns the same bytes as encoding/json v1 MarshalIndent with
// an empty prefix and a two-space indent.
func MarshalIndent(v any) ([]byte, error) {
	return json.Marshal(v, options, jsontext.WithIndent("  "))
}

// Write writes v followed by a newline, matching a v1 Encoder. A non-empty
// indent matches Encoder.SetIndent("", indent).
func Write(w io.Writer, v any, indent string) error {
	opts := options
	if indent != "" {
		opts = json.JoinOptions(options, jsontext.WithIndent(indent))
	}
	raw, err := json.Marshal(v, opts)
	if err != nil {
		return err
	}
	_, err = w.Write(append(raw, '\n'))
	return err
}
