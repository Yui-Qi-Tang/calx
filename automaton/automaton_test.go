package automaton

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/Yui-Qi-Tang/calx/sat"
)

// elementary is a one-bit periodic elementary cellular automaton.
type elementary struct {
	cells int
	rule  uint8
}

func (e elementary) Cells() int { return e.cells }
func (e elementary) Bits() int  { return 1 }
func (e elementary) Inputs(c int) []Bit {
	return []Bit{{Cell: (c + e.cells - 1) % e.cells}, {Cell: c}, {Cell: (c + 1) % e.cells}}
}
func (e elementary) Next(_ int, in, out []bool) {
	index := 0
	for _, v := range in {
		index <<= 1
		if v {
			index |= 1
		}
	}
	out[0] = e.rule>>index&1 == 1
}

// pair has two bits per cell that read across cells.
type pair struct{}

func (pair) Cells() int                 { return 2 }
func (pair) Bits() int                  { return 2 }
func (pair) Inputs(c int) []Bit         { return []Bit{{Cell: c, Index: 0}, {Cell: 1 - c, Index: 1}} }
func (pair) Next(_ int, in, out []bool) { out[0], out[1] = in[1], in[0] != in[1] }

// coin flips each cell's left neighbor when its choice bit is set.
type coin struct{ cells int }

func (k coin) Cells() int                 { return k.cells }
func (k coin) Bits() int                  { return 1 }
func (k coin) Inputs(c int) []Bit         { return []Bit{{Cell: (c + k.cells - 1) % k.cells}} }
func (k coin) Choices(int) int            { return 1 }
func (k coin) Next(_ int, in, out []bool) { out[0] = in[0] != in[1] }

// oracleFollows evaluates the rule directly, independent of the encoder and of
// shape.step, and reports whether t is a valid run.
func oracleFollows(sys System, t Trajectory) bool {
	chooser, hasChoices := sys.(Chooser)
	bits := sys.Bits()
	for step := 0; step+1 < len(t.States); step++ {
		choice := 0
		for c := range sys.Cells() {
			var in []bool
			for _, b := range sys.Inputs(c) {
				in = append(in, t.States[step][b.Cell*bits+b.Index])
			}
			if hasChoices {
				n := chooser.Choices(c)
				in = append(in, t.Choices[step][choice:choice+n]...)
				choice += n
			}
			out := make([]bool, bits)
			sys.Next(c, in, out)
			if !slices.Equal(out, t.States[step+1][c*bits:(c+1)*bits]) {
				return false
			}
		}
	}
	return true
}

func oracleMatches(s State, p Pattern, bits int) bool {
	for b, v := range p {
		if s[b.Cell*bits+b.Index] != v {
			return false
		}
	}
	return true
}

// oracleMeets reports whether t meets the start, goal and forbidden patterns of q.
func oracleMeets(q Query, t Trajectory, bits int) bool {
	if !oracleMatches(t.States[0], q.Start, bits) || !oracleMatches(t.States[q.Steps], q.Goal, bits) {
		return false
	}
	for _, s := range t.States {
		for _, p := range q.Forbidden {
			if oracleMatches(s, p, bits) {
				return false
			}
		}
	}
	return true
}

