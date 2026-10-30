// Command incidence demonstrates topology projection and reconstruction.
package main

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/incidence"
)

func main() {
	graph := incidence.Graph{
		NodeIDs: []string{"c", "a", "b"},
		Edges: []incidence.Edge{
			{ID: "e1", From: "a", To: "b", Relation: "connects"},
			{ID: "e2", From: "a", To: "b", Relation: "connects"},
			{ID: "e3", From: "b", To: "b", Relation: "loops"},
		},
	}
	projection, err := incidence.Project(graph, incidence.Limits{})
	if err != nil {
		log.Fatal(err)
	}
	reconstructed, err := projection.Graph(incidence.Limits{})
	if err != nil {
		log.Fatal(err)
	}
	// Deterministic sorts map keys so the output is reproducible.
	output := jsontext.NewEncoder(os.Stdout, jsontext.WithIndent("  "))
	if err := json.MarshalEncode(output, struct {
		Projection    incidence.Projection `json:"projection"`
		Reconstructed incidence.Graph      `json:"reconstructed"`
	}{Projection: projection, Reconstructed: reconstructed}, json.Deterministic(true)); err != nil {
		log.Fatal(err)
	}
}
