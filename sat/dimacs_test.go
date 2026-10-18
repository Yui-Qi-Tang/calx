package sat

import (
	"errors"
	"strings"
	"testing"
)

func TestReadDIMACS(t *testing.T) {
	for _, raw := range []string{"c note\np cnf 2 2\n1\n-2 0\n0\n", "p cnf 0 0\n", "p cnf 0 1\n0\n"} {
		c, err := ReadDIMACS(strings.NewReader(raw), Limits{})
		if err != nil {
			t.Fatal(err)
		}
		var b strings.Builder
		if err = c.WriteDIMACS(&b); err != nil {
			t.Fatal(err)
		}
		if _, err = ReadDIMACS(strings.NewReader(b.String()), Limits{}); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{"", "1 0", "p cnf -1 0", "p cnf 1 1\n1", "p cnf 1 1\n2 0", "p cnf 1 0\n0", "p cnf 1 1\n0\np cnf 1 1"} {
		if _, e := ReadDIMACS(strings.NewReader(raw), Limits{}); !errors.Is(e, ErrInput) {
			t.Fatal(raw, e)
		}
	}
	if _, e := ReadDIMACS(strings.NewReader("p cnf 2 0"), Limits{MaxVariables: 1}); !errors.Is(e, ErrLimit) {
		t.Fatal(e)
	}
}
