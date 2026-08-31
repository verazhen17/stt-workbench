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
	Model     Model     `json:"model"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	StreamIDs []string  `json:"stream_ids"`
}

type STTResultSummary struct {
	PresetID  string    `json:"preset_id"`
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

type STTSegment struct {
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Text    string `json:"text"`
}

type STTResult struct {
	PresetID  string       `json:"preset_id"`
	StreamID  string       `json:"stream_id"`
	VODID     string       `json:"vod_id,omitempty"`
	CreatedAt time.Time    `json:"created_at"`
	Segments  []STTSegment `json:"segments"`
}

type GoldenSegment struct {
	SegmentID string `json:"segment_id"`
	StartMS   int64  `json:"start_ms"`
	EndMS     int64  `json:"end_ms"`
	Text      string `json:"text"`
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
	Error     *SelectedResultError `json:"error,omitempty"`
}

type AlignmentRow struct {
	Golden GoldenSegment           `json:"golden"`
	Models map[string][]STTSegment `json:"models"`
}

type Alignment struct {
	StreamID        string           `json:"stream_id"`
	VODID           string           `json:"vod_id"`
	SelectedResults []SelectedResult `json:"selected_results"`
	Rows            []AlignmentRow   `json:"rows"`
}

type GoldenRequest struct {
	VODID          string              `json:"vod_id"`
	Mode           string              `json:"mode"`
	SourcePresetID string              `json:"source_preset_id,omitempty"`
	Segments       []GoldenEditSegment `json:"segments,omitempty"`
}

type GoldenEditSegment struct {
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Text    string `json:"text"`
}
