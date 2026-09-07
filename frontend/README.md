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
