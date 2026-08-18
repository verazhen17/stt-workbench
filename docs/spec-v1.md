# STT Comparison + Golden Sample System

v1.0 Architecture & Product Specification



| Document status | Audience | Iteration mode |

| --- | --- | --- |

| Final v1 | Product / Backend / Frontend / Reviewers | Implementation contract |



本文件是 v1 implementation contract。v1 的 critical scope 與決策已確認；未納入 v1 的功能明確標示為 deferred 或 future，不阻擋目前開發。


# 1. Executive summary


目標是建立一個以直播間／媒體為單位的 STT 比較與 Golden Sample 管理系統。STT runner 產生 opaque UUID `preset_id`、preset manifest、每個 stream 的 canonical result 與 completed index；Backend 唯讀 discovery/validation 並提供 preset-first UI。系統以 `preset_id + stream_id` 識別 result，同組重跑 atomic overwrite、不保留 result history；Golden 仍只保留最新版本。

v0.1 核心設計結論：

- Preset manifest 的 nested `model.name`/`model.params` 是 config source of truth；不使用 provider。Canonical STT results 與 Latest Golden 分離，alignment 依 selected preset results + effective Golden 即時計算。

- Golden 初始以 Whisper V1 segmentation 建立，之後可獨立編輯文字與時間軸；不保留 Latest Golden history。

- Backend 擁有 alignment semantics；frontend 只呈現 rows、開放區間、unmatched 與操作結果。

- 第一版優先完成 preset listing/filter、stream 瀏覽、一至兩個 preset results 與 Golden 比較、最新 Golden CRUD、seek 與 deterministic alignment；report/export 延後。


# 2. Scope and success criteria



## 2.1 In scope for v0.1


- Stream/media catalog 與 metadata。

- Runner-owned preset manifests、canonical STT results 與 completed indexes；Backend request-time discovery/validation。

- Optional preset-first stream filter（default All streams）與 stream detail 的全部 selectable results。

- Preset manifest config 與 canonical result metadata 顯示；result JSON 不重複 model/params。

- 以 Whisper V1 segmentation bootstrap Golden。

- Golden segment 的 text、start/end 編輯；v1 不支援 create/delete/split/merge/reorder。

- 選擇一或兩個不同 `preset_id` 的 STT results 與 latest Golden 比較。

- Deterministic alignment：Golden-based rows、single owner、unmatched。

- Video → sentence 與 sentence → video seek。

- Reviewer annotation／labels／notes 不納入 v0.1，延至第二階段。


## 2.2 Success criteria




| Outcome | Acceptance signal | Priority |

| --- | --- | --- |

| 可追溯 | STT result 以 `preset_id + stream_id` 回到 manifest config 與 canonical result `created_at`；Golden lineage 使用 preset identity。 | Must |

| 可比較 | Reviewer 可在同一 Golden row 下閱讀兩個 model 結果並播放對應片段。 | Must |

| Golden 穩定 | 新增或覆寫 result 不會自動改變 Golden 的 segment identity 或時間軸。 | Must |

| 可修訂 | Golden 修改後保存最新內容；不保留 Latest Golden history。 | Must |

| 可一致 | All streams 與 preset-filtered intersection 可預期；orphan result 不可選，completed index 不一致有明確 error。 | Must |

| 可擴充 | 未來加入 V3、Tencent 或其他模型只新增 manifest `model` config，不需改 Golden schema。 | Should |




# 3. User flow


1. Preset filter 預設 All streams；使用者可 optional 選 preset，取得其 completed IDs 與 VOD dirs 交集。

1. 從目前 stream list 選擇直播間／media；若切換 filter 後 current stream 不存在，清除 workspace。

1. Detail 顯示該 stream 全部 selectable `stt_results`；選擇 Model A、optional Model B。

1. Backend 以 latest Golden 為 target 計算 alignment，UI 顯示 Golden rows、兩個 model 的 aligned segments、開放區間與 unmatched。

1. Reviewer 播放整段影片，點擊 sentence 進行 seek；或拖曳 video time 讓目前 sentence/highlight 更新。

1. Reviewer 編輯 Golden text 或 timeline；儲存後覆寫最新 Golden，comparison 重新計算。



1. Runner 完成或重跑 `(preset_id, stream_id)` 後 atomic replace canonical result，再更新 manifest completed list；refresh 顯示最新 result，無歷史列表，既有 Golden 沿用。


# 4. Architecture



```text
STT runner ── UUID preset manifest + canonical results + completed index
             │ atomic filesystem writes
             ▼
Backend read-only discovery ── presets / filtered streams / all stream results
             │ deterministic alignment + Latest Golden
             ▼
Preset-first comparison UI ── media player / rows / Golden editor
```




| Layer | Responsibility | v0.1 boundary |

| --- | --- | --- |

| STT runner | Generate UUID preset manifest, canonical results, completed list; atomic replace. | Outside Backend; owns VOD-independent STT execution. |

| Storage | Preset manifest config, one canonical result per pair, Latest Golden. | No result history; filesystem convention first. |

| Backend | Read-only preset/result discovery, filtering, validation, alignment; Golden CRUD. | Never writes presets/results; single alignment semantics owner. |

| Frontend | Selection, comparison, playback, editing, feedback. | Do not reimplement alignment algorithm. |

| Export | Generate report / dataset package. | Non-goal for v0.1; design extension point only. |




### 4.1 Development environment assumptions

