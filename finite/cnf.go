package finite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"

	"github.com/Yui-Qi-Tang/calx/sat"
)

// Variable is the semantic identity of a one-based DIMACS variable.
type Variable struct {
	ID int `json:"id"`
	// state, action, or different; see docs/contracts.md.
	Kind string `json:"kind"`
	// Zero-based state time or action transition start.
	Time int `json:"time"`
	// Later state time for different; otherwise unused.
	OtherTime int    `json:"other_time,omitempty"`
	Field     string `json:"field,omitempty"`
	Value     string `json:"value,omitempty"`
	Action    string `json:"action,omitempty"`
}

type compiler struct {
	mapping []Variable
	ctx     context.Context
	cnf     sat.CNF
	limit   Options
	err     error
}

func (b *compiler) variable(v Variable) int {
	if b.err != nil {
		return 0
	}
	if len(b.cnf.Variables) >= b.limit.MaxVariables {
		b.err = ErrEncodingLimit
		return 0
	}
	v.ID = len(b.cnf.Variables) + 1
	raw, _ := json.Marshal(v) // Variable has only strings and integers.
	b.mapping = append(b.mapping, v)
	b.cnf.Variables = append(b.cnf.Variables, sat.Variable{ID: v.ID, Metadata: map[string]string{"finite_variable": string(raw)}})
	return v.ID
}
func (b *compiler) add(lits ...int) {
	if b.err != nil {
		return
	}
	if err := b.ctx.Err(); err != nil {
		b.err = err
		return
	}
	if len(b.cnf.Clauses) >= b.limit.MaxClauses {
		b.err = ErrEncodingLimit
		return
	}
	b.cnf.Clauses = append(b.cnf.Clauses, slices.Clone(lits))
}
func (b *compiler) one(vars []int) {
	b.add(vars...)
	for i, x := range vars {
		for _, y := range vars[i+1:] {
			b.add(-x, -y)
		}
	}
}

// Encoding owns a compiled formula and its decoding map. Obtain a copy with
// CNF; changes to that copy cannot alter Decode's validation target.
type Encoding struct {
	formula     sat.CNF
	mapping     []Variable
	querySHA256 string
	states      [][]int
	actions     [][]int
	matrix      Matrix
}

func encode(ctx context.Context, m Matrix, forbidden []State, start, goal State, horizon int, blocked [][]string, opts Options) (Encoding, error) {
	// Bound the quadratic simple-path encoding before allocating time-indexed rows.
	pairs := int64(horizon) * int64(horizon+1) / 2
	variables := int64(horizon+1)*int64(len(m.Coordinates)) + int64(horizon)*int64(len(m.ActionIDs)) + pairs*int64(len(m.Fields))
	domainClauses := int64(len(forbidden))
	for _, field := range m.Fields {
		n := int64(0)
		for _, cell := range m.Coordinates {
			if cell.Field == field {
				n++
			}
		}
		domainClauses += 1 + n*(n-1)/2
	}
	actionCount := int64(len(m.ActionIDs))
	transitionClauses := int64(2) + actionCount*(actionCount-1)/2
	for i := range m.ActionIDs {
		for j := range m.Coordinates {
			transitionClauses += int64(m.Guard[i][j] + m.Effect[i][j])
		}
		for j, field := range m.Fields {
			if m.Frame[i][j] == 1 {
				for _, cell := range m.Coordinates {
					if cell.Field == field {
						transitionClauses += 2
					}
				}
			}
		}
	}
	clauses := int64(horizon+1)*domainClauses + int64(horizon)*transitionClauses + int64(len(start)+len(goal)) + pairs*int64(len(m.Coordinates)+1) + int64(len(blocked))
	if variables > int64(opts.MaxVariables) || clauses > int64(opts.MaxClauses) {
		return Encoding{}, ErrEncodingLimit
	}
	b := compiler{ctx: ctx, limit: opts}
	out := Encoding{states: make([][]int, horizon+1), actions: make([][]int, horizon), matrix: m}
	fieldCells := map[string][]int{}
	cells := map[Coordinate]int{}
	actionIndex := map[string]int{}
	for i, c := range m.Coordinates {
		fieldCells[c.Field] = append(fieldCells[c.Field], i)
		cells[c] = i
	}
	for i, a := range m.ActionIDs {
		actionIndex[a] = i
	}
	pattern := func(t int, p State) []int {
		var vars []int
		for _, field := range m.Fields {
			if value, ok := p[field]; ok {
				vars = append(vars, out.states[t][cells[Coordinate{Field: field, Value: value}]])
			}
		}
		return vars
	}
	for t := 0; t <= horizon; t++ {
		out.states[t] = make([]int, len(m.Coordinates))
		for i, cell := range m.Coordinates {
			out.states[t][i] = b.variable(Variable{Kind: "state", Time: t, Field: cell.Field, Value: cell.Value})
		}
		for _, field := range m.Fields {
			var vars []int
			for _, i := range fieldCells[field] {
				vars = append(vars, out.states[t][i])
			}
			b.one(vars)
		}
		for _, p := range forbidden {
			vars := pattern(t, p)
			for i := range vars {
				vars[i] = -vars[i]
			}
			b.add(vars...)
		}
	}
	for _, lit := range pattern(0, start) {
		b.add(lit)
	}
	for t := 0; t < horizon; t++ {
		for _, a := range m.ActionIDs {
			out.actions[t] = append(out.actions[t], b.variable(Variable{Kind: "action", Time: t, Action: a}))
		}
		b.one(out.actions[t])
		for i, selected := range out.actions[t] {
			for j := range m.Coordinates {
				if m.Guard[i][j] == 1 {
					b.add(-selected, out.states[t][j])
				}
				if m.Effect[i][j] == 1 {
					b.add(-selected, out.states[t+1][j])
				}
			}
			for j, field := range m.Fields {
				if m.Frame[i][j] == 1 {
					for _, cell := range fieldCells[field] {
						before, after := out.states[t][cell], out.states[t+1][cell]
						b.add(-selected, -before, after)
						b.add(-selected, before, -after)
					}
				}
			}
		}
		vars := pattern(t, goal)
		for i := range vars {
			vars[i] = -vars[i]
		}
		b.add(vars...)
	}
	for _, lit := range pattern(horizon, goal) {
		b.add(lit)
	}
	// A pair must differ in at least one field. Equal one-hot values force that
	// field's difference witness false; no action/auxiliary assignments define identity.
	for left := 0; left <= horizon; left++ {
		for right := left + 1; right <= horizon; right++ {
			var different []int
			for _, field := range m.Fields {
				d := b.variable(Variable{Kind: "different", Time: left, OtherTime: right, Field: field})
				different = append(different, d)
				for _, cell := range fieldCells[field] {
					b.add(-out.states[left][cell], -out.states[right][cell], -d)
				}
			}
			b.add(different...)
		}
	}
	for _, sequence := range blocked {
		if len(sequence) != horizon {
			return Encoding{}, fmt.Errorf("blocked path length mismatch")
		}
		clause := make([]int, horizon)
		for t, action := range sequence {
			i, ok := actionIndex[action]
			if !ok {
				return Encoding{}, fmt.Errorf("blocked path action unknown")
			}
			clause[t] = -out.actions[t][i]
		}
		b.add(clause...)
	}
	if b.err != nil {
		return Encoding{}, b.err
	}
	out.formula = b.cnf
	out.mapping = b.mapping
	return out, nil
}

