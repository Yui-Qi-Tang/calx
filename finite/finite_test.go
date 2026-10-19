package finite_test

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Yui-Qi-Tang/calx/finite"
	"github.com/Yui-Qi-Tang/calx/sat"
)

// Independent tiny DPLL oracle for tests only, not a production backend.
func solve(c sat.CNF) (map[int]bool, bool) {
	var visit func(map[int]bool) (map[int]bool, bool)
	visit = func(a map[int]bool) (map[int]bool, bool) {
		for {
			changed := false
			all := true
			for _, row := range c.Clauses {
				satisfied := false
				unknown := 0
				unit := 0
				for _, v := range row {
					id := v
					if id < 0 {
						id = -id
					}
					value, ok := a[id]
					if ok && value == (v > 0) {
						satisfied = true
						break
					}
					if !ok {
						unknown++
						unit = v
					}
				}
				if satisfied {
					continue
				}
				all = false
				if unknown == 0 {
					return nil, false
				}
				if unknown == 1 {
					id := unit
					if id < 0 {
						id = -id
					}
					a[id] = unit > 0
					changed = true
				}
			}
			if all {
				for _, v := range c.Variables {
					if _, ok := a[v.ID]; !ok {
						a[v.ID] = false
					}
				}
				return a, true
			}
			if !changed {
				break
			}
		}
		for _, v := range c.Variables {
			if _, ok := a[v.ID]; ok {
				continue
			}
			for _, value := range []bool{false, true} {
				b := maps.Clone(a)
				b[v.ID] = value
				if got, ok := visit(b); ok {
					return got, true
				}
			}
			return nil, false
		}
		return nil, false
	}
	return visit(map[int]bool{})
}
func model(mask int) finite.Model {
	m := finite.Model{Fields: map[string][]string{"x": {"a", "b", "c"}}}
	for i := range 3 {
		for j := range 3 {
			if mask&(1<<(i*3+j)) != 0 {
				u, v := string(rune('a'+i)), string(rune('a'+j))
				m.Actions = append(m.Actions, finite.Action{ID: u + v, Guard: finite.State{"x": u}, Effect: finite.State{"x": v}})
			}
		}
	}
	return m
}
func query(h int) finite.Query {
	return finite.Query{Profile: finite.Profile, Start: finite.State{"x": "a"}, Goal: finite.State{"x": "c"}, Horizon: h}
}

