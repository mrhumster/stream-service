package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type AsyncDistributor struct {
	client *asynq.Client
}

func NewAsyncDistributor(redisOpt asynq.RedisClientOpt) TaskDistributor {
	return &AsyncDistributor{
		client: asynq.NewClient(redisOpt),
	}
}

func (d *AsyncDistributor) DistributeVideoTranscoding(ctx context.Context, streamUUID uuid.UUID, inputPath string) error {
	slog.Info("🚀 new task")
	payload, err := json.Marshal(VideoTranscodingPayload{
		StreamUUID: streamUUID,
		InputPath:  inputPath,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	task := asynq.NewTask(TaskVideoTranscoding, payload, asynq.MaxRetry(3))
	info, err := d.client.EnqueueContext(ctx, task)
	if err != nil {
		return fmt.Errorf("failed to enqueue task: %w", err)
	}
	slog.Info("📩 Enqueue task:", "id", info.ID, "queue", info.Queue)
	return nil
}
