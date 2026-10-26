package toolrun

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/sat"
)

func TestExecutableDigestContract(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture requires POSIX executable mode bits")
	}
	path := filepath.Join(t.TempDir(), "tool")
	want := []byte("#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(path, want, 0700); err != nil {
		t.Fatal(err)
	}
	encoded := digest.Sum(want).String()
	for _, test := range []struct {
		name, path, digest string
		wantErr            error
	}{
		{name: "matching bytes", path: path, digest: encoded},
		{name: "missing path", digest: encoded, wantErr: sat.ErrInput},
		{name: "missing digest", path: path, wantErr: sat.ErrInput},
		{name: "invalid hex", path: path, digest: strings.Repeat("z", 64), wantErr: sat.ErrInput},
		{name: "short", path: path, digest: encoded[:62], wantErr: sat.ErrInput},
		{name: "long", path: path, digest: encoded + "00", wantErr: sat.ErrInput},
		{name: "odd length", path: path, digest: encoded[:63], wantErr: sat.ErrInput},
		{name: "uppercase", path: path, digest: strings.ToUpper(encoded), wantErr: sat.ErrInput},
		{name: "algorithm prefix", path: path, digest: "sha256:" + encoded, wantErr: sat.ErrInput},
		{name: "different bytes", path: path, digest: strings.Repeat("0", 64), wantErr: sat.ErrValidation},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := readTool(test.path, digest.Digest(test.digest))
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("readTool error = %v, want %v", err, test.wantErr)
			}
			if err == nil && !bytes.Equal(got, want) {
				t.Fatal("tool bytes changed")
			}
			if err != nil && got != nil {
				t.Fatal("returned bytes after failed validation")
			}
		})
	}
	if _, err := readTool(path, digest.Digest(encoded[:63])); !errors.Is(err, hex.ErrLength) {
		t.Fatalf("lost hex decoding error: %v", err)
	}
}
