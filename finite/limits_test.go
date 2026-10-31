package finite_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/Yui-Qi-Tang/calx/finite"
)

// limitModel has one field per entry of sizes. Each action guards and assigns
// field 0 only and frames the rest; IDs and names sort in declaration order.
func limitModel(sizes []int, actions int) finite.Model {
	m := finite.Model{Fields: map[string][]string{}, Forbidden: []finite.State{}}
	var frame []string
	for i, size := range sizes {
		field := fmt.Sprintf("f%02d", i)
		for v := range size {
			m.Fields[field] = append(m.Fields[field], fmt.Sprintf("v%02d", v))
		}
		if i > 0 {
			frame = append(frame, field)
		}
	}
	for i := range actions {
		m.Actions = append(m.Actions, finite.Action{
			ID:     fmt.Sprintf("a%04d", i),
			Guard:  finite.State{"f00": fmt.Sprintf("v%02d", i%sizes[0])},
			Effect: finite.State{"f00": fmt.Sprintf("v%02d", (i+1)%sizes[0])},
			Frame:  frame,
		})
	}
	return m
}

func limitQuery(sizes []int, horizon int) finite.Query {
	start := finite.State{}
	for i := range sizes {
		start[fmt.Sprintf("f%02d", i)] = "v00"
	}
	return finite.Query{Start: start, Goal: finite.State{"f00": fmt.Sprintf("v%02d", horizon%sizes[0])}, Horizon: horizon}
}

func repeated(size, fields int) []int {
	out := make([]int, fields)
	for i := range out {
		out[i] = size
	}
	return out
}

// TestAcceptedEncodingsAreUnchanged pins models that fit the v1 limits before
// the domain product check was removed and actions were raised from 64. The
// values were captured from that code.
func TestAcceptedEncodingsAreUnchanged(t *testing.T) {
	for name, c := range map[string]struct {
		sizes               []int
		actions, horizon    int
		wantDigest, wantCNF string
	}{
		"4096 states, 64 actions": {sizes: repeated(2, 12), actions: 64, horizon: 3, wantDigest: "4bc2e72b08bd8553a90ff84f17eb2dbf62da54bf60185441f195f56152aca506", wantCNF: "1f3182ca63c78db5bc7fdfdf7ac0273f948ab129795f48142b3dbabb5af14008"},
		"64 by 64 states":         {sizes: []int{64, 64}, actions: 64, horizon: 2, wantDigest: "6d438cf20c0f75b82dbd1bd784be6f2d716c1443ff1b6b1a31516a48cf2f7c4d", wantCNF: "7b504cdb9b89cd14ef4147d425093075f95153edd841cd2b712f8509476e971b"},
		"small":                   {sizes: []int{4, 3}, actions: 5, horizon: 3, wantDigest: "bd9f5bbe7268d81cc1a83a985bd4ee13f5a322820561da7465396080507b1da7", wantCNF: "7e50d3721c722e47963ca29b3c47cc9a6e0643a2d780e2d3676a54413670cf71"},
	} {
		t.Run(name, func(t *testing.T) {
			e, err := finite.Compile(context.Background(), limitModel(c.sizes, c.actions), limitQuery(c.sizes, c.horizon))
			if err != nil {
				t.Fatal(err)
			}
			d, err := e.Digest()
			if err != nil {
				t.Fatal(err)
			}
			f, err := e.CNF()
			if err != nil {
				t.Fatal(err)
			}
			var dimacs strings.Builder
			if err := f.WriteDIMACS(&dimacs); err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256([]byte(dimacs.String()))
			if got := hex.EncodeToString(sum[:]); string(d) != c.wantDigest || got != c.wantCNF {
				t.Fatalf("digest %s, CNF %s", d, got)
			}
		})
	}
}

func TestDomainProductIsNotALimit(t *testing.T) {
	for name, c := range map[string]struct {
		sizes   []int
		horizon int
	}{
		"above the former 4096": {sizes: repeated(2, 13), horizon: 3},
		"16 fields of 64":       {sizes: repeated(64, 16), horizon: 2},
	} {
		t.Run(name, func(t *testing.T) {
			m := limitModel(c.sizes, 8)
			matrix, err := finite.Project(m)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := len(matrix.Coordinates), c.sizes[0]*len(c.sizes); got != want {
				t.Fatalf("%d coordinates, want %d", got, want)
			}
			if _, err := finite.Compile(context.Background(), m, limitQuery(c.sizes, c.horizon)); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestActionLimit(t *testing.T) {
	sizes := []int{4, 3}
	for _, actions := range []int{65, 243, 1024} {
		t.Run(fmt.Sprint(actions), func(t *testing.T) {
			m := limitModel(sizes, actions)
			if _, err := finite.Compile(context.Background(), m, limitQuery(sizes, 1)); err != nil {
				t.Fatal(err)
			}
			matrix, err := finite.Project(m)
			if err != nil {
				t.Fatal(err)
			}
			back, err := matrix.Model(m.Forbidden)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(back, m) {
				t.Fatal("round trip changed the model")
			}
		})
	}
	over := limitModel(sizes, 1025)
	if err := over.Validate(); !errors.Is(err, finite.ErrInput) {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := finite.Project(over); !errors.Is(err, finite.ErrInput) {
		t.Fatalf("Project: %v", err)
	}
	matrix, err := finite.Project(limitModel(sizes, 1024))
	if err != nil {
		t.Fatal(err)
	}
	matrix.ActionIDs = append(matrix.ActionIDs, "extra")
	if _, err := matrix.Model(nil); !errors.Is(err, finite.ErrInput) {
		t.Fatalf("Matrix.Model: %v", err)
	}
}

func TestOverBudgetIsAnEncodingLimit(t *testing.T) {
	for name, c := range map[string]struct {
		sizes            []int
		actions, horizon int
	}{
		"actions": {sizes: []int{4, 3}, actions: 1024, horizon: 8},
		"frames":  {sizes: repeated(64, 16), actions: 64, horizon: 8},
		"horizon": {sizes: repeated(2, 13), actions: 8, horizon: 4095},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := finite.Compile(context.Background(), limitModel(c.sizes, c.actions), limitQuery(c.sizes, c.horizon))
			if !errors.Is(err, finite.ErrEncodingLimit) || errors.Is(err, finite.ErrInput) {
				t.Fatal(err)
			}
		})
	}
}
