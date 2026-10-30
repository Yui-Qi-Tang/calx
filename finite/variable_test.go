package finite_test

import (
	"context"
	"errors"
	"testing"

	"github.com/Yui-Qi-Tang/calx/finite"
)

func TestVariableRoleContract(t *testing.T) {
	m := model(4)
	zero, one, two := 0, 1, 2
	valid := []finite.Variable{
		{ID: 1, Kind: finite.StateVariable, Time: 0, Field: "x", Value: "a"},
		{ID: 2, Kind: finite.ActionVariable, Time: 0, Action: "ac"},
		{ID: 3, Kind: finite.DifferenceVariable, Time: 0, OtherTime: &one, Field: "x"},
	}
	for _, variable := range valid {
		if err := variable.Validate(m, 1); err != nil {
			t.Fatal(variable, err)
		}
	}
	invalid := map[string]finite.Variable{
		"empty kind":         {ID: 1},
		"unknown kind":       {ID: 1, Kind: "unsupported"},
		"zero ID":            {Kind: finite.StateVariable, Field: "x", Value: "a"},
		"negative time":      {ID: 1, Kind: finite.StateVariable, Time: -1, Field: "x", Value: "a"},
		"missing field":      {ID: 1, Kind: finite.StateVariable, Value: "a"},
		"missing value":      {ID: 1, Kind: finite.StateVariable, Field: "x"},
		"undeclared value":   {ID: 1, Kind: finite.StateVariable, Field: "x", Value: "missing"},
		"state has action":   {ID: 1, Kind: finite.StateVariable, Field: "x", Value: "a", Action: "ac"},
		"state has other":    {ID: 1, Kind: finite.StateVariable, Field: "x", Value: "a", OtherTime: &zero},
		"missing action":     {ID: 1, Kind: finite.ActionVariable},
		"undeclared action":  {ID: 1, Kind: finite.ActionVariable, Action: "invented"},
		"action at endpoint": {ID: 1, Kind: finite.ActionVariable, Time: 1, Action: "ac"},
		"action has field":   {ID: 1, Kind: finite.ActionVariable, Action: "ac", Field: "x"},
		"action has value":   {ID: 1, Kind: finite.ActionVariable, Action: "ac", Value: "a"},
		"action has other":   {ID: 1, Kind: finite.ActionVariable, Action: "ac", OtherTime: &one},
		"missing other":      {ID: 1, Kind: finite.DifferenceVariable, Field: "x"},
		"other at same time": {ID: 1, Kind: finite.DifferenceVariable, Field: "x", OtherTime: &zero},
		"other past horizon": {ID: 1, Kind: finite.DifferenceVariable, Field: "x", OtherTime: &two},
		"unknown field":      {ID: 1, Kind: finite.DifferenceVariable, Field: "missing", OtherTime: &one},
		"difference value":   {ID: 1, Kind: finite.DifferenceVariable, Field: "x", OtherTime: &one, Value: "a"},
		"difference action":  {ID: 1, Kind: finite.DifferenceVariable, Field: "x", OtherTime: &one, Action: "ac"},
	}
	for name, variable := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := variable.Validate(m, 1); !errors.Is(err, finite.ErrInput) {
				t.Fatalf("accepted %+v: %v", variable, err)
			}
		})
	}
}

func TestEncodingAccessorsRejectUninitializedValues(t *testing.T) {
	for _, encoding := range []*finite.Encoding{nil, new(finite.Encoding)} {
		if _, err := encoding.CNF(); !errors.Is(err, finite.ErrInput) {
			t.Fatal("uninitialized encoding became a true empty CNF", err)
		}
		if _, err := encoding.Variables(); !errors.Is(err, finite.ErrInput) {
			t.Fatal(err)
		}
		if _, err := encoding.Digest(); !errors.Is(err, finite.ErrInput) {
			t.Fatal(err)
		}
	}
}

func TestOtherTimePresenceAndCopyIsolation(t *testing.T) {
	m, q := model(4), query(1)
	encoding, err := finite.Compile(context.Background(), m, q)
	if err != nil {
		t.Fatal(err)
	}
	variables, err := encoding.Variables()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, variable := range variables {
		if err := variable.Validate(m, q.Horizon); err != nil {
			t.Fatal(err)
		}
		if variable.Kind == finite.DifferenceVariable {
			found = true
			*variable.OtherTime = 99
		}
	}
	if !found {
		t.Fatal("missing difference witness")
	}
	again, err := encoding.Variables()
	if err != nil {
		t.Fatal(err)
	}
	for _, variable := range again {
		if err := variable.Validate(m, q.Horizon); err != nil {
			t.Fatal("getter leaked mutable time pointer", err)
		}
	}
}

func TestFailedCompileNeverReturnsPartialEncoding(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for name, input := range map[string]struct {
		ctx     context.Context
		model   finite.Model
		query   finite.Query
		options []finite.Option
	}{
		"canceled":        {ctx: ctx, model: model(4), query: query(1)},
		"budget":          {ctx: context.Background(), model: model(4), query: query(1), options: []finite.Option{finite.WithMaxVariables(1)}},
		"invalid model":   {ctx: context.Background(), model: finite.Model{}, query: query(1)},
		"missing profile": {ctx: context.Background(), model: model(4), query: finite.Query{}},
	} {
		t.Run(name, func(t *testing.T) {
			encoding, err := finite.Compile(input.ctx, input.model, input.query, input.options...)
			if err == nil || encoding != nil {
				t.Fatalf("failed compilation exposed a partial encoding: %v %v", encoding, err)
			}
		})
	}
}
