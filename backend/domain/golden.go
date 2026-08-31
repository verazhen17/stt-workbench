package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/17media/stt-workbench/backend/models"
)

var (
	ErrGoldenNotFound = errors.New("golden not found")
	ErrGoldenInvalid  = errors.New("invalid Golden")
)

type GoldenReader interface {
	Get(context.Context, string, string) (models.Golden, error)
}

type GoldenWriter interface {
	GoldenReader
	Save(context.Context, models.Golden) error
}

type GoldenManager interface {
	Renew(context.Context, string, string, string) (models.Golden, error)
	Edit(context.Context, string, string, []models.GoldenEditSegment) (models.Golden, error)
}

type FilesystemGoldenCatalog struct {
	filesystem ReadFS
}

var _ GoldenReader = (*FilesystemGoldenCatalog)(nil)

func NewFilesystemGoldenCatalog(filesystem ReadFS) *FilesystemGoldenCatalog {
	return &FilesystemGoldenCatalog{filesystem: filesystem}
}

type FilesystemGoldenStore struct {
	*FilesystemGoldenCatalog
	root string
}

var _ GoldenWriter = (*FilesystemGoldenStore)(nil)
var _ GoldenManager = (*GoldenService)(nil)

func NewFilesystemGoldenStore(filesystem ReadFS, root string) *FilesystemGoldenStore {
	return &FilesystemGoldenStore{
		FilesystemGoldenCatalog: NewFilesystemGoldenCatalog(filesystem),
		root:                    filepath.Clean(root),
	}
}

func (store *FilesystemGoldenStore) Save(ctx context.Context, golden models.Golden) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validStreamID(golden.StreamID) || !validVODID(golden.VODID) {
		return fmt.Errorf("%w: invalid identity", ErrGoldenInvalid)
	}
	if err := ValidateGoldenSegments(golden.Segments); err != nil {
		return err
	}
	data, err := json.MarshalIndent(golden, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal Golden: %w", err)
	}
	directory := filepath.Join(store.root, golden.StreamID)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create Golden directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".golden-*.tmp")
	if err != nil {
		return fmt.Errorf("create Golden temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write Golden: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close Golden temporary file: %w", err)
	}
	target := filepath.Join(directory, golden.VODID+".json")
	if err := os.Rename(temporaryName, target); err != nil {
		return fmt.Errorf("publish Golden: %w", err)
	}
	return nil
}

type GoldenService struct {
	store   GoldenWriter
	results SelectableResultProvider
	now     func() time.Time
}

func NewGoldenService(store GoldenWriter, results SelectableResultProvider) *GoldenService {
	return &GoldenService{store: store, results: results, now: time.Now}
}

func (service *GoldenService) Renew(ctx context.Context, streamID, vodID, sourcePresetID string) (models.Golden, error) {
	result, _, err := service.results.Get(ctx, streamID, vodID, sourcePresetID)
	if err != nil {
		return models.Golden{}, err
	}
	if err := ValidateSTTSegments(result.Segments); err != nil {
		return models.Golden{}, err
	}
	golden := models.Golden{
		StreamID:     streamID,
		VODID:        vodID,
		BasePresetID: sourcePresetID,
		UpdatedAt:    service.now().UTC(),
		Segments:     make([]models.GoldenSegment, 0, len(result.Segments)),
	}
	for index, segment := range result.Segments {
		golden.Segments = append(golden.Segments, models.GoldenSegment{
			SegmentID: fmt.Sprintf("golden_segment_%03d", index+1),
			StartMS:   segment.StartMS,
			EndMS:     segment.EndMS,
			Text:      segment.Text,
		})
	}
	if err := service.store.Save(ctx, golden); err != nil {
		return models.Golden{}, err
	}
	return golden, nil
}

func (service *GoldenService) Edit(ctx context.Context, streamID, vodID string, edits []models.GoldenEditSegment) (models.Golden, error) {
	current, err := service.store.Get(ctx, streamID, vodID)
	if err != nil {
		return models.Golden{}, err
	}
	if len(edits) != len(current.Segments) {
		return models.Golden{}, fmt.Errorf("%w: segment count changed", ErrGoldenInvalid)
	}
	updated := current
	updated.UpdatedAt = service.now().UTC()
	updated.Segments = make([]models.GoldenSegment, len(edits))
	for index, edit := range edits {
		updated.Segments[index] = models.GoldenSegment{
			SegmentID: current.Segments[index].SegmentID,
			StartMS:   edit.StartMS,
			EndMS:     edit.EndMS,
			Text:      edit.Text,
		}
	}
	if err := ValidateGoldenSegments(updated.Segments); err != nil {
		return models.Golden{}, err
	}
	if err := service.store.Save(ctx, updated); err != nil {
		return models.Golden{}, err
	}
	return updated, nil
}

func (catalog *FilesystemGoldenCatalog) Get(ctx context.Context, streamID, vodID string) (models.Golden, error) {
	if err := ctx.Err(); err != nil {
		return models.Golden{}, err
	}
	if !validStreamID(streamID) || !validVODID(vodID) {
		return models.Golden{}, fmt.Errorf("%w: invalid identity", ErrGoldenNotFound)
	}
	data, err := fs.ReadFile(catalog.filesystem, path.Join(streamID, vodID+".json"))
	if errors.Is(err, fs.ErrNotExist) {
		return models.Golden{}, fmt.Errorf("%w: %s/%s", ErrGoldenNotFound, streamID, vodID)
	}
	if err != nil {
		return models.Golden{}, fmt.Errorf("read Golden: %w", err)
	}
	var golden models.Golden
	if err := decodeExact(data, &golden); err != nil {
		return models.Golden{}, fmt.Errorf("%w: %v", ErrGoldenInvalid, err)
	}
	if golden.StreamID != streamID || golden.VODID != vodID {
		return models.Golden{}, fmt.Errorf("%w: identity mismatch", ErrGoldenInvalid)
	}
	return golden, nil
}

func validVODID(vodID string) bool {
	return vodID != "" && vodID != "." && vodID != ".." && !strings.ContainsAny(vodID, `/\\`)
}
