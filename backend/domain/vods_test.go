package domain_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/17media/stt-workbench/backend/domain"
)

type fakeDurationProber struct {
	durations map[string]int64
	err       error
	paths     []string
}

func (prober *fakeDurationProber) ProbeMilliseconds(_ context.Context, path string) (int64, error) {
	prober.paths = append(prober.paths, path)
	if prober.err != nil {
		return 0, prober.err
	}
	return prober.durations[path], nil
}

func TestVODCatalogDiscoversSortedVODsWithCumulativeTimeline(t *testing.T) {
	root := t.TempDir()
	streamID := "214744545"
	streamDirectory := filepath.Join(root, streamID)
	if err := os.Mkdir(streamDirectory, 0o755); err != nil {
		t.Fatalf("create stream directory: %v", err)
	}
	for _, name := range []string{
		"1780974764_010_later.flv",
		"1780967564_002_second.flv",
		"1780967564_001_first.flv",
		"notes.txt",
	} {
		if err := os.WriteFile(filepath.Join(streamDirectory, name), []byte("sample"), 0o644); err != nil {
			t.Fatalf("create sample file: %v", err)
		}
	}
	if err := os.Mkdir(filepath.Join(streamDirectory, "1780000000_000_nested.flv"), 0o755); err != nil {
		t.Fatalf("create ignored subdirectory: %v", err)
	}

	prober := &fakeDurationProber{durations: map[string]int64{
		"214744545/1780967564_001_first.flv":  1_000,
		"214744545/1780967564_002_second.flv": 2_500,
		"214744545/1780974764_010_later.flv":  3_000,
	}}
	catalog := domain.NewFilesystemVODCatalog(os.DirFS(root), "/vod/", prober)

	got, err := catalog.List(context.Background(), streamID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []domain.VOD{
		{FileID: "1780967564_001", URL: "/vod/214744545/1780967564_001_first.flv", Sequence: 1, StartTimeUnixS: 1780967564, DurationMS: 1_000, TimelineStart: 0, TimelineEnd: 1_000},
		{FileID: "1780967564_002", URL: "/vod/214744545/1780967564_002_second.flv", Sequence: 2, StartTimeUnixS: 1780967564, DurationMS: 2_500, TimelineStart: 1_000, TimelineEnd: 3_500},
		{FileID: "1780974764_010", URL: "/vod/214744545/1780974764_010_later.flv", Sequence: 10, StartTimeUnixS: 1780974764, DurationMS: 3_000, TimelineStart: 3_500, TimelineEnd: 6_500},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %#v, want %#v", got, want)
	}
	wantPaths := []string{
		"214744545/1780967564_001_first.flv",
		"214744545/1780967564_002_second.flv",
		"214744545/1780974764_010_later.flv",
	}
	if !reflect.DeepEqual(prober.paths, wantPaths) {
		t.Fatalf("probed paths = %#v, want %#v", prober.paths, wantPaths)
	}
}

func TestVODCatalogReturnsEmptySliceWhenStreamHasNoFLVs(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "stream-1"), 0o755); err != nil {
		t.Fatalf("create stream directory: %v", err)
	}
	catalog := domain.NewFilesystemVODCatalog(os.DirFS(root), "/vod", &fakeDurationProber{})

	got, err := catalog.List(context.Background(), "stream-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("List() = %#v, want non-nil empty slice", got)
	}
}

func TestVODCatalogReportsMissingStream(t *testing.T) {
	catalog := domain.NewFilesystemVODCatalog(os.DirFS(t.TempDir()), "/vod", &fakeDurationProber{})

	_, err := catalog.List(context.Background(), "missing")
	if !errors.Is(err, domain.ErrStreamNotFound) {
		t.Fatalf("List() error = %v, want ErrStreamNotFound", err)
	}
}

func TestVODCatalogReportsInvalidFilename(t *testing.T) {
	root := t.TempDir()
	streamDirectory := filepath.Join(root, "stream-1")
	if err := os.Mkdir(streamDirectory, 0o755); err != nil {
		t.Fatalf("create stream directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(streamDirectory, "invalid.flv"), []byte("sample"), 0o644); err != nil {
		t.Fatalf("create invalid FLV: %v", err)
	}
	catalog := domain.NewFilesystemVODCatalog(os.DirFS(root), "/vod", &fakeDurationProber{})

	_, err := catalog.List(context.Background(), "stream-1")
	if !errors.Is(err, domain.ErrVODIngestionFailed) {
		t.Fatalf("List() error = %v, want ErrVODIngestionFailed", err)
	}
}

func TestVODCatalogReportsProbeFailure(t *testing.T) {
	root := t.TempDir()
	streamDirectory := filepath.Join(root, "stream-1")
	if err := os.Mkdir(streamDirectory, 0o755); err != nil {
		t.Fatalf("create stream directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(streamDirectory, "1780967564_000_sample.flv"), []byte("sample"), 0o644); err != nil {
		t.Fatalf("create FLV: %v", err)
	}
	catalog := domain.NewFilesystemVODCatalog(os.DirFS(root), "/vod", &fakeDurationProber{err: errors.New("probe failed")})

	_, err := catalog.List(context.Background(), "stream-1")
	if !errors.Is(err, domain.ErrVODIngestionFailed) {
		t.Fatalf("List() error = %v, want ErrVODIngestionFailed", err)
	}
}

func TestVODCatalogRejectsPathTraversal(t *testing.T) {
	prober := &fakeDurationProber{}
	catalog := domain.NewFilesystemVODCatalog(os.DirFS(t.TempDir()), "/vod", prober)

	for _, streamID := range []string{"", ".", "..", "../outside", "/absolute", `..\outside`} {
		t.Run(streamID, func(t *testing.T) {
			_, err := catalog.List(context.Background(), streamID)
			if !errors.Is(err, domain.ErrStreamNotFound) {
				t.Fatalf("List(%q) error = %v, want ErrStreamNotFound", streamID, err)
			}
		})
	}
	if len(prober.paths) != 0 {
		t.Fatalf("probed paths = %#v, want none", prober.paths)
	}
}
