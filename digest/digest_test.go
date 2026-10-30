package digest_test

import (
	"encoding/json/v2"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/Yui-Qi-Tang/calx/digest"
)

func TestKnownContentDigests(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"", "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		{"abc", "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"},
	} {
		t.Run(test.input, func(t *testing.T) {
			got := digest.Sum([]byte(test.input))
			if got.String() != test.want {
				t.Fatalf("Sum = %s, want %s", got, test.want)
			}
			streamed, n, err := digest.Read(iotest.OneByteReader(strings.NewReader(test.input)))
			if err != nil || streamed != got || n != int64(len(test.input)) {
				t.Fatalf("Read = (%s, %d, %v)", streamed, n, err)
			}
			parsed, err := digest.Parse(test.want)
			if err != nil || parsed != got || parsed.Validate() != nil {
				t.Fatalf("Parse = (%s, %v)", parsed, err)
			}
		})
	}
}

func TestReadFailureDoesNotReturnAPartialDigest(t *testing.T) {
	wantErr := errors.New("source failed")
	r := io.MultiReader(strings.NewReader("abc"), iotest.ErrReader(wantErr))
	got, n, err := digest.Read(r)
	if !errors.Is(err, wantErr) || got != "" || n != 3 {
		t.Fatalf("Read = (%s, %d, %v), want absent digest, 3 bytes and source error", got, n, err)
	}
}

func TestInvalidTextAndFailedSet(t *testing.T) {
	valid := digest.Sum([]byte("abc"))
	for _, text := range []string{
		"", "not a digest", valid.String()[:63], valid.String() + "00",
		strings.ToUpper(valid.String()), " " + valid.String(),
		"sha256:" + valid.String(), strings.Repeat("z", 64),
	} {
		t.Run(text, func(t *testing.T) {
			if got, err := digest.Parse(text); !errors.Is(err, digest.ErrInvalid) || got != "" {
				t.Fatalf("Parse(%q) = (%s, %v)", text, got, err)
			}
			if err := digest.Digest(text).Validate(); !errors.Is(err, digest.ErrInvalid) {
				t.Fatalf("unchecked cast validated: %v", err)
			}
			previous := valid
			if err := previous.Set(text); !errors.Is(err, digest.ErrInvalid) || previous != valid {
				t.Fatalf("failed Set changed receiver: %s, %v", previous, err)
			}
		})
	}
	var destination digest.Digest
	if err := destination.Set(valid.String()); err != nil || destination != valid {
		t.Fatalf("Set = (%s, %v)", destination, err)
	}
	var absent *digest.Digest
	if err := absent.Set(valid.String()); !errors.Is(err, digest.ErrInvalid) {
		t.Fatalf("nil Set error = %v", err)
	}
}

func TestJSONRetainsStringsAndRequiresBoundaryValidation(t *testing.T) {
	want := digest.Sum([]byte("abc"))
	raw, err := json.Marshal(want)
	if err != nil || string(raw) != `"`+want.String()+`"` {
		t.Fatalf("Marshal = %s, %v", raw, err)
	}
	var got digest.Digest
	if err := json.Unmarshal(raw, &got); err != nil || got != want {
		t.Fatalf("Unmarshal = %s, %v", got, err)
	}
	if err := json.Unmarshal([]byte(`"invalid"`), &got); err != nil {
		t.Fatal(err)
	}
	if err := got.Validate(); !errors.Is(err, digest.ErrInvalid) {
		t.Fatalf("unvalidated JSON was accepted: %v", err)
	}
	var absent digest.Digest
	if absent == digest.Sum(nil) || absent.Validate() == nil {
		t.Fatal("absent digest confused with empty-content digest")
	}
}
