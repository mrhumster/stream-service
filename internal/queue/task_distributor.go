//go:generate mockgen -source=task_distributor.go -destination=mock/task_distributor_mock.go -package=mock
package queue

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type TaskDistributor interface {
	DistributeVideoTranscoding(ctx context.Context, streamUUID uuid.UUID, inputPath string) error
}

type AsyncDistributor struct {
	client *asynq.Client
}

func NewAsyncDistributor(redisOpt asynq.RedisClientOpt) TaskDistributor {
	return &AsyncDistributor{
		client: asynq.NewClient(redisOpt),
	}
}

func (d *AsyncDistributor) DistributeVideoTranscoding(ctx context.Context, streamUUID uuid.UUID, inputPath string) error {
	return fmt.Errorf("not implemented")
}
