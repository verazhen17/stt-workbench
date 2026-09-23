package domain

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"

	"github.com/17media/stt-workbench/backend/models"
)

var ErrSegmentsInvalid = errors.New("invalid STT segments")

var timestampPattern = regexp.MustCompile(`^(\d{2,}):([0-5]\d):([0-5]\d)\.(\d{3})$`)

func parseTimestampMS(value string) (int64, error) {
	parts := timestampPattern.FindStringSubmatch(value)
	if parts == nil {
		return 0, fmt.Errorf("timestamp %q must use HH:MM:SS.mmm", value)
	}
	hours, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("timestamp %q has invalid hours: %w", value, err)
	}
	minutes, _ := strconv.ParseInt(parts[2], 10, 64)
	seconds, _ := strconv.ParseInt(parts[3], 10, 64)
	milliseconds, _ := strconv.ParseInt(parts[4], 10, 64)
	remainder := minutes*60_000 + seconds*1_000 + milliseconds
	if hours > (math.MaxInt64-remainder)/3_600_000 {
		return 0, fmt.Errorf("timestamp %q is too large", value)
	}
	return hours*3_600_000 + remainder, nil
}

func parseInterval(from, to string) (int64, int64, error) {
	start, err := parseTimestampMS(from)
	if err != nil {
		return 0, 0, err
	}
	end, err := parseTimestampMS(to)
	if err != nil {
		return 0, 0, err
	}
	if end <= start {
		return 0, 0, errors.New("end must be after start")
	}
	return start, end, nil
}

func ValidateGoldenSegments(segments []models.GoldenSegment) error {
	var previousEnd int64
	for index, segment := range segments {
		start, end, err := parseInterval(segment.Timestamps.From, segment.Timestamps.To)
		if err != nil {
			return fmt.Errorf("%w: Golden segment %d has invalid interval: %v", ErrGoldenInvalid, index, err)
		}
		if index > 0 && start < previousEnd {
			return fmt.Errorf("%w: Golden segments overlap at %d", ErrGoldenInvalid, index)
		}
		previousEnd = end
	}
	return nil
}

func ValidateSTTSegments(segments []models.STTSegment) error {
	for index, segment := range segments {
		if _, _, err := parseInterval(segment.Timestamps.From, segment.Timestamps.To); err != nil {
			return fmt.Errorf("%w: segment %d has invalid interval: %v", ErrSegmentsInvalid, index, err)
		}
	}
	return nil
}

func Align(golden models.Golden, results []models.STTResult) ([]models.AlignmentRow, error) {
	if err := ValidateGoldenSegments(golden.Segments); err != nil {
		return nil, err
	}
	for _, result := range results {
		if err := ValidateSTTSegments(result.Segments); err != nil {
			return nil, err
		}
	}

	type row struct {
		alignment models.AlignmentRow
		start     int64
		end       int64
		order     int
		sequence  int
	}
	rows := make([]row, 0, len(golden.Segments))
	for index, segment := range golden.Segments {
		start, end, _ := parseInterval(segment.Timestamps.From, segment.Timestamps.To)
		modelsByPreset := make(map[string][]models.STTSegment, len(results))
		for _, result := range results {
			modelsByPreset[result.PresetID] = []models.STTSegment{}
		}
		rows = append(rows, row{
			alignment: models.AlignmentRow{Golden: segment, Models: modelsByPreset},
			start:     start,
			end:       end,
			order:     -1,
			sequence:  index,
		})
	}

	for resultIndex, result := range results {
		for segmentIndex, segment := range result.Segments {
			segmentStart, segmentEnd, _ := parseInterval(segment.Timestamps.From, segment.Timestamps.To)
			owner := -1
			for index, goldenSegment := range golden.Segments {
				goldenStart, goldenEnd, _ := parseInterval(goldenSegment.Timestamps.From, goldenSegment.Timestamps.To)
				if segmentStart >= goldenStart && segmentStart < goldenEnd {
					owner = index
					break
				}
			}
			if owner >= 0 {
				rows[owner].alignment.Models[result.PresetID] = append(rows[owner].alignment.Models[result.PresetID], segment)
				continue
			}
			modelsByPreset := make(map[string][]models.STTSegment, len(results))
			for _, selected := range results {
				modelsByPreset[selected.PresetID] = []models.STTSegment{}
			}
			modelsByPreset[result.PresetID] = []models.STTSegment{segment}
			rows = append(rows, row{
				alignment: models.AlignmentRow{
					Golden: models.GoldenSegment{Timestamps: segment.Timestamps},
					Models: modelsByPreset,
				},
				start:    segmentStart,
				end:      segmentEnd,
				order:    resultIndex,
				sequence: segmentIndex,
			})
		}
	}
	sort.SliceStable(rows, func(left, right int) bool {
		if rows[left].start != rows[right].start {
			return rows[left].start < rows[right].start
		}
		if rows[left].end != rows[right].end {
			return rows[left].end < rows[right].end
		}
		if rows[left].order != rows[right].order {
			return rows[left].order < rows[right].order
		}
		return rows[left].sequence < rows[right].sequence
	})
	result := make([]models.AlignmentRow, 0, len(rows))
	for _, item := range rows {
		result = append(result, item.alignment)
	}
	return result, nil
}
