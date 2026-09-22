import { useEffect, useRef, useState } from "react";
import type { RefObject } from "react";
import { api } from "./api/client";
import type { Alignment, GoldenEditSegment, STTSegment } from "./domain/types";
import { FlvMediaPlayer } from "./media/player";
import { useWorkspaceData } from "./state/workspace";
import "./styles.css";

function formatTime(seconds: number): string {
  const safeSeconds = Math.max(0, Math.floor(seconds));
  const minutes = Math.floor(safeSeconds / 60);
  const remainingSeconds = safeSeconds % 60;
  return `${String(minutes).padStart(2, "0")}:${String(remainingSeconds).padStart(2, "0")}`;
}

function timestampToSeconds(timestamp: string): number {
  if (!timestamp) return 0;
  const parts = timestamp.split(":");
  if (parts.length === 3) {
    const hours = Number(parts[0]) || 0;
    const minutes = Number(parts[1]) || 0;
    const seconds = Number(parts[2]) || 0;
    return hours * 3600 + minutes * 60 + seconds;
  }
  return Number(timestamp) || 0;
}

export default function App() {
  const { presets, streams, detail, loadCatalog, loadDetail } = useWorkspaceData();
  const [filterPresetId, setFilterPresetId] = useState("");
  const [streamId, setStreamId] = useState("");
  const [vodId, setVodId] = useState("");
  const [currentTime, setCurrentTime] = useState(0);
  const [playerError, setPlayerError] = useState<string>();
  const [modelA, setModelA] = useState("");
  const [modelB, setModelB] = useState("");
  const [alignment, setAlignment] = useState<{ data?: Alignment; loading: boolean; error?: Error }>({ loading: false });
  const [alignmentRevision, setAlignmentRevision] = useState(0);
  const [editingGolden, setEditingGolden] = useState(false);
  const [goldenDraft, setGoldenDraft] = useState<GoldenEditSegment[]>([]);
  const [goldenSaving, setGoldenSaving] = useState(false);
  const videoRef = useRef<HTMLVideoElement>(null);
  const alignmentTableRef = useRef<HTMLDivElement>(null);
  const activeVod = detail.data?.vods.find((vod) => vod.vod_id === vodId) ?? detail.data?.vods[0];
  const activeVodIndex = detail.data?.vods.findIndex((vod) => vod.vod_id === activeVod?.vod_id) ?? -1;

  useEffect(() => {
    setStreamId("");
    setVodId("");
    void loadCatalog(filterPresetId || undefined);
  }, [filterPresetId, loadCatalog]);

  useEffect(() => {
    if (!streamId) return;
    void loadDetail(streamId);
    setVodId("");
  }, [streamId, loadDetail]);

  useEffect(() => {
    if (detail.data && detail.data.vods.length > 0 && !detail.data.vods.some((vod) => vod.vod_id === vodId)) {
      setVodId(detail.data.vods[0].vod_id);
    }
  }, [detail.data, vodId]);

  useEffect(() => {
    const resultIds = activeVod?.stt_results.map((result) => result.preset_id) ?? [];
    setModelA((current) => resultIds.includes(current) ? current : resultIds[0] ?? "");
    setModelB((current) => resultIds.includes(current) && current !== resultIds[0] ? current : "");
  }, [activeVod]);

  useEffect(() => {
    if (!streamId || !activeVod || !modelA) {
      setAlignment({ loading: false });
      return;
    }
    let cancelled = false;
    setAlignment({ loading: true });
    void api.getAlignment(streamId, activeVod.vod_id, [modelA, ...(modelB ? [modelB] : [])]).then((data) => {
      if (!cancelled) setAlignment({ data, loading: false });
    }).catch((error) => {
      if (!cancelled) setAlignment({ loading: false, error: error instanceof Error ? error : new Error("Unable to load alignment.") });
    });
    return () => { cancelled = true; };
  }, [streamId, activeVod, modelA, modelB, alignmentRevision]);

  useEffect(() => {
    const element = videoRef.current;
    if (!element) return;

    let player: FlvMediaPlayer | undefined;
    let cancelled = false;
    void import("mpegts.js").then((mod) => {
      if (cancelled) return;
      const mpegts = mod.default || mod;
      player = new FlvMediaPlayer(mpegts, (error) => {
        setPlayerError(error.message);
      });
      player.attach(element);
      try {
        if (!activeVod) return;
        player.load(activeVod.flv_url);
        setPlayerError(undefined);
      } catch (error) {
        setPlayerError(error instanceof Error ? error.message : "Unable to load the VOD.");
      }
    });

    return () => {
      cancelled = true;
      player?.destroy();
    };
  }, [activeVod]);

  useEffect(() => {
    setCurrentTime(0);
    setEditingGolden(false);
    setGoldenDraft([]);
  }, [activeVod?.vod_id]);

  const selectRelativeVod = (offset: number) => {
    const vods = detail.data?.vods ?? [];
    const next = vods[activeVodIndex + offset];
    if (next) setVodId(next.vod_id);
  };

  const handleEnded = () => selectRelativeVod(1);
  const persistedGolden = alignment.data?.rows.filter((row) => row.golden.segment_id);
  const canEditGolden = Boolean(persistedGolden?.length);
  const startGoldenEdit = () => {
    if (!persistedGolden) return;
    setGoldenDraft(persistedGolden.map((row) => ({ timestamps: { ...row.golden.timestamps }, text: row.golden.text })));
    setEditingGolden(true);
  };
  const updateGoldenDraft = (index: number, field: "from" | "to" | "text", value: string) => {
    setGoldenDraft((current) => current.map((segment, segmentIndex) => {
      if (segmentIndex !== index) return segment;
      if (field === "text") return { ...segment, text: value };
      return { ...segment, timestamps: { ...segment.timestamps, [field]: value } };
    }));
  };
  const saveGoldenEdits = async () => {
    if (!streamId || !activeVod) return;
    for (let index = 0; index < goldenDraft.length; index += 1) {
      const segment = goldenDraft[index];
      if (!segment.timestamps.from || !segment.timestamps.to || segment.timestamps.to <= segment.timestamps.from) {
        setAlignment({ data: alignment.data, loading: false, error: new Error(`Golden row ${index + 1} has an invalid interval.`) });
        return;
      }
      if (index > 0 && segment.timestamps.from < goldenDraft[index - 1].timestamps.to) {
        setAlignment({ data: alignment.data, loading: false, error: new Error(`Golden row ${index + 1} overlaps the previous row.`) });
        return;
      }
    }
    setGoldenSaving(true);
    try {
      await api.editGolden(streamId, activeVod.vod_id, goldenDraft);
      setEditingGolden(false);
      setAlignmentRevision((revision) => revision + 1);
    } catch (error) {
      setAlignment({ data: alignment.data, loading: false, error: error instanceof Error ? error : new Error("Unable to save Golden.") });
    } finally {
      setGoldenSaving(false);
    }
  };
  const renewGolden = async () => {
    if (!streamId || !activeVod || !modelA) return;
    setGoldenSaving(true);
    try {
      await api.renewGolden(streamId, activeVod.vod_id, modelA);
      setAlignmentRevision((revision) => revision + 1);
    } catch (error) {
      setAlignment({ data: alignment.data, loading: false, error: error instanceof Error ? error : new Error("Unable to renew Golden.") });
    } finally {
      setGoldenSaving(false);
    }
  };
  const seekTo = (timestampStr: string, alignmentRowIndex?: number) => {
    if (!videoRef.current) return;
    videoRef.current.currentTime = timestampToSeconds(timestampStr);
    void videoRef.current.play().catch(() => {
      setPlayerError("瀏覽器阻擋自動播放，請按播放鍵繼續。");
    });
    if (alignmentRowIndex === undefined) return;
    scrollAlignmentToRow(alignmentRowIndex);
  };
  const scrollAlignmentToTimestamp = (currentTimeSec: number) => {
    const container = alignmentTableRef.current;
    if (!container) return;
    const rows = Array.from(container.querySelectorAll<HTMLElement>("[data-alignment-row-index]"));
    const matchingRow = rows.find((row) => {
      const start = timestampToSeconds(row.dataset.alignmentStart ?? "00:00:00.000");
      const end = timestampToSeconds(row.dataset.alignmentEnd ?? "00:00:00.000");
      return currentTimeSec >= start && currentTimeSec < end;
    });
    const nextRow = rows.find((row) => currentTimeSec < timestampToSeconds(row.dataset.alignmentStart ?? "00:00:00.000"));
    const row = matchingRow ?? nextRow ?? rows.at(-1);
    if (row) scrollAlignmentToRow(Number(row.dataset.alignmentRowIndex));
  };
  const scrollAlignmentToRow = (alignmentRowIndex: number) => {
    requestAnimationFrame(() => {
      const row = alignmentTableRef.current?.querySelector<HTMLElement>(`[data-alignment-row-index="${alignmentRowIndex}"]`);
      const container = alignmentTableRef.current;
      if (!row || !container) return;
      const header = container.querySelector<HTMLElement>(".alignment-header");
      const targetTop = Math.max(
        0,
        row.getBoundingClientRect().top - container.getBoundingClientRect().top + container.scrollTop - (header?.offsetHeight ?? 0),
      );
      container.scrollTo({ top: targetTop, behavior: "auto" });
    });
  };

  const catalogMessage = streams.error?.message ?? (streams.loading ? "Loading streams…" : streams.data.length === 0 ? "目前沒有可用的直播間。" : undefined);

  return (
    <main className="app-shell">
      <header className="app-header">
        <div>
          <p className="eyebrow">STT WORKBENCH · V1</p>
          <h1>STT 比較與 Golden 編輯</h1>
        </div>
      </header>

      <section className="control-bar" aria-label="workspace selectors">
        <label>
          Preset filter
          <select value={filterPresetId} onChange={(event) => setFilterPresetId(event.target.value)}>
            <option value="">All streams</option>
            {presets.data.map((preset) => <option key={preset.preset_id} value={preset.preset_id}>{preset.name || preset.model.name}</option>)}
          </select>
        </label>
        <label>
          直播間
          <select value={streamId} onChange={(event) => setStreamId(event.target.value)} disabled={streams.loading || streams.data.length === 0}>
            <option value="">{streams.loading ? "Loading…" : "選擇直播間"}</option>
            {streams.data.map((stream) => <option key={stream.stream_id} value={stream.stream_id}>{stream.stream_id}</option>)}
          </select>
        </label>
        <label>
          Model A
          <select value={modelA} onChange={(event) => setModelA(event.target.value)} disabled={!activeVod || activeVod.stt_results.length === 0}>
            <option value="">{activeVod?.stt_results.length ? "選擇 Model A" : "No STT result"}</option>
            {activeVod?.stt_results.map((result) => <option key={result.preset_id} value={result.preset_id}>{result.name || result.model.name}</option>)}
          </select>
        </label>
        <label>
          Model B <span className="optional">optional</span>
          <select value={modelB} onChange={(event) => setModelB(event.target.value)} disabled={!activeVod || activeVod.stt_results.length < 2}>
            <option value="">None</option>
            {activeVod?.stt_results.filter((result) => result.preset_id !== modelA).map((result) => <option key={result.preset_id} value={result.preset_id}>{result.name || result.model.name}</option>)}
          </select>
        </label>
      </section>

      {catalogMessage && <p className="global-message">{catalogMessage}</p>}

      <section className="workspace">
        <article className="video-panel panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">ACTIVE VOD</p>
              <h2>{activeVod?.vod_id ?? "尚未選擇 VOD"}</h2>
            </div>
          </div>
          <video ref={videoRef} controls playsInline onTimeUpdate={(event) => setCurrentTime(event.currentTarget.currentTime)} onSeeked={(event) => scrollAlignmentToTimestamp(event.currentTarget.currentTime)} onEnded={handleEnded} />
          {playerError && <p className="player-error">{playerError}</p>}
          {!activeVod && <p className="helper-text">選擇直播間與 VOD 後開始播放。</p>}
          {activeVod && (
            <div className="player-meta">
              <button type="button" onClick={() => selectRelativeVod(-1)} disabled={activeVodIndex <= 0}>← Previous VOD</button>
              <span>{formatTime(currentTime)} / {formatTime(activeVod.duration_ms / 1000)}</span>
              <button type="button" onClick={() => selectRelativeVod(1)} disabled={activeVodIndex < 0 || activeVodIndex >= (detail.data?.vods.length ?? 1) - 1}>Next VOD →</button>
            </div>
          )}
        </article>

        <article className="alignment-panel panel">
          <div className="panel-heading">
            <div>
              <p className="eyebrow">ALIGNMENT</p>
              <h2>Golden / Model A / Model B</h2>
            </div>
            <div className="alignment-actions">
              <span className="muted">{alignment.loading ? "Loading…" : alignment.data ? `${alignment.data.rows.length} rows` : "No alignment loaded"}</span>
              {alignment.data && <button type="button" onClick={renewGolden} disabled={goldenSaving || !modelA}>{goldenSaving ? "Saving…" : "Renew Golden"}</button>}
              {canEditGolden && !editingGolden && <button type="button" onClick={startGoldenEdit} disabled={goldenSaving}>Edit</button>}
              {editingGolden && <><button type="button" onClick={saveGoldenEdits} disabled={goldenSaving}>{goldenSaving ? "Saving…" : "Save"}</button><button type="button" onClick={() => setEditingGolden(false)} disabled={goldenSaving}>Cancel</button></>}
            </div>
          </div>
          {alignment.error && <p className="global-message">{alignment.error.message}</p>}
          {!alignment.data && !alignment.loading && <div className="empty-state"><span className="empty-icon">↔</span><p>選擇 VOD 與 Model A 後載入 alignment。</p></div>}
          {alignment.loading && <div className="empty-state"><span className="empty-icon">…</span><p>載入 alignment…</p></div>}
          {alignment.data && <AlignmentTable alignment={alignment.data} modelA={modelA} modelB={modelB} onSeek={seekTo} alignmentTableRef={alignmentTableRef} editing={editingGolden} draft={goldenDraft} onDraftChange={updateGoldenDraft} />}
        </article>
      </section>
    </main>
  );
}

