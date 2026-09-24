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
The overlay uses the GKE GCS Fuse CSI driver and production bucket
`media17-stream-stt-prod`. The backend mounts only `samples/` read-only and
`golden_samples/` read-write; the frontend mounts only `samples/` read-only.
It does not create or share a PVC with `stt-service`.

The Pods use the dedicated Kubernetes ServiceAccount `stt-workbench`. Its access
is granted directly to the KSA principal in GCP IAM; no GSA annotation is required.

```sh
kubectl apply -k deploy/k8s/overlays/stag
```

The staging overlay image tag must match the full commit SHA of the `main` commit
being released. Update both `newTag` values in
`deploy/k8s/overlays/stag/kustomization.yaml` for every release. Changing the
image tag updates the Deployment pod template and triggers a rolling update.

The staging overlay currently uses:

- `gcr.io/media17-streaming/stt/workbench-backend:23cd2964d84405898427dae76133e7ec1faef276`
- `gcr.io/media17-streaming/stt/workbench-frontend:23cd2964d84405898427dae76133e7ec1faef276`

Change those image repositories in the overlay when the image registry is finalized.
The frontend Service is a `LoadBalancer`; use its assigned `EXTERNAL-IP` as the
staging entry point. No DNS name or managed certificate is required.
