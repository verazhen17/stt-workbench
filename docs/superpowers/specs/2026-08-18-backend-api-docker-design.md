# Backend API and Docker Design

**Date:** 2026-08-18  
**Status:** Approved design  
**Scope:** STT Workbench v1 Backend APIs, preset-first discovery, and container runtime

## 1. Purpose and source documents

Required references：

- `docs/spec-v1.md`: product scope, domain semantics, decisions, milestones.
- `docs/api-spec.md`: HTTP API and storage contract.
- `docs/ui-webview.md`: preset-first UI behavior and API consumption.
- This document: Backend implementation and runtime boundaries.

No document silently supersedes another. Conflict resolution must be recorded in the `spec-v1.md` decision log and synchronized across all affected documents. The existing Golden validation `fields` versus common error `details` difference is outside this preset change.

## 2. Confirmed decisions

- Backend uses Go and Gin; filesystem only, no database.
- Nginx is the only external entry point, publishes `8888`, serves `/vod/*`, and proxies `/api/*` to Backend `8080`.
- VOD, preset manifests, and STT results are read-only to Backend; only Golden is writable.
- v1 has no `/healthz`, Compose healthcheck, authentication, Golden history, or media merge. GCE production uses host-managed Cloud Storage FUSE; containers do not mount GCS themselves.
- STT runner generates opaque UUID `preset_id` values and owns writes to manifests, results, and completed indexes.
- Preset manifest at `data/presets/{preset_id}.json` contains exactly `preset_id`, nested `model{name,params}`, completed `stream_ids`, `created_at`, and `updated_at`; it is the sole config source of truth and has no provider field.
- Result identity is `(preset_id, stream_id)` and canonical storage is `data/stt/{stream_id}/{preset_id}.json`.
- Same-pair rerun atomically overwrites the canonical result and leaves no result history.
- Public API and storage schemas do not expose another execution identity. Result JSON contains exactly `preset_id`, `stream_id`, completion `created_at`, and `segments[start_ms,end_ms,text]`; it has no model/params, segment ID, or update timestamp.
- Manifest `stream_ids` means completed streams and is deduplicated/ascending sorted by the runner; Backend normalizes response order without writing it.
- Orphan results are not selectable. Manifest/index claims that cannot resolve to a valid canonical result produce `preset_index_inconsistent`.
- Preset filter is optional. No filter returns all VOD streams; selected filter returns VOD directories intersected with selectable completed results.
- Stream detail always returns all selectable results for that stream, regardless of the UI filter.
- Golden storage is flat `data/golden/{stream_id}.json` with exactly `stream_id`, `base_preset_id`, `updated_at`, and `segments[segment_id,start_ms,end_ms,text]`.
- Alignment ownership, unmatched rows, effective Golden, and Golden edit constraints remain unchanged except that lineage/selection uses preset identity.
- `backend/main.go` is the only entrypoint; `backend/router` owns HTTP/runtime; `backend/models` owns shared types; `backend/domain` owns business services; `backend/samples` owns fixtures.

## 3. Runtime architecture

```text
/app/data/
├── vod/{stream_id}/*.flv
├── presets/{preset_id}.json
├── stt/{stream_id}/{preset_id}.json
└── golden/{stream_id}.json
```

Environment contract：

```text
HTTP_ADDR=:8080
VOD_ROOT=/app/data/vod
PRESET_ROOT=/app/data/presets
STT_ROOT=/app/data/stt
GOLDEN_ROOT=/app/data/golden
VOD_URL_PREFIX=/vod
FFPROBE_PATH=/usr/bin/ffprobe
```

Local development Compose mounts：

```text
backend:  ./data/vod:/app/data/vod:ro
backend:  ./data/presets:/app/data/presets:ro
backend:  ./data/stt:/app/data/stt:ro
backend:  ./data/golden:/app/data/golden:rw
frontend: ./data/vod:/srv/vod:ro
```

Backend never modifies runner-owned preset/result artifacts. The runner may atomically replace them outside the Backend container.

### 3.1 GCE Linux production storage

