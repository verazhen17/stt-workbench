package domain

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
)

var (
	ErrStreamNotFound     = errors.New("stream not found")
	ErrVODIngestionFailed = errors.New("VOD ingestion failed")
)

type DurationProber interface {
	ProbeMilliseconds(context.Context, string) (int64, error)
}

type VODCatalog interface {
	List(context.Context, string) ([]VOD, error)
}

type FilesystemVODCatalog struct {
	filesystem ReadFS
	urlPrefix  string
	prober     DurationProber
}

var _ VODCatalog = (*FilesystemVODCatalog)(nil)

func NewFilesystemVODCatalog(filesystem ReadFS, urlPrefix string, prober DurationProber) *FilesystemVODCatalog {
	return &FilesystemVODCatalog{
		filesystem: filesystem,
		urlPrefix:  strings.TrimRight(urlPrefix, "/"),
		prober:     prober,
	}
}

func (catalog *FilesystemVODCatalog) List(ctx context.Context, streamID string) ([]VOD, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !validStreamID(streamID) {
		return nil, fmt.Errorf("%w: invalid stream ID", ErrStreamNotFound)
	}

	entries, err := fs.ReadDir(catalog.filesystem, streamID)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrStreamNotFound, streamID)
	}
	if err != nil {
		return nil, fmt.Errorf("read VOD stream %q: %w", streamID, err)
	}

	type discoveredVOD struct {
		vod      VOD
		filename string
	}
	discovered := make([]discoveredVOD, 0, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".flv") {
			continue
		}

		vod, err := parseVODFilename(entry.Name())
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrVODIngestionFailed, err)
		}
		discovered = append(discovered, discoveredVOD{vod: vod, filename: entry.Name()})
	}

	sort.Slice(discovered, func(left, right int) bool {
		if discovered[left].vod.StartTimeUnixS != discovered[right].vod.StartTimeUnixS {
			return discovered[left].vod.StartTimeUnixS < discovered[right].vod.StartTimeUnixS
		}
		if discovered[left].vod.Sequence != discovered[right].vod.Sequence {
			return discovered[left].vod.Sequence < discovered[right].vod.Sequence
		}
		return discovered[left].filename < discovered[right].filename
	})

	vods := make([]VOD, 0, len(discovered))
	var timelineMS int64
	for _, item := range discovered {
		relativePath := path.Join(streamID, item.filename)
		durationMS, err := catalog.prober.ProbeMilliseconds(ctx, relativePath)
		if err != nil {
			return nil, fmt.Errorf("%w: probe %q: %v", ErrVODIngestionFailed, relativePath, err)
		}
		if durationMS < 0 {
			return nil, fmt.Errorf("%w: probe %q returned negative duration", ErrVODIngestionFailed, relativePath)
		}

		item.vod.URL = catalog.urlPrefix + "/" + url.PathEscape(streamID) + "/" + url.PathEscape(item.filename)
		item.vod.DurationMS = durationMS
		item.vod.TimelineStart = timelineMS
		timelineMS += durationMS
		item.vod.TimelineEnd = timelineMS
		vods = append(vods, item.vod)
	}
	return vods, nil
}

func parseVODFilename(filename string) (VOD, error) {
	name := strings.TrimSuffix(filename, ".flv")
	parts := strings.SplitN(name, "_", 3)
	if len(parts) != 3 || parts[2] == "" || !decimalDigits(parts[0]) || !decimalDigits(parts[1]) {
		return VOD{}, fmt.Errorf("invalid VOD filename %q", filename)
	}

	startTime, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return VOD{}, fmt.Errorf("invalid VOD start time in %q: %w", filename, err)
	}
	sequence, err := strconv.Atoi(parts[1])
	if err != nil {
		return VOD{}, fmt.Errorf("invalid VOD sequence in %q: %w", filename, err)
	}
	return VOD{
		FileID:         parts[0] + "_" + parts[1],
		Sequence:       sequence,
		StartTimeUnixS: startTime,
	}, nil
}

func decimalDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func validStreamID(streamID string) bool {
	return streamID != "" && streamID != "." && streamID != ".." && fs.ValidPath(streamID) && !strings.ContainsAny(streamID, `/\\`)
}
