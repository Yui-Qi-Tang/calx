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

// Profile identifies exact-length, simple, first-hit, action-sequence queries.
const Profile = "categorical-simple-first-hit/v1"

// ErrInput marks a malformed model or query, not a logical UNSAT result.
var ErrInput = errors.New("invalid finite input")

// ErrUnsupported marks an unsupported semantic profile.
var ErrUnsupported = errors.New("unsupported finite profile")

// ErrEncodingLimit marks an operational encoding budget, not unreachability.
var ErrEncodingLimit = sat.ErrLimit

// State assigns categorical values to field names. Values are opaque strings.
type State map[string]string

// Action has conjunctive guards, constant effects and explicit unchanged fields.
// Every model field must occur exactly once in either Effect or Frame.
type Action struct {
	ID     string   `json:"id"`
	Guard  State    `json:"guard"`
	Effect State    `json:"effect"`
	Frame  []string `json:"frame"`
}

// Model declares finite categorical domains and permitted transitions.
type Model struct {
	Fields    map[string][]string `json:"fields"`
	Forbidden []State             `json:"forbidden"`
	Actions   []Action            `json:"actions"`
}

// Query binds the exact semantics of one CNF. Horizon is an exact action count,
// including zero. Blocked excludes complete action sequences of that length.
type Query struct {
	Profile string     `json:"profile"`
	Start   State      `json:"start"`
	Goal    State      `json:"goal"`
	Horizon int        `json:"horizon"`
	Blocked [][]string `json:"blocked"`
}

// Options bounds encoding work. Zero fields use SAT defaults. Input models are
// capped at 16 fields, 64 values/field, 64 actions, 256 forbidden patterns and
// 4096 Cartesian states; horizons are 0..4095. These are v1 support limits.
type Options struct {
	MaxVariables int `json:"max_variables"`
	MaxClauses   int `json:"max_clauses"`
}

func (o Options) defaults() (Options, error) {
	l, err := (sat.Limits{MaxVariables: o.MaxVariables, MaxClauses: o.MaxClauses}).WithDefaults()
	return Options{l.MaxVariables, l.MaxClauses}, err
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
	if len(m.Fields) == 0 || len(m.Fields) > 16 || len(m.Actions) == 0 || len(m.Actions) > 64 || len(m.Forbidden) > 256 {
		return fmt.Errorf("%w: model bounds", ErrInput)
	}
	size := 1
	for f, vs := range m.Fields {
		if f == "" || !utf8.ValidString(f) || len(f) > 200 || len(vs) == 0 || len(vs) > 64 {
			return ErrInput
		}
		seen := map[string]bool{}
		for _, v := range vs {
			if v == "" || !utf8.ValidString(v) || len(v) > 200 || seen[v] {
				return ErrInput
			}
			seen[v] = true
		}
		size *= len(vs)
		if size > 4096 {
			return fmt.Errorf("%w: domain size", ErrInput)
		}
	}
	for _, p := range m.Forbidden {
		if len(p) == 0 || !partial(m, p) {
			return fmt.Errorf("%w: forbidden pattern", ErrInput)
		}
	}
	ids := map[string]bool{}
	for _, a := range m.Actions {
		if a.ID == "" || !utf8.ValidString(a.ID) || len(a.ID) > 200 || ids[a.ID] || !partial(m, a.Guard) || len(a.Effect) == 0 || !partial(m, a.Effect) {
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
func (q Query) validate(m Model, o Options) error {
	if q.Profile != Profile {
		return ErrUnsupported
	}
	if q.Horizon < 0 || q.Horizon >= 4096 || !valid(m, q.Start) || len(q.Goal) == 0 || !partial(m, q.Goal) {
		return fmt.Errorf("%w: query", ErrInput)
	}
	if len(q.Blocked) > o.MaxClauses {
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
	States  []State  `json:"states"`
	Actions []string `json:"actions"`
}
