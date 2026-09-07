package domain

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/17media/stt-workbench/backend/models"
)

var (
	ErrPresetNotFound          = errors.New("preset not found")
	ErrPresetInvalid           = errors.New("invalid preset manifest")
	ErrPresetIndexInconsistent = errors.New("preset index inconsistent")
)

type PresetCatalog interface {
	List(context.Context) ([]models.Preset, error)
	Get(context.Context, string) (models.Preset, error)
}

type FilesystemPresetCatalog struct {
	filesystem ReadFS
}

var _ PresetCatalog = (*FilesystemPresetCatalog)(nil)

func NewFilesystemPresetCatalog(filesystem ReadFS) *FilesystemPresetCatalog {
	return &FilesystemPresetCatalog{filesystem: filesystem}
}

func (catalog *FilesystemPresetCatalog) List(ctx context.Context) ([]models.Preset, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := fs.ReadDir(catalog.filesystem, "presets")
	if err != nil {
		return nil, fmt.Errorf("read preset root: %w", err)
	}
	presets := make([]models.Preset, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() || path.Ext(entry.Name()) != ".json" {
			continue
		}
		preset, err := catalog.read(ctx, entry.Name())
		if err != nil {
			return nil, err
		}
		presets = append(presets, preset)
	}
	sort.Slice(presets, func(left, right int) bool {
		if !presets[left].CreatedAt.Equal(presets[right].CreatedAt) {
			return presets[left].CreatedAt.Before(presets[right].CreatedAt)
		}
		return presets[left].PresetID < presets[right].PresetID
	})
	return presets, nil
}

func (catalog *FilesystemPresetCatalog) Get(ctx context.Context, presetID string) (models.Preset, error) {
	if !validUUID(presetID) {
		return models.Preset{}, fmt.Errorf("%w: %s", ErrPresetNotFound, presetID)
	}
	return catalog.read(ctx, presetID+".json")
}

func (catalog *FilesystemPresetCatalog) read(ctx context.Context, filename string) (models.Preset, error) {
	if err := ctx.Err(); err != nil {
		return models.Preset{}, err
	}
	presetID := strings.TrimSuffix(filename, ".json")
	if path.Ext(filename) != ".json" || !validUUID(presetID) {
		return models.Preset{}, fmt.Errorf("%w: filename %q", ErrPresetInvalid, filename)
	}
	data, err := fs.ReadFile(catalog.filesystem, path.Join("presets", filename))
	if errors.Is(err, fs.ErrNotExist) {
		return models.Preset{}, fmt.Errorf("%w: %s", ErrPresetNotFound, presetID)
	}
	if err != nil {
		return models.Preset{}, fmt.Errorf("read preset %q: %w", filename, err)
	}
	var preset models.Preset
	if err := decodeExact(data, &preset); err != nil {
		return models.Preset{}, fmt.Errorf("%w: %s: %v", ErrPresetInvalid, filename, err)
	}
	if preset.PresetID != presetID || !validUUID(preset.PresetID) || preset.Model.Name == "" || preset.Model.Params == nil {
		return models.Preset{}, fmt.Errorf("%w: identity or model mismatch in %q", ErrPresetInvalid, filename)
	}
	preset.StreamIDs = normalizeIDs(preset.StreamIDs)
	return preset, nil
}

func decodeExact(data []byte, destination any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON data")
		}
		return fmt.Errorf("trailing JSON data: %w", err)
	}
	return nil
}

func normalizeIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	normalized := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	sort.Strings(normalized)
	return normalized
}

func containsID(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}

func validUUID(value string) bool {
	if len(value) != 36 || value[8] != '-' || value[13] != '-' || value[18] != '-' || value[23] != '-' {
		return false
	}
	compact := strings.ReplaceAll(value, "-", "")
	if _, err := hex.DecodeString(compact); err != nil {
		return false
	}
	return len(compact) == 32
}

func IsValidUUID(value string) bool {
	return validUUID(value)
}
