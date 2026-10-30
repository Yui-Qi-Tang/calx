package finite

import (
	"context"
	"testing"

	"github.com/Yui-Qi-Tang/calx/internal/jsoncompat"
)

// variableJSON is the documented metadata format: the field order, keys and
// omitempty rules that metadata builds by hand.
type variableJSON struct {
	ID        int          `json:"id"`
	Kind      VariableKind `json:"kind"`
	Time      int          `json:"time"`
	OtherTime *int         `json:"other_time,omitempty"`
	Field     FieldName    `json:"field,omitempty"`
	Value     Category     `json:"value,omitempty"`
	Action    ActionID     `json:"action,omitempty"`
}

// TestMetadataMatchesMarshal keeps the hand-built metadata byte-identical to
// jsoncompat.Marshal of variableJSON, including names that need HTML or
// JavaScript escaping.
func TestMetadataMatchesMarshal(t *testing.T) {
	m := Model{
		Fields: map[string][]string{"x<&>": {"a\u2028", `q"uote`}, "y": {"é", "\x01"}},
		Actions: []Action{
			{ID: "go<>", Guard: State{"y": "é"}, Effect: State{"x<&>": "a\u2028"}, Frame: []string{"y"}},
			{ID: "back", Effect: State{"x<&>": `q"uote`, "y": "\x01"}},
		},
	}
	q := Query{Start: State{"x<&>": `q"uote`, "y": "é"}, Goal: State{"x<&>": "a\u2028"}, Horizon: 3}
	e, err := Compile(context.Background(), m, q)
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range e.mapping {
		want, err := jsoncompat.Marshal(variableJSON(v))
		if err != nil {
			t.Fatal(err)
		}
		if got := e.formula.Variables[i].Metadata["finite_variable"]; got != string(want) {
			t.Fatalf("variable %d:\n got  %s\n want %s", v.ID, got, want)
		}
	}
	if len(e.mapping) == 0 {
		t.Fatal("no variables")
	}
}

// BenchmarkCompile encodes 6 fields of 4 values with 12 actions at horizon 8.
func BenchmarkCompile(b *testing.B) {
	fields := map[string][]string{}
	start := State{}
	for f := range 6 {
		name := string(rune('a' + f))
		fields[name] = []string{"v0", "v1", "v2", "v3"}
		start[name] = "v0"
	}
	var actions []Action
	for i := range 12 {
		g, e := string(rune('a'+i%6)), string(rune('a'+(i+1)%6))
		var frame []string
		for f := range 6 {
			if name := string(rune('a' + f)); name != e {
				frame = append(frame, name)
			}
		}
		actions = append(actions, Action{ID: string(rune('A' + i)), Guard: State{g: "v" + string(rune('0'+i%4))}, Effect: State{e: "v" + string(rune('0'+(i+2)%4))}, Frame: frame})
	}
	m := Model{Fields: fields, Actions: actions}
	q := Query{Start: start, Goal: State{"f": "v3"}, Horizon: 8}
	for b.Loop() {
		if _, err := Compile(context.Background(), m, q); err != nil {
			b.Fatal(err)
		}
	}
}
