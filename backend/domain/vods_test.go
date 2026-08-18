package domain_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/17media/stt-workbench/backend/domain"
	"github.com/17media/stt-workbench/backend/models"
)

const (
	helperEnabled = "FFPROBE_HELPER_ENABLED"
	helperMode    = "FFPROBE_HELPER_MODE"
	helperOutput  = "FFPROBE_HELPER_OUTPUT"
	helperSource  = "FFPROBE_HELPER_SOURCE"
)

func TestMain(m *testing.M) {
	if os.Getenv(helperEnabled) == "1" {
		runFFprobeHelper()
		return
	}
	os.Exit(m.Run())
}

func runFFprobeHelper() {
	want := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		os.Getenv(helperSource),
	}
	if !reflect.DeepEqual(os.Args[1:], want) {
		fmt.Fprintf(os.Stderr, "args = %#v, want %#v", os.Args[1:], want)
		os.Exit(2)
	}

	switch os.Getenv(helperMode) {
	case "success":
		fmt.Fprint(os.Stdout, os.Getenv(helperOutput))
	case "failure":
		fmt.Fprint(os.Stderr, "sensitive ffprobe output")
		os.Exit(3)
	case "sleep":
		time.Sleep(10 * time.Second)
	default:
		os.Exit(4)
	}
}

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
		filepath.Join(root, "214744545", "1780967564_001_first.flv"):  1_000,
		filepath.Join(root, "214744545", "1780967564_002_second.flv"): 2_500,
		filepath.Join(root, "214744545", "1780974764_010_later.flv"):  3_000,
	}}
	catalog := newFilesystemVODCatalog(t, root, "/vod/", prober)

	got, err := catalog.List(context.Background(), streamID)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := []models.VOD{
		{FileID: "1780967564_001", URL: "/vod/214744545/1780967564_001_first.flv", Sequence: 1, StartTimeUnixS: 1780967564, DurationMS: 1_000, TimelineStart: 0, TimelineEnd: 1_000},
		{FileID: "1780967564_002", URL: "/vod/214744545/1780967564_002_second.flv", Sequence: 2, StartTimeUnixS: 1780967564, DurationMS: 2_500, TimelineStart: 1_000, TimelineEnd: 3_500},
		{FileID: "1780974764_010", URL: "/vod/214744545/1780974764_010_later.flv", Sequence: 10, StartTimeUnixS: 1780974764, DurationMS: 3_000, TimelineStart: 3_500, TimelineEnd: 6_500},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("List() = %#v, want %#v", got, want)
	}
	wantPaths := []string{
		filepath.Join(root, "214744545", "1780967564_001_first.flv"),
		filepath.Join(root, "214744545", "1780967564_002_second.flv"),
		filepath.Join(root, "214744545", "1780974764_010_later.flv"),
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
	catalog := newFilesystemVODCatalog(t, root, "/vod", &fakeDurationProber{})

	got, err := catalog.List(context.Background(), "stream-1")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("List() = %#v, want non-nil empty slice", got)
	}
}

func TestVODCatalogReportsMissingStream(t *testing.T) {
	root := t.TempDir()
	catalog := newFilesystemVODCatalog(t, root, "/vod", &fakeDurationProber{})

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
	catalog := newFilesystemVODCatalog(t, root, "/vod", &fakeDurationProber{})

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
	catalog := newFilesystemVODCatalog(t, root, "/vod", &fakeDurationProber{err: errors.New("probe failed")})

	_, err := catalog.List(context.Background(), "stream-1")
	if !errors.Is(err, domain.ErrVODIngestionFailed) {
		t.Fatalf("List() error = %v, want ErrVODIngestionFailed", err)
	}
}

func TestVODCatalogRejectsPathTraversal(t *testing.T) {
	prober := &fakeDurationProber{}
	root := t.TempDir()
	catalog := newFilesystemVODCatalog(t, root, "/vod", prober)

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

func newFilesystemVODCatalog(t *testing.T, root, urlPrefix string, prober domain.DurationProber) *domain.FilesystemVODCatalog {
	t.Helper()
	catalog, err := domain.NewFilesystemVODCatalog(os.DirFS(root), root, urlPrefix, prober)
	if err != nil {
		t.Fatalf("NewFilesystemVODCatalog() error = %v", err)
	}
	return catalog
}

func TestFFprobeDurationProberRunsExpectedCommandAndRoundsMilliseconds(t *testing.T) {
	prober := newHelperFFprobeProber(t, "success", "1.2346\n", "stream-1/sample.flv")

	got, err := prober.ProbeMilliseconds(context.Background(), "stream-1/sample.flv")
	if err != nil {
		t.Fatalf("ProbeMilliseconds() error = %v", err)
	}
	if got != 1_235 {
		t.Fatalf("ProbeMilliseconds() = %d, want 1235", got)
	}
}

func TestFFprobeDurationProberReportsCommandFailureWithoutOutput(t *testing.T) {
	prober := newHelperFFprobeProber(t, "failure", "", "stream-1/sample.flv")

	_, err := prober.ProbeMilliseconds(context.Background(), "stream-1/sample.flv")
	if err == nil {
		t.Fatal("ProbeMilliseconds() error = nil, want command failure")
	}
	if strings.Contains(err.Error(), "sensitive ffprobe output") {
		t.Fatalf("error leaks ffprobe output: %v", err)
	}
}

func TestFFprobeDurationProberRejectsInvalidDurationOutput(t *testing.T) {
	for _, output := range []string{"", "not-a-number", "NaN", "+Inf", "-Inf", "0", "-1", "1e100"} {
		t.Run(output, func(t *testing.T) {
			prober := newHelperFFprobeProber(t, "success", output, "stream-1/sample.flv")

			if _, err := prober.ProbeMilliseconds(context.Background(), "stream-1/sample.flv"); err == nil {
				t.Fatalf("ProbeMilliseconds() error = nil for output %q", output)
			}
		})
	}
}

func TestFFprobeDurationProberHonorsCanceledContext(t *testing.T) {
	prober := newHelperFFprobeProber(t, "sleep", "", "stream-1/sample.flv")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	started := time.Now()
	if _, err := prober.ProbeMilliseconds(ctx, "stream-1/sample.flv"); err == nil {
		t.Fatal("ProbeMilliseconds() error = nil, want context cancellation")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("ProbeMilliseconds() took %v with canceled context", elapsed)
	}
}

func TestFFprobeDurationProberTreatsLeadingDashFilenameAsAPath(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "-sample.flv")
	prober := newHelperFFprobeProber(t, "success", "1\n", sourcePath)

	if _, err := prober.ProbeMilliseconds(context.Background(), sourcePath); err != nil {
		t.Fatalf("ProbeMilliseconds() error = %v", err)
	}
}

func newHelperFFprobeProber(t *testing.T, mode, output, sourcePath string) *domain.FFprobeDurationProber {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("find test executable: %v", err)
	}
	t.Setenv(helperEnabled, "1")
	t.Setenv(helperMode, mode)
	t.Setenv(helperOutput, output)
	t.Setenv(helperSource, sourcePath)
	return domain.NewFFprobeDurationProber(executable)
}
