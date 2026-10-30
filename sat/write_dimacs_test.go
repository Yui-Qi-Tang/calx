package sat_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"testing"

	"github.com/Yui-Qi-Tang/calx/automaton"
	"github.com/Yui-Qi-Tang/calx/lga"
	"github.com/Yui-Qi-Tang/calx/sat"
)

// fmtDIMACS is the original fmt-based writer, kept as the byte-for-byte reference.
func fmtDIMACS(c sat.CNF) []byte {
	b := fmt.Appendf(nil, "p cnf %d %d\n", len(c.Variables), len(c.Clauses))
	for _, clause := range c.Clauses {
		for _, lit := range clause {
			b = fmt.Appendf(b, "%d ", lit)
		}
		b = fmt.Appendln(b, "0")
	}
	return b
}

func variables(n int) []sat.Variable {
	v := make([]sat.Variable, n)
	for i := range v {
		v[i].ID = i + 1
	}
	return v
}

func hppCNF(tb testing.TB) sat.CNF {
	tb.Helper()
	e, err := automaton.Compile(context.Background(), lga.HPP{Width: 16, Height: 16}, automaton.Query{Steps: 8})
	if err != nil {
		tb.Fatal(err)
	}
	c, err := e.CNF()
	if err != nil {
		tb.Fatal(err)
	}
	return c
}

func TestWriteDIMACSMatchesFmt(t *testing.T) {
	big := variables(123456)
	for name, c := range map[string]sat.CNF{
		"true":        {Variables: []sat.Variable{}, Clauses: [][]int{}},
		"false":       {Variables: []sat.Variable{}, Clauses: [][]int{{}}},
		"signs":       {Variables: variables(3), Clauses: [][]int{{1, -2, 3}, {-1}, {}, {2, -3}}},
		"large ids":   {Variables: big, Clauses: [][]int{{123456, -123456, 99999}, {-1, 10, -100, 1000}}},
		"hpp 16x16x8": hppCNF(t),
	} {
		var got bytes.Buffer
		if err := c.WriteDIMACS(&got); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !bytes.Equal(got.Bytes(), fmtDIMACS(c)) {
			t.Errorf("%s: bytes differ from the fmt reference", name)
		}
	}
}

type failWriter struct{ err error }

func (f failWriter) Write([]byte) (int, error) { return 0, f.err }

func TestWriteDIMACSReturnsWriterError(t *testing.T) {
	sentinel := errors.New("write failed")
	c := sat.CNF{Variables: variables(1), Clauses: [][]int{{1}}}
	if err := c.WriteDIMACS(failWriter{sentinel}); !errors.Is(err, sentinel) {
		t.Fatalf("got %v, want %v", err, sentinel)
	}
}

func BenchmarkWriteDIMACS(b *testing.B) {
	c := hppCNF(b)
	for b.Loop() {
		if err := c.WriteDIMACS(io.Discard); err != nil {
			b.Fatal(err)
		}
	}
}
