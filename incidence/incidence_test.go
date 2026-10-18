package incidence_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Yui-Qi-Tang/calx/incidence"
)

func TestRoundTripTopologyAndOwnership(t *testing.T) {
	g := incidence.Graph{NodeIDs: []string{"isolated", "b", "a"}, Edges: []incidence.Edge{{ID: "parallel2", From: "a", To: "b", Relation: "r"}, {ID: "loop", From: "a", To: "a", Relation: "other"}, {ID: "parallel1", From: "a", To: "b", Relation: "r"}}}
	p, err := incidence.Project(g, incidence.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	back, err := p.Graph(incidence.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	again, err := incidence.Project(back, incidence.Limits{})
	if err != nil || !reflect.DeepEqual(p, again) {
		t.Fatal("round trip", err)
	}
	if len(back.NodeIDs) != 3 || len(back.Edges) != 3 || back.Edges[0].From != back.Edges[0].To {
		t.Fatal(back)
	}
	g.NodeIDs[0] = "changed"
	g.Edges[0].ID = "changed"
	if p.NodeIDs[2] != "isolated" || p.EdgeIDs[2] != "parallel2" {
		t.Fatal("aliased input")
	}
	back.NodeIDs[0] = "changed"
	if p.NodeIDs[0] != "a" {
		t.Fatal("aliased output")
	}
}
func TestEmptyAndMalformed(t *testing.T) {
	for _, g := range []incidence.Graph{{}, {NodeIDs: []string{"alone"}}} {
		p, e := incidence.Project(g, incidence.Limits{})
		if e != nil {
			t.Fatal(e)
		}
		if _, e = p.Graph(incidence.Limits{}); e != nil {
			t.Fatal(e)
		}
	}
	for _, g := range []incidence.Graph{{NodeIDs: []string{"x", "x"}}, {NodeIDs: []string{""}}, {NodeIDs: []string{"x"}, Edges: []incidence.Edge{{ID: "a", From: "x", To: "missing", Relation: "r"}}}} {
		if _, e := incidence.Project(g, incidence.Limits{}); !errors.Is(e, incidence.ErrInput) {
			t.Fatal(g, e)
		}
	}
	g := incidence.Graph{NodeIDs: []string{"a", "b"}, Edges: []incidence.Edge{{ID: "e", From: "a", To: "b", Relation: "r"}}}
	if _, e := incidence.Project(g, incidence.Limits{MaxCells: 4}); !errors.Is(e, incidence.ErrLimit) {
		t.Fatal(e)
	}
	for _, mutate := range []func(*incidence.Projection){func(p *incidence.Projection) { p.Source[1][0] = 1 }, func(p *incidence.Projection) { p.Target[1][0] = 2 }, func(p *incidence.Projection) { p.RelationMasks["extra"] = []int{1} }, func(p *incidence.Projection) { p.NodeIDs[1] = "a" }, func(p *incidence.Projection) { p.Source = nil }} {
		p, _ := incidence.Project(g, incidence.Limits{})
		mutate(&p)
		if _, e := p.Graph(incidence.Limits{}); !errors.Is(e, incidence.ErrInput) {
			t.Fatal(p, e)
		}
	}
}
