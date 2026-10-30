package automaton

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/internal/jsoncompat"
	"github.com/Yui-Qi-Tang/calx/sat"
)

// Query binds one exact-length run of Steps steps, at least 0; the encoding
// budget bounds how many fit. Start constrains the state at time zero, Goal
// the state at time Steps, and no state at any time may match a Forbidden
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

// Option changes the encoding budget of Compile and reports an invalid value.
// The budget is checked before any rule table is evaluated; exceeding it
// returns ErrEncodingLimit.
type Option func(*budget) error

// WithMaxVariables caps the number of CNF variables (default 100,000). n must
// be at least 1.
func WithMaxVariables(n int) Option {
	return func(b *budget) error {
		if n < 1 {
			return fmt.Errorf("%w: max variables %d, want at least 1", ErrInput, n)
		}
		b.variables = int64(n)
		return nil
	}
}

// WithMaxClauses caps the number of CNF clauses (default 1,000,000). n must be
// at least 1.
func WithMaxClauses(n int) Option {
	return func(b *budget) error {
		if n < 1 {
			return fmt.Errorf("%w: max clauses %d, want at least 1", ErrInput, n)
		}
		b.clauses = int64(n)
		return nil
	}
}

// WithMaxLiterals caps the total number of literals over all clauses
// (default 4,000,000). n must be at least 1.
func WithMaxLiterals(n int) Option {
	return func(b *budget) error {
		if n < 1 {
			return fmt.Errorf("%w: max literals %d, want at least 1", ErrInput, n)
		}
		b.literals = int64(n)
		return nil
	}
}

// budget bounds encoding work. Its defaults equal the sat.Limits defaults.
type budget struct {
	variables, clauses, literals int64
}

func newBudget(opts []Option) (budget, error) {
	b := budget{variables: 100_000, clauses: 1_000_000, literals: 4_000_000}
	for _, opt := range opts {
		if err := opt(&b); err != nil {
			return budget{}, err
		}
	}
	return b, nil
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
// Invalid systems, queries or options return ErrInput, an exceeded budget
// returns ErrEncodingLimit, and a canceled ctx returns its error.
func Compile(ctx context.Context, sys System, q Query, opts ...Option) (*Encoding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limits, err := newBudget(opts)
	if err != nil {
		return nil, err
	}
	if q.Steps < 0 {
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

// counts holds variable, clause and literal numbers of (part of) a formula.
type counts struct {
	variables, clauses, literals int64
}

// checkBudget compares the formula size with the budget. Steps has no upper
// bound, so it divides instead of multiplying to avoid overflow.
func checkBudget(l layout, q Query, limits budget) error {
	fixed, perStep := encodingSize(l, q)
	steps := int64(l.steps)
	if fixed.variables > limits.variables || steps > (limits.variables-fixed.variables)/perStep.variables ||
		fixed.clauses > limits.clauses || steps > (limits.clauses-fixed.clauses)/perStep.clauses ||
		fixed.literals > limits.literals || steps > (limits.literals-fixed.literals)/perStep.literals {
		return ErrEncodingLimit
	}
	return nil
}

// encodingSize counts the formula encode would build, without building it:
// the total is fixed plus Steps times perStep. Every perStep count is at
// least 1, because every cell has at least one bit.
func encodingSize(l layout, q Query) (fixed, perStep counts) {
	stateBits := int64(l.cells * l.bits)
	fixed.variables = stateBits
	perStep.variables = stateBits + int64(l.totalChoices)
	for c := range l.cells {
		fanIn := int64(len(l.inputs[c]) + l.choices[c])
		rows := int64(l.bits) << fanIn
		perStep.clauses += rows
		perStep.literals += rows * (fanIn + 1)
	}
	fixed.clauses += int64(len(q.Start) + len(q.Goal))
	fixed.literals += int64(len(q.Start) + len(q.Goal))
	for _, p := range q.Forbidden { // excluded at every time, including time zero
		fixed.clauses++
		perStep.clauses++
		fixed.literals += int64(len(p))
		perStep.literals += int64(len(p))
	}
	v, c, lits := violationSize(len(q.Violates))
	fixed.variables += v
	fixed.clauses += c
	fixed.literals += lits
	return fixed, perStep
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

// bitJSON, termJSON and patternBit fix the JSON form of the identity, so the
// digest does not depend on the public types.
type bitJSON struct {
	Cell  int `json:"cell"`
	Index int `json:"index"`
}

type termJSON struct {
	Bit    bitJSON `json:"bit"`
	Weight int     `json:"weight"`
}

type patternBit struct {
	Bit   bitJSON `json:"bit"`
	Value bool    `json:"value"`
}

func canonical(p Pattern) []patternBit {
	out := make([]patternBit, 0, len(p))
	for _, b := range sortedBits(p) {
		out = append(out, patternBit{Bit: bitJSON(b), Value: p[b]})
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
	// nil and empty slices stay distinct, as in earlier digests.
	inputs := make([][]bitJSON, len(l.inputs))
	for c, in := range l.inputs {
		if in != nil {
			inputs[c] = make([]bitJSON, len(in))
			for i, b := range in {
				inputs[c][i] = bitJSON(b)
			}
		}
	}
	var violates []termJSON
	if q.Violates != nil {
		sorted := slices.Clone(q.Violates)
		slices.SortFunc(sorted, func(a, b Term) int { return compareBits(a.Bit, b.Bit) })
		violates = make([]termJSON, len(sorted))
		for i, t := range sorted {
			violates[i] = termJSON{Bit: bitJSON(t.Bit), Weight: t.Weight}
		}
	}
	forbidden := make([][]patternBit, len(q.Forbidden))
	for i, p := range q.Forbidden {
		forbidden[i] = canonical(p)
	}
	raw, err := jsoncompat.Marshal(struct {
		Profile   string         `json:"profile"`
		Cells     int            `json:"cells"`
		Bits      int            `json:"bits"`
		Inputs    [][]bitJSON    `json:"inputs"`
		Choices   []int          `json:"choices"`
		Tables    []string       `json:"tables"`
		Steps     int            `json:"steps"`
		Start     []patternBit   `json:"start"`
		Goal      []patternBit   `json:"goal"`
		Forbidden [][]patternBit `json:"forbidden"`
		Violates  []termJSON     `json:"violates"`
	}{Profile, l.cells, l.bits, inputs, l.choices, rows, l.steps, canonical(q.Start), canonical(q.Goal), forbidden, violates})
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
