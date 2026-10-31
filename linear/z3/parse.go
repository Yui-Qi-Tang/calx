package z3

import (
	"errors"
	"strconv"
	"strings"

	"github.com/Yui-Qi-Tang/calx/linear"
)

var errModel = errors.New("missing, partial or malformed get-value response")

// parseAssignment reads only the concrete get-value grammar requested, not
// arbitrary SMT-LIB.
func parseAssignment(raw []byte, booleans, integers int) (linear.Assignment, error) {
	a := linear.Assignment{Booleans: map[int]bool{}, Integers: map[int]string{}}
	tokens := strings.Fields(strings.NewReplacer("(", " ( ", ")", " ) ").Replace(string(raw)))
	pos := 0
	take := func(want string) bool {
		if pos >= len(tokens) || tokens[pos] != want {
			return false
		}
		pos++
		return true
	}
	if !take("sat") {
		return a, errModel
	}
	if booleans+integers == 0 {
		if pos != len(tokens) {
			return a, errModel
		}
		return a, nil
	}
	if !take("(") {
		return a, errModel
	}
	for range booleans + integers {
		if !take("(") || pos >= len(tokens) {
			return a, errModel
		}
		name := tokens[pos]
		pos++
		if len(name) < 2 || pos >= len(tokens) {
			return a, errModel
		}
		id, err := strconv.Atoi(name[1:])
		if err != nil || id < 1 || name[1:] != strconv.Itoa(id) {
			return a, errModel
		}
		switch name[0] {
		case 'b':
			if _, exists := a.Booleans[id]; id > booleans || exists {
				return a, errModel
			}
			if tokens[pos] != "true" && tokens[pos] != "false" {
				return a, errModel
			}
			a.Booleans[id] = tokens[pos] == "true"
			pos++
		case 'i':
			if _, exists := a.Integers[id]; id > integers || exists {
				return a, errModel
			}
			value := tokens[pos]
			pos++
			if value == "(" {
				if !take("-") || pos >= len(tokens) {
					return a, errModel
				}
				value = "-" + tokens[pos]
				pos++
				if !take(")") {
					return a, errModel
				}
			}
			if _, err = linear.Integer(value); err != nil {
				return a, errModel
			}
			a.Integers[id] = value
		default:
			return a, errModel
		}
		if !take(")") {
			return a, errModel
		}
	}
	if !take(")") || pos != len(tokens) {
		return a, errModel
	}
	return a, nil
}
