//go:generate mockgen -source=export_task_distributor.go -destination=mock/export_task_distributor_mock.go -package=mock

package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

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
	// One export per stream at a time: the task id makes a second request while
	// the first is queued or running a no-op instead of a duplicate mux.
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

func exportTaskID(streamUUID uuid.UUID) string {
	return fmt.Sprintf("export-%s", streamUUID)
}
