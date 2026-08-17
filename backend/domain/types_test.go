package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/17media/stt-workbench/backend/domain"
)

func TestSTTRunJSONUsesNestedModelParamsWithoutVersion(t *testing.T) {
	run := domain.STTRun{
		StreamID: "stream-1",
		STTRunMetadata: domain.STTRunMetadata{
			RunID: "run-1",
			Model: domain.Model{
				Name:   "whisper",
				Params: map[string]any{"temperature": 0.2},
			},
		},
	}
	document := marshalObject(t, run)
	assertNoKey(t, document, "schema_version")
	assertNoKey(t, document, "params")
	model, ok := document["model"].(map[string]any)
	if !ok {
		t.Fatalf("model = %#v, want object", document["model"])
	}
	if _, ok := model["params"].(map[string]any); !ok {
		t.Fatalf("model.params = %#v, want object", model["params"])
	}
}

func TestGoldenJSONHasNoVersion(t *testing.T) {
	document := marshalObject(t, domain.Golden{StreamID: "stream-1"})
	assertNoKey(t, document, "schema_version")
}

func marshalObject(t *testing.T, value any) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal value: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(encoded, &document); err != nil {
		t.Fatalf("decode marshaled value: %v", err)
	}
	return document
}

func assertNoKey(t *testing.T, document map[string]any, key string) {
	t.Helper()
	if _, exists := document[key]; exists {
		t.Fatalf("document contains %q", key)
	}
}
