package cadical

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

func exactLine(raw []byte, want string) bool {
	return slices.Contains(strings.FieldsFunc(string(raw), func(r rune) bool { return r == '\n' || r == '\r' }), want)
}

// trivialVerified reports whether raw has a "c trivial UNSAT" line and
// "s VERIFIED" as its only status line.
func trivialVerified(raw []byte) bool {
	var status []string
	for _, line := range strings.FieldsFunc(string(raw), func(r rune) bool { return r == '\n' || r == '\r' }) {
		if strings.HasPrefix(line, "s ") {
			status = append(status, line)
		}
	}
	return exactLine(raw, "c trivial UNSAT") && slices.Equal(status, []string{"s VERIFIED"})
}

// soleStatus reports whether want is the only solver status line in raw.
func soleStatus(raw []byte, want string) bool {
	for _, status := range []string{"s SATISFIABLE", "s UNSATISFIABLE", "s UNKNOWN"} {
		if exactLine(raw, status) != (status == want) {
			return false
		}
	}
	return true
}

func assignment(raw []byte, count int) (map[int]bool, error) {
	values := make(map[int]bool, count)
	ended := false
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 4096), 16<<20)
	for scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) == 0 || parts[0] != "v" {
			continue
		}
		for _, part := range parts[1:] {
			n, err := strconv.Atoi(part)
			if err != nil {
				return nil, errors.New("malformed assignment literal")
			}
			if n == 0 {
				ended = true
				continue
			}
			if ended {
				return nil, errors.New("assignment continues after terminator")
			}
			id := n
			if id < 0 {
				id = -id
			}
			if id < 1 || id > count {
				return nil, errors.New("assignment literal outside variable range")
			}
			value := n > 0
			if prior, ok := values[id]; ok && prior != value {
				return nil, errors.New("contradictory assignment literals")
			}
			values[id] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading assignment: %w", err)
	}
	if !ended || len(values) != count {
		return nil, errors.New("incomplete assignment")
	}
	return values, nil
}

func hasAssignment(raw []byte) bool {
	for line := range strings.SplitSeq(string(raw), "\n") {
		if strings.HasPrefix(line, "v ") {
			return true
		}
	}
	return false
}
