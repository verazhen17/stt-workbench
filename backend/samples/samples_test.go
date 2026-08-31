package samples_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

type sttSample struct {
	PresetID  string `json:"preset_id"`
	StreamID  string `json:"stream_id"`
	VODID     string `json:"vod_id"`
	CreatedAt string `json:"created_at"`
	Segments  []struct {
		StartMS int64  `json:"start_ms"`
		EndMS   int64  `json:"end_ms"`
		Text    string `json:"text"`
	} `json:"segments"`
}

type presetSample struct {
	PresetID  string      `json:"preset_id"`
	Model     sampleModel `json:"model"`
	CreatedAt string      `json:"created_at"`
	UpdatedAt string      `json:"updated_at"`
	StreamIDs []string    `json:"stream_ids"`
}

type sampleModel struct {
	Name   string         `json:"name"`
	Params map[string]any `json:"params"`
}

type goldenSample struct {
	StreamID     string `json:"stream_id"`
	VODID        string `json:"vod_id"`
	BasePresetID string `json:"base_preset_id"`
	UpdatedAt    string `json:"updated_at"`
	Segments     []struct {
		SegmentID string `json:"segment_id"`
	} `json:"segments"`
}

func samplesRoot(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate samples test source")
	}
	return filepath.Dir(filename)
}

func TestSamples(t *testing.T) {
	root := samplesRoot(t)
	dataRoot := filepath.Join(root, "samples")

	t.Run("single and multiple FLV streams", func(t *testing.T) {
		assertFLVCount(t, filepath.Join(dataRoot, "214744544"), 1)
		assertFLVCount(t, filepath.Join(dataRoot, "214744545"), 2)
	})

	t.Run("preset manifests", func(t *testing.T) {
		for _, name := range []string{
			"550e8400-e29b-41d4-a716-446655440000.json",
			"6ba7b810-9dad-11d1-80b4-00c04fd430c8.json",
		} {
			path := filepath.Join(dataRoot, "presets", name)
			assertNoVersionOrTopLevelParams(t, path)
			var sample presetSample
			readJSON(t, path, &sample)
			if sample.PresetID == "" || sample.Model.Name == "" || sample.Model.Params == nil || len(sample.StreamIDs) == 0 {
				t.Fatalf("%s is not a valid preset manifest", name)
			}
		}
	})

	t.Run("per-VOD STT results", func(t *testing.T) {
		for _, name := range []string{
			"1780967564_000_550e8400-e29b-41d4-a716-446655440000.json",
			"1780967564_000_6ba7b810-9dad-11d1-80b4-00c04fd430c8.json",
		} {
			path := filepath.Join(dataRoot, "214744544", name)
			assertNoVersionOrTopLevelParams(t, path)
			var sample sttSample
			readJSON(t, path, &sample)
			if sample.PresetID == "" || sample.StreamID == "" || sample.VODID == "" || sample.CreatedAt == "" || len(sample.Segments) == 0 {
				t.Fatalf("%s is not a selectable STT sample", name)
			}
		}
	})

	t.Run("invalid segment remains selectable", func(t *testing.T) {
		path := filepath.Join(dataRoot, "214744544", "1780967564_000_6ba7b810-9dad-11d1-80b4-00c04fd430c8.json")
		assertNoVersionOrTopLevelParams(t, path)
		var sample sttSample
		readJSON(t, path, &sample)
		if sample.PresetID == "" || sample.StreamID == "" {
			t.Fatal("invalid segment sample must retain selectable identity")
		}
		if len(sample.Segments) != 1 || sample.Segments[0].EndMS > sample.Segments[0].StartMS {
			t.Fatal("invalid segment sample must have end_ms <= start_ms")
		}
	})

	t.Run("saved Golden", func(t *testing.T) {
		path := filepath.Join(root, "golden", "214744544", "1780967564_000.json")
		assertNoVersion(t, path)
		var sample goldenSample
		readJSON(t, path, &sample)
		if sample.StreamID == "" || sample.VODID == "" || sample.BasePresetID == "" || sample.UpdatedAt == "" || len(sample.Segments) == 0 || sample.Segments[0].SegmentID == "" {
			t.Fatal("saved Golden sample must contain stable identities")
		}
	})
}

func assertNoVersionOrTopLevelParams(t *testing.T, path string) {
	t.Helper()
	var document map[string]any
	readJSON(t, path, &document)
	if _, exists := document["params"]; exists {
		t.Fatalf("sample %s contains top-level params", path)
	}
	if _, exists := document["schema_version"]; exists {
		t.Fatalf("sample %s contains schema_version", path)
	}
}

func assertNoVersion(t *testing.T, path string) {
	t.Helper()
	var document map[string]any
	readJSON(t, path, &document)
	if _, exists := document["schema_version"]; exists {
		t.Fatalf("sample %s contains schema_version", path)
	}
}

func assertFLVCount(t *testing.T, directory string, want int) {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("read VOD sample directory: %v", err)
	}
	got := 0
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".flv" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			t.Fatalf("stat FLV sample: %v", err)
		}
		if info.Size() == 0 {
			t.Fatalf("FLV sample %s is empty", entry.Name())
		}
		got++
	}
	if got != want {
		t.Fatalf("FLV sample count = %d, want %d", got, want)
	}
}

func readJSON(t *testing.T, path string, destination any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sample %s: %v", path, err)
	}
	if err := json.Unmarshal(data, destination); err != nil {
		t.Fatalf("decode sample %s: %v", path, err)
	}
}