- Backend 與 frontend 都在開發機上透過 `docker compose up` 啟動；v1 不使用 database。
- Frontend 由 Nginx 對外 publish port `8888`，是 Browser 唯一入口。
- Backend 固定在 Compose internal network 監聽 `8080`，不 publish host port；Nginx 將 `/api/*` proxy 到 `backend:8080`。
- Local development 使用本機 `./data/{vod,presets,stt,golden}` directories。
- 開發機上的 VOD、presets、STT、Golden 分別放在 `data/vod`、`data/presets`、`data/stt`、`data/golden`。
- v0.1 應避免把 media path 寫死在程式碼；使用 environment variable 或 mounted volume config。
- Local Compose 將 VOD/presets/STT 以 read-only mount 給 Backend，Golden 以 read-write mount；`PRESET_ROOT=/app/data/presets`。
- GCE Linux production 由 host Cloud Storage FUSE mount 同一 bucket 的 `vod/`、`presets/`、`stt/`、`golden/` prefixes，再 bind mount 給 Compose；source `:ro`、Golden `:rw`。
- GCE VM 使用 attached service account：bucket-level `roles/storage.objectViewer`，以及 resource-name condition 限制在 `golden/` prefix 的 `roles/storage.objectUser`。Backend 是唯一 Golden writer。
- Nginx 以 read-only 方式將 `./data/vod` mount 到 `/srv/vod`，直接提供 `/vod/...`；VOD 不經 Backend proxy。Browser 不直接讀取 container local path，也不需要另外架設 Backend VOD server。
- v1 不新增 `/healthz` 或 Compose healthcheck；正式部署平台若需要 readiness/liveness probe，後續另行設計。

## 5. Playback strategy

### 5.1 Source constraints

- Mounted FLV 是 read-only source，系統禁止修改、搬移或覆寫。
- 系統只處理檔案已完成且 size／mtime 在掃描期間穩定的 FLV。
- VOD 每約兩小時切成一個 FLV；數量通常不會非常多。
- v0.1 不轉 MP4；單檔直接播 FLV。多檔回傳有順序的 FLV 清單與各檔案 duration，並以 logical continuous playback 呈現為單一影片；merge 延後決定。

VOD identity and ordering convention（以目前 GCS 範例為準）：

```text
{bucket}/{stream_id}/{start_unix_seconds}_{sequence}_{label}--{recorded_at}.flv
```

例如：

```text
media17-stream-stt-prod/samples/214744544/1780967564_000_wansu_vod-d865352e-37fe-4a64-8039-17534c3213de--20260609091244.flv
```

- `214744544` 是直播間／`stream_id`。
- `1780967564` 是該 FLV 的 start time（Unix seconds）。
- `_000` 是同一 start time 下的 sequence；同一 start time 時依 `_000`、`_001`、`_002` 的數字順序排列。
- 不同 start time 時，先依 start time ascending 排列，再套用同 start time 的 sequence 排序。
- 檔名後方的 UUID、label、recorded_at 目前只作為檔案識別資訊，不用於直播歸屬或排序。
- 多個 FLV 之間若有時間空檔，logical playback 直接接續下一個檔案，不在使用者看到的影片時間軸保留空白；檔名 start time 只負責排序，不直接決定 global playback offset。

### 5.2 Recommendation: request-time discovery

v0.1 採 request-time discovery，不做定期掃描：

1. Page load 讀取 preset manifests；All streams 預設只掃描 VOD root 第一層 directories。
2. Optional preset filter 以 manifest `stream_ids`（completed semantics）、valid canonical results 與 VOD directories 取交集。
3. 選定 stream 後，Detail 掃描該 stream 的 VOD 與所有 preset/result pairs，不受 filter 限制。
4. Detail 只回 selectable result metadata；config 由 manifest 提供，完整 segments validation 延後到 Align/Renew。
5. Detail 回傳 FLV ordered array；frontend 依 array 連續播放，不修改 source FLV。

Missing manifest 的 result 或未列入 completed list 的 result 是 orphan，不可選。Manifest 宣告 completed 但 canonical result 缺失、malformed 或 path/body identity 不一致時回 `preset_index_inconsistent`。

這樣可以避免：

- 使用者第一次開啟時同步等待大型 merge。
- 每次 request 重複執行 merge。
- 修改 mounted source FLV。
- Backend 不需要另外維護 VOD streaming endpoint；frontend 使用既有 web server 的 static file serving 處理多檔切換與 seek mapping。

### 5.3 Derived playback artifacts

data/
├── vod/                         # mounted read-only source
│   └── {stream_id}/
│       ├── 001.flv
│       └── 002.flv
├── merged/                      # writable derived output
│   └── {stream_id}/
│       ├── manifest.json
│       ├── merge.status.json
│       └── merged.flv

manifest.json 應記錄：

- source filename
- sequence
- file size / mtime 或 checksum
- duration
- stream timeline 的 start_ms / end_ms
- merge status
- merged artifact 對應的 source list

### 5.4 Multi-FLV behavior in v0.1

- 單一 FLV：直接使用 source path，不產生 merged artifact。
- 多個 FLV：依 sequence 順序回傳 source path、duration 與累計 timeline start/end；v0.1 必須在單一 video player 中支援連續播放與跨檔 seek。
- Backend 使用 `ffprobe` 取得每個 FLV 的 duration；無法取得時回傳 HTTP `422 vod_ingestion_failed`。
- 不修改、搬移、覆寫或轉換 mounted source FLV。

### 5.5 Local and GCE storage layout

Local development 直接使用 `./data/*`。GCE Linux production 使用 host Cloud Storage FUSE；container 不自行 mount GCS，也不需要 privileged mount：

```text
one Cloud Storage bucket
├── vod/      ── host FUSE mount ── Compose :ro ── /app/data/vod + /srv/vod
├── presets/  ── host FUSE mount ── Compose :ro ── /app/data/presets
├── stt/      ── host FUSE mount ── Compose :ro ── /app/data/stt
└── golden/   ── host FUSE mount ── Compose :rw ── /app/data/golden
```

- VM attached service account 不使用 container credential files。
- `roles/storage.objectViewer` 授予 bucket read；`roles/storage.objectUser` 以 IAM condition 限制 resource name prefix 為 bucket `golden/` objects。
- Backend 是唯一 Golden writer。Cloud Storage FUSE 不作 POSIX atomic rename 保證；Golden save 發布完整 JSON object並避免 concurrent writers。
- Container paths 與 local development 相同，所以 Backend/API 不需環境分支。

以下 merge 行為保留給後續版本：產生 derived `merged.flv`、處理 merge job，以及處理 codec／timebase 不相容。

### 5.6 Playback data in stream detail

`GET /api/streams/{stream_id}` 直接回傳依 sequence 排序的 VOD array，提供 `url`、duration 與 cumulative timeline 給單一 video player。Frontend 不需要自己 merge FLV，也不需要額外 playback API。

