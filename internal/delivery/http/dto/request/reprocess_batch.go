package request

import (
	"fmt"

	"github.com/google/uuid"
)

// MaxReprocessBatchSize matches MaxForceErrorBatchSize: one request mutates a
// row per id, and the batch handler reports per-id results the client renders
// individually.
const MaxReprocessBatchSize = 100

type ReprocessBatchRequest struct {
	IDs []uuid.UUID `json:"ids"`
}

func (r *ReprocessBatchRequest) Validate() error {
	if len(r.IDs) == 0 {
		return fmt.Errorf("ids is required")
	}
	if len(r.IDs) > MaxReprocessBatchSize {
		return fmt.Errorf("too many ids, max %d", MaxReprocessBatchSize)
	}
	seen := make(map[uuid.UUID]struct{}, len(r.IDs))
	for _, id := range r.IDs {
		if id == uuid.Nil {
			return fmt.Errorf("invalid id: nil uuid")
		}
		if _, ok := seen[id]; ok {
			return fmt.Errorf("duplicate id: %s", id)
		}
		seen[id] = struct{}{}
	}
	return nil
}
