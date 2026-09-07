package domain_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/17media/stt-workbench/backend/domain"
	"github.com/17media/stt-workbench/backend/models"
)

func TestStreamCatalogListsOnlyFirstLevelDirectoriesSorted(t *testing.T) {
	root := t.TempDir()
	for _, directory := range []string{"stream-z", "stream-a", filepath.Join("stream-a", "nested")} {
		if err := os.MkdirAll(filepath.Join(root, directory), 0o755); err != nil {
			t.Fatalf("create sample directory: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "not-a-stream.flv"), []byte("sample"), 0o644); err != nil {
		t.Fatalf("create sample file: %v", err)
	}

	streamCatalog := domain.NewFilesystemStreamCatalog(os.DirFS(root))
	got, err := streamCatalog.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []models.Stream{{StreamID: "stream-a"}, {StreamID: "stream-z"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %#v, want %#v", got, want)
	}
}

func TestStreamCatalogReturnsAnEmptySlice(t *testing.T) {
	streamCatalog := domain.NewFilesystemStreamCatalog(os.DirFS(t.TempDir()))
	got, err := streamCatalog.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("List() = %#v, want non-nil empty slice", got)
	}
}

func TestStreamCatalogReportsFilesystemFailure(t *testing.T) {
	streamCatalog := domain.NewFilesystemStreamCatalog(os.DirFS(filepath.Join(t.TempDir(), "missing")))
	if _, err := streamCatalog.List(context.Background()); err == nil {
		t.Fatal("List() error = nil, want filesystem error")
	}
}

func TestStreamCatalogHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	streamCatalog := domain.NewFilesystemStreamCatalog(os.DirFS(t.TempDir()))
	if _, err := streamCatalog.List(ctx); err == nil {
		t.Fatal("List() error = nil, want context cancellation")
	}
}