### 5.7 v0.1 logical continuous playback without merge

也可以不產生 `merged.flv`，改由 `ffprobe` 計算每個 FLV 的 duration，建立 ordered manifest：

```text
vod_001: global [0 ms,       7,200,000 ms)
vod_002: global [7,200,000, 14,400,000 ms)
vod_003: global [14,400,000, 21,600,000 ms)
```

Frontend 維護兩種時間：

- `global_time_ms`：直播間的連續時間軸，與 STT timestamp 相同。
- `local_time_ms`：目前 FLV 檔案內的時間。

播放與 seek 行為：

1. 播放到目前 FLV 的結尾時，載入下一個 FLV 並繼續播放。
2. 使用者 seek 到 global time 時，先用 manifest 找到對應 FLV，再換算成 local time。
3. VOD 之間若有 gap，manifest 不建立可見的空白播放區間；下一個 FLV 的 `timeline_start_ms` 直接接續前一個檔案的 `timeline_end_ms`。

優點：

- 不需要等待大型 merge。
- 不產生大型 derived media。
- mounted FLV 保持 read-only。
- 新增 VOD 後只需重新跑 ffprobe 並更新 manifest。

代價：

- Frontend 需要處理 source switching、跨檔播放、seek、buffer 與短暫切換停頓。
- 前端 player 必須支援 FLV；原生 HTML video 是否能直接播放 FLV 需要先驗證。
- 若未來需要單一 URL、下載或報表影片，仍可能需要另外產生 merged asset。

| Option | First load | Frontend complexity | Derived storage | Recommendation |
|---|---|---|---|---|
| Background merge | 等待 merge ready | Low | 需要 merged.flv | 適合單一 player URL |
| Logical continuous playback | 快 | Medium | 只需要 manifest | 適合避免大型 merge |

v0.1 採 logical continuous playback；background merge 保留為未來替代方案。

### 5.6 Storage and file conventions


STT runner 產生 opaque UUID `preset_id`。Preset identity 位於 manifest filename/body；result identity 是 `(preset_id, stream_id)`。Canonical result filename 固定，因此同組重跑 atomic overwrite，不建立 revision/history。Public API 與 storage 不提供另一層 execution identity。


```text
data/
├── vod/                         # read-only mount
│   └── {stream_id}/
│       ├── 001.flv
│       └── 002.flv
├── presets/                     # runner-owned, Backend read-only
│   └── {preset_id}.json
├── stt/
│   └── {stream_id}/{preset_id}.json
└── golden/
    └── {stream_id}.json
```




| Artifact | Mutable? | Identity | Notes |

| --- | --- | --- | --- |

| `presets/{preset_id}.json` | Runner complete-object replace | preset_id UUID | Exact manifest owns nested `model.name`/`model.params`, completed `stream_ids`, created/updated times. |

| `stt/{stream_id}/{preset_id}.json` | Runner atomic overwrite | preset_id + stream_id | Exact canonical result; rerun updates result `created_at`; no model/params, segment IDs, or update timestamp. |

| `golden/{stream_id}.json` | Backend complete-object overwrite | stream_id | Exact flat Latest Golden；Backend single writer。 |




## 6. Preset and STT result discovery behavior

- `GET /api/presets` 只讀 valid manifest；filename/body `preset_id` 必須是同一 UUID。
- Manifest nested `model.name`/`model.params` 是唯一 config source of truth；無 provider。
- No-filter stream list 只讀 VOD directories；preset-filtered list 取 VOD dirs、deduped/sorted completed `stream_ids`、valid canonical results 交集。
- Detail 的 `stt_results` 回傳該 stream 全部 selectable results，不受目前 filter 限制。
- Result body/path 的 `preset_id`、`stream_id` 必須一致；完整 segments validation 延後到 Align/Renew。
- Missing manifest 或 result 未列入 manifest `stream_ids` 是 orphan，不列出、不選取。
- Manifest 宣告 completed 而 result 缺失、malformed 或 identity mismatch，回 `preset_index_inconsistent`。
- 如果 stream 沒有 selectable result，frontend 顯示 No STT；VOD 仍可播放。
- Backend 不修改 manifests/results；runner 先 atomic replace result（更新 result `created_at`），再 dedupe/sort `stream_ids`、更新 manifest `updated_at` 並 replace manifest。

# 7. JSON schemas (v0.1 proposal)



## 7.1 Metadata



```text
{   "stream_id": "stream_123", "title": "optional display name",   "media_url": "gs://bucket/stream_123.mp4",   "duration_ms": 1832400,   "created_at": "2026-08-13T10:00:00Z",   "source": {"platform": "...", "external_id": "..."} }
```



## 7.2 Preset manifest

```json
{
  "preset_id": "550e8400-e29b-41d4-a716-446655440000",
  "model": {
    "name": "large-v3",
    "params": {"temperature": 0.2, "beam_size": 5, "vad_filter": true}
  },
  "created_at": "2026-08-13T14:40:00Z",
  "updated_at": "2026-08-13T14:45:00Z",
  "stream_ids": ["stream_123"]
}
```

## 7.3 STT result

```json
{
  "preset_id": "550e8400-e29b-41d4-a716-446655440000",
  "stream_id": "stream_123",
  "created_at": "2026-08-13T14:41:00Z",
  "segments": [
    {"start_ms": 1200, "end_ms": 4380, "text": "大家晚安"}
  ]
}
```

## 7.4 Latest Golden

```json
{
  "stream_id": "stream_123",
  "base_preset_id": "550e8400-e29b-41d4-a716-446655440000",
  "updated_at": "2026-08-17T08:00:00Z",
  "segments": [
    {"segment_id": "golden_segment_001", "start_ms": 1200, "end_ms": 4380, "text": "大家晚安"}
  ]
}
```


Schema notes：

- 所有時間統一使用整數 milliseconds；區間採 half-open [start_ms, end_ms)。

- STT result segments 不含 IDs；Golden `segment_id` 必須穩定且不可由 array index 推導。未來若支援 split／merge，須產生新 IDs。
- Renew Golden 時 Backend 為 Golden segments 建立 stable IDs；後續 edit 保留既有 IDs。

