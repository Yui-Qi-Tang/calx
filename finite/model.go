// Package finite encodes explicit categorical transition models as Boolean CNF.
// It does not infer actions from evidence, enumerate paths, authorize execution,
// or replace independent replay against the consumer's original rules.
package finite

import (
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/Yui-Qi-Tang/calx/sat"
)

// Profile names the query semantics this package encodes: exact-length,
// simple, first-hit action sequences. Every digest records it, so a later
// change of semantics also changes the digest.
const Profile = "categorical-simple-first-hit/v1"

// ErrInput marks a malformed model or query, not a logical UNSAT result.
var ErrInput = errors.New("invalid finite input")

// ErrEncodingLimit marks an operational encoding budget, not unreachability.
var ErrEncodingLimit = sat.ErrLimit

// State assigns categorical values to field names. Values are opaque strings.
type State map[string]string

// Action has conjunctive guards, constant effects and explicit unchanged fields.
// Every model field must occur exactly once in either Effect or Frame.
type Action struct {
	ID     string
	Guard  State
	Effect State
	Frame  []string
}

// Model declares finite categorical domains and permitted transitions.
type Model struct {
	Fields    map[string][]string
	Forbidden []State
	Actions   []Action
}

// Query binds the exact semantics of one CNF. Horizon is an exact action count,
// including zero. Blocked excludes complete action sequences of that length.
type Query struct {
	Start   State
	Goal    State
	Horizon int
	Blocked [][]string
}

// v1 support limits: models have at most 16 fields, 64 values per field, 1024
// actions and 256 forbidden patterns; horizons are 0..4095; names are at most
// 200 bytes. The number of Cartesian states is not limited: encoding never
// enumerates states, and the encoding budget bounds its size. maxActions still
// has to be finite, because Project allocates dense action-by-coordinate
// matrices before the budget is checked. Limits are not query semantics, so
// raising them keeps Profile while accepted models keep their encoding.
const (
	maxFields    = 16
	maxValues    = 64
	maxActions   = 1024
	maxForbidden = 256
	maxHorizon   = 4095
	maxNameBytes = 200
)

// Option changes the encoding budget of Compile and reports an invalid value.
// Exceeding the budget returns ErrEncodingLimit.
type Option func(*budget) error

// WithMaxVariables caps the number of CNF variables (default 100,000). n must
// be at least 1.
func WithMaxVariables(n int) Option {
	return func(b *budget) error {
		if n < 1 {
			return fmt.Errorf("%w: max variables %d, want at least 1", ErrInput, n)
		}
		b.variables = n
		return nil
	}
}

// WithMaxClauses caps the number of CNF clauses (default 1,000,000). n must be
// at least 1.
func WithMaxClauses(n int) Option {
	return func(b *budget) error {
		if n < 1 {
			return fmt.Errorf("%w: max clauses %d, want at least 1", ErrInput, n)
		}
		b.clauses = n
		return nil
	}
}

// budget bounds encoding work. Its defaults equal the sat.Limits defaults.
type budget struct {
	variables, clauses int
}

func newBudget(opts []Option) (budget, error) {
	b := budget{variables: 100_000, clauses: 1_000_000}
	for _, opt := range opts {
		if err := opt(&b); err != nil {
			return budget{}, err
		}
	}
	return b, nil
}
func partial(m Model, s State) bool {
	for f, v := range s {
		if !slices.Contains(m.Fields[f], v) {
			return false
		}
	}
	return true
}
func matches(s, p State) bool {
	for f, v := range p {
		if s[f] != v {
			return false
		}
	}
	return true
}
func valid(m Model, s State) bool {
	if len(s) != len(m.Fields) || !partial(m, s) {
		return false
	}
	for _, p := range m.Forbidden {
		if matches(s, p) {
			return false
		}
	}
	return true
}

// Validate checks the supported model language independently of any authority.
func (m Model) Validate() error {
	if len(m.Fields) == 0 || len(m.Fields) > maxFields || len(m.Actions) == 0 || len(m.Actions) > maxActions || len(m.Forbidden) > maxForbidden {
		return fmt.Errorf("%w: model bounds", ErrInput)
	}
	for f, vs := range m.Fields {
		if f == "" || !utf8.ValidString(f) || len(f) > maxNameBytes || len(vs) == 0 || len(vs) > maxValues {
			return ErrInput
		}
		seen := map[string]bool{}
		for _, v := range vs {
			if v == "" || !utf8.ValidString(v) || len(v) > maxNameBytes || seen[v] {
				return ErrInput
			}
			seen[v] = true
		}
	}
	for _, p := range m.Forbidden {
		if len(p) == 0 || !partial(m, p) {
			return fmt.Errorf("%w: forbidden pattern", ErrInput)
		}
	}
	ids := map[string]bool{}
	for _, a := range m.Actions {
		if a.ID == "" || !utf8.ValidString(a.ID) || len(a.ID) > maxNameBytes || ids[a.ID] || !partial(m, a.Guard) || len(a.Effect) == 0 || !partial(m, a.Effect) {
			return fmt.Errorf("%w: action", ErrInput)
		}
		ids[a.ID] = true
		covered := map[string]bool{}
		for f := range a.Effect {
			covered[f] = true
		}
		for _, f := range a.Frame {
			if _, ok := m.Fields[f]; !ok || covered[f] {
				return fmt.Errorf("%w: frame partition", ErrInput)
			}
			covered[f] = true
		}
		if len(covered) != len(m.Fields) {
			return fmt.Errorf("%w: incomplete frame", ErrInput)
		}
	}
	return nil
}
func (q Query) validate(m Model, o budget) error {
	if q.Horizon < 0 || q.Horizon > maxHorizon || !valid(m, q.Start) || len(q.Goal) == 0 || !partial(m, q.Goal) {
		return fmt.Errorf("%w: query", ErrInput)
	}
	if len(q.Blocked) > o.clauses {
		return ErrEncodingLimit
	}
	ids := map[string]bool{}
	for _, a := range m.Actions {
		ids[a.ID] = true
	}
	for _, seq := range q.Blocked {
		if len(seq) != q.Horizon {
			return fmt.Errorf("%w: blocked length", ErrInput)
		}
		for _, id := range seq {
			if !ids[id] {
				return fmt.Errorf("%w: blocked action", ErrInput)
			}
		}
	}
	return nil
}

// Trace is one decoded state/action sequence, not an execution receipt.
type Trace struct {
	States  []State
	Actions []string
}

// IsZero reports whether t has no states and no actions. A decoded trace is
// never zero: it holds at least the start state.
func (t Trace) IsZero() bool { return len(t.States) == 0 && len(t.Actions) == 0 }
