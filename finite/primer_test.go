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
)

// The tests in this file pin the claims of docs/finite-primer.md without a SAT
// solver: answers come from the test-only DPLL oracle solve.

func doorLight() finite.Model {
	return finite.Model{
		Fields:    map[string][]string{"door": {"closed", "locked", "open"}, "light": {"off", "on"}},
		Forbidden: []finite.State{{"door": "open", "light": "off"}},
		Actions: []finite.Action{
			{ID: "open", Guard: finite.State{"door": "closed"}, Effect: finite.State{"door": "open"}, Frame: []string{"light"}},
			{ID: "close", Guard: finite.State{"door": "open"}, Effect: finite.State{"door": "closed"}, Frame: []string{"light"}},
			{ID: "lock", Guard: finite.State{"door": "closed"}, Effect: finite.State{"door": "locked"}, Frame: []string{"light"}},
			{ID: "on", Guard: finite.State{"light": "off"}, Effect: finite.State{"light": "on"}, Frame: []string{"door"}},
			{ID: "off", Guard: finite.State{"light": "on"}, Effect: finite.State{"light": "off"}, Frame: []string{"door"}},
		},
	}
}

// primerAnswer compiles a query on the door/light model and returns the
// decoded actions, or nil and false for UNSAT.
func primerAnswer(t *testing.T, q finite.Query) ([]string, bool) {
	t.Helper()
	trace, ok := primerTrace(t, q)
	return trace.Actions, ok
}

func primerTrace(t *testing.T, q finite.Query) (finite.Trace, bool) {
	t.Helper()
	e, err := finite.Compile(context.Background(), doorLight(), q)
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.CNF()
	if err != nil {
		t.Fatal(err)
	}
	assignment, ok := solve(c)
	if !ok {
		return finite.Trace{}, false
	}
	trace, err := e.Decode(assignment)
	if err != nil {
		t.Fatal(err)
	}
	return trace, true
}

func TestFinitePrimerStates(t *testing.T) {
	if finite.Profile != "categorical-simple-first-hit/v1" {
		t.Fatalf("profile %q", finite.Profile)
	}
	allowed := 0
	for _, door := range []string{"closed", "locked", "open"} {
		for _, light := range []string{"off", "on"} {
			s := finite.State{"door": door, "light": light}
			if _, err := finite.Compile(context.Background(), doorLight(), finite.Query{Start: s, Goal: s}); err == nil {
				allowed++
			}
		}
	}
	if allowed != 5 {
		t.Fatalf("%d allowed states, want 5", allowed)
	}
	trace, ok := primerTrace(t, finite.Query{Start: finite.State{"door": "closed", "light": "off"}, Goal: finite.State{"door": "open"}, Horizon: 2})
	want := []finite.State{{"door": "closed", "light": "off"}, {"door": "closed", "light": "on"}, {"door": "open", "light": "on"}}
	if !ok || !slices.EqualFunc(trace.States, want, maps.Equal) {
		t.Fatalf("trace %v, want %v", trace.States, want)
	}
}

func TestFinitePrimerTransitions(t *testing.T) {
	var states []finite.State
	for _, door := range []string{"closed", "locked", "open"} {
		for _, light := range []string{"off", "on"} {
			states = append(states, finite.State{"door": door, "light": light})
		}
	}
	var got []string
	for _, from := range states {
		for _, to := range states {
			if from["door"] == to["door"] && from["light"] == to["light"] {
				continue
			}
			q := finite.Query{Start: from, Goal: to, Horizon: 1}
			if from["door"] == "open" && from["light"] == "off" {
				// The forbidden state is not a valid start.
				if _, err := finite.Compile(context.Background(), doorLight(), q); !errors.Is(err, finite.ErrInput) {
					t.Fatalf("forbidden start: %v", err)
				}
				continue
			}
			if actions, ok := primerAnswer(t, q); ok {
				got = append(got, fmt.Sprintf("%s,%s -%s-> %s,%s", from["door"], from["light"], actions[0], to["door"], to["light"]))
			}
		}
	}
	want := []string{
		"closed,off -on-> closed,on",
		"closed,off -lock-> locked,off",
		"closed,on -off-> closed,off",
		"closed,on -lock-> locked,on",
		"closed,on -open-> open,on",
		"locked,off -on-> locked,on",
		"locked,on -off-> locked,off",
		"open,on -close-> closed,on",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("transitions\n%v\nwant\n%v", got, want)
	}
}