// TestExhaustiveOracle enumerates every assignment of every small formula and
// requires the satisfying assignments to be exactly the runs the oracle accepts.
func TestExhaustiveOracle(t *testing.T) {
	cases := []struct {
		name string
		sys  System
		q    Query
	}{
		{"rule90-free", elementary{cells: 4, rule: 90}, Query{Steps: 3}},
		{"rule110-reach", elementary{cells: 4, rule: 110}, Query{Steps: 2, Start: Pattern{{Cell: 0}: true, {Cell: 1}: false}, Goal: Pattern{{Cell: 3}: true}}},
		{"rule30-predecessor", elementary{cells: 5, rule: 30}, Query{Steps: 1, Goal: Pattern{{Cell: 0}: true, {Cell: 1}: true, {Cell: 2}: true, {Cell: 3}: true, {Cell: 4}: false}}},
		{"rule30-forbidden", elementary{cells: 4, rule: 30}, Query{Steps: 2, Forbidden: []Pattern{{{Cell: 0}: true, {Cell: 1}: true}}}},
		{"rule90-zero-steps", elementary{cells: 4, rule: 90}, Query{Steps: 0, Start: Pattern{{Cell: 2}: true}, Goal: Pattern{{Cell: 2}: false}}},
		{"pair", pair{}, Query{Steps: 2, Goal: Pattern{{Cell: 1, Index: 1}: true}}},
		{"coin", coin{cells: 3}, Query{Steps: 2, Start: Pattern{{Cell: 0}: true, {Cell: 1}: false, {Cell: 2}: false}, Goal: Pattern{{Cell: 2}: true}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, err := Compile(context.Background(), tc.sys, tc.q)
			if err != nil {
				t.Fatal(err)
			}
			cnf, err := e.CNF()
			if err != nil {
				t.Fatal(err)
			}
			n := len(cnf.Variables)
			if n > 20 {
				t.Fatalf("%d variables is too many to enumerate", n)
			}
			values := make(map[int]bool, n)
			accepted := 0
			for mask := range 1 << n {
				for id := 1; id <= n; id++ {
					values[id] = mask>>(id-1)&1 == 1
				}
				run := e.layout.trajectory(values)
				got := cnf.CheckAssignment(values) == nil
				if want := oracleFollows(tc.sys, run) && oracleMeets(tc.q, run, tc.sys.Bits()); got != want {
					t.Fatalf("assignment %b: formula %v, oracle %v", mask, got, want)
				}
				if !got {
					continue
				}
				accepted++
				decoded, err := e.Decode(values)
				if err != nil {
					t.Fatal(err)
				}
				if err := Replay(tc.sys, decoded); err != nil {
					t.Fatal(err)
				}
				simulated, err := Simulate(tc.sys, decoded.States[0], tc.q.Steps, func(step, c int) []bool {
					s := e.layout.shape
					return decoded.Choices[step][s.offsets[c] : s.offsets[c]+s.choices[c]]
				})
				if err != nil {
					t.Fatal(err)
				}
				if !slices.EqualFunc(simulated, decoded.States, slices.Equal) {
					t.Fatal("simulation differs from decoded trajectory")
				}
			}
			t.Logf("%d variables, %d accepted runs", n, accepted)
		})
	}
}

func TestReplayRejectsWrongStep(t *testing.T) {
	sys := elementary{cells: 4, rule: 90}
	states, err := Simulate(sys, State{true, false, false, false}, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	run := Trajectory{States: states, Choices: [][]bool{{}, {}}}
	if err := Replay(sys, run); err != nil {
		t.Fatal(err)
	}
	run.States[2][1] = !run.States[2][1]
	if err := Replay(sys, run); !errors.Is(err, ErrReplay) {
		t.Fatalf("got %v, want ErrReplay", err)
	}
}

// badSystem lets each validation case override one method.
type badSystem struct {
	cells, bits, choices int
	inputs               func(c int) []Bit
}

func (b badSystem) Cells() int            { return b.cells }
func (b badSystem) Bits() int             { return b.bits }
func (b badSystem) Inputs(c int) []Bit    { return b.inputs(c) }
func (b badSystem) Choices(int) int       { return b.choices }
func (badSystem) Next(_ int, _, _ []bool) {}

func TestCompileRejectsInvalidInput(t *testing.T) {
	none := func(int) []Bit { return nil }
	cases := []struct {
		name string
		sys  System
		q    Query
		want error
	}{
		{"nil system", nil, Query{}, ErrInput},
		{"no cells", badSystem{cells: 0, bits: 1, inputs: none}, Query{}, ErrInput},
		{"too many bits", badSystem{cells: 1, bits: maxBits + 1, inputs: none}, Query{}, ErrInput},
		{"too many cells", badSystem{cells: maxCells + 1, bits: 1, inputs: none}, Query{}, ErrEncodingLimit},
		{"input out of range", badSystem{cells: 1, bits: 1, inputs: func(int) []Bit { return []Bit{{Cell: 1}} }}, Query{}, ErrInput},
		{"duplicate input", badSystem{cells: 1, bits: 1, inputs: func(int) []Bit { return []Bit{{}, {}} }}, Query{}, ErrInput},
		{"negative choices", badSystem{cells: 1, bits: 1, choices: -1, inputs: none}, Query{}, ErrInput},
		{"fan-in", badSystem{cells: 13, bits: 1, choices: 1, inputs: func(int) []Bit {
			in := make([]Bit, 12)
			for i := range in {
				in[i] = Bit{Cell: i}
			}
			return in
		}}, Query{}, ErrInput},
		{"negative steps", elementary{cells: 3, rule: 90}, Query{Steps: -1}, ErrInput},
		{"start out of range", elementary{cells: 3, rule: 90}, Query{Start: Pattern{{Cell: 3}: true}}, ErrInput},
		{"goal bit index", elementary{cells: 3, rule: 90}, Query{Goal: Pattern{{Cell: 0, Index: 1}: true}}, ErrInput},
		{"empty forbidden", elementary{cells: 3, rule: 90}, Query{Forbidden: []Pattern{{}}}, ErrInput},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Compile(context.Background(), tc.sys, tc.q); !errors.Is(err, tc.want) {
				t.Fatalf("got %v, want %v", err, tc.want)
			}
		})
	}
}

