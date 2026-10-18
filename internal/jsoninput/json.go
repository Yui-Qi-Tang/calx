// Package jsoninput decodes the CLI's exact field-name JSON contract.
package jsoninput

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Decode rejects duplicate keys, null, unknown/case-aliased struct fields and
// trailing values. Omitted optional fields retain their documented zero values.
func Decode(raw []byte, dst any) error {
	if !utf8.Valid(raw) {
		return fmt.Errorf("invalid UTF-8 JSON")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := value(d, 0); err != nil {
		return err
	}
	if _, err := d.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON data")
	}
	t := reflect.TypeOf(dst)
	if t == nil || t.Kind() != reflect.Pointer {
		return fmt.Errorf("pointer destination required")
	}
	if err := shape(raw, t.Elem()); err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}
func value(d *json.Decoder, depth int) error {
	if depth > 64 {
		return fmt.Errorf("JSON nesting limit")
	}
	tok, err := d.Token()
	if err != nil {
		return err
	}
	if tok == nil {
		return fmt.Errorf("null is not supported")
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			k, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := k.(string)
			if !ok || seen[key] {
				return fmt.Errorf("duplicate or invalid JSON key")
			}
			seen[key] = true
			if err := value(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := value(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("invalid delimiter")
	}
	_, err = d.Token()
	return err
}
func shape(raw json.RawMessage, t reflect.Type) error {
	if t.Kind() == reflect.Pointer {
		return shape(raw, t.Elem())
	}
	switch t.Kind() {
	case reflect.Struct:
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return err
		}
		fields := map[string]reflect.Type{}
		for i := range t.NumField() {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			fields[name] = f.Type
		}
		for name, v := range obj {
			ft, ok := fields[name]
			if !ok {
				return fmt.Errorf("unknown JSON field %q", name)
			}
			if err := shape(v, ft); err != nil {
				return err
			}
		}
	case reflect.Map:
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(raw, &obj); err != nil {
			return err
		}
		for key, v := range obj {
			if t.Key().Kind() == reflect.Int {
				n, err := strconv.Atoi(key)
				if err != nil || strconv.Itoa(n) != key {
					return fmt.Errorf("noncanonical integer key")
				}
			}
			if err := shape(v, t.Elem()); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return err
		}
		for _, v := range arr {
			if err := shape(v, t.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

// Require checks that an object contains each named field, including zero values.
func Require(raw []byte, names ...string) error {
	var obj map[string]json.RawMessage
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
