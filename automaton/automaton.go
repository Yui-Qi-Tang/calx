// Package automaton encodes bounded runs of synchronous Boolean automata as
// CNF, simulates them forward with Simulate, and checks recorded runs with
// Replay. Callers supply the automaton through System; Calx sees only cell and
// bit indices and the local rule, never what a cell or bit means. It does not
// prove unbounded properties or replace validation of the rule itself.
package automaton

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Yui-Qi-Tang/calx/sat"
)

// Profile identifies exact-length synchronous runs encoded from per-cell rule
// tables.
const Profile = "synchronous-boolean/v1"

var (
	// ErrInput marks a malformed system, query or trajectory, not UNSAT.
	ErrInput = errors.New("invalid automaton input")
	// ErrEncodingLimit marks an operational encoding budget, not unreachability.
	ErrEncodingLimit = sat.ErrLimit
	// ErrReplay marks a trajectory that does not follow the system's rule.
	ErrReplay = errors.New("trajectory does not follow the system")
)

// v1 support limits; see Compile for the encoding budget.
const (
	maxCells = 1 << 20
	maxBits  = 16
	maxFanIn = 12
)

// Bit addresses one state bit of one cell.
type Bit struct {
	Cell  int
	Index int
}

// System is a synchronous Boolean automaton: all cells update at once, each
// from the previous state of its inputs. Methods are never called
// concurrently, and panics in them are not recovered.
type System interface {
	// Cells returns the number of cells, from 1 to 1,048,576.
	Cells() int
	// Bits returns the number of state bits per cell, from 1 to 16.
	Bits() int
	// Inputs returns the distinct bits that determine cell's next state. It
	// must return the same bits for a cell throughout one call into this
	// package. Inputs plus choice bits are at most 12 per cell.
	Inputs(cell int) []Bit
	// Next writes cell's next state to out, which has Bits() entries, all false
	// on entry. in holds the input values in Inputs order, followed by any
	// choice bits. Next must be deterministic; it may be called many times
	// and in any order.
	Next(cell int, in, out []bool)
}

// Chooser is an optional System extension for nondeterministic rules. Each
// choice bit is a free variable in the encoding.
type Chooser interface {
	// Choices returns the number of choice bits cell receives after its
	// inputs, at least 0. Inputs plus choices are at most 12 per cell.
	Choices(cell int) int
}

// State holds Cells()*Bits() values; bit (c, i) is at index c*Bits()+i.
type State []bool

// Pattern constrains some bits of one state and leaves the rest free.
type Pattern map[Bit]bool

// Trajectory is a run of Steps+1 states. Choices has one entry per step,
// holding every cell's choice bits in cell order; each entry is empty for
// systems without choices.
type Trajectory struct {
	States  []State
	Choices [][]bool
}

// IsZero reports whether t has no states and no choices. A decoded trajectory
// is never zero: it holds at least the start state.
func (t Trajectory) IsZero() bool { return len(t.States) == 0 && len(t.Choices) == 0 }

// shape is the validated structure of a System, read once per call.
type shape struct {
	cells, bits  int
	inputs       [][]Bit
	choices      []int
	offsets      []int // first choice index of each cell within one step
	totalChoices int
}

func readShape(sys System) (shape, error) {
	if sys == nil {
		return shape{}, fmt.Errorf("%w: nil system", ErrInput)
	}
	s := shape{cells: sys.Cells(), bits: sys.Bits()}
	if s.cells < 1 || s.bits < 1 || s.bits > maxBits {
		return shape{}, fmt.Errorf("%w: cell or bit count", ErrInput)
	}
	if s.cells > maxCells {
		return shape{}, fmt.Errorf("%w: cell count", ErrEncodingLimit)
	}
	chooser, hasChoices := sys.(Chooser)
	s.inputs = make([][]Bit, s.cells)
	s.choices = make([]int, s.cells)
	s.offsets = make([]int, s.cells)
	for c := range s.cells {
		inputs := slices.Clone(sys.Inputs(c))
		choices := 0
		if hasChoices {
			choices = chooser.Choices(c)
		}
		if choices < 0 || len(inputs)+choices > maxFanIn {
			return shape{}, fmt.Errorf("%w: cell %d inputs and choices", ErrInput, c)
		}
		seen := make(map[Bit]bool, len(inputs))
		for _, b := range inputs {
			if !s.valid(b) || seen[b] {
				return shape{}, fmt.Errorf("%w: cell %d input %v", ErrInput, c, b)
			}
			seen[b] = true
		}
		s.inputs[c], s.choices[c], s.offsets[c] = inputs, choices, s.totalChoices
		s.totalChoices += choices
	}
	return s, nil
}

