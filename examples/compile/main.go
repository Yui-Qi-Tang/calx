// Command compile demonstrates finite-model encoding without starting a solver.
package main

import (
	"context"
	"log"
	"os"

	"github.com/Yui-Qi-Tang/calx/finite"
)

func main() {
	// A single action changes state from off to on.
	model := finite.Model{
		Fields: map[string][]string{"state": {"off", "on"}},
		Actions: []finite.Action{{
			ID:     "enable",
			Guard:  finite.State{"state": "off"},
			Effect: finite.State{"state": "on"},
		}},
	}
	query := finite.Query{
		Start:   finite.State{"state": "off"},
		Goal:    finite.State{"state": "on"},
		Horizon: 1, // Exactly one action.
	}

	encoding, err := finite.Compile(context.Background(), model, query)
	if err != nil {
		log.Fatal(err)
	}
	formula, err := encoding.CNF()
	if err != nil {
		log.Fatal(err)
	}
	if err := formula.WriteDIMACS(os.Stdout); err != nil {
		log.Fatal(err)
	}
}
