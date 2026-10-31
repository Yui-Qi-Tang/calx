package finite_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
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

// TestAcceptedEncodingsAreUnchanged pins models accepted before limits were
// raised. The first three fit the limits before the domain product check was
// removed and actions and values were raised from 64; the rest fit the limits
// before fields were raised from 16. The values were captured from that code.
func TestAcceptedEncodingsAreUnchanged(t *testing.T) {
	for name, c := range map[string]struct {
		sizes               []int
		actions, horizon    int
		wantDigest, wantCNF string
	}{
		"4096 states, 64 actions": {sizes: repeated(2, 12), actions: 64, horizon: 3, wantDigest: "4bc2e72b08bd8553a90ff84f17eb2dbf62da54bf60185441f195f56152aca506", wantCNF: "1f3182ca63c78db5bc7fdfdf7ac0273f948ab129795f48142b3dbabb5af14008"},
		"64 by 64 states":         {sizes: []int{64, 64}, actions: 64, horizon: 2, wantDigest: "6d438cf20c0f75b82dbd1bd784be6f2d716c1443ff1b6b1a31516a48cf2f7c4d", wantCNF: "7b504cdb9b89cd14ef4147d425093075f95153edd841cd2b712f8509476e971b"},
		"small":                   {sizes: []int{4, 3}, actions: 5, horizon: 3, wantDigest: "bd9f5bbe7268d81cc1a83a985bd4ee13f5a322820561da7465396080507b1da7", wantCNF: "7e50d3721c722e47963ca29b3c47cc9a6e0643a2d780e2d3676a54413670cf71"},
		"16 fields of 2":          {sizes: repeated(2, 16), actions: 100, horizon: 8, wantDigest: "e3c2c07736011a86b1c6e6436af6185606c87ffcffa40848fd4a7f5a66768de3", wantCNF: "56cd283f80c9666e3270770a2f76c09a955730ef4d64695f0ff67b417eef7eb0"},
		"16 fields of 256":        {sizes: repeated(256, 16), actions: 8, horizon: 0, wantDigest: "b3983d9d9fb35b8a627a8e8083de087191e21391f56a8af4d590aa7e4e7d3d3f", wantCNF: "5dba93f3bfefd336d0462b7d80c6f42c2c87436c4648138d47c70ce0867dc5da"},
		"mixed widths":            {sizes: []int{23, 5, 3, 3, 2, 2}, actions: 243, horizon: 8, wantDigest: "2ea34d9a05b6d19dfdf04f7620b64d43b32425e3ddb1592c5880af8e490bd7cd", wantCNF: "b95c47872d530093e7b7a60f0d6a3d47b5119237519be6d8c9da1a006942d862"},
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
		"16 fields of 256":      {sizes: repeated(256, 16), horizon: 0},
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

func TestValueLimit(t *testing.T) {
	for _, values := range []int{65, 115, 256} {
		t.Run(fmt.Sprint(values), func(t *testing.T) {
			sizes := []int{values, 3}
			m := limitModel(sizes, 243)
			if _, err := finite.Compile(context.Background(), m, limitQuery(sizes, 8)); err != nil {
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
			// Project sorts values, and names past v99 do not sort numerically.
			if got := len(back.Fields["f00"]); got != values {
				t.Fatalf("round trip kept %d values", got)
			}
		})
	}
	if err := limitModel([]int{257, 3}, 1).Validate(); !errors.Is(err, finite.ErrInput) {
		t.Fatalf("Validate: %v", err)
	}
}

func TestFieldLimit(t *testing.T) {
	for _, fields := range []int{17, 64, 256} {
		t.Run(fmt.Sprint(fields), func(t *testing.T) {
			sizes := repeated(2, fields)
			m := limitModel(sizes, 100)
			if _, err := finite.Compile(context.Background(), m, limitQuery(sizes, 8)); err != nil {
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
			// Project sorts fields, and names past f99 do not sort numerically.
			// Actions share one Frame slice.
			slices.Sort(m.Actions[0].Frame)
			if !reflect.DeepEqual(back, m) {
				t.Fatal("round trip changed the model")
			}
		})
	}
	over := limitModel(repeated(2, 257), 1)
	if err := over.Validate(); !errors.Is(err, finite.ErrInput) {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := finite.Project(over); !errors.Is(err, finite.ErrInput) {
		t.Fatalf("Project: %v", err)
	}
	matrix, err := finite.Project(limitModel(repeated(2, 256), 1))
	if err != nil {
		t.Fatal(err)
	}
	matrix.Fields = append(matrix.Fields, "extra")
	matrix.Coordinates = append(matrix.Coordinates, finite.Coordinate{Field: "extra", Value: "v00"})
	matrix.Guard[0] = append(matrix.Guard[0], 0)
	matrix.Effect[0] = append(matrix.Effect[0], 0)
	matrix.Frame[0] = append(matrix.Frame[0], 1)
	if _, err := matrix.Model(nil); !errors.Is(err, finite.ErrInput) {
		t.Fatalf("Matrix.Model: %v", err)
	}
}

func TestCoordinateLimit(t *testing.T) {
	sizes := repeated(16, 256)
	m := limitModel(sizes, 8)
	matrix, err := finite.Project(m)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(matrix.Coordinates); got != 4096 {
		t.Fatalf("%d coordinates", got)
	}
	if _, err := finite.Compile(context.Background(), m, limitQuery(sizes, 0)); err != nil {
		t.Fatal(err)
	}
	sizes[0] = 17
	over := limitModel(sizes, 8)
	if err := over.Validate(); !errors.Is(err, finite.ErrInput) {
		t.Fatalf("Validate: %v", err)
	}
	if _, err := finite.Project(over); !errors.Is(err, finite.ErrInput) {
		t.Fatalf("Project: %v", err)
	}
	matrix.Coordinates = append(matrix.Coordinates, finite.Coordinate{Field: "f00", Value: "extra"})
	for i := range matrix.ActionIDs {
		matrix.Guard[i] = append(matrix.Guard[i], 0)
		matrix.Effect[i] = append(matrix.Effect[i], 0)
	}
	if _, err := matrix.Model(nil); !errors.Is(err, finite.ErrInput) {
		t.Fatalf("Matrix.Model: %v", err)
	}
}

// TestBudgetCountsMixedFields needs the budget check to count each field's own
// values: a formula that exactly fills the budget compiles.
func TestBudgetCountsMixedFields(t *testing.T) {
	sizes := []int{5, 2, 3, 7, 2}
	m, q := limitModel(sizes, 12), limitQuery(sizes, 3)
	e, err := finite.Compile(context.Background(), m, q)
	if err != nil {
		t.Fatal(err)
	}
	f, err := e.CNF()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := finite.Compile(context.Background(), m, q, finite.WithMaxVariables(len(f.Variables)), finite.WithMaxClauses(len(f.Clauses))); err != nil {
		t.Fatal(err)
	}
	_, err = finite.Compile(context.Background(), m, q, finite.WithMaxClauses(len(f.Clauses)-1))
	if !errors.Is(err, finite.ErrEncodingLimit) {
		t.Fatal(err)
	}
}

// toggleModel has 20 on/off fields. Actions set and clear the fields around
// index 16, so paths depend on fields the former limit refused.
func toggleModel() finite.Model {
	sizes := repeated(2, 20)
	m := limitModel(sizes, 0)
	m.Forbidden = []finite.State{{"f15": "v01", "f19": "v01"}}
	for _, i := range []int{15, 17, 19} {
		field := fmt.Sprintf("f%02d", i)
		var frame []string
		for j := range sizes {
			if j != i {
				frame = append(frame, fmt.Sprintf("f%02d", j))
			}
		}
		m.Actions = append(m.Actions,
			finite.Action{ID: "set" + field, Guard: finite.State{field: "v00"}, Effect: finite.State{field: "v01"}, Frame: frame},
			finite.Action{ID: "clear" + field, Guard: finite.State{field: "v01"}, Effect: finite.State{field: "v00"}, Frame: frame},
		)
	}
	return m
}

// statePaths enumerates exact-length simple first-hit paths from the action
// rules, without reading the matrix or the CNF.
func statePaths(m finite.Model, q finite.Query) []string {
	allowed := func(s finite.State) bool {
		return !slices.ContainsFunc(m.Forbidden, func(p finite.State) bool { return includes(s, p) })
	}
	out := []string{}
	var walk func(finite.State, []string, []finite.State)
	walk = func(state finite.State, seq []string, seen []finite.State) {
		if includes(state, q.Goal) {
			if len(seq) == q.Horizon {
				out = append(out, strings.Join(seq, ","))
			}
			return
		}
		if len(seq) == q.Horizon {
			return
		}
		for _, a := range m.Actions {
			if !includes(state, a.Guard) {
				continue
			}
			next := maps.Clone(state)
			maps.Copy(next, a.Effect)
			if !allowed(next) || slices.ContainsFunc(seen, func(s finite.State) bool { return maps.Equal(s, next) }) {
				continue
			}
			walk(next, append(slices.Clone(seq), a.ID), append(slices.Clone(seen), next))
		}
	}
	walk(q.Start, nil, []finite.State{q.Start})
	slices.Sort(out)
	return out
}

func includes(s, pattern finite.State) bool {
	for f, v := range pattern {
		if s[f] != v {
			return false
		}
	}
	return true
}

func TestWideModelPathsMatchEnumeration(t *testing.T) {
	m := toggleModel()
	sizes := repeated(2, 20)
	total := 0
	for horizon := range 5 {
		q := limitQuery(sizes, 0)
		q.Goal, q.Horizon = finite.State{"f17": "v01", "f19": "v01"}, horizon
		want := statePaths(m, q)
		got := []string{}
		for {
			e, err := finite.Compile(context.Background(), m, q)
			if err != nil {
				t.Fatal(horizon, err)
			}
			a, ok := solve(checkedCNF(t, e))
			if !ok {
				break
			}
			trace, err := e.Decode(a)
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, strings.Join(trace.Actions, ","))
			q.Blocked = append(q.Blocked, trace.Actions)
		}
		slices.Sort(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("horizon %d: got %v, want %v", horizon, got, want)
		}
		total += len(want)
	}
	if total == 0 {
		t.Fatal("no paths at any horizon")
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
		"values":  {sizes: repeated(256, 16), actions: 1024, horizon: 1},
		"fields":  {sizes: repeated(2, 256), actions: 1024, horizon: 8},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := finite.Compile(context.Background(), limitModel(c.sizes, c.actions), limitQuery(c.sizes, c.horizon))
			if !errors.Is(err, finite.ErrEncodingLimit) || errors.Is(err, finite.ErrInput) {
				t.Fatal(err)
			}
		})
	}
}
