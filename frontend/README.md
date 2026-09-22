# STT Workbench frontend

## Local development

Prepare the local data directories first:

```sh
mkdir -p backend/data/samples backend/data/golden
```

Start the backend with the local data directories:

```sh
cd backend
HTTP_ADDR=127.0.0.1:8080 \
SAMPLES_ROOT="$PWD/data/samples" \
GOLDEN_ROOT="$PWD/data/golden" \
VOD_URL_PREFIX=/vod \
FFPROBE_PATH="$(command -v ffprobe)" \
go run .
```

In another terminal, start the frontend:

```sh
cd frontend
npm install
npm run dev
```

Vite proxies `/api/*` to the local backend. Production deployments use the Nginx same-origin `/api` proxy.

## FLV 播放

播放器使用 `mpegts.js`，支援 H.264 FLV、傳統 CodecID 12 的 H.265 FLV，以及 Enhanced FLV 的 HEVC（`hvc1`）。錄影檔以 VOD 模式（`isLive: false`）播放，沿用 API 提供的 `flv_url`。

H.265 解碼需要瀏覽器與作業系統支援透過 Media Source Extensions 播放 HEVC；播放器不提供軟體解碼或自動轉碼。基本 MSE 支援檢查通過，不代表所有 HEVC profile 都能播放。載入、播放器與原生影片解碼錯誤會顯示於影片下方。

驗證方式：

- 執行 `npm run build`，確認 TypeScript 與正式版打包成功。
- 在目標瀏覽器分別播放 H.264、傳統 H.265 與 Enhanced HEVC FLV，確認畫面、聲音與字幕時間同步。
- 測試點擊字幕跳轉、拖曳進度、切換 VOD 與播放結束後切換下一段。
- 跳至尚未緩衝的位置時，需確認 FLV 包含可用的關鍵影格索引，且影片伺服器支援 HTTP Range。
- 在不支援 HEVC 的環境確認播放失敗時有錯誤訊息；切換至可播放的影片後，舊錯誤應清除。
