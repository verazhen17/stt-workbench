package models

import "time"

type Stream struct {
	StreamID string `json:"stream_id"`
}

type Model struct {
	Name   string         `json:"name"`
	Params map[string]any `json:"params"`
}

type Preset struct {
	PresetID  string    `json:"preset_id"`
	Name      string    `json:"name,omitempty"`
	Model     Model     `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	StreamIDs []string  `json:"stream_ids"`
}

type STTResultSummary struct {
	PresetID  string    `json:"preset_id"`
	Name      string    `json:"name,omitempty"`
	Model     Model     `json:"model"`
	CreatedAt time.Time `json:"created_at"`
}

type VODSegment struct {
	VODID         string             `json:"vod_id"`
	FileID        string             `json:"file_id"`
	FLVURL        string             `json:"flv_url"`
	WAVURL        string             `json:"wav_url"`
	Sequence      int                `json:"sequence"`
	StartTimeUnix int64              `json:"start_time_unix_s"`
	DurationMS    int64              `json:"duration_ms"`
	TimelineStart int64              `json:"timeline_start_ms"`
	TimelineEnd   int64              `json:"timeline_end_ms"`
	STTResults    []STTResultSummary `json:"stt_results"`
}

type StreamDetail struct {
	StreamID string       `json:"stream_id"`
	VODs     []VODSegment `json:"vods"`
}

type Timestamps struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type STTSegment struct {
	Timestamps Timestamps `json:"timestamps"`
	Text       string     `json:"text"`
}

type STTResult struct {
	PresetID  string       `json:"preset_id"`
	StreamID  string       `json:"stream_id"`
	VODID     string       `json:"vod_id"`
	Language  string       `json:"language,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	Segments  []STTSegment `json:"segments"`
}

type GoldenSegment struct {
	SegmentID  string     `json:"segment_id,omitempty"`
	Timestamps Timestamps `json:"timestamps"`
	Text       string     `json:"text"`
}

type Golden struct {
	StreamID     string          `json:"stream_id"`
	VODID        string          `json:"vod_id"`
	BasePresetID string          `json:"base_preset_id"`
	UpdatedAt    time.Time       `json:"updated_at"`
	Segments     []GoldenSegment `json:"segments"`
}

type SelectedResultError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type SelectedResult struct {
	PresetID  string               `json:"preset_id"`
	Model     Model                `json:"model"`
	CreatedAt time.Time            `json:"created_at"`
	Error     *SelectedResultError `json:"error"`
}

type AlignmentRow struct {
	Golden GoldenSegment           `json:"golden"`
	Models map[string][]STTSegment `json:"models"`
}

type AlignmentWarning struct {
	Type              string `json:"type"`
	Scope             string `json:"scope"`
	PresetID          string `json:"preset_id,omitempty"`
	SegmentID         string `json:"segment_id,omitempty"`
	PreviousSegmentID string `json:"previous_segment_id,omitempty"`
	Index             int    `json:"index"`
	PreviousIndex     int    `json:"previous_index"`
	OverlapMS         int64  `json:"overlap_ms"`
}

type Alignment struct {
	StreamID        string             `json:"stream_id"`
	VODID           string             `json:"vod_id"`
	SelectedResults []SelectedResult   `json:"selected_results"`
	Rows            []AlignmentRow     `json:"rows"`
	Warnings        []AlignmentWarning `json:"warnings,omitempty"`
}

type GoldenRequest struct {
	VODID          string              `json:"vod_id"`
	Mode           string              `json:"mode"`
	SourcePresetID string              `json:"source_preset_id,omitempty"`
	Segments       []GoldenEditSegment `json:"segments,omitempty"`
}

type GoldenEditSegment struct {
	Timestamps Timestamps `json:"timestamps"`
	Text       string     `json:"text"`
}
