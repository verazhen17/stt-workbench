package router_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/17media/stt-workbench/backend/domain"
	"github.com/17media/stt-workbench/backend/router"
)

type fakeStreamCatalog struct {
	streams []domain.Stream
	err     error
}

func (catalog fakeStreamCatalog) List(context.Context) ([]domain.Stream, error) {
	return catalog.streams, catalog.err
}

func TestGetStreams(t *testing.T) {
	engine := router.NewRouter(router.Dependencies{
		Streams: fakeStreamCatalog{streams: []domain.Stream{
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
		Streams []domain.Stream `json:"streams"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := []domain.Stream{{StreamID: "214744544"}, {StreamID: "214744545"}}
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

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