- 是否需要 confidence、language、words、speaker、raw_text、normalized_text 是 open decision；v0.1 可先允許 additional fields。

- `preset_id` 是 runner-generated opaque UUID；manifest config 不得由 result payload 覆蓋。


## 8. Golden prerequisite

Alignment 永遠需要一個 effective Golden。

- 若尚無保存的 Golden，`GET /align` 使用第一個 selected preset result（Model A）作為 effective Golden。
- response schema 不因是否已有保存 Golden 而改變。
- 後續可使用 Renew Golden 將目前 Golden 替換為另一個 preset result；request 使用 `source_preset_id`，lineage 使用 `base_preset_id`。
- 新增或覆寫 STT result 不會自動覆寫已保存的 Golden。

# 9. Golden lifecycle and versioning



```text
Selected preset result ── bootstrap/Renew ──▶ Golden draft ── text/time edit ──▶ Latest Golden
                                                        future preset results align against current Golden
```




| State | Allowed changes | Meaning |

| --- | --- | --- |

| frontend draft | Text and start/end only | 尚未 save，不是 persisted Golden field。 |

| saved latest | Text and start/end only | Flat `{stream_id}.json`；new canonical results align to it。 |

| history | None | No Golden history in v1. |



保存完整 Golden segments 並覆寫最新 object。Local filesystem 可用 temporary file + rename；GCS FUSE production 由單一 Backend writer 發布完整 JSON object，不宣稱 POSIX atomic rename。多人同時編輯與 optimistic locking 延後決定。


# 10. Alignment rules


Alignment 是 derived output：input = effective Golden + selected preset results；output 不作 source of truth。每次 Golden timeline 改變或選擇不同 preset result 時重新計算。


## 10.1 Ownership rule

Alignment 的時間區間以 Golden 為基準。Golden segment 是 UI row 的唯一時間邊界；Golden 沒有覆蓋到的時間（Golden 之間的 gap、Golden 之前與之後）不強行建立 row，保持為開放區間。


```text
Golden G = [G.start_ms, G.end_ms)
Model M = [M.start_ms, M.end_ms)

owner(M) = the one Golden segment G for which:
    G.start_ms <= M.start_ms < G.end_ms

If no G satisfies the rule, M is unmatched.
```


此規則自然支援 1 Golden : N model segments。Model segment 只依 start_ms 歸屬一個 Golden row；若 M 橫跨多個 Golden segments，仍只放在 start_ms 所屬的 row，不拆分、不建立多重 owner。


## 10.2 Unmatched and validation



- Golden segments 預期按 start_ms 排序且不可重疊；若 editor 造成 overlap，save validation 必須拒絕。

- Model segments 可重疊或亂序時，backend 應保留 raw data，但 alignment response 需標記 invalid_segment 或 data_warning。

- Model segment 在 Golden 之前／之後、或落在 gap 內時，每個 unmatched model segment 各自形成一個 Golden text 為空字串的 row；empty-Golden 的 start/end 沿用該 model segment，model 欄保留該 segment。
- Unmatched rows 依 start_ms、end_ms、selected preset 順序與原始 segment 順序排序，確保 response deterministic；model segment 只出現一次。

- v1 不計算或顯示 model 與多個 Golden 的 overlap；跨界 model segment 只保留單一 owner。


## 10.3 Alignment response shape



```json
{
  "selected_results": [
    {"preset_id":"550e8400-e29b-41d4-a716-446655440000", "model":{"name":"whisper_large_v3","params":{"temperature":0.2}}, "created_at":"2026-08-17T05:30:00Z", "error":null},
    {"preset_id":"7c9e6679-7425-40de-944b-e07fc1f90ae7", "model":{"name":"whisper_medium","params":{"temperature":0.0}}, "created_at":"2026-08-17T06:00:00Z", "error":null}
  ],
  "rows": [
    {"golden":{"start_ms":0,"end_ms":5000,"text":"大家晚安"},"models":{"550e8400-e29b-41d4-a716-446655440000":[{"start_ms":300,"end_ms":4200,"text":"大家晚安"}],"7c9e6679-7425-40de-944b-e07fc1f90ae7":[]}},
    {"golden":{"start_ms":5000,"end_ms":6200,"text":""},"models":{"550e8400-e29b-41d4-a716-446655440000":[],"7c9e6679-7425-40de-944b-e07fc1f90ae7":[{"start_ms":5200,"end_ms":6100,"text":"額外內容"}]}}
  ]
}
```



# 11. Backend API proposal




| Method / path | Purpose | Notes |

| --- | --- | --- |

| GET /api/presets | Return valid preset manifests and normalized completed IDs. | Manifest is config source of truth. |

| GET /api/streams[?preset_id=uuid] | No filter returns all VOD streams；filter returns VOD/completed/selectable-result intersection. | Stream IDs ascending. |

| GET /api/streams/{id} | Return VOD player list and all selectable `stt_results` for stream. | Independent of active filter. |
| PUT /api/streams/{id}/golden | Renew Golden from selected preset result or save Golden edits. | Renew uses `source_preset_id`. |

| GET /api/streams/{id}/align?preset_ids=a,b | 驗證 selected results；無 saved Golden 時以第一個 result 作 effective Golden。 | Alignment semantics unchanged. |


`GET /api/streams/{id}` response shape:

```json
{
  "stream_id": "stream_123",
  "vod": [],
  "stt_results": [{"preset_id": "550e8400-e29b-41d4-a716-446655440000", "model": {"name": "whisper_large_v3", "params": {"temperature": 0.2, "beam_size": 5, "vad_filter": true}}, "created_at": "2026-08-17T05:30:00Z"}]
}
```

`stt_results` 包含 manifest completed membership 與 canonical result 的 selectable intersection；config 來自 manifest。完整 segments validation 在 Align/Renew 處理。

Preset-specific errors：`invalid_preset_id`（single filter UUID）、`invalid_preset_ids`（Align count/duplicate/UUID）、`preset_not_found`、`stt_result_not_found`、`preset_index_inconsistent`。完整 HTTP mapping 以 `api-spec.md` 為準。




