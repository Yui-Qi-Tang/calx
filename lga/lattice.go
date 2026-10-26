// Package lga implements lattice gas automata as automaton.System values, so
// the same rule drives forward simulation and bounded SAT analysis. It owns the
// lattice geometry and collision rules; it does not claim that a model
// reproduces a real fluid. HPP's square lattice in particular is known not to
// give isotropic Navier-Stokes flow.
package lga

import (
	"errors"
	"fmt"

	"github.com/Yui-Qi-Tang/calx/automaton"
)

// ErrInput marks a malformed lattice, state or averaging request.
var ErrInput = errors.New("invalid lattice input")

// geometry is a periodic Width×Height lattice in row-major cell order, with
// optional bounce-back wall cells.
type geometry struct {
	width, height int
	walls         []bool
	bits          int          // bits per cell, one per direction
	steps         [][2]int     // lattice offset (dx, dy) moved by each direction
	velocity      [][2]float64 // Cartesian unit velocity of each direction
}

func (g geometry) cells() int { return g.width * g.height }

func (g geometry) validate() error {
	if g.width < 1 || g.height < 1 {
		return fmt.Errorf("%w: width and height must be at least 1", ErrInput)
	}
	if g.walls != nil && len(g.walls) != g.cells() {
		return fmt.Errorf("%w: walls must be nil or have one entry per cell", ErrInput)
	}
	return nil
}

func (g geometry) wrap(x, y int) int {
	x, y = (x%g.width+g.width)%g.width, (y%g.height+g.height)%g.height
	return y*g.width + x
}

func (g geometry) wall(cell int) bool { return cell < len(g.walls) && g.walls[cell] }

// arrivals lists, in direction order, the bit that arrives at cell: direction
// d comes from the neighbor one step against d.
func (g geometry) arrivals(cell int) []automaton.Bit {
	x, y := cell%g.width, cell/g.width
	in := make([]automaton.Bit, len(g.steps))
	for d, s := range g.steps {
		in[d] = automaton.Bit{Cell: g.wrap(x-s[0], y-s[1]), Index: d}
	}
	return in
}

// bounce reverses every arriving particle, the rule at a wall cell.
func (g geometry) bounce(in, out []bool) {
	half := g.bits / 2
	for d := range g.bits {
		out[d] = in[(d+half)%g.bits]
	}
}

func (g geometry) mass() automaton.Quantity {
	q := make(automaton.Quantity, 0, g.cells()*g.bits)
	for c := range g.cells() {
		for d := range g.bits {
			q = append(q, automaton.Term{Bit: automaton.Bit{Cell: c, Index: d}, Weight: 1})
		}
	}
	return q
}

// weighted builds a quantity over cells with weight[d] in {-1, 0, 1} for each
// direction d.
func weighted(cells []int, weight []int) automaton.Quantity {
	var q automaton.Quantity
	for _, c := range cells {
		for d, w := range weight {
			if w != 0 {
				q = append(q, automaton.Term{Bit: automaton.Bit{Cell: c, Index: d}, Weight: w})
			}
		}
	}
	return q
}

func (g geometry) all() []int {
	cells := make([]int, g.cells())
	for c := range cells {
		cells[c] = c
	}
	return cells
}

// invalidQuantity names a nonexistent bit, so Compile fails with
// automaton.ErrInput instead of silently encoding an empty or shifted sum.
func invalidQuantity() automaton.Quantity {
	return automaton.Quantity{{Bit: automaton.Bit{Cell: -1}, Weight: 1}}
}

func (g geometry) density(states []automaton.State, block int) ([][]float64, error) {
	mass, _, _, err := g.flow(states, block)
	if err != nil {
		return nil, err
	}
	cells := float64(block * block * len(states))
	for _, row := range mass {
		for i := range row {
			row[i] /= cells
		}
	}
	return mass, nil
}

func (g geometry) meanVelocity(states []automaton.State, block int) (ux, uy [][]float64, err error) {
	mass, px, py, err := g.flow(states, block)
	if err != nil {
		return nil, nil, err
	}
	for j, row := range mass {
		for i, m := range row {
			if m > 0 {
				px[j][i] /= m
				py[j][i] /= m
			}
		}
	}
	return px, py, nil
}

// flow sums particle count and Cartesian momentum per block over all states.
// Blocks are block×block in lattice indices.
func (g geometry) flow(states []automaton.State, block int) (mass, px, py [][]float64, err error) {
	if err := g.validate(); err != nil {
		return nil, nil, nil, err
	}
	if block < 1 || g.width%block != 0 || g.height%block != 0 || len(states) == 0 {
		return nil, nil, nil, fmt.Errorf("%w: block or states", ErrInput)
	}
	rows, cols := g.height/block, g.width/block
	mass, px, py = grid(rows, cols), grid(rows, cols), grid(rows, cols)
	for _, s := range states {
		if len(s) != g.cells()*g.bits {
			return nil, nil, nil, fmt.Errorf("%w: state size", ErrInput)
		}
		for c := range g.cells() {
			j, i := c/g.width/block, c%g.width/block
			for d, present := range s[c*g.bits : (c+1)*g.bits] {
				if present {
					mass[j][i]++
					px[j][i] += g.velocity[d][0]
					py[j][i] += g.velocity[d][1]
				}
			}
		}
	}
	return mass, px, py, nil
}

func grid(rows, cols int) [][]float64 {
	g := make([][]float64, rows)
	for j := range g {
		g[j] = make([]float64, cols)
	}
	return g
}
