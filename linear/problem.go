// Package linear defines a deliberately small QF_LIA profile: Boolean CNF and a
// conjunction of linear integer comparisons. It accepts no SMT-LIB source,
// quantifiers, arrays, nonlinear terms or mixed Boolean/arithmetic disjunctions.
// It validates, encodes and checks; linear/z3 runs a solver.
package linear

import (
	"errors"
	"fmt"
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/Yui-Qi-Tang/calx/sat"
)

// ErrUnsupported marks a language construct outside the supported profile.
var ErrUnsupported = errors.New("unsupported logic")

// Relation compares a linear sum with a constant integer.
type Relation string

const (
	// Equal requires equality.
	Equal Relation = "="
	// AtMost requires the sum to be at most the right-hand side.
	AtMost Relation = "<="
	// AtLeast requires the sum to be at least the right-hand side.
	AtLeast Relation = ">="
)

// Term is a constant integer coefficient times an integer variable ID.
type Term struct {
	Variable    int
	Coefficient string
}

// Constraint compares a sum of Terms to RHS. An empty sum is zero.
// Origins retains every contributing condition ID without interpreting it.
type Constraint struct {
	Terms    []Term
	Relation Relation
	RHS      string
	Origins  []string
}

// Problem keeps Boolean and integer ID spaces distinct and one-based. Variable
// metadata and origins add no constraints. The caller must not mutate a Problem
// while it is being solved.
type Problem struct {
	Booleans    sat.CNF
	Integers    []sat.Variable
	Constraints []Constraint
}

// Assignment is a total interpretation, with exact decimal integer values.
type Assignment struct {
	Booleans map[int]bool
	Integers map[int]string
}

var decimal = regexp.MustCompile(`^(0|-[1-9][0-9]*|[1-9][0-9]*)$`)

// Integer parses a canonical decimal integer without machine-size overflow.
func Integer(s string) (*big.Int, error) {
	if !decimal.MatchString(s) {
		return nil, fmt.Errorf("%w: noncanonical integer", sat.ErrInput)
	}
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		return nil, fmt.Errorf("%w: noncanonical integer", sat.ErrInput)
	}
	return n, nil
}

// CheckShape validates IDs, sorts and the supported operators.
func (p Problem) CheckShape() error {
	if err := p.Booleans.CheckShape(); err != nil {
		return err
	}
	for i, v := range p.Integers {
		if v.ID != i+1 {
			return fmt.Errorf("%w: integer IDs are not contiguous", sat.ErrInput)
		}
		for key, value := range v.Metadata {
			if !utf8.ValidString(key) || !utf8.ValidString(value) {
				return fmt.Errorf("%w: invalid UTF-8 metadata", sat.ErrInput)
			}
		}
	}
	for i, c := range p.Constraints {
		if c.Relation != Equal && c.Relation != AtMost && c.Relation != AtLeast {
			return fmt.Errorf("%w: comparison %q", ErrUnsupported, c.Relation)
		}
		if _, err := Integer(c.RHS); err != nil {
			return fmt.Errorf("constraint %d: %w", i, err)
		}
		for _, term := range c.Terms {
			if term.Variable < 1 || term.Variable > len(p.Integers) {
				return fmt.Errorf("%w: integer variable in constraint %d", sat.ErrInput, i)
			}
			if _, err := Integer(term.Coefficient); err != nil {
				return fmt.Errorf("constraint %d: %w", i, err)
			}
		}
		for _, id := range c.Origins {
			if id == "" || !utf8.ValidString(id) {
				return fmt.Errorf("%w: empty origin ID", sat.ErrInput)
			}
		}
	}
	return nil
}