func TestFinitePrimerMatrix(t *testing.T) {
	m, err := finite.Project(doorLight())
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(m.ActionIDs, []string{"close", "lock", "off", "on", "open"}) {
		t.Fatalf("actions %v", m.ActionIDs)
	}
	guard := [][]int{{0, 0, 1, 0, 0}, {1, 0, 0, 0, 0}, {0, 0, 0, 0, 1}, {0, 0, 0, 1, 0}, {1, 0, 0, 0, 0}}
	effect := [][]int{{1, 0, 0, 0, 0}, {0, 1, 0, 0, 0}, {0, 0, 0, 1, 0}, {0, 0, 0, 0, 1}, {0, 0, 1, 0, 0}}
	frame := [][]int{{0, 1}, {0, 1}, {1, 0}, {1, 0}, {0, 1}}
	if !slices.EqualFunc(m.Guard, guard, slices.Equal) || !slices.EqualFunc(m.Effect, effect, slices.Equal) || !slices.EqualFunc(m.Frame, frame, slices.Equal) {
		t.Fatalf("guard %v effect %v frame %v", m.Guard, m.Effect, m.Frame)
	}
	back, err := m.Model(doorLight().Forbidden)
	if err != nil {
		t.Fatal(err)
	}
	sorted := doorLight()
	slices.SortFunc(sorted.Actions, func(a, b finite.Action) int { return strings.Compare(a.ID, b.ID) })
	if !reflect.DeepEqual(back, sorted) {
		t.Fatalf("Model round trip\n%v\nwant\n%v", back, sorted)
	}
}

func TestFinitePrimerHorizons(t *testing.T) {
	start := finite.State{"door": "closed", "light": "off"}
	for _, c := range []struct {
		goal    finite.State
		horizon int
		blocked [][]string
		want    []string // nil means UNSAT
	}{
		{finite.State{"door": "open"}, 0, nil, nil},
		{finite.State{"door": "open"}, 1, nil, nil},
		{finite.State{"door": "open"}, 2, nil, []string{"on", "open"}},
		{finite.State{"door": "open"}, 3, nil, nil},
		{finite.State{"door": "open"}, 4, nil, nil},
		{finite.State{"door": "open"}, 5, nil, nil},
		{finite.State{"door": "locked"}, 1, nil, []string{"lock"}},
		{finite.State{"door": "locked"}, 2, nil, []string{"on", "lock"}},
		{finite.State{"door": "locked"}, 3, nil, nil},
		{finite.State{"door": "open"}, 2, [][]string{{"on", "open"}}, nil},
	} {
		got, ok := primerAnswer(t, finite.Query{Start: start, Goal: c.goal, Horizon: c.horizon, Blocked: c.blocked})
		if ok != (c.want != nil) || !slices.Equal(got, c.want) {
			t.Errorf("goal %v horizon %d blocked %v: got %v (SAT %v), want %v", c.goal, c.horizon, c.blocked, got, ok, c.want)
		}
	}
}

func TestFinitePrimerVariables(t *testing.T) {
	e, err := finite.Compile(context.Background(), doorLight(), finite.Query{
		Start: finite.State{"door": "closed", "light": "off"}, Goal: finite.State{"door": "open"}, Horizon: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.CNF()
	if err != nil {
		t.Fatal(err)
	}
	variables, err := e.Variables()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Variables) != 31 || len(c.Clauses) != 134 {
		t.Fatalf("%d variables, %d clauses, want 31 and 134", len(c.Variables), len(c.Clauses))
	}
	// The primer's table: five coordinates per time, five sorted actions per
	// transition, then door and light witnesses for times 0–1, 0–2 and 1–2.
	var want []string
	for time := range 3 {
		for _, cell := range []string{"door=closed", "door=locked", "door=open", "light=off", "light=on"} {
			want = append(want, fmt.Sprintf("state %d %s", time, cell))
		}
	}
	for time := range 2 {
		for _, action := range []string{"close", "lock", "off", "on", "open"} {
			want = append(want, fmt.Sprintf("action %d %s", time, action))
		}
	}
	for _, pair := range [][2]int{{0, 1}, {0, 2}, {1, 2}} {
		for _, field := range []string{"door", "light"} {
			want = append(want, fmt.Sprintf("different %d-%d %s", pair[0], pair[1], field))
		}
	}
	var got []string
	for _, v := range variables {
		switch v.Kind {
		case finite.StateVariable:
			got = append(got, fmt.Sprintf("state %d %s=%s", v.Time, v.Field, v.Value))
		case finite.ActionVariable:
			got = append(got, fmt.Sprintf("action %d %s", v.Time, v.Action))
		default:
			got = append(got, fmt.Sprintf("%s %d-%d %s", v.Kind, v.Time, *v.OtherTime, v.Field))
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("variables\n%v\nwant\n%v", got, want)
	}
}
