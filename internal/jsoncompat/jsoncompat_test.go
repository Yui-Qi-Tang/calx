package jsoncompat_test

import (
	"bytes"
	jsonv1 "encoding/json"
	"math"
	"testing"
	"time"

	"github.com/Yui-Qi-Tang/calx/automaton"
	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/finite"
	"github.com/Yui-Qi-Tang/calx/internal/jsoncompat"
	"github.com/Yui-Qi-Tang/calx/sat"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
)

type sample struct {
	Text     string            `json:"text"`
	Opt      string            `json:"opt,omitempty"`
	Ptr      *int              `json:"ptr,omitempty"`
	Any      any               `json:"any,omitempty"`
	NilSlice []int             `json:"nil_slice"`
	Empty    []int             `json:"empty"`
	NilMap   map[string]string `json:"nil_map"`
	Ints     map[int]bool      `json:"ints"`
	Strs     map[string]int    `json:"strs"`
	Floats   []float64         `json:"floats"`
	Nested   [][]int           `json:"nested"`
	Zero     int               `json:"zero,omitzero"`
}

// values covers the kinds of values Calx marshals (its private JSON forms are
// tagged structs of the same field types), plus the cases where v1 and v2
// defaults differ: map key order, nil collections, escaping and floats.
func values() []any {
	zero := 0
	ints := map[int]bool{}
	for i := 1; i <= 12; i++ {
		ints[i] = i%2 == 0
	}
	other := 3
	return []any{
		sample{Text: "<&>    é \x01", Ptr: &zero, Any: map[string]any{"b": 1, "a": []string{}}, Empty: []int{},
			Ints: ints, Strs: map[string]int{"z": 1, "A": 2, "é": 3, "10": 4, "9": 5},
			Floats: []float64{0, 0.1, 0.25, -1, 1e21, 1e-7, 123456789.125, math.Sqrt(3) / 4}, Nested: [][]int{{}, {1, -2}}},
		sample{},
		sat.Result{Status: sat.SAT, SolverDigest: digest.Sum(nil), Artifacts: []sat.Artifact{{ID: "q/a", Bytes: 3}}},
		cadical.Result{Status: sat.UNSAT, Assignment: ints},
		sat.CNF{Variables: []sat.Variable{{ID: 1, Metadata: map[string]string{"k": `{"a":1}`}}, {ID: 2}}, Clauses: [][]int{{1, -2}, {}}},
		finite.Variable{ID: 3, Kind: finite.DifferenceVariable, Time: 0, OtherTime: &other, Field: "f"},
		finite.Trace{States: []finite.State{{"x": "a<b"}}, Actions: []string{}},
		automaton.Trajectory{States: []automaton.State{{true, false}}, Choices: [][]bool{}},
		[]any{nil, true, "x", 1.5},
		sat.Limits{Timeout: 30 * time.Second, MaxVariables: 7},
		struct {
			Limits        sat.Limits
			MaxProofBytes int64
		}{sat.Limits{Timeout: -1}, 64 << 20},
	}
}

func TestMatchesV1(t *testing.T) {
	for i, v := range values() {
		want, err := jsonv1.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		got, err := jsoncompat.Marshal(v)
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("value %d Marshal:\n v1 %s\n v2 %s (%v)", i, want, got, err)
		}
		if want, err = jsonv1.MarshalIndent(v, "", "  "); err != nil {
			t.Fatal(err)
		}
		if got, err = jsoncompat.MarshalIndent(v); err != nil || !bytes.Equal(got, want) {
			t.Errorf("value %d MarshalIndent:\n v1 %s\n v2 %s (%v)", i, want, got, err)
		}
		for _, indent := range []string{"", "  "} {
			var v1, v2 bytes.Buffer
			e := jsonv1.NewEncoder(&v1)
			e.SetIndent("", indent)
			if err := e.Encode(v); err != nil {
				t.Fatal(err)
			}
			if err := jsoncompat.Write(&v2, v, indent); err != nil || !bytes.Equal(v1.Bytes(), v2.Bytes()) {
				t.Errorf("value %d Write(%q):\n v1 %s\n v2 %s (%v)", i, indent, v1.Bytes(), v2.Bytes(), err)
			}
		}
	}
}
