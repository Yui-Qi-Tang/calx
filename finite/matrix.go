package finite

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
)

// Coordinate identifies one categorical value in the state matrix.
type Coordinate struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

// Matrix uses stable sorted field, value and action axes. A guard or effect cell
// is one exactly when that coordinate is explicitly required or assigned. Frame
// rows identify unchanged fields. An absent guard field has all-zero cells.
type Matrix struct {
	Fields      []string     `json:"fields"`
	Coordinates []Coordinate `json:"coordinates"`
	ActionIDs   []string     `json:"action_ids"`
	Guard       [][]int      `json:"guard"`
	Effect      [][]int      `json:"effect"`
	Frame       [][]int      `json:"frame"`
}

// Project validates a supported model and produces independently owned matrices.
func Project(model Model) (Matrix, error) {
	if err := model.Validate(); err != nil {
		return Matrix{}, err
	}
	var m Matrix
	for field := range model.Fields {
		m.Fields = append(m.Fields, field)
	}
	slices.Sort(m.Fields)
	for _, field := range m.Fields {
		values := slices.Clone(model.Fields[field])
		slices.Sort(values)
		for _, value := range values {
			m.Coordinates = append(m.Coordinates, Coordinate{Field: field, Value: value})
		}
	}
	actions := slices.Clone(model.Actions)
	slices.SortFunc(actions, func(a, b Action) int { return strings.Compare(a.ID, b.ID) })
	for _, action := range actions {
		m.ActionIDs = append(m.ActionIDs, action.ID)
		guard, effect, frame := make([]int, len(m.Coordinates)), make([]int, len(m.Coordinates)), make([]int, len(m.Fields))
		for i, cell := range m.Coordinates {
			if action.Guard[cell.Field] == cell.Value {
				guard[i] = 1
			}
			if action.Effect[cell.Field] == cell.Value {
				effect[i] = 1
			}
		}
		for i, field := range m.Fields {
			if slices.Contains(action.Frame, field) {
				frame[i] = 1
			}
		}
		m.Guard = append(m.Guard, guard)
		m.Effect = append(m.Effect, effect)
		m.Frame = append(m.Frame, frame)
	}
	return m, nil
}

// actions reconstructs rules after validating axes and dimensions.
func (m Matrix) actions() ([]Action, error) {
	if len(m.Fields) == 0 || len(m.Fields) > maxFields || len(m.ActionIDs) == 0 || len(m.ActionIDs) > maxActions || len(m.Coordinates) > maxFields*maxValues {
		return nil, ErrInput
	}
	fields := map[string]bool{}
	for _, f := range m.Fields {
		if f == "" || fields[f] {
			return nil, ErrInput
		}
		fields[f] = true
	}
	coords := map[Coordinate]bool{}
	counts := map[string]int{}
	for _, c := range m.Coordinates {
		if !fields[c.Field] || c.Value == "" || coords[c] {
			return nil, ErrInput
		}
		coords[c] = true
		counts[c.Field]++
	}
	for f := range fields {
		if counts[f] == 0 {
			return nil, ErrInput
		}
	}
	if len(m.Guard) != len(m.ActionIDs) || len(m.Effect) != len(m.ActionIDs) || len(m.Frame) != len(m.ActionIDs) {
		return nil, errors.New("matrix row count mismatch")
	}
	out := make([]Action, 0, len(m.ActionIDs))
	for i, id := range m.ActionIDs {
		if len(m.Guard[i]) != len(m.Coordinates) || len(m.Effect[i]) != len(m.Coordinates) || len(m.Frame[i]) != len(m.Fields) {
			return nil, errors.New("matrix column count mismatch")
		}
		a := Action{ID: id, Guard: State{}, Effect: State{}, Frame: []string{}}
		for j, cell := range m.Coordinates {
			if m.Guard[i][j] < 0 || m.Guard[i][j] > 1 || m.Effect[i][j] < 0 || m.Effect[i][j] > 1 {
				return nil, errors.New("nonbinary matrix cell")
			}
			if m.Guard[i][j] == 1 {
				if _, ok := a.Guard[cell.Field]; ok {
					return nil, errors.New("multiple guard values")
				}
				a.Guard[cell.Field] = cell.Value
			}
			if m.Effect[i][j] == 1 {
				if _, ok := a.Effect[cell.Field]; ok {
					return nil, errors.New("multiple effect values")
				}
				a.Effect[cell.Field] = cell.Value
			}
		}
		for j, field := range m.Fields {
			if m.Frame[i][j] < 0 || m.Frame[i][j] > 1 {
				return nil, errors.New("nonbinary frame cell")
			}
			if m.Frame[i][j] == 1 {
				a.Frame = append(a.Frame, field)
			}
		}
		out = append(out, a)
	}
	return out, nil
}

// Model reconstructs a validated model. Forbidden patterns are explicit input;
// they cannot be recovered from Guard/Effect/Frame matrices alone.
func (m Matrix) Model(forbidden []State) (Model, error) {
	actions, err := m.actions()
	if err != nil {
		return Model{}, fmt.Errorf("%w: %v", ErrInput, err)
	}
	model := Model{Fields: map[string][]string{}, Actions: actions, Forbidden: make([]State, len(forbidden))}
	for _, c := range m.Coordinates {
		model.Fields[c.Field] = append(model.Fields[c.Field], c.Value)
	}
	for i, p := range forbidden {
		model.Forbidden[i] = maps.Clone(p)
	}
	if err := model.Validate(); err != nil {
		return Model{}, err
	}
	return model, nil
}
