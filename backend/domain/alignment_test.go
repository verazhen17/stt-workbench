package domain_test

import (
	"errors"
	"testing"

	"github.com/17media/stt-workbench/backend/domain"
	"github.com/17media/stt-workbench/backend/models"
)

func TestAlignAssignsSegmentsAndCreatesUnmatchedRows(t *testing.T) {
	golden := models.Golden{Segments: []models.GoldenSegment{
		{SegmentID: "g1", StartMS: 0, EndMS: 1000, Text: "one"},
		{SegmentID: "g2", StartMS: 2000, EndMS: 3000, Text: "two"},
	}}
	results := []models.STTResult{{
		PresetID: "preset-a",
		Segments: []models.STTSegment{
			{StartMS: 100, EndMS: 900, Text: "one"},
			{StartMS: 1200, EndMS: 1500, Text: "gap"},
			{StartMS: 2100, EndMS: 2800, Text: "two"},
		},
	}}

	rows, err := domain.Align(golden, results)
	if err != nil {
		t.Fatalf("Align() error = %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %#v, want 3 rows", rows)
	}
	if len(rows[0].Models["preset-a"]) != 1 || rows[0].Golden.SegmentID != "g1" {
		t.Fatalf("first row = %#v, want g1 with model segment", rows[0])
	}
	if rows[1].Golden.StartMS != 1200 || rows[1].Golden.EndMS != 1500 || len(rows[1].Models["preset-a"]) != 1 {
		t.Fatalf("unmatched row = %#v, want gap segment", rows[1])
	}
	if rows[2].Golden.SegmentID != "g2" || len(rows[2].Models["preset-a"]) != 1 {
		t.Fatalf("last row = %#v, want g2 with model segment", rows[2])
	}
}

func TestAlignRejectsOverlappingGoldenSegments(t *testing.T) {
	_, err := domain.Align(models.Golden{Segments: []models.GoldenSegment{
		{StartMS: 0, EndMS: 1000},
		{StartMS: 900, EndMS: 1500},
	}}, nil)
	if !errors.Is(err, domain.ErrGoldenInvalid) {
		t.Fatalf("Align() error = %v, want ErrGoldenInvalid", err)
	}
}

func TestAlignRejectsInvalidResultSegments(t *testing.T) {
	_, err := domain.Align(models.Golden{}, []models.STTResult{{
		PresetID: "preset-a",
		Segments: []models.STTSegment{{StartMS: 1000, EndMS: 500}},
	}})
	if !errors.Is(err, domain.ErrSegmentsInvalid) {
		t.Fatalf("Align() error = %v, want ErrSegmentsInvalid", err)
	}
}