- One Cloud Storage bucket uses distinct `vod/`, `presets/`, `stt/`, and `golden/` object prefixes.
- The GCE VM uses an attached service account; no static credential file is copied into containers.
- Host Cloud Storage FUSE mounts each prefix to a separate host path. Compose then bind-mounts VOD/presets/STT as `:ro` and Golden as `:rw`; Nginx receives the VOD host mount as `:ro`.
- Grant `roles/storage.objectViewer` at bucket scope. Grant `roles/storage.objectUser` with an IAM condition restricting resource names to the bucket's `golden/` object prefix.
- Backend is the single Golden writer. Golden publication writes a complete object; the production contract does not rely on or claim POSIX atomic rename semantics through Cloud Storage FUSE.
- Local development continues to use local `./data/*` directories; local implementation may use temp-file rename without making it a GCS guarantee.

## 4. Component boundaries

- **Models:** shared Preset, STT result metadata/content, Stream, VOD, Golden, and Alignment structures.
- **Preset catalog:** validates exact manifest schema and filename/body UUID identity, returns nested `model.params`, and normalizes completed `stream_ids`.
- **Stream catalog:** lists first-level VOD directories; optional preset service intersects VOD streams with completed/selectable results.
- **Result catalog:** resolves only `stt/{stream_id}/{preset_id}.json`, verifies path/body identity, and never reads config from result.
- **Index consistency service:** classifies orphan results and detects completed-index claims with missing/invalid results.
- **VOD discovery:** parses FLV ordering fields, probes duration, builds cumulative timeline, and emits `/vod` URLs.
- **Alignment:** pure deterministic half-open ownership/unmatched logic using one or two selected results.
- **Golden persistence:** reads/writes flat Golden objects, retains segment IDs on edit, records preset lineage, and enforces single-writer complete-object publication.
- **Router:** query/body validation, HTTP error mapping, response encoding, middleware, logging, recovery, and lifecycle.

Filesystem and duration probing remain narrow interfaces for temporary files and fakes.

## 5. Source and sample layout

```text
backend/
├── main.go
├── models/
│   ├── types.go
│   └── types_test.go
├── router/
│   ├── router.go
│   └── router_test.go
├── domain/
│   ├── streams.go
│   ├── vods.go
│   ├── presets.go
│   ├── results.go
│   ├── alignment.go
│   ├── golden.go
│   └── *_test.go
└── samples/
    ├── samples_test.go
    ├── presets/
    ├── vod/
    ├── stt/
    └── golden/
```

## 6. Normalized behavior

### 6.1 Runner atomic write order

1. For a new preset, runner publishes `{preset_id}.json` with empty `stream_ids`; `created_at` and `updated_at` are initially equal.
2. Runner atomically replaces the canonical result using the backing storage's complete-object semantics. The result `created_at` is the current completion time on every rerun.
3. Only after result replacement succeeds, runner deduplicates/sorts manifest `stream_ids`, updates manifest `updated_at`, and replaces the complete manifest object.
4. Same-pair rerun creates no historical filename/revision. Local filesystem may use temp-file rename; Cloud Storage uses complete object replacement rather than a POSIX rename guarantee.

Crash implications are deterministic：result-before-index leaves an unselectable orphan; index-before-result is prohibited. A manifest claiming completion without a valid result maps to `preset_index_inconsistent`.

### 6.2 Preset and stream discovery

- `GET /api/presets` reads manifests only; results cannot create or override preset config.
- `GET /api/streams` lists all VOD directories.
- `GET /api/streams?preset_id=...` validates one UUID and manifest, then intersects VOD directories, normalized completed `stream_ids`, and valid canonical results.
- Duplicate completed IDs collapse to one sorted stream ID.
- Missing manifest makes any same-named result orphaned and unselectable.

### 6.3 Stream detail and result validation

- Detail response field is `stt_results`.
- Detail returns every selectable result for the stream, independent of active filter.
- Metadata display combines result `created_at` with manifest nested `model.name`/`model.params`.
- Segment validation is deferred until Align/Renew.
- Manifest completion with missing, malformed, or identity-mismatched result is `preset_index_inconsistent`.

### 6.4 Alignment

- Query uses one or two different UUID `preset_ids`.
- No saved Golden uses first selected result as effective Golden.
- Each model segment belongs to the Golden row containing its start in `[start_ms,end_ms)`.
- Cross-boundary segments are not split or duplicated.
- Each unmatched segment forms one empty-Golden row.
- Rows sort by start, end, selected-preset order, and original segment order.
- Invalid selected result is represented in `selected_results[].error`; other valid selections continue.

### 6.5 Golden writes

