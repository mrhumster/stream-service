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

type AsyncDistributor struct {
	client    *asynq.Client
	inspector *asynq.Inspector
}

func NewAsyncDistributor(redisOpt asynq.RedisClientOpt) TaskDistributor {
	return &AsyncDistributor{
		client:    asynq.NewClient(redisOpt),
		inspector: asynq.NewInspector(redisOpt),
	}
}

func (d *AsyncDistributor) DistributeVideoTranscoding(ctx context.Context, streamUUID uuid.UUID, inputPath string) (*string, error) {
	slog.Info("🚀 new task")
	payload, err := json.Marshal(VideoTranscodingPayload{
		StreamUUID: streamUUID,
		InputPath:  inputPath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	task := asynq.NewTask(TaskVideoTranscoding, payload, asynq.MaxRetry(3))
	info, err := d.client.EnqueueContext(ctx, task, asynq.TaskID(streamUUID.String()))
	if err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) {
			slog.Warn("task already equeued", "uuid", streamUUID)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to enqueue task: %w", err)
	}
	slog.Info("📩 Enqueue task:", "id", info.ID, "queue", info.Queue)
	return &info.ID, nil
}

func (d *AsyncDistributor) TerminateTask(ctx context.Context, taskID string) error {
	slog.Info("❌ terminate task", "TaskID", taskID)
	err := d.inspector.CancelProcessing(taskID)
	if err != nil {
		err = d.inspector.DeleteTask("default", taskID)
	}
	if err != nil {
		return fmt.Errorf("terminate task error: %w", err)
	}
	return nil
}
