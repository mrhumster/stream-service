//go:generate mockgen -source=task_distributor.go -destination=mock/task_distributor_mock.go -package=mock
package queue

import (
	"context"

	"github.com/google/uuid"
)

type TaskDistributor interface {
	DistributeVideoTranscoding(ctx context.Context, streamUUID uuid.UUID, inputPath string) (*string, error)
	TerminateTask(ctx context.Context, taskID string) error
}
