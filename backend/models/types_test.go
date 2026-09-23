package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/17media/stt-workbench/backend/models"
)

func TestPresetJSONUsesNestedModelParams(t *testing.T) {
	preset := models.Preset{
		PresetID: "550e8400-e29b-41d4-a716-446655440000",
		Model: models.Model{
			Name:   "whisper-large-v3",
			Params: map[string]any{"temperature": 0.2, "beam_size": 5},
		},
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
		StreamIDs: []string{"214744544"},
	}
	document := marshalObject(t, preset)
	assertNoKey(t, document, "schema_version")
	assertNoKey(t, document, "provider")
	model, ok := document["model"].(map[string]any)
	if !ok {
		t.Fatalf("model = %#v, want object", document["model"])
	}
	if _, ok := model["params"].(map[string]any); !ok {
		t.Fatalf("model.params = %#v, want object", model["params"])
	}
}

func TestGoldenJSONHasVODIDAndNoVersion(t *testing.T) {
	golden := models.Golden{
		StreamID:     "214744544",
		VODID:        "1780967564_000",
		BasePresetID: "550e8400-e29b-41d4-a716-446655440000",
		UpdatedAt:    time.Now().UTC(),
		Segments: []models.GoldenSegment{
			{
				SegmentID: "golden_001",
				StartMS:   0,
				EndMS:     5000,
				Text:      "hello",
			},
		},
	}
	document := marshalObject(t, golden)
	assertNoKey(t, document, "schema_version")
	if document["vod_id"] != "1780967564_000" {
		t.Fatalf("vod_id = %#v, want 1780967564_000", document["vod_id"])
	}
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
