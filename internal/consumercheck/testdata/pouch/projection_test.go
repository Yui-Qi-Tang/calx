package projection

import (
	"reflect"
	"testing"

	"github.com/Yui-Qi-Tang/calx/incidence"
)

func TestCalxTopologyProjection(t *testing.T) {
	snapshot := snapshot(t, []byte(graphFixture))
	graph := snapshot.Graph()
	var input incidence.Graph
	for _, node := range graph.Nodes {
		input.NodeIDs = append(input.NodeIDs, node.ID)
	}
	for _, edge := range graph.Edges {
		input.Edges = append(input.Edges, incidence.Edge{ID: edge.ID, From: edge.From, To: edge.To, Relation: edge.Relation})
	}
	original, err := Project(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := incidence.Project(input, incidence.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(original.NodeIDs, replacement.NodeIDs) || !reflect.DeepEqual(original.EdgeIDs, replacement.EdgeIDs) || !reflect.DeepEqual(original.Source, replacement.Source) || !reflect.DeepEqual(original.Target, replacement.Target) || !reflect.DeepEqual(original.RelationMasks, replacement.RelationMasks) {
		t.Fatal("topology differs")
	}
	original.Source, original.Target, original.RelationMasks = replacement.Source, replacement.Target, replacement.RelationMasks
	if _, err := Reconstruct(original); err != nil {
		t.Fatal(err)
	}
}
