package domain

import (
	"errors"
	"fmt"
	"sort"

	"github.com/17media/stt-workbench/backend/models"
)

var ErrSegmentsInvalid = errors.New("invalid STT segments")

func ValidateGoldenSegments(segments []models.GoldenSegment) error {
	for index, segment := range segments {
		if segment.StartMS < 0 || segment.EndMS <= segment.StartMS {
			return fmt.Errorf("%w: Golden segment %d has invalid interval", ErrGoldenInvalid, index)
		}
		if index > 0 && segment.StartMS < segments[index-1].EndMS {
			return fmt.Errorf("%w: Golden segments overlap at %d", ErrGoldenInvalid, index)
		}
	}
	return nil
}

func ValidateSTTSegments(segments []models.STTSegment) error {
	for index, segment := range segments {
		if segment.StartMS < 0 || segment.EndMS <= segment.StartMS {
			return fmt.Errorf("%w: segment %d has invalid interval", ErrSegmentsInvalid, index)
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
		modelsByPreset := make(map[string][]models.STTSegment, len(results))
		for _, result := range results {
			modelsByPreset[result.PresetID] = []models.STTSegment{}
		}
		rows = append(rows, row{
			alignment: models.AlignmentRow{Golden: segment, Models: modelsByPreset},
			start:     segment.StartMS,
			end:       segment.EndMS,
			order:     -1,
			sequence:  index,
		})
	}

	for resultIndex, result := range results {
		for segmentIndex, segment := range result.Segments {
			owner := -1
			for index, goldenSegment := range golden.Segments {
				if segment.StartMS >= goldenSegment.StartMS && segment.StartMS < goldenSegment.EndMS {
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
					Golden: models.GoldenSegment{StartMS: segment.StartMS, EndMS: segment.EndMS},
					Models: modelsByPreset,
				},
				start:    segment.StartMS,
				end:      segment.EndMS,
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
