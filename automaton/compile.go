package automaton

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/sat"
)

// Query binds one exact-length run. Start constrains the state at time zero,
// Goal the state at time Steps, and no state at any time may match a Forbidden
// pattern. A nonempty Violates additionally requires that quantity to differ
// between times zero and Steps; UNSAT then is a conservation proof bounded to
// this system, length and set of patterns.
type Query struct {
	Steps     int
	Start     Pattern
	Goal      Pattern
	Forbidden []Pattern
	Violates  Quantity
}

// Options bounds encoding work. Zero fields use the sat.Limits defaults. The
// budget is checked before any rule table is evaluated.
type Options struct {
	MaxVariables int
	MaxClauses   int
	MaxLiterals  int
}

// Encoding owns a compiled formula and the layout needed to decode it.
// Accessors return independent copies; Decode checks against the private CNF.
type Encoding struct {
	formula sat.CNF
	digest  digest.Digest // Content digest of the rule tables and query.
	layout  layout
}

// layout assigns contiguous DIMACS IDs: all state bits by time, then all
// choice bits by step. Auxiliary counter variables follow.
type layout struct {
	shape
	steps int
}

func (l layout) stateVar(t int, b Bit) int { return 1 + t*l.cells*l.bits + l.index(b) }

func (l layout) choiceVar(t, cell, j int) int {
	return 1 + (l.steps+1)*l.cells*l.bits + t*l.totalChoices + l.offsets[cell] + j
}

func (l layout) variables() int { return (l.steps+1)*l.cells*l.bits + l.steps*l.totalChoices }

// Compile validates and encodes one query without invoking a solver. Each
// cell's rule is evaluated once per input row and cached for all steps.
func Compile(ctx context.Context, sys System, q Query, opts Options) (*Encoding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limits, err := (sat.Limits{MaxVariables: opts.MaxVariables, MaxClauses: opts.MaxClauses, MaxLiterals: opts.MaxLiterals}).WithDefaults()
	if err != nil {
		return nil, err
	}
	if q.Steps < 0 || q.Steps > maxSteps {
		return nil, fmt.Errorf("%w: steps", ErrInput)
	}
	s, err := readShape(sys)
	if err != nil {
		return nil, err
	}
	if err := q.validate(s); err != nil {
		return nil, err
	}
	l := layout{shape: s, steps: q.Steps}
	if err := checkBudget(l, q, limits); err != nil {
		return nil, err
	}
	tables := make([][]bool, s.cells)
	for c := range s.cells {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		tables[c] = ruleTable(sys, s, c)
	}
	formula, err := encode(ctx, l, tables, q)
	if err != nil {
		return nil, err
	}
	id, err := identity(l, tables, q)
	if err != nil {
		return nil, err
	}
	return &Encoding{formula: formula, digest: id, layout: l}, nil
}

func (q Query) validate(s shape) error {
	if err := s.checkPattern(q.Start); err != nil {
		return err
	}
	if err := s.checkPattern(q.Goal); err != nil {
		return err
	}
	for _, p := range q.Forbidden {
		if len(p) == 0 {
			return fmt.Errorf("%w: empty forbidden pattern", ErrInput)
		}
		if err := s.checkPattern(p); err != nil {
			return err
		}
	}
	return s.checkQuantity(q.Violates)
}

func checkBudget(l layout, q Query, limits sat.Limits) error {
	variables, clauses, literals := encodingSize(l, q)
	if variables > int64(limits.MaxVariables) || clauses > int64(limits.MaxClauses) || literals > int64(limits.MaxLiterals) {
		return ErrEncodingLimit
	}
	return nil
}

