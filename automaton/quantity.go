package automaton

import (
	"fmt"

	"github.com/Yui-Qi-Tang/calx/sat"
)

// Term is one signed contribution to a conserved quantity.
type Term struct {
	Bit    Bit
	Weight int // -1 or +1
}

// Quantity is a sum of signed bits, such as mass or one momentum component.
// Each bit appears at most once.
type Quantity []Term

func (s shape) checkQuantity(q Quantity) error {
	seen := make(map[Bit]bool, len(q))
	for _, t := range q {
		if !s.valid(t.Bit) || seen[t.Bit] || (t.Weight != 1 && t.Weight != -1) {
			return fmt.Errorf("%w: quantity term %v", ErrInput, t)
		}
		seen[t.Bit] = true
	}
	return nil
}

// violationSides returns the literals of L = P(0) + N(k) and R = P(k) + N(0),
// where P and N count the true bits with weight +1 and -1. The quantity
// P - N differs between times 0 and k exactly when the counts L and R differ.
func (l layout) violationSides(q Quantity) (left, right []int) {
	for _, t := range q {
		start, end := l.stateVar(0, t.Bit), l.stateVar(l.steps, t.Bit)
		if t.Weight < 0 {
			start, end = end, start
		}
		left = append(left, start)
		right = append(right, end)
	}
	return left, right
}

// builder appends auxiliary variables and clauses after the fixed layout.
type builder struct {
	cnf  *sat.CNF
	next int // next unused variable ID
}

func (b *builder) newVar() int {
	id := b.next
	b.next++
	b.cnf.Variables = append(b.cnf.Variables, sat.Variable{ID: id})
	return id
}

func (b *builder) add(clause ...int) { b.cnf.Clauses = append(b.cnf.Clauses, clause) }

// totalizer returns outputs where output j is true exactly when at least j+1
// inputs are true. Both directions are encoded, so every output is determined
// by the inputs.
func (b *builder) totalizer(inputs []int) []int {
	if len(inputs) <= 1 {
		return inputs
	}
	left := b.totalizer(inputs[:len(inputs)/2])
	right := b.totalizer(inputs[len(inputs)/2:])
	out := make([]int, len(left)+len(right))
	for i := range out {
		out[i] = b.newVar()
	}
	// i and j are child counts; count zero is always reached and has no literal.
	for i := 0; i <= len(left); i++ {
		for j := 0; j <= len(right); j++ {
			if i+j > 0 { // left >= i and right >= j imply out >= i+j
				clause := []int{out[i+j-1]}
				if i > 0 {
					clause = append(clause, -left[i-1])
				}
				if j > 0 {
					clause = append(clause, -right[j-1])
				}
				b.add(clause...)
			}
			if i+j < len(out) { // left <= i and right <= j imply out <= i+j
				clause := []int{-out[i+j]}
				if i < len(left) {
					clause = append(clause, left[i])
				}
				if j < len(right) {
					clause = append(clause, right[j])
				}
				b.add(clause...)
			}
		}
	}
	return out
}

// differ requires the counts of true literals in left and right to differ.
// Both totalizers are exact, so some output position must disagree.
func (b *builder) differ(left, right []int) {
	l, r := b.totalizer(left), b.totalizer(right)
	disagree := make([]int, len(l))
	for j := range l {
		d := b.newVar()
		b.add(-d, l[j], r[j])
		b.add(-d, -l[j], -r[j])
		disagree[j] = d
	}
	b.add(disagree...)
}

// totalizerSize counts what totalizer adds for m inputs, without building it.
func totalizerSize(m int64) (variables, clauses, literals int64) {
	if m <= 1 {
		return 0, 0, 0
	}
	p, q := m/2, m-m/2
	v1, c1, l1 := totalizerSize(p)
	v2, c2, l2 := totalizerSize(q)
	pairs := (p + 1) * (q + 1)
	// Each direction has pairs-1 clauses; summed literal counts are symmetric.
	return v1 + v2 + m, c1 + c2 + 2*(pairs-1), l1 + l2 + 2*(pairs+p*(q+1)+q*(p+1)-1)
}

// violationSize counts what differ adds for a quantity of m terms.
func violationSize(m int) (variables, clauses, literals int64) {
	if m == 0 {
		return 0, 0, 0
	}
	n := int64(m)
	v, c, l := totalizerSize(n)
	return 2*v + n, 2*c + 2*n + 1, 2*l + 6*n + n
}