```text
PUT /api/streams/12345612/golden {"mode":"edit","segments":[{"start_ms":1200,"end_ms":4500,"text":"大家晚安"}]}  200 OK {"stream_id":"12345612","base_preset_id":"550e8400-e29b-41d4-a716-446655440000","updated_at":"2026-08-17T08:00:00Z","segments":[{"segment_id":"golden_segment_001","start_ms":1200,"end_ms":4500,"text":"大家晚安"}]}
```



# 12. UI state and Golden actions

## 12.1 Alignment mode

無論是否已有保存 Golden，前端都使用同一個 alignment UI：

- Golden segment 作為固定 rows。
- Selected preset results 顯示在各 Golden row 下。
- 以 Golden 的時間區間建立 rows；Golden 未覆蓋的時間顯示為開放／未對齊區域。
- Model segment 依 start_ms 放入單一 Golden row；跨界時不拆分、不重複顯示。
- 顯示 unmatched；不顯示多重 owner 或 overlap 清單。
- Golden header 顯示 source preset 與目前更新時間。
- Golden 欄位排在 Model A、Model B 之前。
- 尚無保存 Golden 時，Golden 欄使用第一次選定的 Model A。
- 使用者可選另一個 preset result 執行 Renew Golden。

## 12.2 Renew Golden

- Renew Golden：以 `source_preset_id` 對應 result 完整覆蓋目前 Golden並記錄 `base_preset_id`；尚無保存 Golden 時建立第一份。
- Renew 會覆蓋目前 Golden 的文字、時間軸與 segmentation。
- Renew 不需要 confirmation 欄位；前端可自行提供確認視窗。
- Renew 完成後，alignment 使用新的 Golden 重新計算。
- Renew 不會修改 runner-owned canonical result。

### 12.4 Golden editing rules

- 允許編輯 Golden segment 的 text、start_ms、end_ms。
- 編輯內容先保留在前端暫存狀態，使用者按 Save Golden 後才覆寫最新 Golden。
- 儲存時若 segments 互相重疊，backend 拒絕儲存並回傳 validation error。
- v1 不支援新增、刪除、split、merge Golden segments；request body 保留未來加入 operations 的擴充空間。
- Save 成功後，alignment 使用更新後的 Golden 重新計算。

# 12. UI behavior and seeking



## 12.4 Comparison layout


### 12.4.1 Comparison definition

Preset filter 位於 Stream selector 前，default All streams。Filter 選定時，stream list 是 manifest completed IDs 與 VOD dirs/selectable results 交集；若 current stream 不再存在，frontend 清除整個 workspace。Detail 仍回該 stream 全部 selectable results。

v0.1 UI 固定提供兩個 result 選擇位置：Model A 與 Model B；兩者各自與 Golden 比較，不定義 A 對 B 評分。

- Selector 顯示 detail `stt_results`；segments validity 延後至 Align。
- 顯示 manifest `name`／`model.name` 與 result `created_at`（日本時間）；opaque `preset_id` 不作主要 label。
- Params 展開區顯示 manifest `params`；result 不重複 config。
- 兩個 selector 不可選相同 `preset_id`；Model A 必選，Model B optional。
- 一或兩個 selected results 都各自與同一 Golden 比較。
- 沒有保存 Golden 時，使用第一次選定的 Model A 作為 effective Golden；response schema 不改變。
- 使用 Golden-based alignment mode：將 selected results 的 segments 放入 Golden rows。
- 比較結果是唯讀衍生資料；Backend 不修改 STT result 或 Golden（Golden 只由明確 Renew/Edit 修改）。
- 使用者切換 selection、refresh、或儲存 Golden 後重新計算。

Align API 的基本驗證：

- `preset_ids` 必須包含一或兩個不同 UUIDs。
- 每個 `(preset_id, stream_id)` 必須 selectable；格式檢查失敗時，錯誤放在 `selected_results[].error`，該 result 不產生 rows。
- 沒有保存 Golden 時，第一次選定的 Model A 作為 effective Golden。
- 只有一個 result 時，照常使用 Model A 與 effective Golden 對齊。


- Header：optional preset filter、stream selector、Latest Golden、result selectors、refresh。

- Result selector label：`{manifest name/model.name} — {result created_at in Japan time}`。

- Player：單一 video/audio element、current time、play/pause、±5s、volume；不暴露底層 FLV 分段。

- Rows：以 Golden segment 為固定 row；每 row 顯示 Golden text/time 與 selected model segments。

- Unmatched drawer：列出沒有 owner 的 model segments，點擊可播放並定位。

- Editor：inline text/time edit；v1 不提供 split/merge。

- Feedback：先定義 label vocabulary（例如 hallucination、missing、bad segmentation、timestamp error）再實作。


## 12.5 Video → sentence


- Player currentTime 更新時，找出包含 current_time_ms 的 Golden segment：[start,end)。

- 若 current time 位於 gap，highlight 清除或顯示 nearest/previous 狀態；行為需固定。

- 若 seek 到 unmatched model segment，UI 顯示其 source preset 與時間範圍，不強行指派 Golden。


## 12.6 Sentence → video


- 點擊 Golden row：seek 至 Golden.start_ms，v0.1 建議 seek + pause。

- 點擊 model segment：seek 至 model.start_ms，並保持該 row 展開。

- 鍵盤上下移動 sentence 時，依 Golden start_ms 順序移動。

- 所有 seek 使用 integer ms 轉換為 player seconds；避免浮點比較直接作為 identity。


# 13. Priorities and milestones




| Milestone | Deliverables | Exit criteria |

| --- | --- | --- |

| M0 Data contract | Exact preset/result/flat-Golden schemas, folders, validator. | `backend/samples` 涵蓋 exact-field fixtures、duplicate `stream_ids`、result rerun completion time、orphan/index inconsistency、flat Golden；拒絕 removed/extra contract fields。 |

| M1 Preset-first read-only comparison | Preset listing、All/filter streams、detail `stt_results`、logical playback、effective Golden、basic rows。 | Filter intersection 正確、filter invalidation 清 workspace、detail 仍列全部 results；一或兩個 presets 可比較。 |

