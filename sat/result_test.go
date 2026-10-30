package sat_test

import (
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
