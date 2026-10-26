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

// HPP directions in bit order. East increases x and north increases y.
const (
	East = iota
	North
	West
	South
)

// HPP is the Hardy-de Pazzis-Pomeau lattice gas on a periodic Width×Height
// square lattice. Cell (x, y) has index y*Width+x and one bit per direction.
// A state holds the post-collision particles leaving each cell; one step
// streams every particle to its neighbor and then collides at that neighbor.
// Width and Height must be at least 1.
type HPP struct {
	Width, Height int
}

// Cells returns Width*Height.
func (h HPP) Cells() int { return h.Width * h.Height }

// Bits returns 4, one bit per direction.
func (HPP) Bits() int { return 4 }

// Inputs lists the particles arriving at cell in direction order: the
// east-moving particle of the west neighbor, the north-moving particle of the
// south neighbor, and so on.
func (h HPP) Inputs(cell int) []automaton.Bit {
	x, y := cell%h.Width, cell/h.Width
	return []automaton.Bit{
		{Cell: h.wrap(x-1, y), Index: East},
		{Cell: h.wrap(x, y-1), Index: North},
		{Cell: h.wrap(x+1, y), Index: West},
		{Cell: h.wrap(x, y+1), Index: South},
	}
}

// Next collides the arriving particles.
func (HPP) Next(_ int, in, out []bool) {
	collided := collideHPP([4]bool(in))
	copy(out, collided[:])
}

func (h HPP) wrap(x, y int) int {
	x, y = (x%h.Width+h.Width)%h.Width, (y%h.Height+h.Height)%h.Height
	return y*h.Width + x
}

// collideHPP turns an exact head-on pair into the perpendicular pair; every
// other configuration passes through. It conserves mass and momentum and is
// its own inverse.
func collideHPP(p [4]bool) [4]bool {
	switch p {
	case [4]bool{East: true, West: true}:
		return [4]bool{North: true, South: true}
	case [4]bool{North: true, South: true}:
		return [4]bool{East: true, West: true}
	}
	return p
}

// reverse swaps every particle's direction.
func reverse(s automaton.State) automaton.State {
	out := make(automaton.State, len(s))
	for base := 0; base+4 <= len(s); base += 4 {
		out[base+East], out[base+West] = s[base+West], s[base+East]
		out[base+North], out[base+South] = s[base+South], s[base+North]
	}
	return out
}

func terms(cells []int, plus, minus int) automaton.Quantity {
	var q automaton.Quantity
	for _, c := range cells {
		if plus >= 0 {
			q = append(q, automaton.Term{Bit: automaton.Bit{Cell: c, Index: plus}, Weight: 1})
		}
		if minus >= 0 {
			q = append(q, automaton.Term{Bit: automaton.Bit{Cell: c, Index: minus}, Weight: -1})
		}
	}
	return q
}

func (h HPP) all() []int {
	cells := make([]int, h.Cells())
	for c := range cells {
		cells[c] = c
	}
	return cells
}

// Mass counts every particle.
func (h HPP) Mass() automaton.Quantity {
	var q automaton.Quantity
	for _, c := range h.all() {
		for d := range 4 {
			q = append(q, automaton.Term{Bit: automaton.Bit{Cell: c, Index: d}, Weight: 1})
		}
	}
	return q
}

// MomentumX sums east-moving minus west-moving particles.
func (h HPP) MomentumX() automaton.Quantity { return terms(h.all(), East, West) }

// MomentumY sums north-moving minus south-moving particles.
func (h HPP) MomentumY() automaton.Quantity { return terms(h.all(), North, South) }

// RowMomentumX is the x momentum of row y alone. HPP conserves it separately
// for every row, a known spurious invariant of the square lattice. An
// out-of-range row yields a quantity that Compile rejects.
func (h HPP) RowMomentumX(y int) automaton.Quantity {
	if y < 0 || y >= h.Height {
		return invalidQuantity()
	}
	cells := make([]int, h.Width)
	for x := range cells {
		cells[x] = y*h.Width + x
	}
	return terms(cells, East, West)
}

// ColumnMomentumY is the y momentum of column x alone, conserved separately
// for every column. An out-of-range column yields a quantity that Compile
// rejects.
func (h HPP) ColumnMomentumY(x int) automaton.Quantity {
	if x < 0 || x >= h.Width {
		return invalidQuantity()
	}
	cells := make([]int, h.Height)
	for y := range cells {
		cells[y] = y*h.Width + x
	}
	return terms(cells, North, South)
}

// invalidQuantity names a nonexistent bit, so Compile fails with
// automaton.ErrInput instead of silently encoding an empty or shifted sum.
func invalidQuantity() automaton.Quantity {
	return automaton.Quantity{{Bit: automaton.Bit{Cell: -1}, Weight: 1}}
}

// Density returns the mean number of particles per cell in each block×block
// region, averaged over every given state. Rows of the result are y.
func (h HPP) Density(states []automaton.State, block int) ([][]float64, error) {
	mass, _, _, err := h.average(states, block)
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

// Velocity returns the mean momentum per particle (ux, uy) in each block×block
// region over every given state. Regions without particles have zero velocity.
func (h HPP) Velocity(states []automaton.State, block int) (ux, uy [][]float64, err error) {
	mass, px, py, err := h.average(states, block)
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

// average sums particle count and momentum per block over all states.
func (h HPP) average(states []automaton.State, block int) (mass, px, py [][]float64, err error) {
	if h.Width < 1 || h.Height < 1 || block < 1 || h.Width%block != 0 || h.Height%block != 0 || len(states) == 0 {
		return nil, nil, nil, fmt.Errorf("%w: lattice, block or states", ErrInput)
	}
	rows, cols := h.Height/block, h.Width/block
	mass, px, py = grid(rows, cols), grid(rows, cols), grid(rows, cols)
	for _, s := range states {
		if len(s) != h.Cells()*4 {
			return nil, nil, nil, fmt.Errorf("%w: state size", ErrInput)
		}
		for c := range h.Cells() {
			j, i := c/h.Width/block, c%h.Width/block
			p := s[c*4 : c*4+4]
			for _, b := range p {
				if b {
					mass[j][i]++
				}
			}
			px[j][i] += sign(p[East]) - sign(p[West])
			py[j][i] += sign(p[North]) - sign(p[South])
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

func sign(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
