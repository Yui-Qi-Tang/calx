package sat

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ReadDIMACS parses bounded DIMACS CNF. Comments and whitespace carry no logical
// force. The caller retains original bytes if their exact digest matters; the
// runner's FormulaDigest identifies its own emitted DIMACS representation.
func ReadDIMACS(r io.Reader, limits Limits) (CNF, error) {
	l, err := limits.WithDefaults()
	if err != nil {
		return CNF{}, err
	}
	raw, err := io.ReadAll(io.LimitReader(r, int64(l.MaxInputBytes)+1))
	if err != nil {
		return CNF{}, err
	}
	if len(raw) > l.MaxInputBytes {
		return CNF{}, ErrLimit
	}
	scan := bufio.NewScanner(strings.NewReader(string(raw)))
	scan.Buffer(make([]byte, 4096), l.MaxInputBytes)
	c := CNF{}
	header := false
	want := 0
	row := []int{}
	literals := 0
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if line == "" || strings.HasPrefix(line, "c") {
			continue
		}
		fields := strings.Fields(line)
		if fields[0] == "p" {
			if header || len(fields) != 4 || fields[1] != "cnf" {
				return CNF{}, ErrInput
			}
			n, e1 := strconv.Atoi(fields[2])
			k, e2 := strconv.Atoi(fields[3])
			if e1 != nil || e2 != nil || n < 0 || k < 0 {
				return CNF{}, ErrInput
			}
			if n > l.MaxVariables || k > l.MaxClauses {
				return CNF{}, ErrLimit
			}
			c.Variables = make([]Variable, n)
			for i := range n {
				c.Variables[i].ID = i + 1
			}
			c.Clauses = make([][]int, 0, k)
			want = k
			header = true
			continue
		}
		if !header {
			return CNF{}, ErrInput
		}
		for _, field := range fields {
			v, err := strconv.Atoi(field)
			if err != nil {
				return CNF{}, ErrInput
			}
			if v == 0 {
				if len(c.Clauses) >= want {
					return CNF{}, ErrInput
				}
				c.Clauses = append(c.Clauses, row)
				row = []int{}
				continue
			}
			if v > len(c.Variables) || v < -len(c.Variables) {
				return CNF{}, ErrInput
			}
			literals++
			if literals > l.MaxLiterals {
				return CNF{}, ErrLimit
			}
			row = append(row, v)
		}
	}
	if err := scan.Err(); err != nil {
		return CNF{}, fmt.Errorf("%w: DIMACS line", ErrInput)
	}
	if !header || len(row) > 0 || len(c.Clauses) != want {
		return CNF{}, ErrInput
	}
	return c, c.CheckShape()
}