// counting records how often the rule is evaluated.
type counting struct {
	elementary
	calls *int
}

func (c counting) Next(cell int, in, out []bool) {
	*c.calls++
	c.elementary.Next(cell, in, out)
}

func TestBudgetIsCheckedBeforeRuleEvaluation(t *testing.T) {
	calls := 0
	sys := counting{cells: 8, rule: 30, calls: &calls}
	for name, opt := range map[string]Option{"variables": WithMaxVariables(15), "clauses": WithMaxClauses(63), "literals": WithMaxLiterals(255)} {
		if _, err := Compile(context.Background(), sys, Query{Steps: 1}, opt); !errors.Is(err, sat.ErrLimit) {
			t.Fatalf("%s: got %v, want limit", name, err)
		}
	}
	if calls != 0 {
		t.Fatalf("rule evaluated %d times before the budget check", calls)
	}
	// Exactly at budget: 16 variables, 8 cells * 8 rows = 64 clauses of 4 literals.
	if _, err := Compile(context.Background(), sys, Query{Steps: 1}, WithMaxVariables(16), WithMaxClauses(64), WithMaxLiterals(256)); err != nil {
		t.Fatal(err)
	}
	if calls != 8*8 {
		t.Fatalf("rule evaluated %d times, want once per cell and row", calls)
	}
}

func TestCompileHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A canceled context is reported before any validation.
	if _, err := Compile(ctx, elementary{cells: 3, rule: 90}, Query{Steps: -1}); !errors.Is(err, context.Canceled) {
		t.Fatalf("invalid query: got %v, want context.Canceled", err)
	}
}

// cancelling cancels its context on the at-th rule evaluation.
type cancelling struct {
	counting
	at     int
	cancel context.CancelFunc
}

func (c cancelling) Next(cell int, in, out []bool) {
	c.counting.Next(cell, in, out)
	if *c.calls == c.at {
		c.cancel()
	}
}

func TestCompileStopsWhenCanceledMidway(t *testing.T) {
	const cells, rows = 8, 8 // rule 30 reads three bits: 8 rows per cell
	for name, tc := range map[string]struct {
		at, maxCalls int
	}{
		"while building rule tables": {at: 1, maxCalls: rows},                    // stops before the next cell
		"while encoding clauses":     {at: cells * rows, maxCalls: cells * rows}, // tables done, encoding stops
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			sys := cancelling{counting: counting{cells: cells, rule: 30, calls: &calls}, at: tc.at, cancel: cancel}
			if _, err := Compile(ctx, sys, Query{Steps: 1}); !errors.Is(err, context.Canceled) {
				t.Fatalf("got %v, want context.Canceled", err)
			}
			if calls > tc.maxCalls {
				t.Fatalf("rule evaluated %d times after cancellation, want at most %d", calls, tc.maxCalls)
			}
		})
	}
}

func TestBudgetOptions(t *testing.T) {
	b, err := newBudget(nil)
	if err != nil {
		t.Fatal(err)
	}
	l, err := sat.Limits{}.WithDefaults()
	if err != nil {
		t.Fatal(err)
	}
	if b.variables != int64(l.MaxVariables) || b.clauses != int64(l.MaxClauses) || b.literals != int64(l.MaxLiterals) {
		t.Fatalf("defaults %+v differ from sat.Limits defaults %+v", b, l)
	}
	if b, err = newBudget([]Option{WithMaxClauses(5), WithMaxClauses(7)}); err != nil || b.clauses != 7 {
		t.Fatalf("later option must win: %+v %v", b, err)
	}
	for name, opts := range map[string][]Option{
		"zero variables":   {WithMaxVariables(0)},
		"negative clauses": {WithMaxClauses(-1)},
		"zero literals":    {WithMaxLiterals(0)},
	} {
		if _, err := Compile(context.Background(), elementary{cells: 3, rule: 90}, Query{Steps: 1}, opts...); !errors.Is(err, ErrInput) {
			t.Errorf("%s: got %v, want ErrInput", name, err)
		}
	}
}

func TestDigestIdentifiesTablesAndQuery(t *testing.T) {
	digestOf := func(sys System, q Query) string {
		t.Helper()
		e, err := Compile(context.Background(), sys, q)
		if err != nil {
			t.Fatal(err)
		}
		d, err := e.Digest()
		if err != nil {
			t.Fatal(err)
		}
		return d.String()
	}
	q := Query{Steps: 2, Start: Pattern{{Cell: 0}: true, {Cell: 2}: false}}
	base := digestOf(elementary{cells: 4, rule: 90}, q)
	if again := digestOf(elementary{cells: 4, rule: 90}, Query{Steps: 2, Start: Pattern{{Cell: 2}: false, {Cell: 0}: true}}); again != base {
		t.Fatal("digest depends on pattern map order")
	}
	for name, other := range map[string]string{
		"rule":  digestOf(elementary{cells: 4, rule: 110}, q),
		"steps": digestOf(elementary{cells: 4, rule: 90}, Query{Steps: 1, Start: q.Start}),
		"start": digestOf(elementary{cells: 4, rule: 90}, Query{Steps: 2, Start: Pattern{{Cell: 0}: false, {Cell: 2}: false}}),
	} {
		if other == base {
			t.Errorf("digest ignores %s", name)
		}
	}
}

