package domain

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/17media/stt-workbench/backend/models"
)

var (
	ErrSTTResultNotFound = errors.New("STT result not found")
	ErrSTTResultInvalid  = errors.New("invalid STT result")
)

type ResultCatalog interface {
	List(context.Context, string, string) ([]models.STTResult, error)
	Get(context.Context, string, string, string) (models.STTResult, error)
}

type SelectableResultLister interface {
	List(context.Context, string, string) ([]models.STTResultSummary, error)
}

type SelectableResultProvider interface {
	SelectableResultLister
	Get(context.Context, string, string, string) (models.STTResult, models.Preset, error)
}

type FilesystemResultCatalog struct {
	filesystem ReadFS
}

var _ ResultCatalog = (*FilesystemResultCatalog)(nil)

func NewFilesystemResultCatalog(filesystem ReadFS) *FilesystemResultCatalog {
	return &FilesystemResultCatalog{filesystem: filesystem}
}

func (catalog *FilesystemResultCatalog) List(ctx context.Context, streamID, vodID string) ([]models.STTResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validStreamID(streamID) || vodID == "" {
		return nil, fmt.Errorf("%w: invalid identity", ErrSTTResultNotFound)
	}
	entries, err := fs.ReadDir(catalog.filesystem, streamID)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrSTTResultNotFound, streamID)
	}
	if err != nil {
		return nil, fmt.Errorf("read STT results for %q: %w", streamID, err)
	}
	results := make([]models.STTResult, 0)
	for _, entry := range entries {
		if entry.IsDir() || path.Ext(entry.Name()) != ".json" {
			continue
		}
		entryVODID, _, ok := parseResultFilename(entry.Name())
		if !ok || entryVODID != vodID {
			continue
		}
		result, err := catalog.read(ctx, streamID, entry.Name())
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	sort.Slice(results, func(left, right int) bool {
		return results[left].PresetID < results[right].PresetID
	})
	return results, nil
}

func (catalog *FilesystemResultCatalog) Get(ctx context.Context, streamID, vodID, presetID string) (models.STTResult, error) {
	if !validStreamID(streamID) || vodID == "" || !validUUID(presetID) {
		return models.STTResult{}, fmt.Errorf("%w: invalid identity", ErrSTTResultNotFound)
	}
	return catalog.read(ctx, streamID, vodID+"_"+presetID+".json")
}

func (catalog *FilesystemResultCatalog) read(ctx context.Context, streamID, filename string) (models.STTResult, error) {
	if err := ctx.Err(); err != nil {
		return models.STTResult{}, err
	}
	vodID, presetID, ok := parseResultFilename(filename)
	if !ok {
		return models.STTResult{}, fmt.Errorf("%w: filename %q", ErrSTTResultInvalid, filename)
	}
	data, err := fs.ReadFile(catalog.filesystem, path.Join(streamID, filename))
	if errors.Is(err, fs.ErrNotExist) {
		return models.STTResult{}, fmt.Errorf("%w: %s/%s", ErrSTTResultNotFound, streamID, filename)
	}
	if err != nil {
		return models.STTResult{}, fmt.Errorf("read STT result %q: %w", filename, err)
	}
	var result models.STTResult
	if err := decodeExact(data, &result); err != nil {
		return models.STTResult{}, fmt.Errorf("%w: %s: %v", ErrSTTResultInvalid, filename, err)
	}
	if result.PresetID != presetID || result.StreamID != streamID || result.VODID != vodID {
		return models.STTResult{}, fmt.Errorf("%w: identity mismatch in %q", ErrSTTResultInvalid, filename)
	}
	return result, nil
}

func parseResultFilename(filename string) (string, string, bool) {
	if path.Ext(filename) != ".json" {
		return "", "", false
	}
	base := strings.TrimSuffix(filename, ".json")
	separator := strings.LastIndex(base, "_")
	if separator <= 0 || separator == len(base)-1 {
		return "", "", false
	}
	vodID, presetID := base[:separator], base[separator+1:]
	return vodID, presetID, validUUID(presetID)
}

type SelectableResultCatalog struct {
	presets PresetCatalog
	results ResultCatalog
}

func NewSelectableResultCatalog(presets PresetCatalog, results ResultCatalog) *SelectableResultCatalog {
	return &SelectableResultCatalog{presets: presets, results: results}
}

func (catalog *SelectableResultCatalog) Get(ctx context.Context, streamID, vodID, presetID string) (models.STTResult, models.Preset, error) {
	preset, err := catalog.presets.Get(ctx, presetID)
	if err != nil {
		return models.STTResult{}, models.Preset{}, err
	}
	if !containsID(preset.StreamIDs, streamID) {
		return models.STTResult{}, models.Preset{}, fmt.Errorf("%w: %s/%s", ErrSTTResultNotFound, streamID, presetID)
	}
	result, err := catalog.results.Get(ctx, streamID, vodID, presetID)
	if errors.Is(err, ErrSTTResultNotFound) || errors.Is(err, ErrSTTResultInvalid) {
		return models.STTResult{}, models.Preset{}, fmt.Errorf("%w: %s/%s", ErrPresetIndexInconsistent, streamID, presetID)
	}
	if err != nil {
		return models.STTResult{}, models.Preset{}, err
	}
	return result, preset, nil
}

func (catalog *SelectableResultCatalog) List(ctx context.Context, streamID, vodID string) ([]models.STTResultSummary, error) {
	presets, err := catalog.presets.List(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make([]models.STTResultSummary, 0)
	for _, preset := range presets {
		if !containsID(preset.StreamIDs, streamID) {
			continue
		}
		result, err := catalog.results.Get(ctx, streamID, vodID, preset.PresetID)
		if errors.Is(err, ErrSTTResultNotFound) || errors.Is(err, ErrSTTResultInvalid) {
			return nil, fmt.Errorf("%w: %s/%s", ErrPresetIndexInconsistent, streamID, preset.PresetID)
		}
		if err != nil {
			return nil, err
		}
		summaries = append(summaries, models.STTResultSummary{
			PresetID:  preset.PresetID,
			Model:     preset.Model,
			CreatedAt: result.CreatedAt,
		})
	}
	return summaries, nil
}
