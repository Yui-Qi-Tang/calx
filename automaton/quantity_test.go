package automaton

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/Yui-Qi-Tang/calx/sat"
)

// shift moves every cell's bit one cell to the right, so mass is conserved.
// A broken cell always outputs true, which can change mass.
type shift struct {
	cells  int
	broken int // cell index, or -1
}

func (s shift) Cells() int         { return s.cells }
func (s shift) Bits() int          { return 1 }
func (s shift) Inputs(c int) []Bit { return []Bit{{Cell: (c + s.cells - 1) % s.cells}} }
func (s shift) Next(c int, in, out []bool) {
	out[0] = in[0] || c == s.broken
}

// satisfiable decides by exhaustive backtracking whether the variables missing
// from fixed can be assigned so that every clause holds. A branch is pruned as
// soon as one clause has all of its literals assigned false.
func satisfiable(cnf sat.CNF, fixed map[int]bool) bool {
	values := maps.Clone(fixed)
	var free []int
	for _, v := range cnf.Variables {
		if _, ok := values[v.ID]; !ok {
			free = append(free, v.ID)
		}
	}
	var search func(i int) bool
	search = func(i int) bool {
		if falsified(cnf, values) {
			return false
		}
		if i == len(free) {
			return true
		}
		for _, v := range []bool{false, true} {
			values[free[i]] = v
			if search(i + 1) {
				return true
			}
		}
		delete(values, free[i])
		return false
	}
	return search(0)
}

func falsified(cnf sat.CNF, values map[int]bool) bool {
	for _, clause := range cnf.Clauses {
		allFalse := true
		for _, lit := range clause {
			id := max(lit, -lit)
			if v, ok := values[id]; !ok || v == (lit > 0) {
				allFalse = false
				break
			}
		}
		if allFalse {
			return true
		}
	}
	return false
}

// TestTotalizerIsExact proves, for every input assignment, that the outputs are
// forced to the unary count: the correct outputs are satisfiable and flipping
// any single output is not.
func TestTotalizerIsExact(t *testing.T) {
	for m := 1; m <= 8; m++ {
		cnf := sat.CNF{}
		inputs := make([]int, m)
		for i := range inputs {
			inputs[i] = i + 1
			cnf.Variables = append(cnf.Variables, sat.Variable{ID: i + 1})
		}
		b := builder{cnf: &cnf, next: m + 1}
		outs := b.totalizer(inputs)
		if len(outs) != m {
			t.Fatalf("m=%d: %d outputs", m, len(outs))
		}
		if m == 1 {
			// A single input is its own count; there is no auxiliary output.
			if outs[0] != inputs[0] || len(cnf.Clauses) != 0 {
				t.Fatal("single-input totalizer is not the identity")
			}
			continue
		}
		for mask := range 1 << m {
			fixed := map[int]bool{}
			count := 0
			for i := range m {
				fixed[i+1] = mask>>i&1 == 1
				if fixed[i+1] {
					count++
				}
			}
			for j, out := range outs {
				fixed[out] = j < count
			}
			if !satisfiable(cnf, fixed) {
				t.Fatalf("m=%d mask=%b: correct count rejected", m, mask)
			}
			for j, out := range outs {
				wrong := maps.Clone(fixed)
				wrong[out] = j >= count
				if satisfiable(cnf, wrong) {
					t.Fatalf("m=%d mask=%b: output %d not forced", m, mask, j)
				}
			}
		}
	}
}

func quantityValue(q Quantity, s State, bits int) int {
	v := 0
	for _, t := range q {
		if s[t.Bit.Cell*bits+t.Bit.Index] {
			v += t.Weight
		}
	}
	return v
}

// oracleRun evaluates the rule directly from a start state and flattened
// per-step choices, independent of Simulate.
func oracleRun(sys System, start State, choices [][]bool) Trajectory {
	chooser, hasChoices := sys.(Chooser)
	bits := sys.Bits()
	run := Trajectory{States: []State{start}, Choices: choices}
	for step := range choices {
		cur := run.States[step]
		next := make(State, len(cur))
		offset := 0
		for c := range sys.Cells() {
			var in []bool
			for _, b := range sys.Inputs(c) {
				in = append(in, cur[b.Cell*bits+b.Index])
			}
			if hasChoices {
				n := chooser.Choices(c)
				in = append(in, choices[step][offset:offset+n]...)
				offset += n
			}
			sys.Next(c, in, next[c*bits:(c+1)*bits])
		}
		run.States = append(run.States, next)
	}
	return run
}

