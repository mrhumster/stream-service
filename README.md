# stream-service

Video upload, HLS serving, stream catalog and real-time updates for GoCast.

## Functionality

- Stream CRUD + lifecycle: `draft → uploading → processing → ready → published`, `error`
  (failed transcode), with **reprocess** from the error state;
- Visibility model: `public` / `private` / `unlisted`;
- Upload: simple single-request for small files, **multipart** (`init` → `part` → `complete`)
  for larger ones;
- Publish/unpublish, per-owner access via identity permissions (gRPC);
- HLS serving from MinIO with Bearer auth, anti-cache `?t=` and path-traversal protection;
- Thumbnail + transcoding tasks dispatched to asynq workers (thumbnail/transcoder);
- Video metadata (recorded_at / location / camera) extracted by the transcoder and stored in
  the JSONB `streams.metadata` column;
- WebSocket hub (`STREAM_UPDATED` / `STREAM_READY`) to the stream owner;
- Prometheus `/metrics` (RED + business counters).

## Ports

| Protocol | Addr |
|---|---|
| HTTP API (incl. `/metrics`) | `:8080` (`SERVER_ADDR`) |
| gRPC (mTLS) | `:50051` |

## API endpoints

Routes are defined in `internal/delivery/http/routes/routes.go`.

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/stream` | – | Public catalog (paged, limit ≤ 100) |
| `GET` | `/stream/:id` | optional | Stream detail (access by owner/visibility) |
| `GET` | `/stream/:id/status` | – | Public status probe (`status`/`visibility`, `owner_id`/`title`) — used by comments/stats gates |
| `GET` | `/stream/:id/download` | bearer | Generate a signed download URL |
| `GET` | `/stream/:id/hls/*file` | optional | HLS playlist/segments from MinIO |
| `GET` | `/stream/ws/updates` | WS subprotocol | WebSocket updates for the owner |
| `POST` | `/stream/` | bearer | Create stream |
| `GET` | `/stream/own` | bearer | My streams (paged) |
| `PATCH` | `/stream/:id` | bearer + `stream/write` | Update stream |
| `DELETE` | `/stream/:id` | bearer + `stream/delete` | Delete stream |
| `POST` | `/stream/:id/publish` | bearer + `stream/write` | Publish |
| `POST` | `/stream/:id/unpublish` | bearer + `stream/write` | Unpublish |
| `POST` | `/stream/:id/upload` | bearer + `stream/write` | Simple upload (files < 5 MB) |
| `POST` | `/stream/:id/upload/init` | bearer + `stream/write` | Init multipart (metadata → uploadID) |
| `PUT` | `/stream/:id/upload/part` | bearer + `stream/write` | Upload a part (≥ 5 MB, except last) |
| `POST` | `/stream/:id/upload/complete` | bearer + `stream/write` | Complete multipart → process |
| `POST` | `/stream/:id/reprocess` | bearer + `stream/write` | Retry failed processing tasks (error → processing) |
| `GET` | `/stream/health` | – | Liveness/DB check |
| `GET` | `/metrics` | – | Prometheus metrics |

Most routes under the auth group run `authorize(permissionClient, "stream", ...)` so the
permissions come from the identity permission service via gRPC.

## Uploads

- **Simple**: `POST /stream/:id/upload` (multipart/form-data) for files ≤ 5 MB.
- **Multipart** (`internal/service/multipart`): `init` returns `uploadID`, parts are uploaded
  with `part` (a part must be ≥ 5 MB except the last — S3/MinIO protocol limit), then `complete`
  concats the parts, stores the final object in MinIO and dispatches thumbnail/transcoding tasks.
- The frontend sends chunks of 5 MB, 3 in parallel, then completes.

## Processing (task array)

`stream.processing` is a JSON **array** of `StreamProcessingTask` entries
(`task_type` `transcode` | `thumbnail`, `progress`, `steps`, `error`, `task_id`).
On upload completion stream-service enqueues **both** transcode and thumbnail tasks and
persists the initial array in one save. Workers report progress per task via gRPC
`UpdateStreamProcessingRequest.task`; a failed transcode task sets the stream status to
`error`, a failed thumbnail task only records its error (the stream stays playable).

`POST /stream/:id/reprocess` re-enqueues the failed/absent tasks (unique asynq IDs),
clears their errors and resets the stream to `processing`. Mapping: `404` not found,
`400` "stream is not in an error state", `409` source video missing in MinIO, else `500`.

## Video metadata

`streams.metadata` is a JSONB column (`models.StreamMetadata`) with:

```json
{
  "duration": 2,
  "size": 21694,
  "format": "hls",
  "resolution": "1280x720",
  "recorded_at": "2024-06-01T10:15:30Z",
  "location": "55.75580,37.61760",
  "camera": "PixelCam XC-42"
}
```

`duration`/`size`/`format`/`resolution` come from the processing pipelines; `recorded_at`,
`location`, `camera` and the authoritative `size` are reported by the transcoder via gRPC
`UpdateStreamMetadata` (parsed from ffprobe of the original file — see transcoder-service
README). New optional fields are `omitempty`, and the gRPC mapping parses `recorded_at`
tolerantly: an unparseable timestamp is ignored (`nil`) rather than failing the whole update
(`parseOptionalTime` in `internal/delivery/grpc/stream_handler.go`).

## HLS / access control

`GET /stream/:id/hls/*file` resolves the stream, checks visibility/owner
(`OptionalAuth`), then streams files from MinIO key `processed/<streamUUID>/<fileName>`.
- Bearer/anti-cache handled by the frontend (`?t=`), 403 → "Access denied";
- File names are validated (`TrimPrefix `/` `, rejects empty, `..` and `\`) to prevent path
  traversal — `GetFileByKey` in `stream_service_impl.go`.

## WebSocket

`/stream/ws/updates` requires auth via the **`Sec-WebSocket-Protocol`** subprotocol
(`WSProtocolAuth` middleware) — the server echoes the subprotocol during the handshake
(gorilla/websocket only echoes what the handler passes via `responseHeader`). `CheckOrigin`
is restricted to the configured allowed origins. The hub addresses updates by userID:
`STREAM_UPDATED` on stream changes, `STREAM_READY` when processing finishes.

## gRPC

- **Server** on `:50051` with mTLS — allowed OUs: `thumbnail-service`, `transcoder-service`.
  Workers call `UpdateStreamProcessing` (progress/error reporting, task-aware),
  `UpdateStreamMetadata` (recorded_at/location/camera/size via `UpdateStreamMetadataRequest`,
  fields `duration=4 size=5 recorded_at=6 location=7 camera=8`) and file helpers.
- **Client** to identity's permission service (`AUTH_SERVICE_ADDRESS`, mTLS, serverName
  `identity-service`) for `Enforce`/`CheckPermission`.

## Configuration (env)

| Variable | Purpose | Default |
|---|---|---|
| `SERVER_ADDR` | HTTP listen addr | `:8080` |
| `DB_HOST` / `DB_PORT` / `DB_NAME` | Postgres | `postgresql`/`5432`/`database1` |
| `DB_USER` / `DB_PASS` | Postgres credentials | _secret_ |
| `REDIS_ADDR` / `REDIS_PASS` | asynq queue + WS (asynq uses DB 2) | `localhost` / `password` |
| `MINIO_ENDPOINT` / `MINIO_ACCESS_KEY` / `MINIO_SECRET_KEY` | MinIO | `minio:9000` / `admin` / `minio123` |
| `MINIO_BUCKET_NAME` / `MINIO_REGION` / `MINIO_USE_SSL` | bucket | `go-app-bucket` / `us-east-1` / `false` |
| `JWT_ACCESS_PUBLIC_KEY_URL` | identity public key (token verification) | – |
| `AUTH_SERVICE_ADDRESS` | identity permission gRPC addr | – |
| `CORS_ALLOW_ORIGINS` | Allowed origins for CORS + WS | `http://localhost:5173,https://example.com,https://api.example.com` |
| `GRPC_TLS_CERT` / `GRPC_TLS_KEY` / `GRPC_TLS_CA` | mTLS server/client certs | _secret_ |
| `GRPC_TLS_ALLOWED_OUS` | Allowed peer OUs on gRPC server | – |
| `GRPC_TLS_ENABLED` | Enable mTLS on gRPC | `false` |
| `KEEP_ORIGINAL_FILE` | Keep source file after transcoding | `true` |
| `MODE` | `debug` / `release` | `debug` |
| `METRICS_ADDR` | (REST serves `/metrics` on the same port) | – |

In K8s values come from ConfigMap `stream-service-config` (envFrom) + Secret `go-app-secret`.

## Database

Schema is **owned by migrations** (`services/db-migrate`):

```bash
make -C services/db-migrate all   # rebuild runner image
make apply-db-migrate             # runs pending stream migrations (Job db-migrate-stream)
```

Version table: `schema_migrations_stream`. Note: `streams.owner_id` is `text` (a known legacy
inconsistency with `users.id uuid`). See `services/db-migrate/README.md`.

The stream image is built from the repo root structure; the identity module is consumed as an
external Go module (see `go.mod`).

## Build / run

```bash
make build        # Docker image xomrkob/stream:<git-tag>
make push
make deploy
```

## Deploy structure

```
deploy/k8s/
├── base/               # deployment (envFrom configMap + secrets) + service
├── minio/              # MinIO stateful deployment + PVC + secret
├── scaling/hpa.yaml    # CPU/mem autoscaling
└── (Metrics Service for the worker, in thumbnail/transcoder)
```

KEDA scales the workers: thumbnail/transcoder pods drop to 0 when there is no work — that is
normal.