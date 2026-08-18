# STT Comparison + Golden Sample v1

## Backend API specification

本文件與 [產品規格](spec-v1.md)、[前端 WebView 規格](ui-webview.md) 及 Backend implementation design 配套，描述 v1 HTTP contract。若文件衝突，必須交叉比對、在 `spec-v1.md` decision log 記錄 resolution，並於同一個 change 同步更新所有受影響文件。

## 1. General conventions

- Base path: `/api`
- Content type: `application/json`
- Time values: integer milliseconds unless otherwise stated.
- `created_at`: ISO 8601；frontend 顯示時轉為日本時間。
- `preset_id` 是 STT runner 產生的 opaque UUID；result identity 是 `preset_id + stream_id`。
- Public API 與 storage schema 不提供另一層 execution identity。相同 preset/stream 重跑時，runner atomic overwrite canonical result，不保留 result history。
- Preset manifest 的 `model.name` 與 `model.params` 是唯一 config source of truth；不使用 provider，STT result 不重複 model/params。
- Backend 唯讀 VOD、preset manifests 與 STT results，只寫入 latest Golden。
- v1 不使用 database，不處理 authentication、multi-user permission 或 result revision history。
- Nginx 是唯一對外入口並 publish host port `8888`；Backend 僅在 Compose network 監聽 `8080`。
- v1 不提供 `/healthz` 或 Compose healthcheck。
- Backend 唯一入口是 `backend/main.go`；HTTP/runtime concerns 位於 `backend/router`，共用資料型別位於 `backend/models`，業務 services 位於 `backend/domain`，sample fixtures 位於 `backend/samples`。

## 2. Storage and runner-owned artifacts

```text
/app/data/
├── vod/
│   └── {stream_id}/*.flv                    # source, read-only to Backend
├── presets/
│   └── {preset_id}.json                     # manifest + completed stream_ids
├── stt/
│   └── {stream_id}/{preset_id}.json         # canonical result for the pair
└── golden/
    └── {stream_id}.json                     # latest Golden only
```

Preset manifest example:

```json
{
  "preset_id": "550e8400-e29b-41d4-a716-446655440000",
  "model": {
    "name": "large-v3",
    "params": {
      "temperature": 0.2,
      "beam_size": 5,
      "vad_filter": true
    }
  },
  "created_at": "2026-08-17T05:30:00Z",
  "updated_at": "2026-08-17T06:00:00Z",
  "stream_ids": ["214744544", "214744545"]
}
```

STT result example:

```json
{
  "preset_id": "550e8400-e29b-41d4-a716-446655440000",
  "stream_id": "214744544",
  "created_at": "2026-08-17T05:35:00Z",
  "segments": [
    {
      "start_ms": 1200,
      "end_ms": 4380,
      "text": "大家晚安"
    }
  ]
}
```

Runner write protocol：

1. Runner 第一次建立 preset 時產生 UUID `preset_id`，以 backing storage 支援的 complete-object atomic replacement 寫入 manifest；local filesystem 可使用 temporary file + rename。初始 `stream_ids` 可為空，`created_at` 與 `updated_at` 相同。
2. 每次完成 stream 時，runner atomic replace `stt/{stream_id}/{preset_id}.json`，並將 result `created_at` 更新為本次完成時間；result 不含 `updated_at`。
3. Result replacement 成功後，runner 將 `stream_id` 加入 manifest `stream_ids`，去除 duplicates、ascending sort、更新 manifest `updated_at`，再 atomic replace manifest。
4. 相同 `preset_id + stream_id` 重跑只覆寫 canonical result；不建立 timestamped copy 或歷史清單。
5. Backend 不修補或寫回 runner-owned artifacts。

Consistency rules：

