package request

import (
	"fmt"

	"github.com/google/uuid"
)

const MaxFacesBatchSize = 100

type FacesBatchRequest struct {
	IDs []uuid.UUID `json:"ids"`
}

func (r *FacesBatchRequest) Validate() error {
	if len(r.IDs) == 0 {
		return fmt.Errorf("ids is required")
	}
	if len(r.IDs) > MaxFacesBatchSize {
		return fmt.Errorf("too many ids, max %d", MaxFacesBatchSize)
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