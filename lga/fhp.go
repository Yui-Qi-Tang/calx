package lga

import (
	"math"

	"github.com/Yui-Qi-Tang/calx/automaton"
)

// FHP directions in bit order, counterclockwise at 60° steps from east.
const (
	HexE = iota
	HexNE
	HexNW
	HexW
	HexSW
	HexSE
)

// fhpSteps are axial offsets: q along the east direction a1 = (1, 0) and r
// along the north-east direction a2 = (1/2, √3/2).
var fhpSteps = [][2]int{HexE: {1, 0}, HexNE: {0, 1}, HexNW: {-1, 1}, HexW: {-1, 0}, HexSW: {0, -1}, HexSE: {1, -1}}

var fhpVelocity = func() [][2]float64 {
	v := make([][2]float64, 6)
	for d := range v {
		angle := float64(d) * math.Pi / 3
		v[d] = [2]float64{math.Cos(angle), math.Sin(angle)}
	}
	return v
}()

// FHP is the Frisch-Hasslacher-Pomeau FHP-I lattice gas: six moving particles
// per cell on a hexagonal lattice, no rest particle. Cells are indexed in axial
// coordinates on a periodic Width×Height parallelogram: cell (q, r) has index
// r*Width+q, and its six neighbors are at axial offsets (1,0), (0,1), (-1,1),
// (-1,0), (0,-1) and (1,-1), in direction order.
// As in HPP, a state holds the post-collision particles leaving each cell.
//
// Collisions: an exact head-on pair rotates by 60° in either direction, chosen
// by one choice bit per cell (true is counterclockwise); an exact symmetric
// triple rotates to the other triple; everything else passes through. Wall
// cells reflect every arriving particle and have no choice bit. Walls is nil
// or has one entry per cell. Call Validate before use.
type FHP struct {
	Width, Height int
	Walls         []bool
}

func (f FHP) geometry() geometry {
	return geometry{width: f.Width, height: f.Height, walls: f.Walls, bits: 6, steps: fhpSteps, velocity: fhpVelocity}
}

// Validate checks the dimensions and the length of Walls.
func (f FHP) Validate() error { return f.geometry().validate() }

// Cells returns Width*Height.
func (f FHP) Cells() int { return f.Width * f.Height }

// Bits returns 6, one bit per direction.
func (FHP) Bits() int { return 6 }

// Inputs lists the particles arriving at cell in direction order.
func (f FHP) Inputs(cell int) []automaton.Bit { return f.geometry().arrivals(cell) }

// Choices is 1 for a fluid cell, the rotation direction of a head-on pair, and
// 0 for a wall cell.
func (f FHP) Choices(cell int) int {
	if f.geometry().wall(cell) {
		return 0
	}
	return 1
}

// Next collides the arriving particles with the cell's choice bit, or reflects
// them at a wall cell.
func (f FHP) Next(cell int, in, out []bool) {
	if g := f.geometry(); g.wall(cell) {
		g.bounce(in, out)
		return
	}
	collided := collideFHP([6]bool(in[:6]), in[6])
	copy(out, collided[:])
}

func pairFHP(i int) [6]bool {
	var p [6]bool
	p[i%6], p[(i+3)%6] = true, true
	return p
}

// collideFHP applies the FHP-I table. It conserves mass and momentum. Rotating
// a pair one way and then the other restores it, and the triple exchange is
// its own inverse.
func collideFHP(p [6]bool, counterclockwise bool) [6]bool {
	for i := range 3 {
		if p == pairFHP(i) {
			if counterclockwise {
				return pairFHP(i + 1)
			}
			return pairFHP(i + 5)
		}
	}
	switch p {
	case [6]bool{HexE: true, HexNW: true, HexSW: true}:
		return [6]bool{HexNE: true, HexW: true, HexSE: true}
	case [6]bool{HexNE: true, HexW: true, HexSE: true}:
		return [6]bool{HexE: true, HexNW: true, HexSW: true}
	}
	return p
}

// Mass counts every particle.
func (f FHP) Mass() automaton.Quantity { return f.geometry().mass() }

// MomentumQ and MomentumR are the total momentum in the axial basis: the
// Cartesian momentum equals MomentumQ*(1, 0) + MomentumR*(1/2, √3/2). Both
// have integer weights, so they fit automaton.Quantity, and the momentum is
// conserved exactly when both are.
func (f FHP) MomentumQ() automaton.Quantity {
	return weighted(f.geometry().all(), axialWeights(0))
}

// MomentumR is the r component of the axial momentum; see MomentumQ.
func (f FHP) MomentumR() automaton.Quantity {
	return weighted(f.geometry().all(), axialWeights(1))
}

func axialWeights(component int) []int {
	w := make([]int, 6)
	for d, s := range fhpSteps {
		w[d] = s[component]
	}
	return w
}

// Density returns the mean number of particles per cell in each block×block
// region of axial indices, averaged over every given state.
func (f FHP) Density(states []automaton.State, block int) ([][]float64, error) {
	return f.geometry().density(states, block)
}

// Velocity returns the mean Cartesian momentum per particle (ux, uy) in each
// block×block region of axial indices over every given state.
func (f FHP) Velocity(states []automaton.State, block int) (ux, uy [][]float64, err error) {
	return f.geometry().meanVelocity(states, block)
}