- Manifest 缺失的 result 是 orphan result，不可列出或選取。
- Result 存在但 stream 不在 `stream_ids` 時視為尚未完成／orphan，不可列出或選取。
- Manifest 宣告 stream completed，但 canonical result 缺失、路徑 identity 不符或內容無法解析時，回傳 `preset_index_inconsistent`。
- Backend 讀取 `stream_ids` 時去重並 ascending sort response；不修改 manifest。
- Result 內的 `preset_id`、`stream_id` 必須符合 path identity；`model.name`/`model.params` 只從 manifest 取得。

Local development 使用 local directories：

```text
backend:  ./data/vod:/app/data/vod:ro
backend:  ./data/presets:/app/data/presets:ro
backend:  ./data/stt:/app/data/stt:ro
backend:  ./data/golden:/app/data/golden:rw
frontend: ./data/vod:/srv/vod:ro
```

Nginx 直接提供 `/vod/*`；VOD bytes 不經 Backend。

GCE Linux production 使用 host Cloud Storage FUSE：同一 bucket 的 `vod/`、`presets/`、`stt/`、`golden/` prefixes 分別 mount 到 host paths，再由 Compose bind mount；source mounts 是 `:ro`，Golden 是 `:rw`。VM 使用 attached service account：bucket-level `roles/storage.objectViewer`，另以 resource-name condition 將 `roles/storage.objectUser` 限制於 `golden/` object prefix。Backend 是唯一 Golden writer。Cloud Storage/FUSE contract 不宣稱 POSIX atomic rename；Golden save 必須發布完整 JSON object，避免多 writer。

## 3. Endpoint summary

| Method | Path | Purpose |
|---|---|---|
| GET | `/api/presets` | 取得可用 preset manifests |
| GET | `/api/streams` | 取得所有 VOD streams，或依 optional `preset_id` 篩選 completed intersection |
| GET | `/api/streams/{stream_id}` | 取得 VOD player list 與該 stream 所有 selectable STT results |
| GET | `/api/streams/{stream_id}/align` | 依 Golden 與一至兩個 selected preset results 計算 alignment |
| PUT | `/api/streams/{stream_id}/golden` | Renew Golden 或儲存 Golden 編輯 |

## 4. GET /api/presets

只列出 filename、manifest `preset_id` 均為相同 valid UUID 的 manifests。Response 中 config 直接來自 manifest；orphan result 不會產生 preset。

### Response `200`

```json
{
  "presets": [
    {
      "preset_id": "550e8400-e29b-41d4-a716-446655440000",
      "model": {
        "name": "large-v3",
        "params": {"temperature": 0.2, "beam_size": 5, "vad_filter": true}
      },
      "created_at": "2026-08-17T05:30:00Z",
      "updated_at": "2026-08-17T06:00:00Z",
      "stream_ids": ["214744544", "214744545"]
    }
  ]
}
```

- Presets 依 `created_at`、再依 `preset_id` ascending deterministic sort。
- `stream_ids` 表示 completed streams，去重並 ascending sort。
- Manifest JSON 不合法屬 infrastructure/data consistency failure；Backend log 原因並回 common error，不從 result 推導 config。

## 5. GET /api/streams

### Query

- 無 `preset_id`：只掃描 `/app/data/vod/` 第一層 directories，回傳所有 VOD streams。
- `preset_id={uuid}`：回傳 VOD directories 與該 manifest `stream_ids`（completed semantics）的交集；每個交集項目的 canonical result 必須存在且 path identity 一致。

### Response `200`

```json
{
  "streams": [
    {"stream_id": "214744544"},
    {"stream_id": "214744545"}
  ]
}
```

排序：`stream_id` ascending。Filter 不改變 stream detail contract。

## 6. GET /api/streams/{stream_id}

Workspace 進入或 refresh 時使用。無論 UI 的 preset filter 為何，detail 都回傳該 stream 全部 selectable results；filter 只限制 Stream selector。

### Response `200`

