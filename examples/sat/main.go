// Command sat demonstrates bounded DIMACS and total-assignment checking.
package main

import (
	"bytes"
	"encoding/json"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/sat"
)

func main() {
	// x AND (NOT x OR y) is satisfied by x=true, y=true.
	formula := sat.CNF{
		Variables: []sat.Variable{{ID: 1}, {ID: 2}},
		Clauses:   [][]int{{1}, {-1, 2}},
	}
	limits, err := (sat.Limits{
		MaxVariables: 2,
		MaxClauses:   2,
		MaxLiterals:  3,
	}).WithDefaults()
	if err != nil {
		log.Fatal(err)
	}
	if err := limits.CheckCNF(formula); err != nil {
		log.Fatal(err)
	}
	var dimacs bytes.Buffer
	if err := formula.WriteDIMACS(&dimacs); err != nil {
		log.Fatal(err)
	}
	parsed, err := sat.ReadDIMACS(bytes.NewReader(dimacs.Bytes()), limits)
	if err != nil {
		log.Fatal(err)
	}
	// This witness is supplied explicitly; no solver is started.
	assignment := map[int]bool{1: true, 2: true}
	if err := parsed.CheckAssignment(assignment); err != nil {
		log.Fatal(err)
	}
	output := json.NewEncoder(os.Stdout)
	output.SetIndent("", "  ")
	if err := output.Encode(struct {
		DIMACS            string       `json:"dimacs"`
		Assignment        map[int]bool `json:"assignment"`
		AssignmentChecked bool         `json:"assignment_checked"`
	}{DIMACS: dimacs.String(), Assignment: assignment, AssignmentChecked: true}); err != nil {
		log.Fatal(err)
	}
}