func mass(cells int) Quantity {
	q := make(Quantity, cells)
	for c := range q {
		q[c] = Term{Bit: Bit{Cell: c}, Weight: 1}
	}
	return q
}

// TestViolationOracle enumerates every start state and choice sequence, and
// requires the formula to admit a run exactly when the oracle sees the run
// meet the patterns and change the quantity at some step.
func TestViolationOracle(t *testing.T) {
	cases := []struct {
		name string
		sys  System
		q    Query
	}{
		{"shift-conserves", shift{cells: 4, broken: -1}, Query{Steps: 3, Violates: mass(4)}},
		{"shift-broken", shift{cells: 4, broken: 2}, Query{Steps: 2, Violates: mass(4)}},
		{"rule90-mass", elementary{cells: 4, rule: 90}, Query{Steps: 2, Violates: mass(4)}},
		{"rule90-zero-steps", elementary{cells: 4, rule: 90}, Query{Steps: 0, Violates: mass(4)}},
		{"rule30-partial", elementary{cells: 5, rule: 30}, Query{Steps: 1, Start: Pattern{{Cell: 2}: true}, Violates: Quantity{{Bit: Bit{Cell: 0}, Weight: 1}, {Bit: Bit{Cell: 4}, Weight: -1}}}},
		{"pair-mixed", pair{}, Query{Steps: 2, Violates: Quantity{{Bit: Bit{Cell: 0, Index: 0}, Weight: 1}, {Bit: Bit{Cell: 1, Index: 1}, Weight: -1}, {Bit: Bit{Cell: 0, Index: 1}, Weight: 1}}}},
		{"coin-mass", coin{cells: 3}, Query{Steps: 2, Goal: Pattern{{Cell: 0}: true}, Violates: mass(3)}},
		{"single-term", elementary{cells: 3, rule: 110}, Query{Steps: 1, Violates: Quantity{{Bit: Bit{Cell: 1}, Weight: -1}}}},
		// Rule 51 negates every cell, so the mass changes and changes back.
		{"rule51-returns", elementary{cells: 3, rule: 51}, Query{Steps: 2, Violates: mass(3)}},
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
			l := e.layout
			stateBits, choiceBits := l.cells*l.bits, l.steps*l.totalChoices
			admitted := 0
			for startMask := range 1 << stateBits {
				for choiceMask := range 1 << choiceBits {
					start := make(State, stateBits)
					for i := range start {
						start[i] = startMask>>i&1 == 1
					}
					choices := make([][]bool, l.steps)
					for step := range choices {
						choices[step] = make([]bool, l.totalChoices)
						for j := range choices[step] {
							choices[step][j] = choiceMask>>(step*l.totalChoices+j)&1 == 1
						}
					}
					run := oracleRun(tc.sys, start, choices)
					changed := false
					for step := 1; step <= l.steps; step++ {
						if quantityValue(tc.q.Violates, run.States[step-1], l.bits) != quantityValue(tc.q.Violates, run.States[step], l.bits) {
							changed = true
						}
					}
					want := oracleMeets(tc.q, run, l.bits) && changed
					fixed := map[int]bool{}
					for time, s := range run.States {
						for c := range l.cells {
							for i := range l.bits {
								fixed[l.stateVar(time, Bit{Cell: c, Index: i})] = s[c*l.bits+i]
							}
						}
					}
					for step := range choices {
						for c := range l.cells {
							for j := range l.choices[c] {
								fixed[l.choiceVar(step, c, j)] = choices[step][l.offsets[c]+j]
							}
						}
					}
					if got := satisfiable(cnf, fixed); got != want {
						t.Fatalf("start %b choices %b: formula %v, oracle %v", startMask, choiceMask, got, want)
					}
					if want {
						admitted++
					}
				}
			}
			t.Logf("%d variables, %d runs change the quantity", len(cnf.Variables), admitted)
		})
	}
}

