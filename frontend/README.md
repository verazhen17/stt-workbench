# STT Workbench frontend

## Local development

Start the backend with the repository sample data:

```sh
cd backend
HTTP_ADDR=127.0.0.1:8080 \
SAMPLES_ROOT="$PWD/samples/samples" \
GOLDEN_ROOT="$PWD/samples/golden" \
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
