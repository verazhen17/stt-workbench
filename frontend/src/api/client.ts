import type { Alignment, ApiError, Golden, GoldenEditSegment, Preset, Stream, StreamDetail } from "../domain/types";

export class ApiRequestError extends Error {
  constructor(
    public readonly status: number,
    public readonly code: string,
    message: string,
    public readonly details: Record<string, unknown> = {},
  ) {
    super(message);
    this.name = "ApiRequestError";
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    headers: { Accept: "application/json", ...init?.headers },
    ...init,
  });
  const payload = (await response.json().catch(() => undefined)) as T | ApiError | undefined;
  if (!response.ok) {
    const error = payload as ApiError | undefined;
    throw new ApiRequestError(
      response.status,
      error?.error.code ?? "internal_error",
      error?.error.message ?? "The request failed.",
      error?.error.details,
    );
  }
  return payload as T;
}

export const api = {
  listPresets: () => request<{ presets: Preset[] }>("/api/presets"),
  listStreams: (presetId?: string) => {
    const query = presetId ? `?preset_id=${encodeURIComponent(presetId)}` : "";
    return request<{ streams: Stream[] }>(`/api/streams${query}`);
  },
  getStreamDetail: (streamId: string) =>
    request<StreamDetail>(`/api/streams/${encodeURIComponent(streamId)}`),
  getAlignment: (streamId: string, vodId: string, presetIds: string[]) => {
    const query = new URLSearchParams({ vod_id: vodId, preset_ids: presetIds.join(",") });
    return request<Alignment>(`/api/streams/${encodeURIComponent(streamId)}/align?${query}`);
  },
  renewGolden: (streamId: string, vodId: string, sourcePresetId: string) =>
    saveGolden(streamId, { vod_id: vodId, mode: "renew", source_preset_id: sourcePresetId }),
  editGolden: (streamId: string, vodId: string, segments: GoldenEditSegment[]) =>
    saveGolden(streamId, { vod_id: vodId, mode: "edit", segments }),
};

function saveGolden(streamId: string, body: { vod_id: string; mode: "renew" | "edit"; source_preset_id?: string; segments?: GoldenEditSegment[] }) {
  return request<Golden>(`/api/streams/${encodeURIComponent(streamId)}/golden`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}