| M2 Golden editing | Text/time edit, save latest Golden. | 修改 Golden 後最新內容可讀；align 自動重算。 |

| M3 Alignment hardening | single owner/unmatched、edge-case tests。 | 涵蓋 model split pattern、跨界、gap、out-of-range；Golden edit scope不變。 |

| Phase 2 Reviewer feedback | annotations、labels、notes、selection state。 | 可標記 hallucination 等並隨 preset/result identity 可追溯；功能延至第二階段。 |

| Later Report/export | Export comparison report / Golden dataset package. | 另立 spec；不阻擋 v0.1。 |




# 14. Non-goals


- 在此系統內執行 Whisper、調參、GPU job scheduling 或 model training。

- v0.1 自動從 audio 重新切句或自動修正 Golden。

- 以 semantic similarity 取代 deterministic timestamp alignment。

- 即時串流 STT 或 live collaborative editing。

- 權限、SSO、audit compliance、跨團隊 workflow。

- 完整報表、批次 export、統計 dashboard。

- 將 alignment 結果永久存為唯一真相。


# 15. Risks and mitigations




| Risk | Impact | Mitigation / v0.1 stance |

| --- | --- | --- |

| Golden timeline 編輯造成 alignment 變動 | High | 以 latest Golden 重新計算 alignment；不保存舊 Golden revision。 |

| 不同 preset result 的切句差異過大 | High | single owner + unmatched；跨界 segment 不拆分。 |

| Golden 缺少 stable segment IDs | Medium | Renew 產生 Golden `segment_id`；edit 依位置保留。STT result segments 不含 IDs。 |

| Runner partial write | High | 先 complete-object replace result，再更新 manifest `stream_ids`/`updated_at`；index-before-result 禁止。 |

| Orphan result／missing manifest | Medium | 不列出、不選取；Backend 不從 result 推導 config。 |

| Completed index 與 result 不一致 | High | `preset_index_inconsistent`，不可假裝 empty transcription。 |

| Duplicate completed IDs | Low | Runner 與 Backend response 都去重、ascending sort。 |

| 多人同時編輯 Golden | Medium | v0.1 先假設單一 editor；optimistic locking 待後續決定。 |

| 影片與 STT time base 不一致 | High | metadata 記錄 duration/timebase/offset；validation warning。 |

| 未來 config 欄位需求變多 | Medium | Manifest `model`／`params` 作 source of truth；result schema 保持精簡。 |




# 16. Assumptions


- 一個 stream_id 對應一個可播放 media；若同一直播間有多段 media，應以 media_id 取代或補充 stream_id。

- 所有 STT results 與 Golden 使用同一 media time base；offset / trim 若存在必須在 metadata 表示。

- Runner 可靠地產生 UUID preset manifest、canonical result 與 completed list，Backend 對其唯讀。

- Manifest 是 config authority；同一 `(preset_id, stream_id)` 最多一份 canonical result，重跑不保留歷史。

- 第一版 reviewers 可以在單一 workspace／單一 backend instance 使用；權限不是主要 blocker。

- Golden edit 直接覆寫 flat `golden/{stream_id}.json`，不設計 revision history。

- Comparison 主要以 Golden row 閱讀；不要求每個 preset result 使用相同 segment count。


# 17. Deferred and future decisions


建議依以下順序逐節確認，因為前面的決策會影響後面的 schema、API 與 UI。



| ID | Decision | Recommended v0.1 default | Status |

| --- | --- | --- | --- |

| D-01 | Golden 是否允許修改 start/end？ | 允許；直接更新最新 Golden。 | Decided |

| D-02 | Golden 是否允許重疊 segments？ | 不允許；save validation 必須 reject。 | Decided |

| D-03 | Golden 是否需要 revision？ | 不需要；只保留最新 Golden。 | Decided |

| D-04 | Alignment 跨界 segment 如何呈現？ | 依 model segment 的 start_ms 歸屬單一 Golden row；不拆分、不重複顯示。 | Decided |

| D-05 | Model segment 在 gap/outside 是否 unmatched？ | 是；Golden 未覆蓋時間保持開放，segment 仍可播放。 | Decided |

| D-06 | 是否支援 Golden split/merge？ | 第二階段再做；v0.1 不支援。 | Decided |

| D-07 | Reviewer label vocabulary？ | 第二階段再規劃；v0.1 不規劃。 | Decided |

| D-08 | Reviewer annotation 是什麼、綁定哪些資料？ | Annotation 是人工標記／備註；整體功能延至第二階段。 | Deferred |

| D-09 | STT artifacts 如何進入系統？ | STT runner 寫入 local/shared preset manifests、canonical results、completed indexes；Backend request-time 唯讀 scan。 | Decided |

| D-10 | 是否保存完整 Golden history 可讀？ | 不保存；只保留最新 Golden。 | Decided |

| D-11 | stream_id 與 VOD 關係？ | STT 直接綁 stream_id；VOD 依 sequence 組成連續播放時間軸。 | Decided |