- Renew request uses `source_preset_id`; persisted lineage uses `base_preset_id`.
- Renew writes flat `golden/{stream_id}.json`, generates stable Golden `segment_id` values, and sets `base_preset_id` plus `updated_at`; removed legacy Golden metadata is not stored.
- Edit retains positional IDs and row count; only text/start/end change.
- Reorder, insert, delete, split, merge, invalid timestamps, or overlap are rejected.
- Local storage may use temp-file rename. GCS FUSE production uses the single-writer complete-object publication contract and does not claim POSIX atomic rename.

### 6.6 Error mapping

- `400 invalid_preset_id`: malformed optional single filter ID.
- `400 invalid_preset_ids`: Align selection count, duplicates, or malformed UUIDs.
- `404 preset_not_found`: manifest absent.
- `404 stt_result_not_found`: known preset has no selectable result for stream.
- `409 preset_index_inconsistent`: completed index points to missing/invalid/mismatched canonical result.
- Existing stream, VOD, Golden validation, and internal errors retain their documented behavior.

## 7. Backend implementation order

1. **Preset/result fixtures:** exact manifest/result/flat-Golden schemas, duplicate `stream_ids`, valid results, orphan, index inconsistency, same-pair overwrite/completion timestamp, VOD/STT/Golden samples.
2. **Models and ports:** preset manifest, result, stream, VOD, Golden, alignment, filesystem and prober boundaries.
3. **Preset catalog:** UUID/filename validation, config source of truth, normalized completed IDs.
4. **Preset API:** `GET /api/presets` handler and filesystem tests.
5. **Base stream catalog:** first-level VOD directories, sorted.
6. **Result/index services:** canonical pair resolution, orphan exclusion, consistency errors.
7. **Filtered stream API:** optional `preset_id` intersection while preserving All streams default.
8. **VOD discovery:** filename order, ffprobe, cumulative timeline, `/vod` URL.
9. **Detail API:** VOD + all stream `stt_results`, manifest-enriched display metadata.
10. **Selected-result validation:** full segments, identity, missing result, per-selection errors.
11. **Alignment engine/API:** `preset_ids`, effective Golden, deterministic rows/unmatched.
12. **Golden Renew/Edit:** source/base preset lineage, flat path/exact schema, validation, single-writer persistence.
13. **Acceptance tests:** HTTP, temporary local filesystem, GCS storage abstraction/fakes, runner write-state fixtures, table-driven alignment, fake/real ffprobe.

## 8. Docker implementation order

1. Add `PRESET_ROOT=/app/data/presets` to runtime config and `.env.example`.
2. Build non-root Backend image with ffprobe, CA certificates, and timezone data.
3. Local dev mounts local VOD/presets/STT read-only and Golden read-write.
4. Keep Backend without host port mapping; Nginx publishes `8888:80`.
5. Nginx proxies API, serves VOD alias/ranges, and provides SPA fallback.
6. GCE host setup mounts four same-bucket prefixes with Cloud Storage FUSE and attached-service-account IAM; Compose bind-mounts source `:ro`, Golden `:rw`.
7. Smoke-test `/api/presets`, all/filtered streams, detail, align, flat Golden persistence, IAM denial outside Golden writes, and no direct Backend access.

## 9. Completion criteria

- All five APIs pass handler and filesystem integration tests.
- Preset-first UI can load presets, default to All streams, and request filtered intersection.
- Filter change clearing behavior is supported by deterministic stream responses; detail remains unfiltered by preset choice.
- Exact manifest/result/Golden fields and flat Golden path are enforced; API display uses nested `model.params` with no provider.
- Runner UUID, atomic same-pair overwrite, no history, dedupe/sort, orphan exclusion, and inconsistent-index errors are tested.
- One/two selected preset results preserve existing alignment semantics.
- Renew/Edit response matches flat Golden schema, is visible to next Align, and survives container/VM recreation; Golden edit rules remain unchanged.
- Backend never modifies VOD, presets, or STT; only Golden is writable.
- Local binds and GCE host FUSE binds, attached service account, conditional Golden IAM, and all environment roots are documented/smoke-tested.

## 10. Documentation change protocol

Every contract change must search all existing docs, resolve product/API/UI/runtime conflicts in `spec-v1.md`, update all affected documents together, and re-run obsolete wording plus JSON-block validation.

## 11. Deferred work

Database indexing, container-managed GCS mounting, merged media, authentication, collaborative editing, Golden history, annotations, export, health probes, and asynchronous job orchestration remain deferred. Manifest `stream_ids` is required v1 filesystem/object metadata, not a database index.
