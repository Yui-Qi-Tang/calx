// Command matrix demonstrates categorical operation matrices and reconstruction.
package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/finite"
)

func main() {
	model := finite.Model{
		Fields: map[string][]string{
			"mode":  {"auto", "manual"},
			"state": {"off", "on"},
		},
		Actions: []finite.Action{{
			ID:     "enable",
			Guard:  finite.State{"state": "off"},
			Effect: finite.State{"state": "on"},
			Frame:  []string{"mode"},
		}},
		Forbidden: []finite.State{{"mode": "manual", "state": "on"}},
	}
	matrix, err := finite.Project(model)
	if err != nil {
		log.Fatal(err)
	}
	// Forbidden patterns belong to the model; the matrix does not encode them.
	reconstructed, err := matrix.Model(model.Forbidden)
	if err != nil {
		log.Fatal(err)
	}
	// Deterministic sorts map keys so the output is reproducible.
	output := jsontext.NewEncoder(os.Stdout, jsontext.WithIndent("  "))
	if err := json.MarshalEncode(output, struct {
		Matrix        matrixJSON `json:"matrix"`
		Reconstructed modelJSON  `json:"reconstructed"`
	}{Matrix: toMatrixJSON(matrix), Reconstructed: toModelJSON(reconstructed)}, json.Deterministic(true)); err != nil {
		log.Fatal(err)
	}
}

// The example chooses its own JSON names; finite types have no JSON tags.
type matrixJSON struct {
	Fields      []string         `json:"fields"`
	Coordinates []coordinateJSON `json:"coordinates"`
	ActionIDs   []string         `json:"action_ids"`
	Guard       [][]int          `json:"guard"`
	Effect      [][]int          `json:"effect"`
	Frame       [][]int          `json:"frame"`
}

type coordinateJSON struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

type modelJSON struct {
	Fields    map[string][]string `json:"fields"`
	Forbidden []finite.State      `json:"forbidden"`
	Actions   []actionJSON        `json:"actions"`
}

type actionJSON struct {
	ID     string       `json:"id"`
	Guard  finite.State `json:"guard"`
	Effect finite.State `json:"effect"`
	Frame  []string     `json:"frame"`
}

func toMatrixJSON(m finite.Matrix) matrixJSON {
	coordinates := make([]coordinateJSON, len(m.Coordinates))
	for i, c := range m.Coordinates {
		coordinates[i] = coordinateJSON(c)
	}
	return matrixJSON{Fields: m.Fields, Coordinates: coordinates, ActionIDs: m.ActionIDs, Guard: m.Guard, Effect: m.Effect, Frame: m.Frame}
}

func toModelJSON(m finite.Model) modelJSON {
	actions := make([]actionJSON, len(m.Actions))
	for i, a := range m.Actions {
		actions[i] = actionJSON(a)
	}
	return modelJSON{Fields: m.Fields, Forbidden: m.Forbidden, Actions: actions}
}
