package jsoncompat_test

import (
	"context"
	jsonv1 "encoding/json"
	"testing"

	"github.com/Yui-Qi-Tang/calx/automaton"
	"github.com/Yui-Qi-Tang/calx/internal/jsoncompat"
	"github.com/Yui-Qi-Tang/calx/lga"
	"github.com/Yui-Qi-Tang/calx/sat"
)

// benchCNF is a 16x16 HPP run of 8 steps: 131,072 clauses, about 1.9 MB of JSON.
func benchCNF(b *testing.B) sat.CNF {
	b.Helper()
	e, err := automaton.Compile(context.Background(), lga.HPP{Width: 16, Height: 16}, automaton.Query{Steps: 8})
	if err != nil {
		b.Fatal(err)
	}
	c, err := e.CNF()
	if err != nil {
		b.Fatal(err)
	}
	return c
}

// BenchmarkMarshalCNF compares encoding/json (v1) with the v2-based
// jsoncompat.Marshal that Calx uses for input.json and digests.
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
