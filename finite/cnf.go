package finite

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/sat"
)

// Encoding owns the compiled formula and its decoding data. Accessors return
// independent copies; Decode always checks against the private original CNF.
type Encoding struct {
	formula sat.CNF
	mapping []Variable
	digest  digest.Digest // Content digest of the serialized model and query.
	states  [][]int
	actions [][]int
	matrix  Matrix
}

// Compile validates and encodes one exact-length query without invoking a solver.
// The profile is explicit. Inputs must not be mutated concurrently.
func Compile(ctx context.Context, model Model, query Query, options Options) (*Encoding, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	limits, err := options.defaults()
	if err != nil {
		return nil, err
	}
	// Project validates the model, which query.validate requires.
	matrix, err := Project(model)
	if err != nil {
		return nil, err
	}
	if err := query.validate(model, limits); err != nil {
		return nil, err
	}
	if err := checkEncodingBudget(matrix, model.Forbidden, query, limits); err != nil {
		return nil, err
	}
	b := compiler{
		ctx: ctx, limit: limits, scope: newVariableScope(model, query.Horizon), matrix: matrix,
		states: make([][]int, query.Horizon+1), actions: make([][]int, query.Horizon),
		fieldCells: map[string][]int{}, cells: map[Coordinate]int{}, actionIndex: map[string]int{},
	}
	for i, cell := range matrix.Coordinates {
		b.fieldCells[cell.Field] = append(b.fieldCells[cell.Field], i)
		b.cells[cell] = i
	}
	for i, action := range matrix.ActionIDs {
		b.actionIndex[action] = i
	}
	if err := b.encodeStates(model.Forbidden); err != nil {
		return nil, err
	}
	if err := b.requirePattern(0, query.Start); err != nil {
		return nil, err
	}
	if err := b.encodeTransitions(query.Goal); err != nil {
		return nil, err
	}
	if err := b.requirePattern(query.Horizon, query.Goal); err != nil {
		return nil, err
	}
	if err := b.encodeDistinctStates(); err != nil {
		return nil, err
	}
	if err := b.encodeBlocked(query.Blocked); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(struct {
		Model Model `json:"model"`
		Query Query `json:"query"`
	}{Model: model, Query: query})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &Encoding{
		formula: b.cnf, mapping: b.mapping, digest: digest.Sum(raw),
		states: b.states, actions: b.actions, matrix: matrix,
	}, nil
}

func checkEncodingBudget(matrix Matrix, forbidden []State, query Query, limits Options) error {
	horizon := int64(query.Horizon)
	pairs := horizon * (horizon + 1) / 2
	variables := (horizon+1)*int64(len(matrix.Coordinates)) + horizon*int64(len(matrix.ActionIDs)) + pairs*int64(len(matrix.Fields))
	domainClauses := int64(len(forbidden))
	for _, field := range matrix.Fields {
		var count int64
		for _, cell := range matrix.Coordinates {
			if cell.Field == field {
				count++
			}
		}
		domainClauses += 1 + count*(count-1)/2
	}
	actions := int64(len(matrix.ActionIDs))
	transitionClauses := 2 + actions*(actions-1)/2
	for i := range matrix.ActionIDs {
		for j := range matrix.Coordinates {
			transitionClauses += int64(matrix.Guard[i][j] + matrix.Effect[i][j])
		}
		for j, field := range matrix.Fields {
			if matrix.Frame[i][j] != 1 {
				continue
			}
			for _, cell := range matrix.Coordinates {
				if cell.Field == field {
					transitionClauses += 2
				}
			}
		}
	}
	clauses := (horizon+1)*domainClauses + horizon*transitionClauses + int64(len(query.Start)+len(query.Goal)) + pairs*int64(len(matrix.Coordinates)+1) + int64(len(query.Blocked))
	if variables > int64(limits.MaxVariables) || clauses > int64(limits.MaxClauses) {
		return ErrEncodingLimit
	}
	return nil
}

// Decode checks a total assignment against the owned original CNF and returns
// its state/action witness. Independent original-rule replay remains separate.
func (e *Encoding) Decode(values map[int]bool) (Trace, error) {
	if e == nil || e.digest == "" {
		return Trace{}, ErrInput
	}
	if err := e.formula.CheckAssignment(values); err != nil {
		return Trace{}, err
	}
	trace := Trace{States: make([]State, len(e.states)), Actions: make([]string, len(e.actions))}
	for time, variables := range e.states {
		trace.States[time] = State{}
		for i, id := range variables {
			if values[id] {
				cell := e.matrix.Coordinates[i]
				trace.States[time][cell.Field] = cell.Value
			}
		}
	}
	for time, variables := range e.actions {
		for i, id := range variables {
			if values[id] {
				trace.Actions[time] = e.matrix.ActionIDs[i]
			}
		}
	}
	return trace, nil
}

// Digest identifies the serialized model and query.
// Uninitialized encodings return an error.
func (e *Encoding) Digest() (digest.Digest, error) {
	if e == nil || e.digest == "" {
		return "", fmt.Errorf("%w: uninitialized encoding", ErrInput)
	}
	return e.digest, nil
}

// Variables returns a deep copy of the validated DIMACS-to-model mapping.
// Uninitialized encodings return an error.
func (e *Encoding) Variables() ([]Variable, error) {
	if e == nil || e.digest == "" {
		return nil, fmt.Errorf("%w: uninitialized encoding", ErrInput)
	}
	return cloneVariables(e.mapping), nil
}

// CNF returns an independently owned formula for solver use. Uninitialized
// encodings return an error, not a logically true empty formula.
func (e *Encoding) CNF() (sat.CNF, error) {
	if e == nil || e.digest == "" {
		return sat.CNF{}, fmt.Errorf("%w: uninitialized encoding", ErrInput)
	}
	c := sat.CNF{Variables: make([]sat.Variable, len(e.formula.Variables)), Clauses: make([][]int, len(e.formula.Clauses))}
	for i, v := range e.formula.Variables {
		c.Variables[i] = sat.Variable{ID: v.ID, Metadata: maps.Clone(v.Metadata)}
	}
	for i, row := range e.formula.Clauses {
		c.Clauses[i] = append([]int{}, row...)
	}
	return c, nil
}
