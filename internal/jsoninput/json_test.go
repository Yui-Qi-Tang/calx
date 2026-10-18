package jsoninput

import "testing"

func TestExactJSONContract(t *testing.T) {
	type input struct {
		Name string       `json:"name"`
		IDs  map[int]bool `json:"ids"`
	}
	for _, raw := range []string{`{"name":"a","name":"b"}`, `{"Name":"a"}`, `{"name":null}`, `{"name":"a"} {}`, `{"ids":{"1":true,"01":false}}`, `{"unknown":1}`} {
		var dst input
		if e := Decode([]byte(raw), &dst); e == nil {
			t.Fatal(raw)
		}
	}
	var dst input
	if e := Decode([]byte(`{"name":"a","ids":{"1":false}}`), &dst); e != nil {
		t.Fatal(e)
	}
}