// TestEncodingSizeMatchesFormula keeps the budget check exact.
func TestEncodingSizeMatchesFormula(t *testing.T) {
	cases := []struct {
		sys System
		q   Query
	}{
		{elementary{cells: 5, rule: 30}, Query{Steps: 3, Start: Pattern{{Cell: 0}: true}, Goal: Pattern{{Cell: 1}: false}, Forbidden: []Pattern{{{Cell: 2}: true, {Cell: 3}: true}}}},
		{coin{cells: 4}, Query{Steps: 2}},
		{pair{}, Query{Steps: 0, Violates: Quantity{{Bit: Bit{Cell: 1, Index: 1}, Weight: -1}}}},
	}
	for m := 1; m <= 9; m++ {
		cases = append(cases, struct {
			sys System
			q   Query
		}{shift{cells: 9, broken: -1}, Query{Steps: 2, Violates: mass(9)[:m]}})
	}
	for _, tc := range cases {
		e, err := Compile(context.Background(), tc.sys, tc.q)
		if err != nil {
			t.Fatal(err)
		}
		fixed, perStep := encodingSize(e.layout, tc.q)
		steps := int64(tc.q.Steps)
		variables, clauses, literals := fixed.variables+steps*perStep.variables, fixed.clauses+steps*perStep.clauses, fixed.literals+steps*perStep.literals
		gotLiterals := 0
		for _, clause := range e.formula.Clauses {
			gotLiterals += len(clause)
		}
		if cap(e.formula.Variables) != len(e.formula.Variables) || cap(e.formula.Clauses) != len(e.formula.Clauses) {
			t.Fatalf("%+v: formula grew past its preallocation: variables %d/%d, clauses %d/%d", tc.q,
				len(e.formula.Variables), cap(e.formula.Variables), len(e.formula.Clauses), cap(e.formula.Clauses))
		}
		if variables != int64(len(e.formula.Variables)) || clauses != int64(len(e.formula.Clauses)) || literals != int64(gotLiterals) {
			t.Fatalf("%+v: predicted %d/%d/%d, built %d/%d/%d", tc.q, variables, clauses, literals,
				len(e.formula.Variables), len(e.formula.Clauses), gotLiterals)
		}
	}
}

func TestQuantityValidation(t *testing.T) {
	sys := elementary{cells: 3, rule: 90}
	for name, q := range map[string]Quantity{
		"zero weight":  {{Bit: Bit{Cell: 0}, Weight: 0}},
		"weight two":   {{Bit: Bit{Cell: 0}, Weight: 2}},
		"out of range": {{Bit: Bit{Cell: 3}, Weight: 1}},
		"bit index":    {{Bit: Bit{Cell: 0, Index: 1}, Weight: 1}},
		"duplicate":    {{Bit: Bit{Cell: 1}, Weight: 1}, {Bit: Bit{Cell: 1}, Weight: -1}},
	} {
		if _, err := Compile(context.Background(), sys, Query{Steps: 1, Violates: q}); !errors.Is(err, ErrInput) {
			t.Errorf("%s: got %v, want ErrInput", name, err)
		}
	}
}

func TestDigestIdentifiesQuantity(t *testing.T) {
	digestOf := func(q Quantity) string {
		t.Helper()
		e, err := Compile(context.Background(), elementary{cells: 3, rule: 90}, Query{Steps: 1, Violates: q})
		if err != nil {
			t.Fatal(err)
		}
		d, err := e.Digest()
		if err != nil {
			t.Fatal(err)
		}
		return d.String()
	}
	a := Term{Bit: Bit{Cell: 0}, Weight: 1}
	b := Term{Bit: Bit{Cell: 2}, Weight: -1}
	if digestOf(Quantity{a, b}) != digestOf(Quantity{b, a}) {
		t.Fatal("digest depends on term order")
	}
	if digestOf(Quantity{a, b}) == digestOf(Quantity{a, {Bit: b.Bit, Weight: 1}}) {
		t.Fatal("digest ignores weights")
	}
	if digestOf(Quantity{a}) == digestOf(nil) {
		t.Fatal("digest ignores the violation query")
	}
}