// encodingSize counts the formula encode would build, without building it.
func encodingSize(l layout, q Query) (variables, clauses, literals int64) {
	steps := int64(l.steps)
	variables = int64(l.variables())
	for c := range l.cells {
		fanIn := int64(len(l.inputs[c]) + l.choices[c])
		rows := steps * int64(l.bits) << fanIn
		clauses += rows
		literals += rows * (fanIn + 1)
	}
	clauses += int64(len(q.Start) + len(q.Goal))
	literals += int64(len(q.Start) + len(q.Goal))
	for _, p := range q.Forbidden {
		clauses += steps + 1
		literals += (steps + 1) * int64(len(p))
	}
	v, c, lits := violationSize(len(q.Violates))
	return variables + v, clauses + c, literals + lits
}

// ruleTable evaluates cell c over every input row. Bit k of row r is input k
// (inputs first, then choices); entry r*bits+o holds output bit o.
func ruleTable(sys System, s shape, c int) []bool {
	fanIn := len(s.inputs[c]) + s.choices[c]
	in := make([]bool, fanIn)
	out := make([]bool, s.bits)
	table := make([]bool, s.bits<<fanIn)
	for r := range 1 << fanIn {
		for k := range fanIn {
			in[k] = r>>k&1 == 1
		}
		clear(out)
		sys.Next(c, in, out)
		copy(table[r*s.bits:], out)
	}
	return table
}

func encode(ctx context.Context, l layout, tables [][]bool, q Query) (sat.CNF, error) {
	cnf := sat.CNF{Variables: make([]sat.Variable, l.variables())}
	for i := range cnf.Variables {
		cnf.Variables[i].ID = i + 1
	}
	for t := range l.steps {
		for c := range l.cells {
			if err := ctx.Err(); err != nil {
				return sat.CNF{}, err
			}
			vars := make([]int, 0, len(l.inputs[c])+l.choices[c])
			for _, b := range l.inputs[c] {
				vars = append(vars, l.stateVar(t, b))
			}
			for j := range l.choices[c] {
				vars = append(vars, l.choiceVar(t, c, j))
			}
			// One clause per row and output bit: these input values force
			// this output value at the next time.
			for r := range 1 << len(vars) {
				for o := range l.bits {
					clause := make([]int, 0, len(vars)+1)
					for k, v := range vars {
						if r>>k&1 == 1 {
							v = -v
						}
						clause = append(clause, v)
					}
					out := l.stateVar(t+1, Bit{Cell: c, Index: o})
					if !tables[c][r*l.bits+o] {
						out = -out
					}
					cnf.Clauses = append(cnf.Clauses, append(clause, out))
				}
			}
		}
	}
	for _, unit := range []struct {
		t int
		p Pattern
	}{{0, q.Start}, {l.steps, q.Goal}} {
		for _, b := range sortedBits(unit.p) {
			lit := l.stateVar(unit.t, b)
			if !unit.p[b] {
				lit = -lit
			}
			cnf.Clauses = append(cnf.Clauses, []int{lit})
		}
	}
	for _, p := range q.Forbidden {
		bits := sortedBits(p)
		for t := range l.steps + 1 {
			clause := make([]int, 0, len(bits))
			for _, b := range bits {
				lit := l.stateVar(t, b)
				if p[b] {
					lit = -lit
				}
				clause = append(clause, lit)
			}
			cnf.Clauses = append(cnf.Clauses, clause)
		}
	}
	if len(q.Violates) > 0 {
		b := builder{cnf: &cnf, next: len(cnf.Variables) + 1}
		b.differ(l.violationSides(q.Violates))
	}
	return cnf, cnf.CheckShape()
}

func sortedBits(p Pattern) []Bit {
	return slices.SortedFunc(maps.Keys(p), compareBits)
}

func compareBits(a, b Bit) int {
	if a.Cell != b.Cell {
		return a.Cell - b.Cell
	}
	return a.Index - b.Index
}

type patternBit struct {
	Bit   Bit  `json:"bit"`
	Value bool `json:"value"`
}

