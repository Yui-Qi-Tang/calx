// Package incidence projects directed multigraph topology into binary incidence
// matrices. Relations are opaque labels, never logical operators. Payload,
// provenance, scope and truncation records remain caller-owned sidecars.
package incidence

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

// ErrInput identifies malformed topology or a malformed projection.
var ErrInput = errors.New("invalid incidence input")

// ErrLimit identifies a projection exceeding its resource limits.
var ErrLimit = errors.New("incidence limit exceeded")

// Edge is an independently identified directed edge with an opaque relation.
type Edge struct {
	ID       string
	From     string
	To       string
	Relation string
}

// Graph contains topology only. IDs and relation labels must be nonempty.
// Parallel edges and self-loops are allowed; duplicate IDs are rejected.
type Graph struct {
	NodeIDs []string
	Edges   []Edge
}

// Limits bounds allocations. Zero fields use conservative defaults.
type Limits struct{ MaxNodes, MaxEdges, MaxCells int }

func (l Limits) defaults() (Limits, error) {
	if l.MaxNodes < 0 || l.MaxEdges < 0 || l.MaxCells < 0 {
		return l, ErrInput
	}
	if l.MaxNodes == 0 {
		l.MaxNodes = 4096
	}
	if l.MaxEdges == 0 {
		l.MaxEdges = 8192
	}
	if l.MaxCells == 0 {
		l.MaxCells = 4_000_000
	}
	return l, nil
}
func (l Limits) check(n, e, r int) error {
	// Count both incidence matrices and all relation masks, without overflow.
	if n > l.MaxNodes || e > l.MaxEdges {
		return ErrLimit
	}
	left := l.MaxCells
	for _, rows := range []int{n, n, r} {
		if e != 0 && rows > left/e {
			return ErrLimit
		}
		left -= rows * e
	}
	return nil
}

// Projection has node rows and edge columns. Separate source/target matrices
// preserve self-loops and parallel-edge identity. Axes are sorted by ID.
// Numeric matrices alone do not preserve labels, IDs or application records.
type Projection struct {
	NodeIDs       []string
	EdgeIDs       []string
	Source        [][]int
	Target        [][]int
	RelationMasks map[string][]int
}

// Project validates and copies topology into deterministic matrices. Inputs
// must not be mutated concurrently. No source payload is interpreted or copied.
func Project(g Graph, limits Limits) (Projection, error) {
	l, err := limits.defaults()
	if err != nil {
		return Projection{}, err
	}
	if len(g.NodeIDs) > l.MaxNodes || len(g.Edges) > l.MaxEdges {
		return Projection{}, ErrLimit
	}
	nodes := append([]string{}, g.NodeIDs...)
	slices.Sort(nodes)
	index := make(map[string]int, len(nodes))
	for i, id := range nodes {
		if id == "" || !utf8.ValidString(id) {
			return Projection{}, ErrInput
		}
		if _, ok := index[id]; ok {
			return Projection{}, fmt.Errorf("%w: duplicate node", ErrInput)
		}
		index[id] = i
	}
	edges := slices.Clone(g.Edges)
	slices.SortFunc(edges, func(a, b Edge) int { return strings.Compare(a.ID, b.ID) })
	masks := map[string][]int{}
	for i, e := range edges {
		if e.ID == "" || !utf8.ValidString(e.ID) || e.Relation == "" || !utf8.ValidString(e.Relation) || (i > 0 && edges[i-1].ID == e.ID) {
			return Projection{}, ErrInput
		}
		if _, ok := index[e.From]; !ok {
			return Projection{}, fmt.Errorf("%w: missing source", ErrInput)
		}
		if _, ok := index[e.To]; !ok {
			return Projection{}, fmt.Errorf("%w: missing target", ErrInput)
		}
		masks[e.Relation] = nil
	}
	if err := l.check(len(nodes), len(edges), len(masks)); err != nil {
		return Projection{}, err
	}
	p := Projection{NodeIDs: nodes, EdgeIDs: make([]string, len(edges)), Source: make([][]int, len(nodes)), Target: make([][]int, len(nodes)), RelationMasks: masks}
	for i := range nodes {
		p.Source[i] = make([]int, len(edges))
		p.Target[i] = make([]int, len(edges))
	}
	for label := range masks {
		masks[label] = make([]int, len(edges))
	}
	for j, e := range edges {
		p.EdgeIDs[j] = e.ID
		p.Source[index[e.From]][j] = 1
		p.Target[index[e.To]][j] = 1
		masks[e.Relation][j] = 1
	}
	return p, nil
}

// Graph reconstructs topology and rejects conflicting axes or matrix columns.
// It does not recreate source payloads or claim that the input view was complete.
func (p Projection) Graph(limits Limits) (Graph, error) {
	l, err := limits.defaults()
	if err != nil {
		return Graph{}, err
	}
	n, e := len(p.NodeIDs), len(p.EdgeIDs)
	if err := l.check(n, e, len(p.RelationMasks)); err != nil {
		return Graph{}, err
	}
	unique := func(ids []string) bool {
		seen := map[string]bool{}
		for _, id := range ids {
			if id == "" || !utf8.ValidString(id) || seen[id] {
				return false
			}
			seen[id] = true
		}
		return true
	}
	if !unique(p.NodeIDs) || !unique(p.EdgeIDs) || len(p.Source) != n || len(p.Target) != n {
		return Graph{}, ErrInput
	}
	source, target := make([]int, e), make([]int, e)
	for j := range e {
		source[j] = -1
		target[j] = -1
	}
	for i := range n {
		if len(p.Source[i]) != e || len(p.Target[i]) != e {
			return Graph{}, ErrInput
		}
		for j := range e {
			if p.Source[i][j] < 0 || p.Source[i][j] > 1 || p.Target[i][j] < 0 || p.Target[i][j] > 1 {
				return Graph{}, ErrInput
			}
			if p.Source[i][j] == 1 {
				if source[j] != -1 {
					return Graph{}, ErrInput
				}
				source[j] = i
			}
			if p.Target[i][j] == 1 {
				if target[j] != -1 {
					return Graph{}, ErrInput
				}
				target[j] = i
			}
		}
	}
	labels := make([]string, e)
	for label, row := range p.RelationMasks {
		if label == "" || !utf8.ValidString(label) || len(row) != e {
			return Graph{}, ErrInput
		}
		for j, v := range row {
			if v < 0 || v > 1 {
				return Graph{}, ErrInput
			}
			if v == 1 {
				if labels[j] != "" {
					return Graph{}, ErrInput
				}
				labels[j] = label
			}
		}
	}
	g := Graph{NodeIDs: slices.Clone(p.NodeIDs), Edges: make([]Edge, e)}
	for j, id := range p.EdgeIDs {
		if source[j] < 0 || target[j] < 0 || labels[j] == "" {
			return Graph{}, ErrInput
		}
		g.Edges[j] = Edge{ID: id, From: p.NodeIDs[source[j]], To: p.NodeIDs[target[j]], Relation: labels[j]}
	}
	return g, nil
}
