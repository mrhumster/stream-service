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
	task, err := d.newVideoTranscodingTask(streamUUID, inputPath, streamUUID.String())
	if err != nil {
		return nil, err
	}
	info, err := d.client.EnqueueContext(ctx, task, asynq.MaxRetry(1))
	if err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) {
			slog.Warn("task already equeued", "uuid", streamUUID)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to enqueue task: %w", err)
	}
	slog.Info("enqueue task:", "id", info.ID, "queue", info.Queue)
	return &info.ID, nil
}

func (d *AsyncDistributor) ReprocessVideoTranscoding(ctx context.Context, streamUUID uuid.UUID, inputPath string) (*string, error) {
	task, err := d.newVideoTranscodingTask(streamUUID, inputPath, fmt.Sprintf("reprocess-%s", uuid.New().String()))
	if err != nil {
		return nil, err
	}
	info, err := d.client.EnqueueContext(ctx, task, asynq.MaxRetry(1))
	if err != nil {
		return nil, fmt.Errorf("failed to enqueue re-process task: %w", err)
	}
	slog.Info("enqueue re-process task:", "id", info.ID, "queue", info.Queue)
	return &info.ID, nil
}

func (d *AsyncDistributor) newVideoTranscodingTask(streamUUID uuid.UUID, inputPath, taskID string) (*asynq.Task, error) {
	payload, err := json.Marshal(VideoTranscodingPayload{
		StreamUUID: streamUUID,
		InputPath:  inputPath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	return asynq.NewTask(TaskVideoTranscoding, payload, asynq.TaskID(taskID)), nil
}

func (d *AsyncDistributor) TerminateTask(ctx context.Context, taskID string) error {
	slog.Info("terminate task", "TaskID", taskID)
	err := d.inspector.CancelProcessing(taskID)
	if err != nil {
		err = d.inspector.DeleteTask("default", taskID)
	}
	if err != nil {
		return fmt.Errorf("terminate task error: %w", err)
	}
	return nil
}

func (d *AsyncDistributor) DistributeThumbsnailProcessor(ctx context.Context, streamUUID uuid.UUID, inputPath string) (*string, error) {
	task, err := d.newThumbnailTask(streamUUID, inputPath, fmt.Sprintf("thumbs-%s", streamUUID))
	if err != nil {
		return nil, err
	}
	info, err := d.client.EnqueueContext(
		ctx,
		task,
		asynq.MaxRetry(1),
		asynq.Queue("thumbsnails"),
	)
	if err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) {
			slog.Warn("task already equeued", "uuid", streamUUID)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to enqueue task: %w", err)
	}
	slog.Info("enqueue task:", "id", info.ID, "queue", info.Queue)
	return &info.ID, nil
}

func (d *AsyncDistributor) ReprocessThumbsnailProcessor(ctx context.Context, streamUUID uuid.UUID, inputPath string) (*string, error) {
	task, err := d.newThumbnailTask(streamUUID, inputPath, fmt.Sprintf("reprocess-thumbs-%s", uuid.New().String()))
	if err != nil {
		return nil, err
	}
	info, err := d.client.EnqueueContext(
		ctx,
		task,
		asynq.MaxRetry(1),
		asynq.Queue("thumbsnails"),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to enqueue re-process thumbsnail task: %w", err)
	}
	slog.Info("enqueue re-process thumbsnail task:", "id", info.ID, "queue", info.Queue)
	return &info.ID, nil
}

func (d *AsyncDistributor) newThumbnailTask(streamUUID uuid.UUID, inputPath, taskID string) (*asynq.Task, error) {
	payload, err := json.Marshal(ThumbsnailProcessorPayload{
		StreamUUID: streamUUID,
		InputPath:  inputPath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	return asynq.NewTask(TaskThumbsnailProcessor, payload, asynq.TaskID(taskID)), nil
}
