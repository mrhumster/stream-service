//go:generate mockgen -source=export_task_distributor.go -destination=mock/export_task_distributor_mock.go -package=mock

package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// ExportTaskDistributor asks the export worker to build the single-file
// rendition of a stream. It is deliberately a separate interface from
// TaskDistributor: the export queue sits in its own Redis DB so the export
// service scales independently of transcoding.
type ExportTaskDistributor interface {
	DistributeVideoExport(ctx context.Context, streamUUID uuid.UUID, ownerUUID uuid.UUID, ownerEmail string) (*string, error)
}

type AsyncExportDistributor struct {
	client    *asynq.Client
	inspector *asynq.Inspector
}

func NewAsyncExportDistributor(redisOpt asynq.RedisClientOpt) ExportTaskDistributor {
	return &AsyncExportDistributor{
		client:    asynq.NewClient(redisOpt),
		inspector: asynq.NewInspector(redisOpt),
	}
}

func (d *AsyncExportDistributor) DistributeVideoExport(ctx context.Context, streamUUID uuid.UUID, ownerUUID uuid.UUID, ownerEmail string) (*string, error) {
	task, err := newVideoExportTask(streamUUID, ownerUUID, ownerEmail)
	if err != nil {
		return nil, err
	}
	// The id carries the attempt, not just the stream. A fixed id per stream
	// blocks the retry button for as long as asynq keeps the finished task
	// around, which made a retry fail with "task ID conflicts with another
	// task" long after the export had already settled. One export at a time per
	// stream is decided by the export row, which is reset conditionally so two
	// retries cannot both queue a mux.
	info, err := d.client.EnqueueContext(ctx, task,
		asynq.TaskID(exportTaskID(streamUUID)),
		asynq.MaxRetry(1),
		asynq.Timeout(exportTaskTimeout),
	)
	if err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) {
			slog.Warn("export task already enqueued", "stream", streamUUID)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to enqueue export task: %w", err)
	}
	slog.Info("enqueue export task:", "id", info.ID, "queue", info.Queue, "stream", streamUUID)
	return &info.ID, nil
}

func newVideoExportTask(streamUUID, ownerUUID uuid.UUID, ownerEmail string) (*asynq.Task, error) {
	payload, err := json.Marshal(VideoExportPayload{
		StreamUUID: streamUUID,
		OwnerUUID:  ownerUUID,
		OwnerEmail: ownerEmail,
	})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TaskVideoExport, payload), nil
}

// exportTaskID names a single attempt. The timestamp suffix keeps a retry from
// colliding with the id of the attempt that just failed, which asynq keeps
// holding for its retention window.
func exportTaskID(streamUUID uuid.UUID) string {
	return fmt.Sprintf("export-%s-%d", streamUUID, time.Now().UnixNano())
}
