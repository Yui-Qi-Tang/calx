package jsoninput_test

import (
	"context"
	jsonv1 "encoding/json"
	"testing"

	"github.com/Yui-Qi-Tang/calx/automaton"
	"github.com/Yui-Qi-Tang/calx/internal/jsoninput"
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

// BenchmarkDecodeCNF measures the CLI's strict decode of a CNF in its JSON form
// (16x16 HPP, 8 steps, about 3.9 MB) against a plain, non-strict encoding/json
// (v1) Unmarshal of the same bytes.
func BenchmarkDecodeCNF(b *testing.B) {
	e, err := automaton.Compile(context.Background(), lga.HPP{Width: 16, Height: 16}, automaton.Query{Steps: 8})
	if err != nil {
		b.Fatal(err)
	}
	c, err := e.CNF()
	if err != nil {
		b.Fatal(err)
	}
	in := cnfJSON{Variables: make([]variableJSON, len(c.Variables)), Clauses: c.Clauses}
	for i, v := range c.Variables {
		in.Variables[i] = variableJSON(v)
	}
	raw, err := jsonv1.Marshal(in)
	if err != nil {
		b.Fatal(err)
	}
	b.Run("jsoninput-strict-v2", func(b *testing.B) {
		for b.Loop() {
			var got cnfJSON
			if err := jsoninput.Decode(raw, &got); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("encoding-json-v1-plain", func(b *testing.B) {
		for b.Loop() {
			var got cnfJSON
			if err := jsonv1.Unmarshal(raw, &got); err != nil {
				b.Fatal(err)
			}
		}
	})
}
