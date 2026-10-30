package sat

import (
	"bufio"
	"bytes"
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
	scan := bufio.NewScanner(bytes.NewReader(raw))
	scan.Buffer(make([]byte, 4096), l.MaxInputBytes)
	// Lines are read as bytes and literals go into one shared backing slice:
	// per-line strings and per-clause slices were most of the parse time.
	c := CNF{}
	header := false
	want := 0
	lits := make([]int, 0, 64) // non-nil, so an empty clause stays non-nil
	start := 0                 // first literal of the clause being read
	literals := 0
	for scan.Scan() {
		line := bytes.TrimSpace(scan.Bytes())
		if len(line) == 0 || line[0] == 'c' {
			continue
		}
		if isProblemLine(line) {
			fields := strings.Fields(string(line))
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
		for field := range bytes.FieldsSeq(line) {
			v, err := strconv.Atoi(string(field))
			if err != nil {
				return CNF{}, ErrInput
			}
			if v == 0 {
				if len(c.Clauses) >= want {
					return CNF{}, ErrInput
				}
				// The full slice expression keeps an append to one clause from
				// overwriting the next.
				c.Clauses = append(c.Clauses, lits[start:len(lits):len(lits)])
				start = len(lits)
				continue
			}
			if v > len(c.Variables) || v < -len(c.Variables) {
				return CNF{}, ErrInput
			}
			literals++
			if literals > l.MaxLiterals {
				return CNF{}, ErrLimit
			}
			lits = append(lits, v)
		}
	}
	if err := scan.Err(); err != nil {
		return CNF{}, fmt.Errorf("%w: DIMACS line", ErrInput)
	}
	if !header || len(lits) > start || len(c.Clauses) != want {
		return CNF{}, ErrInput
	}
	return c, c.CheckShape()
}

// isProblemLine reports whether the first field of line, as strings.Fields
// would split it, is "p".
func isProblemLine(line []byte) bool {
	for field := range bytes.FieldsSeq(line) {
		return string(field) == "p"
	}
	return false
}