// Decode checks the complete assignment against the owned original CNF and
// returns its state/action witness. Independent original-rule replay is separate.
func (e *Encoding) Decode(values map[int]bool) (Trace, error) {
	if e == nil || e.querySHA256 == "" {
		return Trace{}, ErrInput
	}
	if err := e.formula.CheckAssignment(values); err != nil {
		return Trace{}, err
	}
	trace := Trace{States: make([]State, len(e.states)), Actions: make([]string, len(e.actions))}
	for t, vars := range e.states {
		trace.States[t] = State{}
		for i, id := range vars {
			if values[id] {
				cell := e.matrix.Coordinates[i]
				trace.States[t][cell.Field] = cell.Value
			}
		}
	}
	for t, vars := range e.actions {
		for i, id := range vars {
			if values[id] {
				trace.Actions[t] = e.matrix.ActionIDs[i]
			}
		}
	}
	return trace, nil
}

// Compile validates the model/query and encodes one exact-length query. The
// supported simple/first-hit profile is mandatory, never an implicit default.
// Callers must not mutate inputs concurrently. No solver is invoked.
func Compile(ctx context.Context, model Model, q Query, options Options) (*Encoding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opts, err := options.defaults()
	if err != nil {
		return nil, err
	}
	if err := model.Validate(); err != nil {
		return nil, err
	}
	if err := q.validate(model, opts); err != nil {
		return nil, err
	}
	matrix, err := Project(model)
	if err != nil {
		return nil, err
	}
	e, err := encode(ctx, matrix, model.Forbidden, q.Start, q.Goal, q.Horizon, q.Blocked, opts)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(struct {
		Model Model `json:"model"`
		Query Query `json:"query"`
	}{model, q})
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	e.querySHA256 = hex.EncodeToString(sum[:])
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &e, nil
}

// QuerySHA256 identifies compact Go JSON of the typed model/query, including
// profile, horizon and all blocked sequences. It is not semantic equivalence.
func (e *Encoding) QuerySHA256() string {
	if e == nil {
		return ""
	}
	return e.querySHA256
}

// Variables returns a copy of the DIMACS-to-finite-model decoding map.
func (e *Encoding) Variables() []Variable {
	if e == nil {
		return nil
	}
	return slices.Clone(e.mapping)
}

// CNF returns an independently owned copy for solver use.
func (e *Encoding) CNF() sat.CNF {
	if e == nil {
		return sat.CNF{}
	}
	c := sat.CNF{Variables: make([]sat.Variable, len(e.formula.Variables)), Clauses: make([][]int, len(e.formula.Clauses))}
	for i, v := range e.formula.Variables {
		c.Variables[i] = sat.Variable{ID: v.ID, Metadata: maps.Clone(v.Metadata)}
	}
	for i, row := range e.formula.Clauses {
		c.Clauses[i] = append([]int{}, row...)
	}
	return c
}
