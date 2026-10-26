package finite

import (
	"fmt"
	"slices"
)

// VariableKind identifies the semantic role of a finite-model SAT variable.
type VariableKind string

const (
	// StateVariable represents a field/category at one state time.
	StateVariable VariableKind = "state"
	// ActionVariable represents a declared action at one transition time.
	ActionVariable VariableKind = "action"
	// DifferenceVariable witnesses that one field differs between two states.
	DifferenceVariable VariableKind = "different"
)

// FieldName identifies a field declared in Model.Fields.
type FieldName string

// Category identifies a value in one field's declared categorical domain.
type Category string

// ActionID identifies an action declared in Model.Actions.
type ActionID string

// Variable maps a one-based SAT ID to one validated semantic role. Fields that
// do not apply to Kind must be absent; applicable fields must name model members.
// Validate checks this contract against a model and exact query horizon.
type Variable struct {
	ID        int          `json:"id"`
	Kind      VariableKind `json:"kind"`
	Time      int          `json:"time"`
	OtherTime *int         `json:"other_time,omitempty"`
	Field     FieldName    `json:"field,omitempty"`
	Value     Category     `json:"value,omitempty"`
	Action    ActionID     `json:"action,omitempty"`
}

// Validate checks role-specific presence, model membership and time bounds.
// It validates one variable, not the uniqueness of a whole variable mapping.
func (v Variable) Validate(model Model, horizon int) error {
	if err := model.Validate(); err != nil {
		return err
	}
	if horizon < 0 || horizon > maxHorizon {
		return fmt.Errorf("%w: variable horizon", ErrInput)
	}
	return newVariableScope(model, horizon).check(v)
}

type variableScope struct {
	fields  map[string][]string
	actions map[ActionID]struct{}
	horizon int
}

// newVariableScope is used only after the model and horizon have been validated.
func newVariableScope(model Model, horizon int) variableScope {
	actions := make(map[ActionID]struct{}, len(model.Actions))
	for _, action := range model.Actions {
		actions[ActionID(action.ID)] = struct{}{}
	}
	return variableScope{fields: model.Fields, actions: actions, horizon: horizon}
}

func (s variableScope) check(v Variable) error {
	if v.ID <= 0 || v.Time < 0 || v.Time > s.horizon {
		return fmt.Errorf("%w: variable ID or time", ErrInput)
	}
	switch v.Kind {
	case StateVariable:
		if v.OtherTime != nil || v.Action != "" || v.Field == "" || v.Value == "" {
			return fmt.Errorf("%w: state variable fields", ErrInput)
		}
		if !slices.Contains(s.fields[string(v.Field)], string(v.Value)) {
			return fmt.Errorf("%w: undeclared field/category", ErrInput)
		}
	case ActionVariable:
		if v.OtherTime != nil || v.Field != "" || v.Value != "" || v.Action == "" || v.Time >= s.horizon {
			return fmt.Errorf("%w: action variable fields or time", ErrInput)
		}
		if _, ok := s.actions[v.Action]; !ok {
			return fmt.Errorf("%w: undeclared action %q", ErrInput, v.Action)
		}
	case DifferenceVariable:
		if v.OtherTime == nil || *v.OtherTime <= v.Time || *v.OtherTime > s.horizon || v.Field == "" || v.Value != "" || v.Action != "" {
			return fmt.Errorf("%w: difference variable fields or times", ErrInput)
		}
		if _, ok := s.fields[string(v.Field)]; !ok {
			return fmt.Errorf("%w: undeclared difference field", ErrInput)
		}
	default:
		return fmt.Errorf("%w: unknown variable kind %q", ErrInput, v.Kind)
	}
	return nil
}

func cloneVariables(in []Variable) []Variable {
	out := slices.Clone(in)
	for i, v := range out {
		if v.OtherTime != nil {
			other := *v.OtherTime
			out[i].OtherTime = &other
		}
	}
	return out
}