| D-12 | report/export 何時進入？ | M4 後另立 spec。 | Deferred |
| D-13 | STT／VOD 如何被發現？ | All streams 讀 VOD dirs；optional preset filter 取 VOD/completed/selectable-result 交集；detail 再掃該 stream 全部 results。 | Decided |
| D-14 | 是否採 logical continuous playback？ | v0.1 採用；以 ffprobe duration 建立 manifest，由 player integration 處理跨檔播放與 seek。 | Decided |
| D-16 | 沒有 Golden 時 UI 如何呈現？ | 第一個 selected preset result（Model A）作為 effective Golden，維持相同 alignment rows schema。 | Decided |
| D-17 | Renew Golden 的行為？ | 使用者以 `source_preset_id` 選 result 覆蓋 latest Golden，並記錄 `base_preset_id`；Renew 前確認。 | Decided |
| D-18 | v0.1 Golden 編輯範圍？ | 可修改 text/start/end；Save 後覆寫；禁止重疊；不支援新增、刪除、split、merge。 | Decided |
| D-19 | STT JSON 檔案位置？ | Preset manifest 位於 `data/presets/{preset_id}.json`；canonical result 位於 `data/stt/{stream_id}/{preset_id}.json`。 | Decided |
| D-20 | 比較需要幾個 STT result？ | UI 固定 Model A/B；A 必選、B optional，一或兩個不同 presets 各自與 Golden 比較。 | Decided |
| D-21 | 只有一個 result 時如何建立 Golden？ | Align 使用第一個 Model A result 作 effective Golden；之後可 Renew 保存或替換。 | Decided |
| D-22 | 選定 stream 後資料如何取得？ | Detail 回 VOD list 與全部 selectable `stt_results`；display config 來自 manifest，segments validation 延後。 | Decided |
| D-23 | STT segments 是否分頁？ | v0.1 一次回傳所有有效 STT segments；暫不做分頁或 lazy loading。 | Decided |
| D-24 | v0.1 多個 FLV 如何播放？ | 回傳依 sequence 排序的 FLV 清單與 ffprobe duration；單一 player 必須支援多檔連續播放與跨檔 seek。 | Decided |
| D-25 | 前端是否顯示多個 FLV？ | 不顯示；使用者只看到一個 video player，分段細節由 backend／player integration 隱藏。 | Decided |
| D-27 | 多個 FLV 之間有 gap 時如何播放？ | 直接接續下一個 FLV，不保留可見空白；start time 只用於排序。 | Decided |
| D-28 | GCS 如何提供給 Docker？ | GCE host Cloud Storage FUSE mount 同 bucket 四 prefixes，再 bind 至 Compose；container 不自行 mount。Local dev 用 local dirs。細節由 D-43 supersede。 | Decided |
| D-29 | 本地開發 VOD sample 格式？ | 沿用實際命名格式：`data/vod/{stream_id}/{starttime}_{sequence}_...flv`。 | Decided |
| D-30 | M0 需要哪些測試資料？ | VOD、preset manifests、duplicate completed IDs、valid/invalid results、orphan、index inconsistency、same-pair overwrite、Golden。 | Decided |
| D-31 | Result selector 顯示哪些欄位？ | 顯示 manifest name/model 與 result `created_at` 日本時間；opaque `preset_id` 不作主要 label。 | Decided |
| D-32 | Selector label 相同如何區分？ | UI 可附短 preset UUID；檔名不顯示，完整 identity 使用 `preset_id`。 | Decided |
| D-33 | Golden Renew 如何確認？ | API 不需要 `confirmation` 欄位；前端可自行顯示覆蓋確認視窗。 | Decided |
| D-34 | 舊 execution identity convention？ | 已由 D-42 supersede；v1 不保留額外 execution identity 或 result history。 | Superseded |
| D-15 | 如何判斷 FLV 屬於同一直播，以及播放順序？ | 第一層資料夾為 `stream_id`；檔名開頭 Unix start time 先排序；同 start time 再依 `_000` sequence 數字排序。 | Decided |
| D-38 | 如何判斷 mount 進來的 FLV 已完成？ | v1 假設 `data/vod` 中的檔案已完成；無法由 `ffprobe` 讀取時回傳 `422 vod_ingestion_failed`。live completion marker 與穩定大小檢查留待後續。 | Deferred |
| D-26 | 大型 merged asset 如何處理？ | v0.1 不產生 merged asset；未來若需要再評估完整檔案或 chunks + playlist。 | Proposed |
| D-35 | v1 是否定版？ | 是；critical scope 與 implementation decisions 已確認。Deferred／future items 不阻擋 v1 開發。 | Decided |
| D-36 | 設計文件衝突或 contract 更新如何處理？ | 必須交叉比對所有既有設計文件；先在 decision log 記錄 resolution，再同步更新所有受影響文件，不可只更新單一文件。 | Decided |
| D-37 | v1 Docker/runtime boundary？ | Nginx `8888`、Backend internal `8080`；source read-only、Golden read-write、`PRESET_ROOT`。GCE FUSE/IAM/single-writer 由 D-43 supersede。 | Decided |
| D-39 | Backend data contract？ | Preset manifest owns nested model config；result schema不重複 config；Latest Golden lineage 使用 preset identity。Exact fields 由 D-43 supersede，layout 由 D-40/D-41 supersede。 | Decided |
| D-40 | Backend source layout simplification？ | 確立 `backend/main.go` 單一入口與第一版扁平 Backend ownership，並 supersede D-39 的 source-layout 部分；package、tests 與 samples 的最終位置由 D-41 supersede。 | Superseded |
| D-41 | Post-Stage-2.2 final Backend ownership？ | `backend/router/router_test.go` 集中 router tests；`backend/samples` 擁有 validator/fixtures；`backend/models/{types.go,types_test.go}` 使用 `models` package，`domain`／`router` import `models`；`backend/domain` 保留業務 services。本決策 supersede D-40 layout，D-42 再擴充 preset fixtures。 | Decided |
| D-42 | Preset-first identity、filter 與 runner contract？ | Runner 產生 UUID manifest、canonical pair result、completed list；same-pair overwrite/no history。UI default All streams、selected filter取交集並清除 invalid current workspace；detail列全部 results。Exact schemas/storage/GCE contract由 D-43 supersede。 | Decided |
| D-43 | Exact preset/result/Golden schemas and GCE storage？ | Manifest exact fields為 `preset_id`、nested `model{name,params}`、completed `stream_ids`、`created_at`、`updated_at`；result exact fields為 pair identity、completion `created_at`、segments無ID；Golden flat `data/golden/{stream_id}.json` exact fields為 `stream_id`、`base_preset_id`、`updated_at`、stable-ID segments。GCE host FUSE mounts same-bucket four prefixes；attached service account具 bucket objectViewer及conditional golden-prefix objectUser；source :ro、Golden :rw、Backend single writer且不宣稱POSIX atomic rename。本決策 supersede D-28、D-37、D-39、D-42 中相衝突部分。 | Decided |




# 18. Decision log and iteration protocol


每次討論只需要指定要改哪個 decision／section；更新時保留原則與日期，避免 spec 變成沒有脈絡的最後版本。



