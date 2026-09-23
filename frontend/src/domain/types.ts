export type ModelConfig = {
  name: string;
  params: Record<string, unknown>;
};

export type Preset = {
  preset_id: string;
  model: ModelConfig;
  created_at: string;
  updated_at: string;
  stream_ids: string[];
};

export type Stream = { stream_id: string };

export type STTResultSummary = {
  preset_id: string;
  model: ModelConfig;
  created_at: string;
};

export type VOD = {
  vod_id: string;
  file_id: string;
  flv_url: string;
  wav_url: string;
  sequence: number;
  start_time_unix_s: number;
  duration_ms: number;
  timeline_start_ms: number;
  timeline_end_ms: number;
  stt_results: STTResultSummary[];
};

export type StreamDetail = { stream_id: string; vods: VOD[] };

export type STTSegment = { start_ms: number; end_ms: number; text: string };
export type GoldenSegment = STTSegment & { segment_id?: string };
export type GoldenEditSegment = STTSegment;
export type Golden = {
  stream_id: string;
  vod_id: string;
  base_preset_id: string;
  updated_at: string;
  segments: GoldenSegment[];
};
export type SelectedResult = {
  preset_id: string;
  model: ModelConfig;
  created_at: string;
  error: { code: string; message: string } | null;
};
export type AlignmentRow = {
  golden: GoldenSegment;
  models: Record<string, STTSegment[]>;
};
export type Alignment = {
  stream_id: string;
  vod_id: string;
  selected_results: SelectedResult[];
  rows: AlignmentRow[];
};

export type ApiError = {
  error: { code: string; message: string; details: Record<string, unknown> };
};
