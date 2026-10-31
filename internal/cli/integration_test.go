//go:build resolverintegration

package cli

import (
	"encoding/json/v2"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSolveFiniteWithRealTools(t *testing.T) {
	for _, name := range []string{"CALX_CADICAL", "CALX_CADICAL_DIGEST", "CALX_DRAT", "CALX_DRAT_DIGEST"} {
		if os.Getenv(name) == "" {
			t.Fatalf("%s is required for explicit acceptance", name)
		}
	}
	args := []string{
		"solve-finite",
		"--solver", os.Getenv("CALX_CADICAL"), "--solver-digest", os.Getenv("CALX_CADICAL_DIGEST"),
		"--checker", os.Getenv("CALX_DRAT"), "--checker-digest", os.Getenv("CALX_DRAT_DIGEST"),
		"--artifacts", filepath.Join(t.TempDir(), "run"),
	}
	raw := `{"model":{"fields":{"x":["a","b"]},"actions":[{"id":"ab","guard":{"x":"a"},"effect":{"x":"b"},"frame":[]}]},"query":{"profile":"categorical-simple-first-hit/v1","start":{"x":"a"},"goal":{"x":"b"},"horizon":1}}`
	status, doc := call(t, args, raw)
	if status != 0 {
		t.Fatalf("status %d: %s", status, doc["error"])
	}
	var data struct {
		Result struct {
			Status string `json:"status"`
		} `json:"result"`
		Trace struct {
			Actions []string `json:"actions"`
		} `json:"trace"`
	}
	if err := json.Unmarshal(doc["data"], &data); err != nil {
		t.Fatal(err)
	}
	if data.Result.Status != "SAT" || !reflect.DeepEqual(data.Trace.Actions, []string{"ab"}) {
		t.Fatalf("%+v", data)
	}
}