```json
{
  "stream_id": "214744544",
  "vod": [
    {
      "file_id": "1780967564_000",
      "url": "/vod/214744544/1780967564_000_wansu_vod-a.flv",
      "sequence": 0,
      "start_time_unix_s": 1780967564,
      "duration_ms": 7200000,
      "timeline_start_ms": 0,
      "timeline_end_ms": 7200000
    }
  ],
  "stt_results": [
    {
      "preset_id": "550e8400-e29b-41d4-a716-446655440000",
      "model": {
        "name": "large-v3",
        "params": {"temperature": 0.2, "beam_size": 5, "vad_filter": true}
      },
      "created_at": "2026-08-17T05:35:00Z"
    }
  ]
}
```

Rules：

- `stt_results` 由 valid manifest、manifest completed membership 與 canonical result 三者交集產生。
- `model.name`、`model.params` 來自 manifest；result 的 `created_at` 是本次 canonical completion time。
- Detail 不驗證完整 segments；完整 validation 延後到 Align 或 Renew。
- Orphan results 不列出。Manifest 宣告本 stream completed 但 result 缺失或 identity 不一致時回 `preset_index_inconsistent`。
- `vod` 是 player manifest，採 duration 累加；filename start time 只用於排序，gap 不形成 visible timeline。

## 7. GET /api/streams/{stream_id}/align

### Query

```text
GET /api/streams/214744544/align?preset_ids=550e8400-e29b-41d4-a716-446655440000
GET /api/streams/214744544/align?preset_ids=550e8400-e29b-41d4-a716-446655440000,7c9e6679-7425-40de-944b-e07fc1f90ae7
```

- `preset_ids` 必須是一或兩個不同 valid UUIDs；Model A 必選，Model B 可選。
- 每個 preset 必須存在，且 `(preset_id, stream_id)` result 必須 selectable。
- 有保存 Golden 時，各 valid result 與同一份 Golden 比較。
- 沒有保存 Golden 時，第一個 selected result（Model A）作為 effective Golden，response schema 不變。
- Result segments 格式錯誤時，錯誤放在 `selected_results` 對應項目，其他 valid result 照常產生 rows。

### Response `200`

```json
{
  "stream_id": "214744544",
  "selected_results": [
    {
      "preset_id": "550e8400-e29b-41d4-a716-446655440000",
      "model": {
        "name": "large-v3",
        "params": {"temperature": 0.2, "beam_size": 5, "vad_filter": true}
      },
      "created_at": "2026-08-17T05:35:00Z",
      "error": null
    }
  ],
  "rows": [
    {
      "golden": {"start_ms": 0, "end_ms": 5000, "text": "大家晚安"},
      "models": {
        "550e8400-e29b-41d4-a716-446655440000": [
          {"start_ms": 300, "end_ms": 4200, "text": "大家晚安"}
        ]
      }
    },
    {
      "golden": {"start_ms": 5200, "end_ms": 6100, "text": ""},
      "models": {
        "550e8400-e29b-41d4-a716-446655440000": [
          {"start_ms": 5200, "end_ms": 6100, "text": "額外內容"}
        ]
      }
    }
  ]
}
```

Alignment semantics 保持不變：

- Golden row 使用半開區間 `[start_ms, end_ms)`。
- Model segment 以 `start_ms` 歸屬單一 Golden row。
- 跨 Golden 邊界不拆分、不重複顯示。
- 每個 unmatched model segment 形成一個 empty-Golden row；Golden `text` 為空，start/end 沿用該 segment。
- Rows 依 start、end、selected preset order、原始 segment order deterministic sort。
- `selected_results[].error` 非 null 時，該 result 不產生 models alignment rows。

## 8. PUT /api/streams/{stream_id}/golden

此 endpoint 有兩種用途：Renew 或儲存 Golden 編輯。

### 8.1 Renew from preset result

