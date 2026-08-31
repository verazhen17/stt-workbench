package router_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/17media/stt-workbench/backend/domain"
	"github.com/17media/stt-workbench/backend/models"
	"github.com/17media/stt-workbench/backend/router"
)

const (
	testPresetID = "550e8400-e29b-41d4-a716-446655440000"
	testStreamID = "214744544"
	testVODID    = "1780967564_000"
)

type fakePresetCatalog struct {
	presets []models.Preset
	err     error
}

func (catalog fakePresetCatalog) List(context.Context) ([]models.Preset, error) {
	return catalog.presets, catalog.err
}

func (catalog fakePresetCatalog) Get(_ context.Context, presetID string) (models.Preset, error) {
	if catalog.err != nil {
		return models.Preset{}, catalog.err
	}
	for _, preset := range catalog.presets {
		if preset.PresetID == presetID {
			return preset, nil
		}
	}
	return models.Preset{}, domain.ErrPresetNotFound
}

type fakeVODCatalog struct {
	vods []models.VODSegment
	err  error
}

func (catalog fakeVODCatalog) List(context.Context, string) ([]models.VODSegment, error) {
	return catalog.vods, catalog.err
}

type fakeSelectableResultLister struct {
	results []models.STTResultSummary
	err     error
}

func (lister fakeSelectableResultLister) List(context.Context, string, string) ([]models.STTResultSummary, error) {
	return lister.results, lister.err
}

func TestGetPresets(t *testing.T) {
	preset := models.Preset{
		PresetID:  testPresetID,
		Model:     models.Model{Name: "large-v3", Params: map[string]any{"temperature": 0.2}},
		CreatedAt: time.Date(2026, 8, 17, 5, 30, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 8, 17, 5, 30, 0, 0, time.UTC),
		StreamIDs: []string{testStreamID},
	}
	engine := router.NewRouter(router.Dependencies{
		Presets: fakePresetCatalog{presets: []models.Preset{preset}},
		Logger:  discardLogger(),
	})

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/presets", nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var got struct {
		Presets []models.Preset `json:"presets"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !reflect.DeepEqual(got.Presets, []models.Preset{preset}) {
		t.Fatalf("presets = %#v, want %#v", got.Presets, []models.Preset{preset})
	}
}

func TestGetStreamsFiltersByPresetManifest(t *testing.T) {
	engine := router.NewRouter(router.Dependencies{
		Streams: fakeStreamCatalog{streams: []models.Stream{{StreamID: testStreamID}, {StreamID: "other"}}},
		Presets: fakePresetCatalog{presets: []models.Preset{{PresetID: testPresetID, StreamIDs: []string{testStreamID}}}},
		Logger:  discardLogger(),
	})

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/streams?preset_id="+testPresetID, nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var got struct {
		Streams []models.Stream `json:"streams"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := []models.Stream{{StreamID: testStreamID}}
	if !reflect.DeepEqual(got.Streams, want) {
		t.Fatalf("streams = %#v, want %#v", got.Streams, want)
	}
}

func TestGetStreamsRejectsInvalidPresetID(t *testing.T) {
	engine := router.NewRouter(router.Dependencies{Logger: discardLogger()})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/streams?preset_id=not-a-uuid", nil))

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	var got struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Error.Code != "invalid_preset_id" {
		t.Fatalf("error code = %q, want invalid_preset_id", got.Error.Code)
	}
}

func TestGetStreamDetailReturnsPerVODResults(t *testing.T) {
	vod := models.VODSegment{VODID: testVODID, FileID: testVODID, FLVURL: "/vod/" + testStreamID + "/video.flv", WAVURL: "/vod/" + testStreamID + "/video.wav", DurationMS: 1000, TimelineEnd: 1000}
	result := models.STTResultSummary{PresetID: testPresetID, Model: models.Model{Name: "large-v3", Params: map[string]any{}}, CreatedAt: time.Now().UTC()}
	engine := router.NewRouter(router.Dependencies{
		VODs:    fakeVODCatalog{vods: []models.VODSegment{vod}},
		Results: fakeSelectableResultLister{results: []models.STTResultSummary{result}},
		Logger:  discardLogger(),
	})

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/streams/"+testStreamID, nil))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var got models.StreamDetail
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.StreamID != testStreamID || len(got.VODs) != 1 || !reflect.DeepEqual(got.VODs[0].STTResults, []models.STTResultSummary{result}) {
		t.Fatalf("detail = %#v, want stream with per-VOD result", got)
	}
}

func TestGetStreamDetailMapsPresetIndexInconsistency(t *testing.T) {
	engine := router.NewRouter(router.Dependencies{
		VODs:    fakeVODCatalog{vods: []models.VODSegment{{VODID: testVODID}}},
		Results: fakeSelectableResultLister{err: domain.ErrPresetIndexInconsistent},
		Logger:  discardLogger(),
	})

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/streams/"+testStreamID, nil))

	if response.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusConflict)
	}
	var got struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.Error.Code != "preset_index_inconsistent" {
		t.Fatalf("error code = %q, want preset_index_inconsistent", got.Error.Code)
	}
}

func TestGetPresetsMapsCatalogFailure(t *testing.T) {
	engine := router.NewRouter(router.Dependencies{
		Presets: fakePresetCatalog{err: errors.New("read failed")},
		Logger:  discardLogger(),
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/presets", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
}