func TestUninitializedEncoding(t *testing.T) {
	var e *Encoding
	if _, err := e.CNF(); !errors.Is(err, ErrInput) {
		t.Fatal(err)
	}
	if _, err := e.Digest(); !errors.Is(err, ErrInput) {
		t.Fatal(err)
	}
	if _, err := e.Decode(nil); !errors.Is(err, ErrInput) {
		t.Fatal(err)
	}
}

func TestCNFIsIndependentCopy(t *testing.T) {
	e, err := Compile(context.Background(), elementary{cells: 3, rule: 90}, Query{Steps: 1})
	if err != nil {
		t.Fatal(err)
	}
	c, err := e.CNF()
	if err != nil {
		t.Fatal(err)
	}
	c.Clauses[0][0] = -c.Clauses[0][0]
	again, err := e.CNF()
	if err != nil {
		t.Fatal(err)
	}
	if again.Clauses[0][0] == c.Clauses[0][0] {
		t.Fatal("CNF shares clause storage with the encoding")
	}
}

func TestSimulateValidatesInput(t *testing.T) {
	cases := []struct {
		name   string
		sys    System
		start  State
		steps  int
		choose func(step, cell int) []bool
	}{
		{"short start", elementary{cells: 3, rule: 90}, State{true}, 1, nil},
		{"negative steps", elementary{cells: 3, rule: 90}, State{true, false, false}, -1, nil},
		{"missing chooser", coin{cells: 3}, State{true, false, false}, 1, nil},
		{"wrong choice count", coin{cells: 3}, State{true, false, false}, 1, func(int, int) []bool { return nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Simulate(tc.sys, tc.start, tc.steps, tc.choose); !errors.Is(err, ErrInput) {
				t.Fatalf("got %v, want ErrInput", err)
			}
		})
	}
}

func ExampleCompile() {
	// Can rule 90 on four cells turn a single live cell into all-dead in two steps?
	sys := elementary{cells: 4, rule: 90}
	q := Query{
		Steps: 2,
		Start: Pattern{{Cell: 0}: true, {Cell: 1}: false, {Cell: 2}: false, {Cell: 3}: false},
		Goal:  Pattern{{Cell: 0}: false, {Cell: 1}: false, {Cell: 2}: false, {Cell: 3}: false},
	}
	e, err := Compile(context.Background(), sys, q)
	if err != nil {
		panic(err)
	}
	cnf, err := e.CNF()
	if err != nil {
		panic(err)
	}
	if _, err := fmt.Println(len(cnf.Variables), "variables,", len(cnf.Clauses), "clauses"); err != nil {
		panic(err)
	}
	// Output: 12 variables, 72 clauses
}

func TestTrajectoryIsZero(t *testing.T) {
	if !(Trajectory{}).IsZero() {
		t.Fatal("zero Trajectory must report IsZero")
	}
	states, err := Simulate(elementary{cells: 3, rule: 90}, State{true, false, false}, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if (Trajectory{States: states}).IsZero() || (Trajectory{Choices: [][]bool{{}}}).IsZero() {
		t.Fatal("non-zero Trajectory reported zero")
	}
}

func TestStepsAreBoundedOnlyByBudget(t *testing.T) {
	sys := elementary{cells: 3, rule: 90}
	// 5,000 steps: 15,003 variables and 120,000 clauses fit the default budget.
	if _, err := Compile(context.Background(), sys, Query{Steps: 5000}); err != nil {
		t.Fatal(err)
	}
	states, err := Simulate(sys, State{true, false, false}, 5000, nil)
	if err != nil || len(states) != 5001 {
		t.Fatalf("%d states, %v", len(states), err)
	}
	// Huge step counts must fail the budget check, not overflow it.
	unlimited := []Option{WithMaxVariables(math.MaxInt), WithMaxClauses(math.MaxInt), WithMaxLiterals(math.MaxInt)}
	for name, opts := range map[string][]Option{"default budget": nil, "maximal budget": unlimited} {
		if _, err := Compile(context.Background(), sys, Query{Steps: math.MaxInt}, opts...); !errors.Is(err, ErrEncodingLimit) {
			t.Errorf("%s: got %v, want ErrEncodingLimit", name, err)
		}
	}
}
