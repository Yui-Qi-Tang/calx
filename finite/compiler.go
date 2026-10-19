package finite

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/Yui-Qi-Tang/calx/sat"
)

type compiler struct {
	ctx         context.Context
	limit       Options
	scope       variableScope
	cnf         sat.CNF
	mapping     []Variable
	matrix      Matrix
	states      [][]int
	actions     [][]int
	fieldCells  map[string][]int
	cells       map[Coordinate]int
	actionIndex map[string]int
}

func (b *compiler) variable(v Variable) (int, error) {
	if err := b.ctx.Err(); err != nil {
		return 0, err
	}
	if len(b.cnf.Variables) >= b.limit.MaxVariables {
		return 0, ErrEncodingLimit
	}
	v.ID = len(b.cnf.Variables) + 1
	if err := b.scope.check(v); err != nil {
		return 0, err
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return 0, fmt.Errorf("encoding variable metadata: %w", err)
	}
	b.mapping = append(b.mapping, v)
	b.cnf.Variables = append(b.cnf.Variables, sat.Variable{
		ID: v.ID, Metadata: map[string]string{"finite_variable": string(raw)},
	})
	return v.ID, nil
}

func (b *compiler) add(literals ...int) error {
	if err := b.ctx.Err(); err != nil {
		return err
	}
	if len(b.cnf.Clauses) >= b.limit.MaxClauses {
		return ErrEncodingLimit
	}
	for _, literal := range literals {
		if literal == 0 || literal > len(b.cnf.Variables) || literal < -len(b.cnf.Variables) {
			return fmt.Errorf("%w: undeclared clause literal", ErrInput)
		}
	}
	b.cnf.Clauses = append(b.cnf.Clauses, slices.Clone(literals))
	return nil
}

func (b *compiler) one(variables []int) error {
	if err := b.add(variables...); err != nil {
		return err
	}
	for i, left := range variables {
		for _, right := range variables[i+1:] {
			if err := b.add(-left, -right); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *compiler) pattern(time int, pattern State) ([]int, error) {
	if time < 0 || time >= len(b.states) {
		return nil, fmt.Errorf("%w: pattern time", ErrInput)
	}
	variables := make([]int, 0, len(pattern))
	for _, field := range b.matrix.Fields {
		value, present := pattern[field]
		if !present {
			continue
		}
		cell, ok := b.cells[Coordinate{Field: field, Value: value}]
		if !ok {
			return nil, fmt.Errorf("%w: unknown pattern category", ErrInput)
		}
		variables = append(variables, b.states[time][cell])
	}
	if len(variables) != len(pattern) {
		return nil, fmt.Errorf("%w: unknown pattern field", ErrInput)
	}
	return variables, nil
}

func (b *compiler) requirePattern(time int, pattern State) error {
	variables, err := b.pattern(time, pattern)
	if err != nil {
		return err
	}
	for _, variable := range variables {
		if err := b.add(variable); err != nil {
			return err
		}
	}
	return nil
}

func (b *compiler) excludePattern(time int, pattern State) error {
	variables, err := b.pattern(time, pattern)
	if err != nil {
		return err
	}
	for i := range variables {
		variables[i] = -variables[i]
	}
	return b.add(variables...)
}

func (b *compiler) encodeStates(forbidden []State) error {
	for time := range b.states {
		b.states[time] = make([]int, len(b.matrix.Coordinates))
		for i, cell := range b.matrix.Coordinates {
			id, err := b.variable(Variable{Kind: StateVariable, Time: time, Field: FieldName(cell.Field), Value: Category(cell.Value)})
			if err != nil {
				return err
			}
			b.states[time][i] = id
		}
		for _, field := range b.matrix.Fields {
			variables := make([]int, 0, len(b.fieldCells[field]))
			for _, cell := range b.fieldCells[field] {
				variables = append(variables, b.states[time][cell])
			}
			if err := b.one(variables); err != nil {
				return err
			}
		}
		for _, pattern := range forbidden {
			if err := b.excludePattern(time, pattern); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *compiler) encodeTransitions(goal State) error {
	for time := range b.actions {
		for _, action := range b.matrix.ActionIDs {
			id, err := b.variable(Variable{Kind: ActionVariable, Time: time, Action: ActionID(action)})
			if err != nil {
				return err
			}
			b.actions[time] = append(b.actions[time], id)
		}
		if err := b.one(b.actions[time]); err != nil {
			return err
		}
		for action, selected := range b.actions[time] {
			if err := b.encodeAction(time, action, selected); err != nil {
				return err
			}
		}
		if err := b.excludePattern(time, goal); err != nil {
			return err
		}
	}
	return nil
}

func (b *compiler) encodeAction(time, action, selected int) error {
	for cell := range b.matrix.Coordinates {
		if b.matrix.Guard[action][cell] == 1 {
			if err := b.add(-selected, b.states[time][cell]); err != nil {
				return err
			}
		}
		if b.matrix.Effect[action][cell] == 1 {
			if err := b.add(-selected, b.states[time+1][cell]); err != nil {
				return err
			}
		}
	}
	for fieldIndex, field := range b.matrix.Fields {
		if b.matrix.Frame[action][fieldIndex] != 1 {
			continue
		}
		for _, cell := range b.fieldCells[field] {
			before, after := b.states[time][cell], b.states[time+1][cell]
			if err := b.add(-selected, -before, after); err != nil {
				return err
			}
			if err := b.add(-selected, before, -after); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *compiler) encodeDistinctStates() error {
	for left := range b.states {
		for right := left + 1; right < len(b.states); right++ {
			var different []int
			for _, field := range b.matrix.Fields {
				other := right
				id, err := b.variable(Variable{Kind: DifferenceVariable, Time: left, OtherTime: &other, Field: FieldName(field)})
				if err != nil {
					return err
				}
				different = append(different, id)
				for _, cell := range b.fieldCells[field] {
					if err := b.add(-b.states[left][cell], -b.states[right][cell], -id); err != nil {
						return err
					}
				}
			}
			if err := b.add(different...); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *compiler) encodeBlocked(blocked [][]string) error {
	for _, sequence := range blocked {
		if len(sequence) != len(b.actions) {
			return fmt.Errorf("%w: blocked sequence length", ErrInput)
		}
		clause := make([]int, len(sequence))
		for time, action := range sequence {
			index, ok := b.actionIndex[action]
			if !ok {
				return fmt.Errorf("%w: blocked action", ErrInput)
			}
			clause[time] = -b.actions[time][index]
		}
		if err := b.add(clause...); err != nil {
			return err
		}
	}
	return nil
}
