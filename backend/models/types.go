package models

import "time"

type Stream struct {
	StreamID string `json:"stream_id"`
}

type VOD struct {
	FileID         string `json:"file_id"`
	URL            string `json:"url"`
	Sequence       int    `json:"sequence"`
	StartTimeUnixS int64  `json:"start_time_unix_s"`
	DurationMS     int64  `json:"duration_ms"`
	TimelineStart  int64  `json:"timeline_start_ms"`
	TimelineEnd    int64  `json:"timeline_end_ms"`
}

type Model struct {
	Provider string         `json:"provider,omitempty"`
	Name     string         `json:"name"`
	Params   map[string]any `json:"params"`
}

type Segment struct {
	SegmentID        string   `json:"segment_id,omitempty"`
	StartMS          int64    `json:"start_ms"`
	EndMS            int64    `json:"end_ms"`
	Text             string   `json:"text"`
	SourceSegmentIDs []string `json:"source_segment_ids,omitempty"`
}

type STTRunMetadata struct {
	RunID     string    `json:"run_id"`
	Model     Model     `json:"model"`
	CreatedAt time.Time `json:"created_at"`
}

type STTRun struct {
	StreamID string `json:"stream_id"`
	STTRunMetadata
	Segments []Segment `json:"segments"`
}

type Golden struct {
	StreamID      string    `json:"stream_id"`
	GoldenID      string    `json:"golden_id"`
	BaseRunID     string    `json:"base_run_id"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	CreatedBy     string    `json:"created_by"`
	Segments      []Segment `json:"segments"`
	ChangeSummary string    `json:"change_summary,omitempty"`
}

type SelectedRunError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type SelectedRun struct {
	STTRunMetadata
	Error *SelectedRunError `json:"error"`
}

type AlignmentRow struct {
	Golden Segment              `json:"golden"`
	Models map[string][]Segment `json:"models"`
}

type Alignment struct {
	StreamID     string         `json:"stream_id"`
	SelectedRuns []SelectedRun  `json:"selected_runs"`
	Rows         []AlignmentRow `json:"rows"`
}
