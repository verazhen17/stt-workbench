# Deployment

## Local

Create the local storage directories before starting the stack:

```text
backend/data/samples/
backend/data/golden/
```

Run from the repository root:

```sh
docker compose up --build
```

The browser entry point is `http://localhost:8888`. `samples` is mounted read-only;
`golden` is the only writable data directory.

## stt-service stag

The staging deployment follows the same split as local: Nginx/frontend is the only
public service and the Go backend remains a ClusterIP-only service on port 8080.
The overlay uses the same GKE GCS Fuse CSI driver and staging bucket as the
existing `stt-service`: `media17-stream-stt-stag`. The workbench reads
`samples/` and writes only `golden/`; it does not create or share a PVC with
`stt-service`.

```sh
kubectl apply -k deploy/k8s/overlays/stag
```

The staging overlay currently uses:

- `gcr.io/media17-streaming/stt/workbench-backend:stag`
- `gcr.io/media17-streaming/stt/workbench-frontend:stag`

Change those image repositories in the overlay when the image registry is finalized.
The frontend Service is a `LoadBalancer`; use its assigned `EXTERNAL-IP` as the
staging entry point. No DNS name or managed certificate is required.
