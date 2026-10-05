package response

// ReprocessBatchResponse has the same shape as ForceErrorBatchResponse on
// purpose: the client renders "done / skipped and why" identically for both.
type ReprocessBatchResponse struct {
	Processed []string                `json:"processed"`
	Failed    []ReprocessBatchFailure `json:"failed"`
}

type ReprocessBatchFailure struct {
	StreamID string `json:"stream_id"`
	Reason   string `json:"reason"`
}

func NewReprocessBatchResponse(processed []string, failed []ReprocessBatchFailure) ReprocessBatchResponse {
	return ReprocessBatchResponse{Processed: processed, Failed: failed}
}
