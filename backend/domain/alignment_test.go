package domain_test

import (
	"errors"
	"testing"

	"github.com/17media/stt-workbench/backend/domain"
	"github.com/17media/stt-workbench/backend/models"
)

func TestAlignAssignsSegmentsAndCreatesUnmatchedRows(t *testing.T) {
	golden := models.Golden{Segments: []models.GoldenSegment{
		{SegmentID: "g1", Timestamps: models.Timestamps{From: "00:00:00.000", To: "00:00:01.000"}, Text: "one"},
		{SegmentID: "g2", Timestamps: models.Timestamps{From: "00:00:02.000", To: "00:00:03.000"}, Text: "two"},
	}}
	results := []models.STTResult{{
		PresetID: "preset-a",
		Segments: []models.STTSegment{
			{Timestamps: models.Timestamps{From: "00:00:00.100", To: "00:00:00.900"}, Text: "one"},
			{Timestamps: models.Timestamps{From: "00:00:01.200", To: "00:00:01.500"}, Text: "gap"},
			{Timestamps: models.Timestamps{From: "00:00:02.100", To: "00:00:02.800"}, Text: "two"},
		},
	}}

	rows, warnings, err := domain.Align(golden, results)
	if err != nil {
		t.Fatalf("Align() error = %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("rows = %#v, want 3 rows", rows)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if len(rows[0].Models["preset-a"]) != 1 || rows[0].Golden.SegmentID != "g1" {
		t.Fatalf("first row = %#v, want g1 with model segment", rows[0])
	}
	if rows[1].Golden.Timestamps.From != "00:00:01.200" || rows[1].Golden.Timestamps.To != "00:00:01.500" || len(rows[1].Models["preset-a"]) != 1 {
		t.Fatalf("unmatched row = %#v, want gap segment", rows[1])
	}
	if rows[2].Golden.SegmentID != "g2" || len(rows[2].Models["preset-a"]) != 1 {
		t.Fatalf("last row = %#v, want g2 with model segment", rows[2])
	}
}

func TestAlignWarnsForOverlappingGoldenSegments(t *testing.T) {
	_, warnings, err := domain.Align(models.Golden{Segments: []models.GoldenSegment{
		{Timestamps: models.Timestamps{From: "00:00:00.000", To: "00:00:01.000"}},
		{Timestamps: models.Timestamps{From: "00:00:00.900", To: "00:00:01.500"}},
	}}, nil)
	if err != nil {
		t.Fatalf("Align() error = %v, want nil", err)
	}
	if len(warnings) != 1 || warnings[0].Scope != "golden" || warnings[0].Index != 1 || warnings[0].OverlapMS != 100 {
		t.Fatalf("warnings = %#v, want one Golden overlap warning", warnings)
	}
}

func TestAlignRejectsInvalidResultSegments(t *testing.T) {
	_, _, err := domain.Align(models.Golden{}, []models.STTResult{{
		PresetID: "preset-a",
		Segments: []models.STTSegment{{Timestamps: models.Timestamps{From: "00:00:01.000", To: "00:00:00.500"}}},
	}})
	if !errors.Is(err, domain.ErrSegmentsInvalid) {
		t.Fatalf("Align() error = %v, want ErrSegmentsInvalid", err)
	}
}

func TestAlignRejectsMalformedTimestamp(t *testing.T) {
	_, _, err := domain.Align(models.Golden{Segments: []models.GoldenSegment{
		{Timestamps: models.Timestamps{From: "not-a-time", To: "00:00:01.000"}},
	}}, nil)
	if !errors.Is(err, domain.ErrGoldenInvalid) {
		t.Fatalf("Align() error = %v, want ErrGoldenInvalid", err)
	}
}

func TestAlignComparesParsedTimestampsBeyondTwoDigitHours(t *testing.T) {
	rows, warnings, err := domain.Align(models.Golden{Segments: []models.GoldenSegment{
		{SegmentID: "earlier", Timestamps: models.Timestamps{From: "99:00:00.000", To: "99:00:01.000"}},
		{SegmentID: "later", Timestamps: models.Timestamps{From: "100:00:00.000", To: "100:00:01.000"}},
	}}, nil)
	if err != nil {
		t.Fatalf("Align() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if len(rows) != 2 || rows[0].Golden.SegmentID != "earlier" || rows[1].Golden.SegmentID != "later" {
		t.Fatalf("Align() rows = %#v, want numeric timestamp order", rows)
	}
}