```json
{
  "mode": "renew",
  "source_preset_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

行為：以 `(source_preset_id, stream_id)` canonical result 完整覆寫 Golden segments、text、timestamps 與 segmentation；不保留舊 Golden revision。Renew 為每個 Golden segment 建立 stable `segment_id`，記錄 `base_preset_id` 與本次 `updated_at`，完整覆寫 flat `data/golden/{stream_id}.json`。Local filesystem 可用 temporary file + rename；GCS FUSE production 只保證 single-writer complete-object replacement，不宣稱 POSIX atomic rename。

### 8.2 Save Golden edits

```json
{
  "mode": "edit",
  "segments": [
    {
      "start_ms": 0,
      "end_ms": 5000,
      "text": "大家晚安"
    }
  ]
}
```

Validation 與既有 Golden edit 規則不變：

- Payload 不要求 `segment_id`；依目前 Golden row 順序對應，只允許修改 text、start_ms、end_ms。
- Row 數量必須與 current Golden 相同；Backend 依位置保留 stable segment IDs。
- `start_ms >= 0`、`end_ms > start_ms`，且 segments 依 start_ms 排序、不可重疊。
- v1 不允許新增、刪除、split、merge。

### Response `200`

```json
{
  "stream_id": "214744544",
  "base_preset_id": "550e8400-e29b-41d4-a716-446655440000",
  "updated_at": "2026-08-17T08:00:00Z",
  "segments": [
    {
      "segment_id": "golden_segment_001",
      "start_ms": 0,
      "end_ms": 5000,
      "text": "大家晚安"
    }
  ]
}
```

Golden validation error 的既有 `fields` representation 與 common error `details` 差異不在本次 preset design 處理範圍。

```json
{
  "error": {
    "code": "golden_validation_failed",
    "message": "Golden segments must not overlap.",
    "fields": [
      {"path": "segments[1].start_ms", "reason": "overlaps segments[0]"}
    ]
  }
}
```

## 9. Common errors

Common envelope：

```json
{
  "error": {
    "code": "error_code",
    "message": "Human-readable message",
    "details": {}
  }
}
```

| HTTP | Code | Meaning |
|---:|---|---|
| 400 | `invalid_request` | Request body 或 query 格式錯誤 |
| 400 | `invalid_preset_id` | Optional single `preset_id` 缺失值或不是 UUID |
| 400 | `invalid_preset_ids` | Align selection 不是一至兩個不同 UUIDs |
| 404 | `stream_not_found` | VOD stream directory 不存在 |
| 404 | `preset_not_found` | Preset manifest 不存在 |
| 404 | `stt_result_not_found` | Preset exists，但該 stream 沒有 selectable completed result |
| 409 | `preset_index_inconsistent` | Manifest completed index 與 canonical result 不一致 |
| 422 | `vod_ingestion_failed` | VOD 存在但無法取得 duration |
| 422 | `golden_validation_failed` | Golden 編輯內容不合法 |
| 500 | `internal_error` | 未預期 Backend 或 malformed manifest failure |

## 10. Acceptance criteria

- `GET /api/presets` 只回 exact manifest fields；config 使用 nested `model.name`/`model.params`。
- `GET /api/streams` 預設回所有 VOD streams；optional preset filter 回 completed IDs 與 VOD dirs 交集。
- Detail 使用 `stt_results` 並仍顯示該 stream 全部 selectable results。
- Align 接受一或兩個 `preset_ids`；alignment 與 effective Golden semantics 不變。
- Result/storage/API 不含 execution-history identity，且 result 不重複 model/params、`segment_id` 或 `updated_at`。
- Same-pair rerun atomic overwrite，不保留 history。
- Manifest `stream_ids` 代表 completed streams，duplicates 在 response 去重排序；orphan result 不可選；index inconsistency 有明確 error。
- Golden Renew 使用 `source_preset_id`，flat Golden 只保存 `stream_id`、`base_preset_id`、`updated_at`、segments；Golden edit 規則不變。
- GCE production 使用 host Cloud Storage FUSE、attached service account、read-only source binds、read-write Golden bind；Backend 是唯一 Golden writer。
- VOD single/multiple logical playback、Nginx `/vod/*` 與外部 port `8888` contract 不變。
