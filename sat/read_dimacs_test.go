package sat_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/Yui-Qi-Tang/calx/sat"
)

// referenceReadDIMACS is the original line-and-string ReadDIMACS, kept as the
// behavioral reference for the current version, which reads lines as bytes and
// shares one literal slice across clauses.
func referenceReadDIMACS(r io.Reader, limits sat.Limits) (sat.CNF, error) {
	l, err := limits.WithDefaults()
	if err != nil {
		return sat.CNF{}, err
	}
	raw, err := io.ReadAll(io.LimitReader(r, int64(l.MaxInputBytes)+1))
	if err != nil {
		return sat.CNF{}, err
	}
	if len(raw) > l.MaxInputBytes {
		return sat.CNF{}, sat.ErrLimit
	}
	scan := bufio.NewScanner(bytes.NewReader(raw))
	scan.Buffer(make([]byte, 4096), l.MaxInputBytes)
	c := sat.CNF{}
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
				return sat.CNF{}, sat.ErrInput
			}
			n, e1 := strconv.Atoi(fields[2])
			k, e2 := strconv.Atoi(fields[3])
			if e1 != nil || e2 != nil || n < 0 || k < 0 {
				return sat.CNF{}, sat.ErrInput
			}
			if n > l.MaxVariables || k > l.MaxClauses {
				return sat.CNF{}, sat.ErrLimit
			}
			c.Variables = make([]sat.Variable, n)
			for i := range n {
				c.Variables[i].ID = i + 1
			}
			c.Clauses = make([][]int, 0, k)
			want = k
			header = true
			continue
		}
		if !header {
			return sat.CNF{}, sat.ErrInput
		}
		for _, field := range fields {
			v, err := strconv.Atoi(field)
			if err != nil {
				return sat.CNF{}, sat.ErrInput
			}
			if v == 0 {
				if len(c.Clauses) >= want {
					return sat.CNF{}, sat.ErrInput
				}
				c.Clauses = append(c.Clauses, row)
				row = []int{}
				continue
			}
			if v > len(c.Variables) || v < -len(c.Variables) {
				return sat.CNF{}, sat.ErrInput
			}
			literals++
			if literals > l.MaxLiterals {
				return sat.CNF{}, sat.ErrLimit
			}
			row = append(row, v)
		}
	}
	if err := scan.Err(); err != nil {
		return sat.CNF{}, fmt.Errorf("%w: DIMACS line", sat.ErrInput)
	}
	if !header || len(row) > 0 || len(c.Clauses) != want {
		return sat.CNF{}, sat.ErrInput
	}
	return c, c.CheckShape()
}

func TestReadDIMACSMatchesReference(t *testing.T) {
	var hpp bytes.Buffer
	if err := hppCNF(t).WriteDIMACS(&hpp); err != nil {
		t.Fatal(err)
	}
	inputs := []string{
		hpp.String(),
		// Accepted.
		"c note\np cnf 2 2\n1\n-2 0\n0\n", "p cnf 0 0\n", "p cnf 0 1\n0\n", "p cnf 1 2\n0 1 0\n",
		"p\tcnf 1 1\n1 0\n", "p cnf 1 1\r\n+1 0\r\n", "  c x\n p cnf 2 1 \n 1  -2 0 \n",
		"p cnf 2 1\n1 -0\n", "p cnf 1 1\n1 0\n", "p cnf 1 1\n1 0\n", "p cnf 1 2\n1 0 -1 0\n",
		"p cnf 2 1\n1\n\n-2\n0", "cp cnf 1 1\np cnf 1 1\n1 0\n", "p cnf 01 1\n1 0\n",
		// Rejected.
		"", "1 0", "p cnf -1 0", "p cnf 1 1\n1", "p cnf 1 1\n2 0", "p cnf 1 0\n0", "p cnf 1 1\n0\np cnf 1 1",
		"pcnf 1 1\n1 0\n", "px cnf 1 1\n1 0\n", "p cnf 2 1\n1 x 0\n", "p cnf 1 1 extra\n1 0\n", "p dnf 1 1\n1 0\n", "p cnf 1 1\n-2 0\n",
		"p cnf 1 1\n1 0\np cnf 1 1\n", "p cnf x 1\n1 0\n", "p cnf 1 1\n1 0 1\n", "p cnf 1 1\n99999999999999999999 0\n",
		"p\n", "p cnf 1\n",
	}
	limits := []sat.Limits{
		{MaxVariables: 1 << 20, MaxClauses: 1 << 20, MaxLiterals: 1 << 22, MaxInputBytes: 1 << 24},
		{MaxLiterals: 1}, {MaxClauses: 1}, {MaxVariables: 1}, {MaxInputBytes: 12},
	}
	for _, l := range limits {
		for _, in := range inputs {
			want, wantErr := referenceReadDIMACS(strings.NewReader(in), l)
			got, err := sat.ReadDIMACS(strings.NewReader(in), l)
			name := in[:min(len(in), 24)]
			if (err == nil) != (wantErr == nil) || err != nil && err.Error() != wantErr.Error() {
				t.Errorf("%q %+v: error %v, want %v", name, l, err, wantErr)
				continue
			}
			// DeepEqual distinguishes nil from empty clauses, which JSON
			// output encodes differently.
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%q %+v: CNF differs from the reference", name, l)
			}
		}
	}
}

func TestReadDIMACSClausesDoNotAlias(t *testing.T) {
	c, err := sat.ReadDIMACS(strings.NewReader("p cnf 2 2\n1 0\n2 0\n"), sat.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	_ = append(c.Clauses[0], -1)
	if !reflect.DeepEqual(c.Clauses[1], []int{2}) {
		t.Fatalf("append to clause 0 changed clause 1: %v", c.Clauses[1])
	}
}

func BenchmarkReadDIMACS(b *testing.B) {
	var raw bytes.Buffer
	if err := hppCNF(b).WriteDIMACS(&raw); err != nil {
		b.Fatal(err)
	}
	limits := sat.Limits{MaxVariables: 1 << 20, MaxClauses: 1 << 20, MaxLiterals: 1 << 22, MaxInputBytes: 1 << 24}
	for name, read := range map[string]func(io.Reader, sat.Limits) (sat.CNF, error){
		"current": sat.ReadDIMACS, "reference": referenceReadDIMACS,
	} {
		b.Run(name, func(b *testing.B) {
			for b.Loop() {
				if _, err := read(bytes.NewReader(raw.Bytes()), limits); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