func canonical(p Pattern) []patternBit {
	out := make([]patternBit, 0, len(p))
	for _, b := range sortedBits(p) {
		out = append(out, patternBit{Bit: b, Value: p[b]})
	}
	return out
}

// identity digests what the formula means: structure, evaluated rule tables
// and query. It does not identify the caller's System implementation.
func identity(l layout, tables [][]bool, q Query) (digest.Digest, error) {
	rows := make([]string, len(tables))
	for c, table := range tables {
		row := make([]byte, len(table))
		for i, v := range table {
			row[i] = '0'
			if v {
				row[i] = '1'
			}
		}
		rows[c] = string(row)
	}
	violates := slices.Clone(q.Violates)
	slices.SortFunc(violates, func(a, b Term) int { return compareBits(a.Bit, b.Bit) })
	forbidden := make([][]patternBit, len(q.Forbidden))
	for i, p := range q.Forbidden {
		forbidden[i] = canonical(p)
	}
	raw, err := json.Marshal(struct {
		Profile   string         `json:"profile"`
		Cells     int            `json:"cells"`
		Bits      int            `json:"bits"`
		Inputs    [][]Bit        `json:"inputs"`
		Choices   []int          `json:"choices"`
		Tables    []string       `json:"tables"`
		Steps     int            `json:"steps"`
		Start     []patternBit   `json:"start"`
		Goal      []patternBit   `json:"goal"`
		Forbidden [][]patternBit `json:"forbidden"`
		Violates  []Term         `json:"violates"`
	}{Profile, l.cells, l.bits, l.inputs, l.choices, rows, l.steps, canonical(q.Start), canonical(q.Goal), forbidden, violates})
	if err != nil {
		return "", fmt.Errorf("encoding automaton identity: %w", err)
	}
	return digest.Sum(raw), nil
}

// CNF returns an independently owned formula for solver use. Uninitialized
// encodings return an error, not a logically true empty formula.
func (e *Encoding) CNF() (sat.CNF, error) {
	if e == nil || e.digest == "" {
		return sat.CNF{}, fmt.Errorf("%w: uninitialized encoding", ErrInput)
	}
	c := sat.CNF{Variables: slices.Clone(e.formula.Variables), Clauses: make([][]int, len(e.formula.Clauses))}
	for i, clause := range e.formula.Clauses {
		c.Clauses[i] = slices.Clone(clause)
	}
	return c, nil
}

// Digest identifies the structure, rule tables and query of the encoding.
// Uninitialized encodings return an error.
func (e *Encoding) Digest() (digest.Digest, error) {
	if e == nil || e.digest == "" {
		return "", fmt.Errorf("%w: uninitialized encoding", ErrInput)
	}
	return e.digest, nil
}

// Decode checks a total assignment against the owned original CNF and returns
// its trajectory. Use Replay for an independent step-by-step check.
func (e *Encoding) Decode(values map[int]bool) (Trajectory, error) {
	if e == nil || e.digest == "" {
		return Trajectory{}, fmt.Errorf("%w: uninitialized encoding", ErrInput)
	}
	if err := e.formula.CheckAssignment(values); err != nil {
		return Trajectory{}, err
	}
	return e.layout.trajectory(values), nil
}

func (l layout) trajectory(values map[int]bool) Trajectory {
	t := Trajectory{States: make([]State, l.steps+1), Choices: make([][]bool, l.steps)}
	for time := range t.States {
		t.States[time] = make(State, l.cells*l.bits)
		for c := range l.cells {
			for i := range l.bits {
				b := Bit{Cell: c, Index: i}
				t.States[time][l.index(b)] = values[l.stateVar(time, b)]
			}
		}
	}
	for step := range t.Choices {
		t.Choices[step] = make([]bool, l.totalChoices)
		for c := range l.cells {
			for j := range l.choices[c] {
				t.Choices[step][l.offsets[c]+j] = values[l.choiceVar(step, c, j)]
			}
		}
	}
	return t
}