| Date | Decision / change | Sections affected | Owner |

| --- | --- | --- | --- |

| 2026-08-17 | Created v0.1 baseline from referenced conversation | All sections; open decisions captured | Codex + user |
| 2026-08-17 | Finalized v1 after section-by-section review | Scope, storage, ingestion, playback, comparison, Golden, alignment, API, UI, milestones | Codex + user |
| 2026-08-18 | Synchronized Backend/Docker contract and established cross-document governance | Development environment, discovery, alignment, API, decisions | OpenCode + user |
| 2026-08-18 | Approved the previous STT parameter/schema simplification, sample naming, and Backend Go source layout | JSON schemas, discovery, API responses, UI params, milestones, Backend implementation design, D-39 | OpenCode + user |
| 2026-08-18 | Simplified Backend source layout and superseded the source-layout portion of D-39 | Backend entrypoint, router/domain responsibilities, sample validation path, implementation design, D-40 | OpenCode + user |
| 2026-08-18 | Finalized post-Stage-2.2 Backend package, test, and sample ownership | Models package, domain services, consolidated router tests, sample layout, implementation design, D-41 | OpenCode + user |
| 2026-08-18 | Approved preset-first filter, runner-owned manifests/results, canonical pair identity, and no result history | Scope, storage, APIs, UI, Docker, milestones, tests, terminology, D-42 | OpenCode + user |
| 2026-08-18 | Finalized exact preset/result/flat-Golden schemas and GCE host Cloud Storage FUSE/IAM contract | Storage, JSON schemas, API responses, Docker/runtime, tests, terminology, D-43 | OpenCode + user |

|  |  |  |  |




## 18.1 Document governance

- `spec-v1.md`、`api-spec.md`、`ui-webview.md` 與相關 implementation design 都是後續實作的必要參考，不可因新增文件而停止參考既有文件。
- 發現衝突時，先交叉比對 product/domain intent、API contract、UI behavior 與 implementation constraints，不可靜默選擇其中一份。
- Resolution 必須記錄於本文件 decision log，並在同一個 change 中更新所有受影響文件。
- Review 時應搜尋相同名詞、endpoint、error code、storage path 與狀態行為，確認沒有殘留舊規則。

## 18.2 Suggested first discussion order


1. Preset manifest/result/completed-index contract 與 runner atomic order。
2. Preset catalog、All/filter stream discovery 與 consistency errors。
3. Detail `stt_results`、Align `preset_ids` 與 Golden preset lineage。
4. Docker `PRESET_ROOT`/mount、fixtures、acceptance tests。


# Appendix A. Minimal test cases




| Case | Golden | Model | Expected |

| --- | --- | --- | --- |

| A1: exact containment | G [0,5000) | M [100,4000) | owner=G |

| A2: model split | G [0,5000) | M1 [100,2000), M2 [2100,4000) | both owner=G |

| A3: model crosses boundary | G1 [0,5000), G2 [5000,10000) | M [4000,7000) | owner=G1; 不拆分、不重複顯示 |

| A4: model in gap | G1 [0,5000), G2 [6000,10000) | M [5200,5500) | owner=null; unmatched |

| A5: model before/after | G [1000,5000) | M [0,500), M2 [6000,7000) | both unmatched |

| A6: Golden edit | Latest Golden | same M | Alignment recomputes against latest Golden |

| A7: stale save | Latest Golden | PUT Golden | Latest Golden is overwritten |

| A8: invalid Golden overlap | G1 [0,5000), G2 [4500,8000) | save | Reject per D-02 |

| P1: All streams default | VOD A,B | no preset filter | returns A,B ascending |

| P2: preset intersection | VOD A,B,C | completed B,C,D + valid results | returns B,C |

| P3: duplicate completion | manifest `stream_ids` B,B,A | valid results | response A,B |

| P4: orphan missing manifest | result exists | manifest absent | not listed/selectable |

| P5: unindexed result | result exists | stream absent completed list | not listed/selectable |

| P6: missing canonical result | manifest says completed | result absent | `preset_index_inconsistent` |

| P7: same-pair rerun | old canonical result | runner completes rerun | overwrite; result `created_at` changes; no history |

| P8: filter invalidates stream | selected stream not in new intersection | switch filter | clear workspace |

| P9: detail independent of filter | stream has presets A,B | filter A | detail returns A and B |

| P10: Golden lineage | Renew from preset result | save | flat path, `base_preset_id`, `updated_at`, stable Golden segment IDs |

| P11: exact schemas | manifests/results/Golden with obsolete or misplaced fields | validation | reject; API enrichment remains nested `model.params` |

| P12: GCE storage IAM | attached service account | source write + Golden write | source denied; Golden prefix allowed |

| P13: Golden writer | concurrent writer attempt | production | operationally prohibited; Backend is sole writer |




# Appendix B. Terminology




| Term | Definition |

| --- | --- |

| Preset | Runner-defined nested `model.name`/`model.params` configuration identified by opaque UUID `preset_id`. |

| Preset manifest | Exact config/index object with nested model, normalized completed `stream_ids`, created/updated times, stored under `data/presets`. |

| STT result | Canonical transcription for one `preset_id + stream_id`; same-pair rerun overwrites it atomically. |

| Completed index | Manifest `stream_ids`; runner updates it only after canonical result replacement succeeds. |

| Orphan result | Result without a valid manifest or completed membership; never selectable. |

| Preset index inconsistency | Manifest claims completion but canonical result is missing, malformed, or identity-mismatched. |

| Golden Sample / Golden | Human-maintained reference segments and text for a media; independent after bootstrap. |

| Latest Golden | Complete immutable snapshot of Golden after a save. |

| Base preset | The preset result used to bootstrap or Renew the latest Golden. |

| Owner | Deterministic Golden segment assigned to a model segment by start timestamp. |

| Overlap | Any Golden segment whose time interval intersects a model segment. |

| Unmatched | A model segment with no Golden owner under the current rule. |

| Time base | The shared media clock used by all timestamps. |



---

*STT Comparison + Golden Sample System · v1 · Final implementation contract*