// CheckLimits bounds dimensions and term counts before encoding. Variables,
// clauses plus constraints, and literals plus terms share the sat.Limits budgets.
func (p Problem) CheckLimits(l sat.Limits) error {
	if len(p.Booleans.Variables) > l.MaxVariables || len(p.Integers) > l.MaxVariables-len(p.Booleans.Variables) {
		return sat.ErrLimit
	}
	if len(p.Booleans.Clauses) > l.MaxClauses || len(p.Constraints) > l.MaxClauses-len(p.Booleans.Clauses) {
		return sat.ErrLimit
	}
	remaining := l.MaxLiterals
	for _, clause := range p.Booleans.Clauses {
		if len(clause) > remaining {
			return sat.ErrLimit
		}
		remaining -= len(clause)
	}
	for _, c := range p.Constraints {
		if len(c.Terms) > remaining {
			return sat.ErrLimit
		}
		remaining -= len(c.Terms)
	}
	return p.CheckShape()
}

// WriteSMTLIB writes exactly this profile, without check-sat or model commands.
// Boolean variable n is named bn and integer variable n is named in.
func (p Problem) WriteSMTLIB(w io.Writer) error {
	if err := p.CheckShape(); err != nil {
		return err
	}
	b := []byte("(set-logic QF_LIA)\n")
	for _, v := range p.Booleans.Variables {
		b = fmt.Appendf(b, "(declare-const b%d Bool)\n", v.ID)
	}
	for _, v := range p.Integers {
		b = fmt.Appendf(b, "(declare-const i%d Int)\n", v.ID)
	}
	for _, clause := range p.Booleans.Clauses {
		if len(clause) == 0 {
			b = append(b, "(assert false)\n"...)
			continue
		}
		b = append(b, "(assert (or"...)
		for _, lit := range clause {
			if lit < 0 {
				b = append(b, " (not b"...)
				b = strconv.AppendInt(b, int64(-lit), 10)
				b = append(b, ')')
			} else {
				b = append(b, " b"...)
				b = strconv.AppendInt(b, int64(lit), 10)
			}
		}
		b = append(b, " false))\n"...)
	}
	for _, c := range p.Constraints {
		b = fmt.Appendf(b, "(assert (%s (+ 0 0", c.Relation)
		for _, term := range c.Terms {
			b = fmt.Appendf(b, " (* %s i%d)", numeral(term.Coefficient), term.Variable)
		}
		b = fmt.Appendf(b, ") %s))\n", numeral(c.RHS))
	}
	_, err := w.Write(b)
	return err
}

// numeral renders a canonical integer: SMT-LIB numerals have no negative form.
func numeral(s string) string {
	if rest, negative := strings.CutPrefix(s, "-"); negative {
		return "(- " + rest + ")"
	}
	return s
}

// CheckAssignment checks all original constraints with exact integer arithmetic.
// This evaluates a witness; it cannot prove an UNSAT response.
func (p Problem) CheckAssignment(a Assignment) error {
	if err := p.CheckShape(); err != nil {
		return err
	}
	if err := p.Booleans.CheckAssignment(a.Booleans); err != nil {
		return err
	}
	if len(a.Integers) != len(p.Integers) {
		return errors.New("integer model is not total")
	}
	values := make(map[int]*big.Int, len(p.Integers))
	for _, v := range p.Integers {
		n, err := Integer(a.Integers[v.ID])
		if err != nil {
			return fmt.Errorf("integer variable %d: %w", v.ID, err)
		}
		values[v.ID] = n
	}
	for i, c := range p.Constraints {
		sum := new(big.Int)
		for _, term := range c.Terms {
			coefficient, err := Integer(term.Coefficient)
			if err != nil {
				return fmt.Errorf("constraint %d: %w", i, err)
			}
			sum.Add(sum, new(big.Int).Mul(coefficient, values[term.Variable]))
		}
		rhs, err := Integer(c.RHS)
		if err != nil {
			return fmt.Errorf("constraint %d: %w", i, err)
		}
		cmp := sum.Cmp(rhs)
		if (c.Relation == Equal && cmp != 0) || (c.Relation == AtMost && cmp > 0) || (c.Relation == AtLeast && cmp < 0) {
			return fmt.Errorf("integer model violates constraint %d", i)
		}
	}
	return nil
}
