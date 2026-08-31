package domain

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/17media/stt-workbench/backend/models"
)

var (
	ErrGoldenNotFound = errors.New("golden not found")
	ErrGoldenInvalid  = errors.New("invalid Golden")
)

type GoldenReader interface {
	Get(context.Context, string, string) (models.Golden, error)
}

type FilesystemGoldenCatalog struct {
	filesystem ReadFS
}

var _ GoldenReader = (*FilesystemGoldenCatalog)(nil)

func NewFilesystemGoldenCatalog(filesystem ReadFS) *FilesystemGoldenCatalog {
	return &FilesystemGoldenCatalog{filesystem: filesystem}
}

func (catalog *FilesystemGoldenCatalog) Get(ctx context.Context, streamID, vodID string) (models.Golden, error) {
	if err := ctx.Err(); err != nil {
		return models.Golden{}, err
	}
	if !validStreamID(streamID) || vodID == "" {
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
