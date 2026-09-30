package queue

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// Cascade contract with faces-worker: when a stream is deleted, faces-worker
// must drop all stored face data (occurrences, orphaned clusters, MinIO frames
// and crops). This is a dedicated queue + task type so it never competes with
// the events-service consumer on the shared "events" queue (asynq partitions
// tasks among consumers). Both queues live on the same Redis DB the faces
// worker already listens to (cfg.Redis.DB, DB 2).
const (
	TaskFacesStreamDeleted = "faces:stream.deleted"
	FacesCascadeQueue      = "faces-cascade"
)

type StreamDeletedPayload struct {
	StreamUUID uuid.UUID `json:"stream_uuid"`
}

// FacesCascadeRecorder enqueues face-cascade work after a stream deletion.
type FacesCascadeRecorder interface {
	RecordStreamDeleted(ctx context.Context, streamID uuid.UUID) error
}

// AsyncFacesCascadeRecorder enqueues cascade tasks via its own asynq client.
// Enqueue is best-effort: callers log the error and never fail the request.
type AsyncFacesCascadeRecorder struct {
	client *asynq.Client
}

func NewAsyncFacesCascadeRecorder(redisOpt asynq.RedisClientOpt) *AsyncFacesCascadeRecorder {
	return &AsyncFacesCascadeRecorder{client: asynq.NewClient(redisOpt)}
}

func (r *AsyncFacesCascadeRecorder) Close() error {
	return r.client.Close()
}

func (r *AsyncFacesCascadeRecorder) RecordStreamDeleted(ctx context.Context, streamID uuid.UUID) error {
	payload, err := json.Marshal(StreamDeletedPayload{StreamUUID: streamID})
	if err != nil {
		return err
	}
	task := asynq.NewTask(TaskFacesStreamDeleted, payload)
	_, err = r.client.EnqueueContext(ctx, task,
		asynq.Queue(FacesCascadeQueue),
		asynq.MaxRetry(3),
		asynq.Timeout(60*time.Second),
	)
	return err
}