func (s shape) valid(b Bit) bool {
	return b.Cell >= 0 && b.Cell < s.cells && b.Index >= 0 && b.Index < s.bits
}

func (s shape) index(b Bit) int { return b.Cell*s.bits + b.Index }

func (s shape) checkPattern(p Pattern) error {
	for b := range p {
		if !s.valid(b) {
			return fmt.Errorf("%w: pattern bit %v", ErrInput, b)
		}
	}
	return nil
}

// step applies the rule to every cell of cur. choice returns cell c's choice
// bits and is called only for cells that have them.
func (s shape) step(sys System, cur State, choice func(c int) ([]bool, error)) (State, error) {
	next := make(State, len(cur))
	for c := range s.cells {
		in := make([]bool, 0, len(s.inputs[c])+s.choices[c])
		for _, b := range s.inputs[c] {
			in = append(in, cur[s.index(b)])
		}
		if s.choices[c] > 0 {
			bits, err := choice(c)
			if err != nil {
				return nil, err
			}
			if len(bits) != s.choices[c] {
				return nil, fmt.Errorf("%w: cell %d choice count", ErrInput, c)
			}
			in = append(in, bits...)
		}
		start := c * s.bits
		sys.Next(c, in, next[start:start+s.bits:start+s.bits])
	}
	return next, nil
}

// Simulate runs sys forward from start for steps steps and returns all
// steps+1 states, so memory grows with steps. choose supplies each cell's
// choice bits; it may be nil when the system has none. A start of the wrong
// length, negative steps, or missing or wrongly sized choices return ErrInput. sys is validated as in Compile, so
// more than 1,048,576 cells return ErrEncodingLimit.
func Simulate(sys System, start State, steps int, choose func(step, cell int) []bool) ([]State, error) {
	s, err := readShape(sys)
	if err != nil {
		return nil, err
	}
	if steps < 0 || len(start) != s.cells*s.bits {
		return nil, fmt.Errorf("%w: start state or steps", ErrInput)
	}
	if s.totalChoices > 0 && choose == nil {
		return nil, fmt.Errorf("%w: choices required", ErrInput)
	}
	states := []State{slices.Clone(start)}
	for t := range steps {
		next, err := s.step(sys, states[t], func(c int) ([]bool, error) { return choose(t, c), nil })
		if err != nil {
			return nil, err
		}
		states = append(states, next)
	}
	return states, nil
}

// Replay checks that every step of t follows sys with the recorded choices.
// It uses the caller's rule again, so it checks an encoding or a decoded
// result, not whether the rule models anything real.
func Replay(sys System, t Trajectory) error {
	s, err := readShape(sys)
	if err != nil {
		return err
	}
	if len(t.States) == 0 || len(t.Choices) != len(t.States)-1 {
		return fmt.Errorf("%w: trajectory length", ErrInput)
	}
	for _, state := range t.States {
		if len(state) != s.cells*s.bits {
			return fmt.Errorf("%w: state size", ErrInput)
		}
	}
	for _, choices := range t.Choices {
		if len(choices) != s.totalChoices {
			return fmt.Errorf("%w: choice count", ErrInput)
		}
	}
	for i := range len(t.Choices) {
		next, err := s.step(sys, t.States[i], func(c int) ([]bool, error) {
			return t.Choices[i][s.offsets[c] : s.offsets[c]+s.choices[c]], nil
		})
		if err != nil {
			return err
		}
		if !slices.Equal(next, t.States[i+1]) {
			return fmt.Errorf("%w: step %d", ErrReplay, i)
		}
	}
	return nil
}
