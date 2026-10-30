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
		Projection    projectionJSON `json:"projection"`
		Reconstructed graphJSON      `json:"reconstructed"`
	}{Projection: projectionJSON(projection), Reconstructed: toGraphJSON(reconstructed)}, json.Deterministic(true)); err != nil {
		log.Fatal(err)
	}
}

// incidence types carry no JSON tags, so the example defines its own output.
type projectionJSON struct {
	NodeIDs       []string         `json:"node_ids"`
	EdgeIDs       []string         `json:"edge_ids"`
	Source        [][]int          `json:"source"`
	Target        [][]int          `json:"target"`
	RelationMasks map[string][]int `json:"relation_masks"`
}

type graphJSON struct {
	NodeIDs []string   `json:"node_ids"`
	Edges   []edgeJSON `json:"edges"`
}

type edgeJSON struct {
	ID       string `json:"id"`
	From     string `json:"from"`
	To       string `json:"to"`
	Relation string `json:"relation"`
}

func toGraphJSON(g incidence.Graph) graphJSON {
	edges := make([]edgeJSON, len(g.Edges))
	for i, e := range g.Edges {
		edges[i] = edgeJSON(e)
	}
	return graphJSON{NodeIDs: g.NodeIDs, Edges: edges}
}
