package domain_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/17media/stt-workbench/backend/domain"
	"github.com/17media/stt-workbench/backend/models"
)

type goldenResultProvider struct {
	result models.STTResult
	preset models.Preset
}

func (provider goldenResultProvider) List(context.Context, string, string) ([]models.STTResultSummary, error) {
	return nil, nil
}

func (provider goldenResultProvider) Get(context.Context, string, string, string) (models.STTResult, models.Preset, error) {
	return provider.result, provider.preset, nil
}

func TestGoldenServiceRenewAndEdit(t *testing.T) {
	root := t.TempDir()
	store := domain.NewFilesystemGoldenStore(os.DirFS(root), root)
	service := domain.NewGoldenService(store, goldenResultProvider{
		result: models.STTResult{PresetID: presetA, StreamID: streamA, VODID: vodA, Segments: []models.STTSegment{
			{StartMS: 0, EndMS: 1000, Text: "hello"},
		}},
		preset: models.Preset{PresetID: presetA},
	})

	created, err := service.Renew(context.Background(), streamA, vodA, presetA)
	if err != nil {
		t.Fatalf("Renew() error = %v", err)
	}
	if created.BasePresetID != presetA || created.Segments[0].SegmentID != "golden_segment_001" {
		t.Fatalf("Renew() = %#v, want lineage and stable segment ID", created)
	}

	edited, err := service.Edit(context.Background(), streamA, vodA, []models.GoldenEditSegment{{StartMS: 10, EndMS: 900, Text: "edited"}})
	if err != nil {
		t.Fatalf("Edit() error = %v", err)
	}
	if edited.Segments[0].SegmentID != created.Segments[0].SegmentID || edited.Segments[0].Text != "edited" {
		t.Fatalf("Edit() = %#v, want preserved ID and updated text", edited)
	}
	loaded, err := store.Get(context.Background(), streamA, vodA)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !reflect.DeepEqual(loaded, edited) {
		t.Fatalf("stored Golden = %#v, want %#v", loaded, edited)
	}
}

func TestGoldenServiceRejectsOverlapAndSegmentCountChange(t *testing.T) {
	root := t.TempDir()
	store := domain.NewFilesystemGoldenStore(os.DirFS(root), root)
	if err := store.Save(context.Background(), models.Golden{
		StreamID: streamA, VODID: vodA, BasePresetID: presetA,
		Segments: []models.GoldenSegment{{SegmentID: "g1", StartMS: 0, EndMS: 1000, Text: "one"}, {SegmentID: "g2", StartMS: 1200, EndMS: 2000, Text: "two"}},
	}); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	service := domain.NewGoldenService(store, goldenResultProvider{})
	_, err := service.Edit(context.Background(), streamA, vodA, []models.GoldenEditSegment{{StartMS: 0, EndMS: 1500, Text: "one"}, {StartMS: 1000, EndMS: 2000, Text: "two"}})
	if !errors.Is(err, domain.ErrGoldenInvalid) {
		t.Fatalf("Edit() overlap error = %v, want ErrGoldenInvalid", err)
	}
	_, err = service.Edit(context.Background(), streamA, vodA, []models.GoldenEditSegment{{StartMS: 0, EndMS: 1000, Text: "one"}})
	if !errors.Is(err, domain.ErrGoldenInvalid) {
		t.Fatalf("Edit() count error = %v, want ErrGoldenInvalid", err)
	}
}

func TestFilesystemGoldenStoreWritesPerVODPath(t *testing.T) {
	root := t.TempDir()
	store := domain.NewFilesystemGoldenStore(os.DirFS(root), root)
	golden := models.Golden{StreamID: streamA, VODID: vodA, BasePresetID: presetA}
	if err := store.Save(context.Background(), golden); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, streamA, vodA+".json")); err != nil {
		t.Fatalf("Golden path missing: %v", err)
	}
}
