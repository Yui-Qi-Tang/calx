// Package jsoninput decodes the CLI's exact field-name JSON contract.
package jsoninput

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"reflect"
	"unicode/utf8"
)

// maxDepth is the deepest nesting level a value may have; the top-level value
// is at depth zero.
const maxDepth = 64

// Decode rejects duplicate keys, null, unknown/case-aliased struct fields and
// trailing values. Omitted optional fields retain their documented zero values.
func Decode(raw []byte, dst any) error {
	if !utf8.Valid(raw) {
		return errors.New("invalid UTF-8 JSON")
	}
	if err := scan(raw); err != nil {
		return err
	}
	t := reflect.TypeOf(dst)
	if t == nil || t.Kind() != reflect.Pointer {
		return errors.New("pointer destination required")
	}
	// v2 matches names exactly and rejects invalid UTF-8, duplicate names,
	// trailing data and noncanonical integer map keys by default.
	err := json.Unmarshal(raw, dst, json.RejectUnknownMembers(true))
	var semantic *json.SemanticError
	if errors.Is(err, json.ErrUnknownName) && errors.As(err, &semantic) {
		return fmt.Errorf("unknown JSON field %q", semantic.JSONPointer.LastToken())
	}
	return err
}

// scan reads every token once and rejects what v2 decoding would accept but
// the contract forbids: null anywhere, nesting deeper than maxDepth, and more
// than one top-level value.
func scan(raw []byte) error {
	d := jsontext.NewDecoder(bytes.NewReader(raw))
	for {
		depth := d.StackDepth()
		tok, err := d.ReadToken()
		if err != nil {
			if errors.Is(err, jsontext.ErrDuplicateName) {
				return fmt.Errorf("duplicate or invalid JSON key: %w", err)
			}
			return err
		}
		switch kind := tok.Kind(); {
		case kind == 'n':
			return errors.New("null is not supported")
		case kind != ']' && kind != '}' && depth > maxDepth:
			return errors.New("JSON nesting limit")
		}
		if d.StackDepth() == 0 {
			break
		}
	}
	if _, err := d.ReadToken(); err != io.EOF {
		return errors.New("trailing JSON data")
	}
	return nil
}

// Require checks that an object contains each named field, including zero values.
func Require(raw []byte, names ...string) error {
	var obj map[string]jsontext.Value
	if err := json.Unmarshal(raw, &obj); err != nil {
		return err
	}
	for _, name := range names {
		if _, ok := obj[name]; !ok {
			return fmt.Errorf("missing field %q", name)
		}
	}
	return nil
}
