package domain_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/17media/stt-workbench/backend/domain"
	"github.com/17media/stt-workbench/backend/models"
)

const (
	presetA = "550e8400-e29b-41d4-a716-446655440000"
	presetB = "6ba7b810-9dad-11d1-80b4-00c04fd430c8"
	streamA = "214744544"
	vodA    = "1780967564_000"
)

func TestFilesystemPresetCatalogListsNormalizedPresets(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "presets", presetA+".json"), models.Preset{
		PresetID:  presetA,
		Model:     models.Model{Name: "large-v3", Params: map[string]any{"temperature": 0.2}},
		CreatedAt: time.Date(2026, 8, 17, 5, 30, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 8, 17, 5, 30, 0, 0, time.UTC),
		StreamIDs: []string{streamA, streamA},
	})

	catalog := domain.NewFilesystemPresetCatalog(os.DirFS(root))
	got, err := catalog.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []models.Preset{{
		PresetID:  presetA,
		Model:     models.Model{Name: "large-v3", Params: map[string]any{"temperature": 0.2}},
		CreatedAt: time.Date(2026, 8, 17, 5, 30, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 8, 17, 5, 30, 0, 0, time.UTC),
		StreamIDs: []string{streamA},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %#v, want %#v", got, want)
	}
}

func TestFilesystemPresetCatalogRejectsManifestIdentityMismatch(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "presets", presetA+".json"), models.Preset{PresetID: presetB})

	_, err := domain.NewFilesystemPresetCatalog(os.DirFS(root)).List(context.Background())
	if !errors.Is(err, domain.ErrPresetInvalid) {
		t.Fatalf("List() error = %v, want ErrPresetInvalid", err)
	}
}

func TestFilesystemResultCatalogListsPerVODResults(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, streamA, vodA+"_"+presetA+".json"), models.STTResult{
		PresetID: presetA, StreamID: streamA, VODID: vodA,
		CreatedAt: time.Date(2026, 8, 17, 5, 35, 0, 0, time.UTC),
		Segments:  []models.STTSegment{{StartMS: 0, EndMS: 1000, Text: "hello"}},
	})

	results := domain.NewFilesystemResultCatalog(os.DirFS(root))
	got, err := results.List(context.Background(), streamA, vodA)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got) != 1 || got[0].PresetID != presetA || got[0].VODID != vodA {
		t.Fatalf("List() = %#v, want one result for %s/%s", got, vodA, presetA)
	}
}

func TestSelectableResultCatalogRejectsMissingCompletedResult(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, "presets", presetA+".json"), models.Preset{
		PresetID:  presetA,
		Model:     models.Model{Name: "large-v3", Params: map[string]any{}},
		StreamIDs: []string{streamA},
	})
	presets := domain.NewFilesystemPresetCatalog(os.DirFS(root))
	results := domain.NewFilesystemResultCatalog(os.DirFS(root))
	_, err := domain.NewSelectableResultCatalog(presets, results).List(context.Background(), streamA, vodA)
	if !errors.Is(err, domain.ErrPresetIndexInconsistent) {
		t.Fatalf("List() error = %v, want ErrPresetIndexInconsistent", err)
	}
}

func writeJSON(t *testing.T, filename string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}
