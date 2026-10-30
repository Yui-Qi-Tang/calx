package lga

import (
	"fmt"

	"github.com/Yui-Qi-Tang/calx/automaton"
)

// plane holds one bit per cell, row by row, in 64-bit words. Bits past the
// row width in a row's last word are always zero.
type plane struct {
	width, height, words int
	bits                 []uint64
}

func newPlane(width, height int) plane {
	words := (width + 63) / 64
	return plane{width: width, height: height, words: words, bits: make([]uint64, words*height)}
}

func (p plane) row(y int) []uint64 { return p.bits[y*p.words : (y+1)*p.words] }

func (p plane) get(x, y int) bool { return p.row(y)[x/64]>>(x%64)&1 == 1 }

func (p plane) set(x, y int) { p.row(y)[x/64] |= 1 << (x % 64) }

// shiftInto writes dst(x, y) = p(x-dx, y-dy) on the torus, for dx and dy in
// {-1, 0, 1}.
func (p plane) shiftInto(dst plane, dx, dy int) {
	for y := range p.height {
		src := p.row(((y-dy)%p.height + p.height) % p.height)
		out := dst.row(y)
		switch dx {
		case 0:
			copy(out, src)
		case 1:
			rotateUp(out, src, p.width)
		case -1:
			rotateDown(out, src, p.width)
		}
	}
}

// rotateUp sets bit x of dst to bit x-1 of src, wrapping bit width-1 to bit 0.
func rotateUp(dst, src []uint64, width int) {
	var carry uint64
	for i, w := range src {
		dst[i] = w<<1 | carry
		carry = w >> 63
	}
	last := len(dst) - 1
	dst[0] |= src[(width-1)/64] >> ((width - 1) % 64) & 1
	if width%64 != 0 {
		dst[last] &= 1<<(width%64) - 1
	}
}

// rotateDown sets bit x of dst to bit x+1 of src, wrapping bit 0 to bit width-1.
func rotateDown(dst, src []uint64, width int) {
	for i, w := range src {
		dst[i] = w >> 1
		if i+1 < len(src) {
			dst[i] |= src[i+1] << 63
		}
	}
	dst[(width-1)/64] |= (src[0] & 1) << ((width - 1) % 64)
}

// packed is a whole lattice state as one plane per direction, plus planes for
// wall cells and per-step choice bits.
type packed struct {
	g      geometry
	dirs   []plane
	next   []plane
	walls  plane
	choice plane
}

func pack(g geometry, s automaton.State) (*packed, error) {
	if err := g.validate(); err != nil {
		return nil, err
	}
	if len(s) != g.cells()*g.bits {
		return nil, fmt.Errorf("%w: state size", ErrInput)
	}
	p := &packed{g: g, walls: newPlane(g.width, g.height), choice: newPlane(g.width, g.height)}
	for range g.bits {
		p.dirs = append(p.dirs, newPlane(g.width, g.height))
		p.next = append(p.next, newPlane(g.width, g.height))
	}
	for c := range g.cells() {
		x, y := c%g.width, c/g.width
		if g.wall(c) {
			p.walls.set(x, y)
		}
		for d := range g.bits {
			if s[c*g.bits+d] {
				p.dirs[d].set(x, y)
			}
		}
	}
	return p, nil
}

func (p *packed) state() automaton.State {
	g := p.g
	s := make(automaton.State, g.cells()*g.bits)
	for c := range g.cells() {
		x, y := c%g.width, c/g.width
		for d := range g.bits {
			s[c*g.bits+d] = p.dirs[d].get(x, y)
		}
	}
	return s
}

// stream moves every particle one step, leaving the arrivals in p.next.
func (p *packed) stream() {
	for d, s := range p.g.steps {
		p.dirs[d].shiftInto(p.next[d], s[0], s[1])
	}
}

// bounceWalls replaces the collided values at wall cells with the reversed
// arrivals, then makes the result current.
func (p *packed) bounceWalls(arrived [][]uint64, k int, collided []uint64) {
	wall := p.walls.bits[k]
	half := len(collided) / 2
	for d := range collided {
		reflected := arrived[(d+half)%len(collided)][k]
		p.dirs[d].bits[k] = collided[d]&^wall | reflected&wall
	}
}

// Run advances an HPP state by steps with a bit-packed implementation and
// returns the final state. It gives the same result as automaton.Simulate on
// h, typically much faster, and keeps only the current state in memory.
func (h HPP) Run(start automaton.State, steps int) (automaton.State, error) {
	p, err := pack(h.geometry(), start)
	if err != nil {
		return nil, err
	}
	if steps < 0 {
		return nil, fmt.Errorf("%w: negative steps", ErrInput)
	}
	arrived := make([][]uint64, 4)
	collided := make([]uint64, 4)
	for range steps {
		p.stream()
		for d := range arrived {
			arrived[d] = p.next[d].bits
		}
		for k := range p.walls.bits {
			e, n, w, s := arrived[East][k], arrived[North][k], arrived[West][k], arrived[South][k]
			// An exact head-on pair flips all four bits: EW becomes NS and back.
			swap := e&w&^n&^s | n&s&^e&^w
			collided[East], collided[North], collided[West], collided[South] = e^swap, n^swap, w^swap, s^swap
			p.bounceWalls(arrived, k, collided)
		}
	}
	return p.state(), nil
}

// Run advances an FHP state by steps with a bit-packed implementation and
// returns the final state. choose supplies the choice bit of every fluid cell
// at every step, as Simulate's choose does; it is not called for wall cells.
// It gives the same result as automaton.Simulate with the same choices, and
// keeps only the current state in memory.
func (f FHP) Run(start automaton.State, steps int, choose func(step, cell int) bool) (automaton.State, error) {
	g := f.geometry()
	p, err := pack(g, start)
	if err != nil {
		return nil, err
	}
	if steps < 0 || choose == nil {
		return nil, fmt.Errorf("%w: negative steps or nil choose", ErrInput)
	}
	arrived := make([][]uint64, 6)
	collided := make([]uint64, 6)
	for step := range steps {
		clear(p.choice.bits)
		for c := range g.cells() {
			if !g.wall(c) && choose(step, c) {
				p.choice.set(c%g.width, c/g.width)
			}
		}
		p.stream()
		for d := range arrived {
			arrived[d] = p.next[d].bits
		}
		for k := range p.walls.bits {
			var n [6]uint64
			for d := range n {
				n[d] = arrived[d][k]
			}
			ccw := p.choice.bits[k]
			copy(collided, n[:])
			// Each collision flips exactly its input and output bits.
			for i := range 3 {
				others := ^uint64(0)
				for d := range 6 {
					if d != i && d != i+3 {
						others &^= n[d]
					}
				}
				pair := n[i] & n[i+3] & others
				for _, d := range []int{i, i + 3, (i + 1) % 6, (i + 4) % 6} {
					collided[d] ^= pair & ccw
				}
				for _, d := range []int{i, i + 3, (i + 5) % 6, (i + 2) % 6} {
					collided[d] ^= pair &^ ccw
				}
			}
			even := n[HexE] & n[HexNW] & n[HexSW] &^ (n[HexNE] | n[HexW] | n[HexSE])
			odd := n[HexNE] & n[HexW] & n[HexSE] &^ (n[HexE] | n[HexNW] | n[HexSW])
			for d := range collided {
				collided[d] ^= even | odd
			}
			p.bounceWalls(arrived, k, collided)
		}
	}
	return p.state(), nil
}