// Enumerate paths directly from action rules, without reading matrix or CNF.
func paths(m finite.Model, h int) []string {
	out := []string{}
	var walk func(string, []string, map[string]bool)
	walk = func(state string, seq []string, seen map[string]bool) {
		if state == "c" {
			if len(seq) == h {
				out = append(out, strings.Join(seq, ","))
			}
			return
		}
		if len(seq) == h {
			return
		}
		for _, a := range m.Actions {
			if a.Guard["x"] != state {
				continue
			}
			next := a.Effect["x"]
			if seen[next] {
				continue
			}
			n := maps.Clone(seen)
			n[next] = true
			walk(next, append(slices.Clone(seq), a.ID), n)
		}
	}
	walk("a", nil, map[string]bool{"a": true})
	slices.Sort(out)
	return out
}
func TestExhaustiveThreeStateTransitionSystems(t *testing.T) {
	for mask := 1; mask < 512; mask++ {
		m := model(mask)
		for h := 0; h <= 3; h++ {
			q := query(h)
			got := []string{}
			for {
				e, err := finite.Compile(context.Background(), m, q, finite.Options{})
				if err != nil {
					t.Fatal(mask, h, err)
				}
				a, ok := solve(checkedCNF(t, e))
				if !ok {
					break
				}
				trace, err := e.Decode(a)
				if err != nil {
					t.Fatal(err)
				}
				key := strings.Join(trace.Actions, ",")
				if slices.Contains(got, key) {
					t.Fatal("duplicate path")
				}
				got = append(got, key)
				q.Blocked = append(q.Blocked, trace.Actions)
			}
			slices.Sort(got)
			want := paths(m, h)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("mask=%d horizon=%d got=%v want=%v", mask, h, got, want)
			}
		}
	}
}
func TestValidationAndBinding(t *testing.T) {
	m := model(1 << 2)
	q := query(1)
	e, err := finite.Compile(context.Background(), m, q, finite.Options{})
	if err != nil {
		t.Fatal(err)
	}
	a, ok := solve(checkedCNF(t, e))
	if !ok {
		t.Fatal("expected path")
	}
	formula := checkedCNF(t, e)
	formula.Clauses[0][0] = 0
	formula.Variables[0].Metadata["finite_variable"] = "changed"
	if _, err := e.Decode(a); err != nil {
		t.Fatal("alias", err)
	}
	q.Blocked = [][]string{{"ac"}}
	blocked, err := finite.Compile(context.Background(), m, q, finite.Options{})
	if err != nil {
		t.Fatal(err)
	}
	blockedHash, err := blocked.Digest()
	if err != nil {
		t.Fatal(err)
	}
	originalHash, err := e.Digest()
	if err != nil {
		t.Fatal(err)
	}
	if blockedHash == originalHash {
		t.Fatal("unbound blocking")
	}
	if _, ok := solve(checkedCNF(t, blocked)); ok {
		t.Fatal("blocked query SAT")
	}
	for name, edit := range map[string]func(*finite.Model, *finite.Query){
		"missing field":        func(m *finite.Model, q *finite.Query) { q.Start = finite.State{} },
		"unknown goal":         func(m *finite.Model, q *finite.Query) { q.Goal = finite.State{"missing": "a"} },
		"effect frame overlap": func(m *finite.Model, q *finite.Query) { m.Actions[0].Frame = []string{"x"} },
		"invalid effect":       func(m *finite.Model, q *finite.Query) { m.Actions[0].Effect["x"] = "d" },
		"negative horizon":     func(m *finite.Model, q *finite.Query) { q.Horizon = -1 },
		"blocked length":       func(m *finite.Model, q *finite.Query) { q.Blocked = [][]string{{}} },
		"unknown action":       func(m *finite.Model, q *finite.Query) { q.Blocked = [][]string{{"missing"}} },
	} {
		t.Run(name, func(t *testing.T) {
			m, q := model(4), query(1)
			edit(&m, &q)
			if _, err := finite.Compile(context.Background(), m, q, finite.Options{}); !errors.Is(err, finite.ErrInput) {
				t.Fatal(err)
			}
		})
	}
	q = query(1)
	q.Profile = "all-walks"
	if _, err := finite.Compile(context.Background(), m, q, finite.Options{}); !errors.Is(err, finite.ErrUnsupported) {
		t.Fatal(err)
	}
	if _, err := finite.Compile(context.Background(), m, query(1), finite.Options{MaxVariables: 1}); !errors.Is(err, sat.ErrLimit) {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := finite.Compile(ctx, m, query(1), finite.Options{}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	var zero finite.Encoding
	if _, err := zero.Decode(nil); err == nil {
		t.Fatal("zero encoding accepted")
	}
}
func TestMatrixRoundTripAndFrames(t *testing.T) {
	m := finite.Model{Fields: map[string][]string{"x": {"a", "b"}, "y": {"keep", "other"}}, Actions: []finite.Action{{ID: "move", Guard: finite.State{"x": "a"}, Effect: finite.State{"x": "b"}, Frame: []string{"y"}}}}
	p, err := finite.Project(m)
	if err != nil {
		t.Fatal(err)
	}
	back, err := p.Model(nil)
	if err != nil {
		t.Fatal(err)
	}
	again, err := finite.Project(back)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(p, again) {
		t.Fatal("matrix roundtrip")
	}
	q := finite.Query{Profile: finite.Profile, Start: finite.State{"x": "a", "y": "keep"}, Goal: finite.State{"x": "b"}, Horizon: 1}
	e, err := finite.Compile(context.Background(), m, q, finite.Options{})
	if err != nil {
		t.Fatal(err)
	}
	a, ok := solve(checkedCNF(t, e))
	if !ok {
		t.Fatal("SAT expected")
	}
	trace, err := e.Decode(a)
	if err != nil || trace.States[1]["y"] != "keep" {
		t.Fatal(trace, err)
	}
	p.Effect[0][1] = 1
	p.Effect[0][0] = 1
	if _, err := p.Model(nil); err == nil {
		t.Fatal("multiple effects accepted")
	}
}
func ExampleCompile() {
	m := finite.Model{Fields: map[string][]string{"state": {"off", "on"}}, Actions: []finite.Action{{ID: "enable", Guard: finite.State{"state": "off"}, Effect: finite.State{"state": "on"}}}}
	q := finite.Query{Profile: finite.Profile, Start: finite.State{"state": "off"}, Goal: finite.State{"state": "on"}, Horizon: 1}
	e, err := finite.Compile(context.Background(), m, q, finite.Options{})
	if err != nil {
		panic(err)
	}
	cnf, err := e.CNF()
	if err != nil {
		panic(err)
	}
	if _, err := fmt.Println(len(cnf.Variables) > 0); err != nil {
		panic(err)
	}
	// Output: true
}

func TestNativeInputIsolationAndUTF8(t *testing.T) {
	m, q := model(4), query(1)
	e, err := finite.Compile(context.Background(), m, q, finite.Options{})
	if err != nil {
		t.Fatal(err)
	}
	a, ok := solve(checkedCNF(t, e))
	if !ok {
		t.Fatal("expected SAT")
	}
	before, err := e.Decode(a)
	if err != nil {
		t.Fatal(err)
	}
	m.Fields["x"][0] = "mutated"
	m.Actions[0].ID = "mutated"
	m.Actions[0].Effect["x"] = "mutated"
	q.Start["x"] = "mutated"
	q.Goal["x"] = "mutated"
	v, err := e.Variables()
	if err != nil {
		t.Fatal(err)
	}
	v[0].Value = "mutated"
	after, err := e.Decode(a)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("input alias", after, err)
	}
	m = model(4)
	m.Fields["x"][0] = string([]byte{255})
	if _, err := finite.Project(m); !errors.Is(err, finite.ErrInput) {
		t.Fatal("invalid UTF-8 model accepted", err)
	}
	m = model(4)
	m.Actions[0].ID = string([]byte{254})
	if err := m.Validate(); !errors.Is(err, finite.ErrInput) {
		t.Fatal(err)
	}
}

func TestZeroHorizonBlockingAndForbiddenStates(t *testing.T) {
	m := model(4)
	q := query(0)
	q.Goal = finite.State{"x": "a"}
	e, err := finite.Compile(context.Background(), m, q, finite.Options{})
	if err != nil {
		t.Fatal(err)
	}
	a, ok := solve(checkedCNF(t, e))
	if !ok {
		t.Fatal("zero-step goal must be SAT")
	}
	trace, err := e.Decode(a)
	if err != nil || len(trace.Actions) != 0 || len(trace.States) != 1 {
		t.Fatal(trace, err)
	}
	q.Blocked = [][]string{{}}
	e, err = finite.Compile(context.Background(), m, q, finite.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := solve(checkedCNF(t, e)); ok {
		t.Fatal("empty action sequence was not blocked")
	}
	q = query(1)
	m.Forbidden = []finite.State{{"x": "c"}}
	e, err = finite.Compile(context.Background(), m, q, finite.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := solve(checkedCNF(t, e)); ok {
		t.Fatal("forbidden goal was accepted")
	}
}

func checkedCNF(t *testing.T, encoding *finite.Encoding) sat.CNF {
	t.Helper()
	formula, err := encoding.CNF()
	if err != nil {
		t.Fatal(err)
	}
	return formula
}
