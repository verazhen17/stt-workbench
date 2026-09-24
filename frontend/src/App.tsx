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
  const [activeRowIndex, setActiveRowIndex] = useState<number | null>(null);
  const activeRowIndexRef = useRef<number | null>(null);
  const [isAutoScrollPaused, setIsAutoScrollPaused] = useState(false);
  const isAutoScrollPausedRef = useRef(false);
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
    let nextA = "";
    setModelA((current) => {
      nextA = resultIds.includes(current) ? current : resultIds[0] ?? "";
      return nextA;
    });
    setModelB((current) => (resultIds.includes(current) && current !== nextA ? current : ""));
  }, [activeVod]);

  useEffect(() => {
    if (!streamId || !activeVod || !modelA) {
      setAlignment({ loading: false });
      return;
    }
    const candidateId = modelB && modelB !== modelA ? modelB : undefined;
    const presetIds = candidateId ? [modelA, candidateId] : [modelA];

    let cancelled = false;
    setAlignment({ loading: true });
    void api.getAlignment(streamId, activeVod.vod_id, presetIds).then((data) => {
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
    setActiveRowIndex(null);
    activeRowIndexRef.current = null;
    setIsAutoScrollPaused(false);
    isAutoScrollPausedRef.current = false;
    setEditingGolden(false);
    setGoldenDraft([]);
  }, [activeVod?.vod_id]);

  const handleBaselineChange = (nextModelA: string) => {
    setModelA(nextModelA);
    if (nextModelA && nextModelA === modelB) {
      setModelB("");
    }
  };

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
    pauseAutoScroll();
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
  const clearGolden = async () => {
    if (!streamId || !activeVod || !alignment.data || alignment.data.rows.length === 0) return;
    if (editingGolden) {
      setGoldenDraft((current) => current.map((segment) => ({ ...segment, text: "" })));
      return;
    }
    setGoldenSaving(true);
    try {
      if (!canEditGolden) {
        if (!modelA) return;
        await api.renewGolden(streamId, activeVod.vod_id, modelA);
      }
      const baseRows = alignment.data.rows;
      const segmentsToClear = baseRows.map((row) => ({
        timestamps: { ...row.golden.timestamps },
        text: "",
      }));
      await api.editGolden(streamId, activeVod.vod_id, segmentsToClear);
      setAlignmentRevision((revision) => revision + 1);
    } catch (error) {
      setAlignment({ data: alignment.data, loading: false, error: error instanceof Error ? error : new Error("Unable to clear Golden.") });
    } finally {
      setGoldenSaving(false);
    }
  };

  const pauseAutoScroll = () => {
    if (!isAutoScrollPausedRef.current) {
      isAutoScrollPausedRef.current = true;
      setIsAutoScrollPaused(true);
    }
  };

  const resumeAutoScroll = (seekTimestamp?: number, targetIndex?: number) => {
    isAutoScrollPausedRef.current = false;
    setIsAutoScrollPaused(false);
    if (targetIndex !== undefined) {
      activeRowIndexRef.current = targetIndex;
      setActiveRowIndex(targetIndex);
      scrollAlignmentToRow(targetIndex, "auto");
    } else if (seekTimestamp !== undefined) {
      scrollAlignmentToTimestamp(seekTimestamp, "auto");
    } else if (videoRef.current) {
      scrollAlignmentToTimestamp(videoRef.current.currentTime, "smooth");
    }
  };

  const findRowIndexForTimestamp = (currentTimeSec: number): number | null => {
    const rows = alignment.data?.rows;
    if (!rows || rows.length === 0) return null;

    for (let i = 0; i < rows.length; i += 1) {
      const start = timestampToSeconds(rows[i].golden.timestamps.from);
      const end = timestampToSeconds(rows[i].golden.timestamps.to);
      if (currentTimeSec >= start && currentTimeSec < end) {
        return i;
      }
    }

    const firstStart = timestampToSeconds(rows[0].golden.timestamps.from);
    if (currentTimeSec < firstStart) {
      return null;
    }

    for (let i = rows.length - 1; i >= 0; i -= 1) {
      const start = timestampToSeconds(rows[i].golden.timestamps.from);
      if (currentTimeSec >= start) {
        return i;
      }
    }
    return null;
  };

  const seekTo = (timestampStr: string, alignmentRowIndex?: number) => {
    if (!videoRef.current) return;
    const timeSec = timestampToSeconds(timestampStr);
    videoRef.current.currentTime = timeSec;
    void videoRef.current.play().then(() => {
      setPlayerError(undefined);
    }).catch((error: unknown) => {
      if (error instanceof Error && error.name === "NotAllowedError") {
        setPlayerError("Autoplay was blocked by browser. Press play to start.");
      } else if (error instanceof Error && error.name !== "AbortError") {
        setPlayerError(error.message);
      }
    });
    const targetIndex = alignmentRowIndex ?? findRowIndexForTimestamp(timeSec) ?? undefined;
    resumeAutoScroll(timeSec, targetIndex);
  };

  const scrollAlignmentToTimestamp = (currentTimeSec: number, behavior: ScrollBehavior = "auto") => {
    const targetIndex = findRowIndexForTimestamp(currentTimeSec) ?? 0;
    activeRowIndexRef.current = targetIndex;
    setActiveRowIndex(targetIndex);
    scrollAlignmentToRow(targetIndex, behavior);
  };

  const scrollAlignmentToRow = (alignmentRowIndex: number, behavior: ScrollBehavior = "auto") => {
    requestAnimationFrame(() => {
      const row = alignmentTableRef.current?.querySelector<HTMLElement>(`[data-alignment-row-index="${alignmentRowIndex}"]`);
      const container = alignmentTableRef.current;
      if (!row || !container) return;
      const header = container.querySelector<HTMLElement>(".alignment-header");
      const targetTop = Math.max(
        0,
        row.getBoundingClientRect().top - container.getBoundingClientRect().top + container.scrollTop - (header?.offsetHeight ?? 0),
      );
      container.scrollTo({ top: targetTop, behavior });
    });
  };

  const handleTimeUpdate = (currentTimeSec: number) => {
    setCurrentTime(currentTimeSec);
    const newActiveIndex = findRowIndexForTimestamp(currentTimeSec);
    if (newActiveIndex !== activeRowIndexRef.current) {
      activeRowIndexRef.current = newActiveIndex;
      setActiveRowIndex(newActiveIndex);
      if (!isAutoScrollPausedRef.current && !editingGolden && newActiveIndex !== null) {
        scrollAlignmentToRow(newActiveIndex, "smooth");
      }
    }
  };

  const getPresetName = (presetId: string): string => {
    if (!presetId) return "";
    const activeResult = activeVod?.stt_results.find((result) => result.preset_id === presetId);
    if (activeResult?.name || activeResult?.model?.name) {
      return activeResult.name || activeResult.model.name;
    }
    const preset = presets.data.find((p) => p.preset_id === presetId);
    if (preset?.name || preset?.model?.name) {
      return preset.name || preset.model.name;
    }
    return presetId;
  };

  const modelAName = getPresetName(modelA);
  const modelBName = getPresetName(modelB);
  const catalogMessage = streams.error?.message ?? (streams.loading ? "Loading streams…" : streams.data.length === 0 ? "No streams available." : undefined);

  return (
    <main className="app-shell">
      <header className="app-header">
        <div>
          <h1>STT Comparison & Golden Editor</h1>
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
          Stream
          <select value={streamId} onChange={(event) => setStreamId(event.target.value)} disabled={streams.loading || streams.data.length === 0}>
            <option value="">{streams.loading ? "Loading…" : "Select stream"}</option>
            {streams.data.map((stream) => <option key={stream.stream_id} value={stream.stream_id}>{stream.stream_id}</option>)}
          </select>
        </label>
        <label>
          Baseline Model (Control)
          <select value={modelA} onChange={(event) => handleBaselineChange(event.target.value)} disabled={!activeVod || activeVod.stt_results.length === 0}>
            <option value="">{activeVod?.stt_results.length ? "Select Baseline Model" : "No STT result"}</option>
            {activeVod?.stt_results.map((result) => <option key={result.preset_id} value={result.preset_id}>{result.name || result.model.name}</option>)}
          </select>
        </label>
        <label>
          Candidate Model (Experimental) <span className="optional">optional</span>
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
              <h2>{activeVod?.vod_id ?? "No VOD selected"}</h2>
            </div>
          </div>
          <video
            ref={videoRef}
            controls
            playsInline
            onPlay={() => setPlayerError(undefined)}
            onTimeUpdate={(event) => handleTimeUpdate(event.currentTarget.currentTime)}
            onSeeked={(event) => resumeAutoScroll(event.currentTarget.currentTime)}
            onEnded={handleEnded}
          />
          {playerError && <p className="player-error">{playerError}</p>}
          {!activeVod && <p className="helper-text">Select a stream and VOD to begin playback.</p>}
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
              <h2>Golden / Baseline / Candidate</h2>
            </div>
            <div className="alignment-actions">
              <span className="muted">{alignment.loading ? "Loading…" : alignment.data ? `${alignment.data.rows.length} rows` : "No alignment loaded"}</span>
              {isAutoScrollPaused && alignment.data && !editingGolden && (
                <button
                  type="button"
                  className="resume-scroll-button"
                  title="Resume following video playback"
                  onClick={() => resumeAutoScroll()}
                >
                  ▶ Resume Auto-scroll
                </button>
              )}
              {alignment.data && !editingGolden && (
                <button
                  type="button"
                  title="Clicking this button will copy the Baseline model's results into Golden."
                  onClick={renewGolden}
                  disabled={goldenSaving || !modelA}
                >
                  {goldenSaving ? "Saving…" : "Overwrite Golden by Baseline"}
                </button>
              )}
              {canEditGolden && !editingGolden && <button type="button" onClick={startGoldenEdit} disabled={goldenSaving}>Edit</button>}
              {editingGolden && (
                <>
                  <button
                    type="button"
                    title="Clears the text of all Golden segments."
                    onClick={clearGolden}
                    disabled={goldenSaving}
                  >
                    Clear Golden
                  </button>
                  <button type="button" onClick={saveGoldenEdits} disabled={goldenSaving}>{goldenSaving ? "Saving…" : "Save"}</button>
                  <button type="button" onClick={() => setEditingGolden(false)} disabled={goldenSaving}>Cancel</button>
                </>
              )}
            </div>
          </div>
          {alignment.error && <p className="global-message">{alignment.error.message}</p>}
          {alignment.data?.warnings && alignment.data.warnings.length > 0 && (
            <p className="global-message global-warning">
              Golden timestamps overlap. Rows with a red outline contain overlapping timestamps.
            </p>
          )}
          {!alignment.data && !alignment.loading && <div className="empty-state"><span className="empty-icon">↔</span><p>Select a VOD and Baseline Model to load alignment.</p></div>}
          {alignment.loading && <div className="empty-state"><span className="empty-icon">…</span><p>Loading alignment…</p></div>}
          {alignment.data && (
            <AlignmentTable
              alignment={alignment.data}
              modelA={modelA}
              modelB={modelB}
              modelAName={modelAName}
              modelBName={modelBName}
              onSeek={seekTo}
              alignmentTableRef={alignmentTableRef}
              editing={editingGolden}
              draft={goldenDraft}
              onDraftChange={updateGoldenDraft}
              activeRowIndex={activeRowIndex}
              onUserScroll={pauseAutoScroll}
            />
          )}
        </article>
      </section>
    </main>
  );
}

function AlignmentTable({
  alignment,
  modelA,
  modelB,
  modelAName,
  modelBName,
  onSeek,
  alignmentTableRef,
  editing,
  draft,
  onDraftChange,
  activeRowIndex,
  onUserScroll,
}: {
  alignment: Alignment;
  modelA: string;
  modelB: string;
  modelAName?: string;
  modelBName?: string;
  onSeek: (timestampStr: string, alignmentRowIndex?: number) => void;
  alignmentTableRef: RefObject<HTMLDivElement | null>;
  editing: boolean;
  draft: GoldenEditSegment[];
  onDraftChange: (index: number, field: "from" | "to" | "text", value: string) => void;
  activeRowIndex?: number | null;
  onUserScroll?: () => void;
}) {
  const goldenWarningIds = new Set((alignment.warnings ?? []).flatMap((warning) => [warning.segment_id, warning.previous_segment_id]).filter((id): id is string => Boolean(id)));
  const model = (segment: STTSegment) => (
    <button type="button" className="segment-card segment-button segment-card-model" onClick={() => onSeek(segment.timestamps.from)}>
      <span className="segment-timestamps">{segment.timestamps.from}–{segment.timestamps.to}</span>
      <span className="segment-text">{segment.text || "(empty)"}</span>
    </button>
  );
  let goldenIndex = 0;
  const golden = (row: Alignment["rows"][number], rowIndex: number, hasWarning: boolean) => {
    if (!editing || !row.golden.segment_id) {
      return (
        <button type="button" className={`segment-card golden-button segment-card-golden${hasWarning ? " segment-card-warning" : ""}`} title={hasWarning ? "These Golden timestamps overlap another Golden segment." : undefined} onClick={() => onSeek(row.golden.timestamps.from, rowIndex)}>
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
    <div
      className="alignment-table"
      ref={alignmentTableRef}
      onWheel={onUserScroll}
      onTouchMove={onUserScroll}
    >
      <div className="alignment-row alignment-header">
        <div className="alignment-cell">
          <strong>Golden</strong>
        </div>
        <div className="alignment-cell">
          <strong>Baseline (Control)</strong>
          {modelAName && <span className="header-preset-name">{modelAName}</span>}
        </div>
        {modelB && (
          <div className="alignment-cell">
            <strong>Candidate (Experimental)</strong>
            {modelBName && <span className="header-preset-name">{modelBName}</span>}
          </div>
        )}
      </div>
      {alignment.rows.map((row, index) => {
        const isActive = activeRowIndex === index;
        return (
          <div
            className={`alignment-row${isActive ? " alignment-row-active" : ""}${goldenWarningIds.has(row.golden.segment_id ?? "") ? " alignment-row-warning" : ""}`}
            data-alignment-row-index={index}
            data-alignment-start={row.golden.timestamps.from}
            data-alignment-end={row.golden.timestamps.to}
            key={`${row.golden.segment_id ?? "unmatched"}-${row.golden.timestamps.from}-${index}`}
          >
            <div className="alignment-cell alignment-cell-golden">{golden(row, index, goldenWarningIds.has(row.golden.segment_id ?? ""))}</div>
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
        );
      })}
    </div>
  );
}
