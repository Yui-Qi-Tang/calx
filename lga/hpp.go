package lga

import (
	"github.com/Yui-Qi-Tang/calx/automaton"
)

// HPP directions in bit order. East increases x and north increases y.
const (
	East = iota
	North
	West
	South
)

var (
	hppSteps    = [][2]int{East: {1, 0}, North: {0, 1}, West: {-1, 0}, South: {0, -1}}
	hppVelocity = [][2]float64{East: {1, 0}, North: {0, 1}, West: {-1, 0}, South: {0, -1}}
)

// HPP is the Hardy-de Pazzis-Pomeau lattice gas on a periodic Width×Height
// square lattice. Cell (x, y) has index y*Width+x and one bit per direction.
// A state holds the post-collision particles leaving each cell; one step
// streams every particle to its neighbor and then collides at that neighbor.
//
// Walls is nil or has one entry per cell. A wall cell reflects every arriving
// particle back the way it came (bounce-back), which conserves mass but not
// momentum. Call Validate before use; the System methods assume a valid value.
type HPP struct {
	Width, Height int
	Walls         []bool
}

func (h HPP) geometry() geometry {
	return geometry{width: h.Width, height: h.Height, walls: h.Walls, bits: 4, steps: hppSteps, velocity: hppVelocity}
}

// Validate checks the dimensions and the length of Walls.
func (h HPP) Validate() error { return h.geometry().validate() }

// Cells returns Width*Height.
func (h HPP) Cells() int { return h.Width * h.Height }

// Bits returns 4, one bit per direction.
func (HPP) Bits() int { return 4 }

// Inputs lists the particles arriving at cell in direction order: the
// east-moving particle of the west neighbor, the north-moving particle of the
// south neighbor, and so on.
func (h HPP) Inputs(cell int) []automaton.Bit { return h.geometry().arrivals(cell) }

// Next collides the arriving particles, or reflects them at a wall cell.
func (h HPP) Next(cell int, in, out []bool) {
	if g := h.geometry(); g.wall(cell) {
		g.bounce(in, out)
		return
	}
	collided := collideHPP([4]bool(in))
	copy(out, collided[:])
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

// Mass counts every particle.
func (h HPP) Mass() automaton.Quantity { return h.geometry().mass() }

// MomentumX sums east-moving minus west-moving particles.
func (h HPP) MomentumX() automaton.Quantity {
	return weighted(h.geometry().all(), []int{East: 1, West: -1, North: 0, South: 0})
}

// MomentumY sums north-moving minus south-moving particles.
func (h HPP) MomentumY() automaton.Quantity {
	return weighted(h.geometry().all(), []int{North: 1, South: -1, East: 0, West: 0})
}

// RowMomentumX is the x momentum of row y alone. Without walls, HPP conserves
// it separately for every row, a known spurious invariant of the square
// lattice. An out-of-range row yields a quantity that Compile rejects.
func (h HPP) RowMomentumX(y int) automaton.Quantity {
	if y < 0 || y >= h.Height {
		return invalidQuantity()
	}
	cells := make([]int, h.Width)
	for x := range cells {
		cells[x] = y*h.Width + x
	}
	return weighted(cells, []int{East: 1, West: -1, North: 0, South: 0})
}

// ColumnMomentumY is the y momentum of column x alone, conserved separately
// for every column without walls. An out-of-range column yields a quantity
// that Compile rejects.
func (h HPP) ColumnMomentumY(x int) automaton.Quantity {
	if x < 0 || x >= h.Width {
		return invalidQuantity()
	}
	cells := make([]int, h.Height)
	for y := range cells {
		cells[y] = y*h.Width + x
	}
	return weighted(cells, []int{North: 1, South: -1, East: 0, West: 0})
}

// Density returns the mean number of particles per cell in each block×block
// region, averaged over every given state. Rows of the result are y.
func (h HPP) Density(states []automaton.State, block int) ([][]float64, error) {
	return h.geometry().density(states, block)
}

// Velocity returns the mean momentum per particle (ux, uy) in each block×block
// region over every given state. Regions without particles have zero velocity.
func (h HPP) Velocity(states []automaton.State, block int) (ux, uy [][]float64, err error) {
	return h.geometry().meanVelocity(states, block)
}
