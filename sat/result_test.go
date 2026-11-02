package sat_test

import (
	"errors"
	"testing"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/sat"
)

func TestResultIsZero(t *testing.T) {
	if !(sat.Result{}).IsZero() || !(sat.Validation{}).IsZero() {
		t.Fatal("zero values must report IsZero")
	}
	d := digest.Sum([]byte("x"))
	for name, r := range map[string]sat.Result{
		"status":         {Status: sat.Unknown},
		"input digest":   {InputDigest: d},
		"formula digest": {FormulaDigest: d},
		"solver digest":  {SolverDigest: d},
		"checker digest": {CheckerDigest: d},
		"model":          {Model: sat.Validation{Present: true}},
		"proof":          {Proof: sat.Validation{TargetDigest: d}},
		"artifacts":      {Artifacts: []sat.Artifact{{ID: "a"}}},
	} {
		if r.IsZero() {
			t.Errorf("%s: reported zero", name)
		}
	}
	for name, v := range map[string]sat.Validation{
		"present": {Present: true}, "decoded": {Decoded: true}, "checked": {Checked: true},
		"method": {Method: "m"}, "target": {TargetDigest: d},
	} {
		if v.IsZero() {
			t.Errorf("validation %s: reported zero", name)
		}
	}
}

func TestLimitKinds(t *testing.T) {
	kinds := map[error]string{
		sat.ErrInputLimit:      "resolver limit exceeded: input",
		sat.ErrOutputLimit:     "resolver limit exceeded: tool output",
		sat.ErrProofLimit:      "resolver limit exceeded: proof size",
		sat.ErrExecutableLimit: "resolver limit exceeded: executable size",
	}
	for kind, message := range kinds {
		if !errors.Is(kind, sat.ErrLimit) || kind.Error() != message {
			t.Errorf("%v: want %q wrapping ErrLimit", kind, message)
		}
		for other := range kinds {
			if other != kind && errors.Is(kind, other) {
				t.Errorf("%v matches %v", kind, other)
			}
		}
	}
}
