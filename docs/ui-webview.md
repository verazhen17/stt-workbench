# STT 比較與 Golden 編輯 v1

## Based on provided mockup

本版沿用左側固定影片區、右側 alignment table 與上方 selectors，並採用已核准的 preset-first filter。本文必須與 `spec-v1.md`、`api-spec.md` 及 Backend implementation design 一起參考；contract resolution 必須同步更新所有受影響文件。

## 1. Overall layout

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ STT 比較與 Golden 編輯                                                        │
├──────────────────────────────────────────────────────────────────────────────┤
│ Preset filter [ All streams ▼ ]   直播間 ID [ filter stream ID… ▼ ]          │
│ Model A [ whisper large-v3 / preset name ▼ ]  Model B [ optional preset ▼ ]  │
├──────────────────────┬───────────────────────────────────────────────────────┤
│ 直播畫面              │ Golden │ Model A │ Model B                           │
│   ┌──────────────┐   ├────────┼─────────┼───────────────────────────────────┤
│   │    video     │   │ rows / model segments / unmatched / inline edit       │
│   └──────────────┘   │                                                       │
│ 00:01:23 / 01:25:30  │                                                       │
└──────────────────────┴───────────────────────────────────────────────────────┘
```

Layout principles：

- Optional Preset filter 必須位於 Stream selector 前；預設值是 **All streams**。
- Preset filter 只限制可選 stream，不限制 stream detail 的 results。
- 右側 table 欄位固定為 Golden、Model A、Model B；Golden 是 alignment 基準，不因切換 preset result 改變既有 saved Golden rows。
- 使用者只看到一個 video player；FLV 分段、logical playback 與跨檔 seek 不顯示。
- Desktop 優先；table 過窄時允許水平捲動。

## 2. Preset filter and Stream selector

```text
┌──────────────────────────────────────────────────────────────────────────────┐
│ Preset filter [ All streams ▼ ]   直播間 ID [ 214744544 ▼ ]                  │
│ Model A [ whisper-large-v3-default ▼ ]  Model B [ optional preset ▼ ]        │
└──────────────────────────────────────────────────────────────────────────────┘
```

### 2.1 Preset filter

- Page load 先呼叫 `GET /api/presets` 建立 optional filter choices。
- Default **All streams** 呼叫 `GET /api/streams`，顯示所有 VOD root 第一層 directories。
- 選定 preset 後呼叫 `GET /api/streams?preset_id={preset_id}`；Backend 回傳 manifest `stream_ids`（completed semantics）與 VOD directories 的交集。
- Preset manifest 的 nested `model.name`、`model.params` 是 config source of truth；不使用 provider，UI 不從 STT result 推導 config。
- Filter choice 與 Model A／B selection 是不同 state：filter 只縮小 stream 清單，Model A／B 仍從 stream detail 的全部 `stt_results` 選取。
- 若切換 filter 後 current stream 不在新清單，frontend 必須清除 current stream、VOD player、Model A/B selections、alignment rows、Golden draft/error state，回到未選 stream workspace。
- 若 current stream 仍在清單，保留 selection 並 refresh detail。
- `invalid_preset_id`、`preset_not_found` 或 `preset_index_inconsistent` 顯示在 filter/stream 區，不保留一份看似有效的 filtered list。

### 2.2 Stream selector

- Stream selector 只顯示 API 回傳的 `stream_id`。
- Dropdown 內文字欄仍可在已取得的 list 上做 frontend substring search；這與 server-side preset filter 不同，不新增其他 query。
- 沒有 matching stream 時顯示「No streams found」。Selected preset 沒有 completed/VOD intersection 時顯示「This preset has no completed streams with VOD」。
- 選定 stream 後呼叫 `GET /api/streams/{stream_id}`。
- Detail 一律顯示該 stream 全部 selectable `stt_results`，不因 preset filter 隱藏其他 results。

### 2.3 Model selectors

- Model A 必選，Model B optional；兩者必須使用不同 `preset_id`。
- Selector 顯示 manifest `model.name` 與 result `created_at`（日本時間）；opaque UUID 不作主要顯示文字。
- Model name 下方可展開 manifest `params`，例如 `temperature`、`beam_size`、`vad_filter`。
- Selector 列出 detail 的所有 `stt_results`；segments validity 延後到 Align。
- Public UI/API/storage 不使用另一層 execution identity。相同 preset/stream 重跑後只看到最新 canonical result。

## 3. Video panel

- Playback manifest 使用 detail response 的 `vod` array。
- 顯示單一 player，不顯示 FLV list。
- 點擊 sentence、Golden row 或 unmatched segment時，seek 到該 segment `start_ms` 並 pause。
- 播放器時間更新時，highlight 對應 row；FLV 之間直接接續，不保留 visible gap。
- Playback error 只影響影片區，已載入的 STT/Golden table 仍可閱讀。

## 4. Comparison table

```text
┌──────────────┬──────────────────────┬──────────────────────┐
│ Golden       │ whisper large-v3     │ whisper medium       │
│ time + text  │ [manifest params ▾]  │ [manifest params ▾]  │
├──────────────┼──────────────────────┼──────────────────────┤
│ 00:00–00:05  │ model segment        │ model segment        │
├──────────────┼──────────────────────┼──────────────────────┤
│ empty Golden │ unmatched segment    │                      │
└──────────────┴──────────────────────┴──────────────────────┘
```

Alignment presentation 保持不變：

- Golden row 使用 `start_ms–end_ms`；model segment 顯示自己的 interval 與 text。
- Model segment 依 `start_ms` 歸屬單一 Golden row；跨 row 不拆分、不重複。
- Golden gap、之前、之後的每個 unmatched segment 各形成一個 empty-Golden row。
- 不顯示 overlap list 或多重 owner。
- 點擊 model segment seek 到 model start；點擊 Golden row seek 到 Golden start。
- Golden pencil icon 進入 inline edit。
- Model A/B 欄展開的 config 來自 selected preset manifest，不來自 result JSON。

## 5. Golden column actions

### Existing Golden

```text
Golden（來源 preset：whisper-large-v3-default）
[ Renew Golden ]
```

- Golden lineage 使用 `base_preset_id`，header 的時間來自 Golden `updated_at`。
- Renew 來源是使用者明確指定的 selected preset result；request 使用 `source_preset_id`。
- Renew 完整覆寫 latest Golden，不保留 Golden revision history。
- Renew/Edit 成功 response 是 flat Golden schema：`stream_id`、`base_preset_id`、`updated_at`、`segments[segment_id,start_ms,end_ms,text]`；UI 不依賴其他 Golden metadata。

### Edit state

- v1 只允許修改既有 rows 的 text、start、end。
- 編輯先留在 frontend draft；Save 才呼叫 `PUT /api/streams/{stream_id}/golden`。
- Golden segments 重疊時顯示 validation error，不清除輸入。
- v1 不提供新增、刪除、split、merge。

## 6. Initial Golden behavior

若尚未保存 Golden，Align 使用 `preset_ids[0]` 對應的 Model A result 作為 effective Golden，維持相同 response/layout：

- Golden 欄使用第一個 selected result 的 segments。
- Model A 可單獨顯示；Model B optional。
- 不顯示 Set as Golden；後續使用 Renew Golden。
- Model A result 格式錯誤時，`selected_results[].error` 顯示錯誤，無法形成 rows。

## 7. Error states

### Selected result validation

- `invalid_preset_ids`：selection 不是一至兩個不同 UUIDs。
- `preset_not_found`：manifest 不存在，要求 refresh presets/detail。
- `stt_result_not_found`：preset exists，但 current stream 沒有 selectable result。
- Result segments 格式錯誤時，錯誤留在該 Model 欄；其他 valid model 仍照常顯示。

### Preset index consistency

- `preset_index_inconsistent` 是 manifest completed index 與 canonical result 不一致；顯示 global data consistency error，不將缺失 result 當作空 transcription。
- Result 缺 manifest 或不在 manifest `stream_ids` 時是 orphan，不出現在 filters、detail 或 selectors。
- Refresh 重新呼叫 `GET /api/presets`、目前 filter 對應的 streams request，以及 current stream detail。

## 8. Empty and loading states

- Loading：video panel 與 table 顯示 skeleton，不顯示假 empty state。
- No presets：顯示「目前沒有可用 preset」，但 All streams 仍可列出 VOD streams。
- Selected preset no streams：顯示「This preset has no completed streams with VOD」。
- Stream no results：顯示「此直播間目前沒有 selectable STT result；請由 STT runner 完成 preset result 後 Refresh。」Video 仍可播放，Model selectors 與 Align disabled。
- No VOD：Stream list 不出現沒有 VOD directory 的 stream；detail 後 VOD 消失則顯示 playback error。
- `vod_ingestion_failed`：影片區顯示 duration 解析錯誤，不清除已載入的 STT/Golden workspace。

## 9. Seek mapping

Backend 只提供 VOD cumulative timeline 與 align segment times；不提供 seek endpoint。

1. Player 使用 `global_time_ms` 尋找 `[timeline_start_ms, timeline_end_ms)` 對應 VOD。
2. `local_time_ms = global_time_ms - timeline_start_ms`。
3. Segment → video 時，切換至該 `vod[].url`、seek local time 並 pause。
4. Video → segment 時，以 Golden/model half-open intervals highlight row。

## 10. API mapping

| UI area | API |
|---|---|
| Preset filter choices | `GET /api/presets` |
| All streams default | `GET /api/streams` |
| Preset-filtered streams | `GET /api/streams?preset_id={preset_id}` |
| Workspace data / Model choices | `GET /api/streams/{stream_id}` → `stt_results` |
| Refresh | Presets + active streams query + current detail |
| Video player | Detail `vod` and `vod[].url` |
| Alignment rows | `GET /api/streams/{stream_id}/align?preset_ids=a[,b]` |
| Renew Golden | `PUT /api/streams/{stream_id}/golden` with `mode=renew`, `source_preset_id` |
| Save Golden edits | `PUT /api/streams/{stream_id}/golden` with `mode=edit` |

開發環境由 Nginx host port `8888` 提供唯一外部入口：`/api/*` proxy 至 Backend，`/vod/*` 直接由 read-only VOD mount 提供。

Local development 使用 local data directories。GCE Linux production 由 host Cloud Storage FUSE mount 同 bucket 的四個 prefixes，再 bind 到 Compose；frontend behavior 與 API paths 不變。

Frontend 只依賴 HTTP contract，不依賴 Go package path。Backend canonical ownership 維持：`backend/main.go` 單一入口、`backend/router` HTTP/runtime、`backend/models` shared types、`backend/domain` business services、`backend/samples` fixtures。
