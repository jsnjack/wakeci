package main

import (
	"encoding/json"
	"testing"
)

func TestBuildUpdateDataSendsEmptyLists(t *testing.T) {
	for _, tt := range []struct {
		name string
		json string
	}{
		{"new build", `{"id":1,"name":"job"}`},
		{"legacy null lists", `{"id":1,"name":"job","params":null,"artifacts":null,"build_artifacts":null,"tasks":null}`},
		{"empty lists", `{"id":1,"name":"job","params":[],"artifacts":[],"build_artifacts":[],"tasks":[]}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var build BuildUpdateData
			if err := json.Unmarshal([]byte(tt.json), &build); err != nil {
				t.Fatalf("decode build: %v", err)
			}
			data, err := json.Marshal(build)
			if err != nil {
				t.Fatalf("marshal build: %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatalf("decode fields: %v", err)
			}
			for _, field := range []string{"params", "artifacts", "build_artifacts", "tasks"} {
				if got := string(fields[field]); got != "[]" {
					t.Errorf("%s = %s, want []", field, got)
				}
			}
		})
	}
}
