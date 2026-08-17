package router_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/17media/stt-workbench/backend/domain"
	"github.com/17media/stt-workbench/backend/router"
)

func TestGetStreamsFilesystemVerticalSlice(t *testing.T) {
	root := t.TempDir()
	for _, streamID := range []string{"214744545", "214744544"} {
		if err := os.Mkdir(filepath.Join(root, streamID), 0o755); err != nil {
			t.Fatalf("create stream directory: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "ignored.flv"), []byte("sample"), 0o644); err != nil {
		t.Fatalf("create non-directory sample: %v", err)
	}

	engine := router.NewRouter(router.Dependencies{
		Streams: domain.NewFilesystemStreamCatalog(os.DirFS(root)),
		Logger:  discardLogger(),
	})
	request := httptest.NewRequest(http.MethodGet, "/api/streams", nil)
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got, want := response.Body.String(), "{\"streams\":[{\"stream_id\":\"214744544\"},{\"stream_id\":\"214744545\"}]}"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}
