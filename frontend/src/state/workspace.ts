import { useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import type { Preset, Stream, StreamDetail } from "../domain/types";

type AsyncState<T> = {
  data: T;
  loading: boolean;
  error?: Error;
};

export function useWorkspaceData() {
  const [presets, setPresets] = useState<AsyncState<Preset[]>>({ data: [], loading: true });
  const [streams, setStreams] = useState<AsyncState<Stream[]>>({ data: [], loading: true });
  const [detail, setDetail] = useState<AsyncState<StreamDetail | undefined>>({ data: undefined, loading: false });

  const loadCatalog = useCallback(async (presetId?: string) => {
    setStreams((current) => ({ ...current, loading: true, error: undefined }));
    try {
      const response = await api.listStreams(presetId);
      setStreams({ data: response.streams, loading: false });
    } catch (error) {
      setStreams({ data: [], loading: false, error: error instanceof Error ? error : new Error("Unable to load streams.") });
    }
  }, []);

  const loadDetail = useCallback(async (streamId: string) => {
    setDetail({ data: undefined, loading: true });
    try {
      setDetail({ data: await api.getStreamDetail(streamId), loading: false });
    } catch (error) {
      setDetail({ data: undefined, loading: false, error: error instanceof Error ? error : new Error("Unable to load stream.") });
    }
  }, []);

  useEffect(() => {
    void Promise.all([
      api.listPresets().then((response) => setPresets({ data: response.presets, loading: false })).catch((error) => {
        setPresets({ data: [], loading: false, error: error instanceof Error ? error : new Error("Unable to load presets.") });
      }),
      loadCatalog(),
    ]);
  }, [loadCatalog]);

  return { presets, streams, detail, loadCatalog, loadDetail };
}
