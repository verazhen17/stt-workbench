package domain

import (
	"context"
	"fmt"
	"io/fs"
	"sort"

	"github.com/17media/stt-workbench/backend/models"
)

type FilesystemStreamCatalog struct {
	filesystem ReadFS
}

var _ StreamCatalog = (*FilesystemStreamCatalog)(nil)

func NewFilesystemStreamCatalog(filesystem ReadFS) *FilesystemStreamCatalog {
	return &FilesystemStreamCatalog{filesystem: filesystem}
}

func (catalog *FilesystemStreamCatalog) List(ctx context.Context) ([]models.Stream, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	entries, err := fs.ReadDir(catalog.filesystem, ".")
	if err != nil {
		return nil, fmt.Errorf("read VOD root: %w", err)
	}

	streams := make([]models.Stream, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			streams = append(streams, models.Stream{StreamID: entry.Name()})
		}
	}
	sort.Slice(streams, func(left, right int) bool {
		return streams[left].StreamID < streams[right].StreamID
	})
	return streams, nil
}

type ReadFS interface {
	fs.FS
}

type StreamCatalog interface {
	List(context.Context) ([]models.Stream, error)
}
