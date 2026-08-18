package router_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/17media/stt-workbench/backend/domain"
	"github.com/17media/stt-workbench/backend/models"
	"github.com/17media/stt-workbench/backend/router"
)

type fakeStreamCatalog struct {
	streams []models.Stream
	err     error
}

func (catalog fakeStreamCatalog) List(context.Context) ([]models.Stream, error) {
	return catalog.streams, catalog.err
}

func TestGetStreams(t *testing.T) {
	engine := router.NewRouter(router.Dependencies{
		Streams: fakeStreamCatalog{streams: []models.Stream{
			{StreamID: "214744544"},
			{StreamID: "214744545"},
		}},
		Logger: discardLogger(),
	})

	request := httptest.NewRequest(http.MethodGet, "/api/streams", nil)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if got := response.Header().Get("Content-Type"); got != "application/json; charset=utf-8" {
		t.Fatalf("Content-Type = %q, want application/json; charset=utf-8", got)
	}
	var got struct {
		Streams []models.Stream `json:"streams"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := []models.Stream{{StreamID: "214744544"}, {StreamID: "214744545"}}
	if !reflect.DeepEqual(got.Streams, want) {
		t.Fatalf("streams = %#v, want %#v", got.Streams, want)
	}
}

func TestGetStreamsReturnsAnEmptyJSONList(t *testing.T) {
	engine := router.NewRouter(router.Dependencies{
		Streams: fakeStreamCatalog{},
		Logger:  discardLogger(),
	})
	request := httptest.NewRequest(http.MethodGet, "/api/streams", nil)
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, request)

	if got, want := response.Body.String(), "{\"streams\":[]}"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestGetStreamsMapsCatalogFailureToCommonError(t *testing.T) {
	engine := router.NewRouter(router.Dependencies{
		Streams: fakeStreamCatalog{err: errors.New("read failed")},
		Logger:  discardLogger(),
	})
	request := httptest.NewRequest(http.MethodGet, "/api/streams", nil)
	response := httptest.NewRecorder()

	engine.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	var got struct {
		Error struct {
			Code    string         `json:"code"`
			Message string         `json:"message"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Error.Code != "internal_error" || got.Error.Message == "" || got.Error.Details == nil {
		t.Fatalf("error = %#v, want common internal_error envelope", got.Error)
	}
}

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

func TestServeShutsDownWhenContextIsCanceled(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	httpServer := router.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusNoContent)
	}), logger)

	done := make(chan error, 1)
	go func() {
		done <- httpServer.Serve(ctx, listener)
	}()

	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + listener.Addr().String())
	if err != nil {
		t.Fatalf("GET running server: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not shut down after context cancellation")
	}
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
