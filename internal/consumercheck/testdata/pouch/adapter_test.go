package planning

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/finite"
	"github.com/Yui-Qi-Tang/calx/sat"
	"github.com/Yui-Qi-Tang/calx/sat/cadical"
	"github.com/Yui-Qi-Tang/pouch/internal/validation"
)

type calxAdapter struct{ runner *cadical.Runner }

func (a calxAdapter) Solve(ctx context.Context, id string, formula CNF) (SolveResult, error) {
	input := sat.CNF{Clauses: formula.Clauses}
	for _, variable := range formula.Variables {
		raw, err := json.Marshal(variable)
		if err != nil {
			return SolveResult{}, err
		}
		input.Variables = append(input.Variables, sat.Variable{ID: variable.ID, Metadata: map[string]string{"pouch_variable": string(raw)}})
	}
	result, err := a.runner.Solve(ctx, id, input)
	out := SolveResult{Status: string(result.Status), Assignment: result.Assignment}
	for _, artifact := range result.Artifacts {
		out.Artifacts = append(out.Artifacts, Artifact{ID: artifact.ID, SHA256: artifact.Digest.String(), Bytes: artifact.Bytes})
	}
	if err != nil {
		return out, err
	}
	if err := result.Require(sat.Requirements{SATModelChecked: true, UNSATProofChecked: true}); err != nil {
		return out, err
	}
	out.Verified = true
	return out, nil
}

// finiteModel converts the planning model field by field; finite types have no
// JSON tags to round-trip through.
func finiteModel(m validation.Model) finite.Model {
	out := finite.Model{Fields: m.Fields, Actions: make([]finite.Action, len(m.Actions))}
	for _, s := range m.Forbidden {
		out.Forbidden = append(out.Forbidden, finite.State(s))
	}
	for i, a := range m.Actions {
		out.Actions[i] = finite.Action{ID: a.ID, Guard: finite.State(a.Guard), Effect: finite.State(a.Effect), Frame: a.Frame}
	}
	return out
}

func marshalValue(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCalxExactEncoding(t *testing.T) {
	contract := twoBits()
	model := finiteModel(contract.Model)
	matrix, err := Project(contract)
	if err != nil {
		t.Fatal(err)
	}
	replacementMatrix, err := finite.Project(model)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(matrix.Guard, replacementMatrix.Guard) || !reflect.DeepEqual(matrix.Effect, replacementMatrix.Effect) || !reflect.DeepEqual(matrix.Frame, replacementMatrix.Frame) {
		t.Fatal("matrix mismatch")
	}
	options, err := (Options{}).defaults()
	if err != nil {
		t.Fatal(err)
	}
	for horizon := 0; horizon <= 3; horizon++ {
		original, err := encode(context.Background(), matrix, contract.Model.Forbidden, contract.Start, contract.Goal, horizon, nil, options)
		if err != nil {
			t.Fatal(err)
		}
		query := finite.Query{Start: finite.State(contract.Start), Goal: finite.State(contract.Goal), Horizon: horizon}
		replacement, err := finite.Compile(context.Background(), model, query)
		if err != nil {
			t.Fatal(err)
		}
		formula, err := replacement.CNF()
		if err != nil {
			t.Fatal(err)
		}
		var oldBytes, newBytes bytes.Buffer
		if err := original.CNF.WriteDIMACS(&oldBytes); err != nil {
			t.Fatal(err)
		}
		if err := formula.WriteDIMACS(&newBytes); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(oldBytes.Bytes(), newBytes.Bytes()) {
			t.Fatalf("CNF differs at horizon %d", horizon)
		}
		values, found := solveTruth(original.CNF.Clauses, map[int]bool{}, len(original.CNF.Variables))
		if !found {
			continue
		}
		oldTrace, err := original.trace(values)
		if err != nil {
			t.Fatal(err)
		}
		newTrace, err := replacement.Decode(values)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(marshalValue(t, oldTrace.States), marshalValue(t, newTrace.States)) || !reflect.DeepEqual(oldTrace.Actions, newTrace.Actions) {
			t.Fatal("trace mismatch")
		}
	}
}

func TestCalxNativeSearchAndIndependentReplay(t *testing.T) {
	runner, err := cadical.New(cadical.Config{
		SolverPath: os.Getenv("CALX_CADICAL"), SolverDigest: digest.Digest(os.Getenv("CALX_CADICAL_DIGEST")),
		CheckerPath: os.Getenv("CALX_DRAT"), CheckerDigest: digest.Digest(os.Getenv("CALX_DRAT_DIGEST")),
		ArtifactDir: filepath.Join(os.Getenv("CALX_CONSUMER_OUTPUT"), "pouch-solver"),
	})
	if err != nil {
		t.Fatal(err)
	}
	contract := twoBits()
	result, err := Search(context.Background(), contract, calxAdapter{runner: runner}, Options{})
	if err != nil || !result.Complete || len(result.Forward) != 2 {
		t.Fatal(result, err)
	}
	raw := marshalValue(t, contract)
	authority, err := validation.NewAuthority(raw, validation.Digest(raw))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range result.Forward {
		if _, err := authority.ReplayForward(context.Background(), path.Trace); err != nil {
			t.Fatal(err)
		}
	}
	for _, back := range result.Returns {
		if back.Status != "RETURN_FOUND" || back.Trace == nil || len(back.Trace.Actions) != 2 {
			t.Fatal(back)
		}
		reverse := contract
		reverse.Start, reverse.Goal, reverse.ForwardLimit = back.Endpoint, contract.Baseline, contract.ReturnLimit
		raw := marshalValue(t, reverse)
		reverseAuthority, err := validation.NewAuthority(raw, validation.Digest(raw))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := reverseAuthority.ReplayForward(context.Background(), *back.Trace); err != nil {
			t.Fatal(err)
		}
	}
}
