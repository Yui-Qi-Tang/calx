package jsoncompat_test

import (
	"context"
	jsonv1 "encoding/json"
	"testing"

	"github.com/Yui-Qi-Tang/calx/automaton"
	"github.com/Yui-Qi-Tang/calx/internal/jsoncompat"
	"github.com/Yui-Qi-Tang/calx/lga"
)

// cnfJSON is the JSON form of a CNF used by the CLI and input.json; sat types
// carry no JSON tags.
type cnfJSON struct {
	Variables []variableJSON `json:"variables"`
	Clauses   [][]int        `json:"clauses"`
	Origins   [][]string     `json:"origins,omitempty"`
}

type variableJSON struct {
	ID       int               `json:"id"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// benchCNF is a 16x16 HPP run of 8 steps in its JSON form: 131,072 clauses,
// about 3.9 MB of JSON.
func benchCNF(b *testing.B) cnfJSON {
	b.Helper()
	e, err := automaton.Compile(context.Background(), lga.HPP{Width: 16, Height: 16}, automaton.Query{Steps: 8})
	if err != nil {
		b.Fatal(err)
	}
	c, err := e.CNF()
	if err != nil {
		b.Fatal(err)
	}
	out := cnfJSON{Variables: make([]variableJSON, len(c.Variables)), Clauses: c.Clauses}
	for i, v := range c.Variables {
		out.Variables[i] = variableJSON(v)
	}
	return out
}

// BenchmarkMarshalCNF compares encoding/json (v1) with the v2-based
// jsoncompat.Marshal on the JSON form of a CNF, as Calx writes input.json.
func BenchmarkMarshalCNF(b *testing.B) {
	c := benchCNF(b)
	b.Run("encoding-json-v1", func(b *testing.B) {
		for b.Loop() {
			if _, err := jsonv1.Marshal(c); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("jsoncompat-v2", func(b *testing.B) {
		for b.Loop() {
			if _, err := jsoncompat.Marshal(c); err != nil {
				b.Fatal(err)
			}
		}
	})
}