function AlignmentTable({
  alignment,
  modelA,
  modelB,
  onSeek,
  alignmentTableRef,
  editing,
  draft,
  onDraftChange,
}: {
  alignment: Alignment;
  modelA: string;
  modelB: string;
  onSeek: (timestampStr: string, alignmentRowIndex?: number) => void;
  alignmentTableRef: RefObject<HTMLDivElement | null>;
  editing: boolean;
  draft: GoldenEditSegment[];
  onDraftChange: (index: number, field: "from" | "to" | "text", value: string) => void;
}) {
  const model = (segment: STTSegment) => (
    <button type="button" className="segment-card segment-button segment-card-model" onClick={() => onSeek(segment.timestamps.from)}>
      <span className="segment-timestamps">{segment.timestamps.from}–{segment.timestamps.to}</span>
      <span className="segment-text">{segment.text || "(empty)"}</span>
    </button>
  );
  let goldenIndex = 0;
  const golden = (row: Alignment["rows"][number], rowIndex: number) => {
    if (!editing || !row.golden.segment_id) {
      return (
        <button type="button" className="segment-card golden-button segment-card-golden" onClick={() => onSeek(row.golden.timestamps.from, rowIndex)}>
          <span className="segment-timestamps">{row.golden.timestamps.from}–{row.golden.timestamps.to}</span>
          <span className="segment-text">{row.golden.text || "(empty Golden)"}</span>
        </button>
      );
    }
    const draftIndex = goldenIndex++;
    const segment = draft[draftIndex] ?? row.golden;
    return (
      <div className="segment-card golden-editor">
        <div className="time-fields">
          <input aria-label="Golden start" value={segment.timestamps.from} onChange={(event) => onDraftChange(draftIndex, "from", event.target.value)} />
          <span>–</span>
          <input aria-label="Golden end" value={segment.timestamps.to} onChange={(event) => onDraftChange(draftIndex, "to", event.target.value)} />
        </div>
        <input aria-label="Golden text" value={segment.text} onChange={(event) => onDraftChange(draftIndex, "text", event.target.value)} />
      </div>
    );
  };
  return (
    <div className="alignment-table" ref={alignmentTableRef}>
      <div className="alignment-row alignment-header">
        <div className="alignment-cell"><strong>Golden</strong></div>
        <div className="alignment-cell"><strong>Model A</strong></div>
        {modelB && <div className="alignment-cell"><strong>Model B</strong></div>}
      </div>
      {alignment.rows.map((row, index) => (
        <div
          className="alignment-row"
          data-alignment-row-index={index}
          data-alignment-start={row.golden.timestamps.from}
          data-alignment-end={row.golden.timestamps.to}
          key={`${row.golden.segment_id ?? "unmatched"}-${row.golden.timestamps.from}-${index}`}
        >
          <div className="alignment-cell alignment-cell-golden">{golden(row, index)}</div>
          <div className="alignment-cell alignment-cell-model">
            {(row.models[modelA] ?? []).map((segment, segmentIndex) => (
              <span key={`${segment.timestamps.from}-${segmentIndex}`}>{model(segment)}</span>
            ))}
          </div>
          {modelB && (
            <div className="alignment-cell alignment-cell-model">
              {(row.models[modelB] ?? []).map((segment, segmentIndex) => (
                <span key={`${segment.timestamps.from}-${segmentIndex}`}>{model(segment)}</span>
              ))}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
