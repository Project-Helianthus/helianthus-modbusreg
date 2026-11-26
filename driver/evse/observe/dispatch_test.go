package observe

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestTeslaObserveEarlyDispatchVectors(t *testing.T) {
	type vector struct {
		Argv    []string `json:"argv"`
		Handled bool     `json:"handled"`
		Status  *int     `json:"status"`
		Opens   int      `json:"opens"`
	}
	data, err := os.ReadFile("testdata/early-dispatch.json")
	if err != nil {
		t.Fatal(err)
	}
	var got []vector
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	refused := 2
	want := []vector{
		{Argv: []string{"gateway"}},
		{Argv: []string{"gateway", "-scan"}},
		{Argv: []string{"gateway", "tesla-observe-x"}},
		{Argv: []string{"gateway", "-tesla-observe"}},
		{Argv: []string{"gateway", "tesla-observe"}, Handled: true, Status: &refused},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected dispatch vectors: %+v", got)
	}
	ports := &teslaObserveTestPorts{}
	bare := got[len(got)-1]
	if status := Run(bare.Argv[2:], ports.build()); status != *bare.Status || len(ports.opened) != bare.Opens {
		t.Fatalf("bare command: status=%d opens=%d", status, len(ports.opened))
	}
}
