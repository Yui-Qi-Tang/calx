package sat_test

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/Yui-Qi-Tang/calx/digest"
	"github.com/Yui-Qi-Tang/calx/internal/acceptance"
	"github.com/Yui-Qi-Tang/calx/sat"
)

func TestDIMACSPreservesEveryClause(t *testing.T) {
	for name, c := range acceptance.Cases() {
		var raw bytes.Buffer
		if err := c.WriteDIMACS(&raw); err != nil {
			t.Fatal(err)
		}
		var marker, kind string
		var vars, clauses int
		if _, err := fmt.Fscan(&raw, &marker, &kind, &vars, &clauses); err != nil {
			t.Fatal(err)
		}
		if marker != "p" || kind != "cnf" || vars != len(c.Variables) || clauses != len(c.Clauses) {
			t.Fatalf("%s: header", name)
		}
		for i := 0; i < clauses; i++ {
			got := []int{}
			for {
				var lit int
				if _, err := fmt.Fscan(&raw, &lit); err != nil {
					t.Fatal(err)
				}
				if lit == 0 {
					break
				}
				got = append(got, lit)
			}
			if !reflect.DeepEqual(got, c.Clauses[i]) {
				t.Fatalf("%s: lost clause %d", name, i)
			}
		}
		if len(bytes.TrimSpace(raw.Bytes())) != 0 {
			t.Fatalf("%s: trailing formula", name)
		}
	}
}

func TestMalformedInputAndIncompleteAssignments(t *testing.T) {
	for _, c := range []sat.CNF{
		{Variables: []sat.Variable{{ID: 2}}},
		{Clauses: [][]int{{0}}},
		{Variables: []sat.Variable{{ID: 1}}, Clauses: [][]int{{-2}}},
		{Clauses: [][]int{{}}, Origins: [][]string{}},
	} {
		if !errors.Is(c.CheckShape(), sat.ErrInput) {
			t.Fatal("accepted malformed formula")
		}
	}
	c := acceptance.CNF(2, [][]int{{1}})
	for _, a := range []map[int]bool{{1: true}, {1: true, 3: true}, {1: false, 2: true}} {
		if c.CheckAssignment(a) == nil {
			t.Fatalf("accepted %v", a)
		}
	}
	if err := c.CheckAssignment(map[int]bool{1: true, 2: false}); err != nil {
		t.Fatal(err)
	}
}

func TestConsumerGuarantees(t *testing.T) {
	want := sat.Requirements{SATModelChecked: true, UNSATProofChecked: true}
	if err := (sat.Result{Status: sat.Unknown}).Require(want); !errors.Is(err, sat.ErrInconclusive) {
		t.Fatalf("UNKNOWN error = %v, want ErrInconclusive", err)
	}
	for _, r := range []sat.Result{
		{},
		{Status: "invalid"},
		{Status: sat.SAT},
		{Status: sat.UNSAT},
		{Status: sat.SAT, InputDigest: "original", Model: sat.Validation{Present: true, Decoded: true, Checked: true, TargetDigest: "other"}},
	} {
		if !errors.Is(r.Require(want), sat.ErrGuarantee) {
			t.Fatal("accepted insufficient guarantee")
		}
	}
}

func TestDigestBindingsRejectMalformedAndMismatchedValues(t *testing.T) {
	want := sat.Requirements{SATModelChecked: true, UNSATProofChecked: true}
	original := digest.Sum([]byte("original"))
	for _, status := range []sat.Status{sat.SAT, sat.UNSAT} {
		for _, test := range []struct {
			name           string
			source, target digest.Digest
			accepted       bool
		}{
			{name: "matching", source: original, target: original, accepted: true},
			{name: "different", source: original, target: digest.Sum([]byte("other"))},
			{name: "absent"},
			{name: "invalid matching cast", source: "invalid", target: "invalid"},
		} {
			t.Run(string(status)+"/"+test.name, func(t *testing.T) {
				r := sat.Result{
					Status: status, InputDigest: test.source, FormulaDigest: test.source,
					Model: sat.Validation{Present: true, Decoded: true, Checked: true, TargetDigest: test.target},
					Proof: sat.Validation{Present: true, Checked: true, TargetDigest: test.target},
				}
				err := r.Require(want)
				if test.accepted && err != nil {
					t.Fatal(err)
				}
				if !test.accepted && !errors.Is(err, sat.ErrGuarantee) {
					t.Fatalf("Require error = %v, want ErrGuarantee", err)
				}
			})
		}
	}
}
