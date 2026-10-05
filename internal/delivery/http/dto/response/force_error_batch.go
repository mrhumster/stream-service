package response

// ForceErrorBatchResponse has the same shape as FacesBatchResponse on
// purpose: the client renders "done / skipped and why" identically for both.
type ForceErrorBatchResponse struct {
	Processed []string                 `json:"processed"`
	Failed    []ForceErrorBatchFailure `json:"failed"`
}

type ForceErrorBatchFailure struct {
	StreamID string `json:"stream_id"`
	Reason   string `json:"reason"`
}

func NewForceErrorBatchResponse(processed []string, failed []ForceErrorBatchFailure) ForceErrorBatchResponse {
	return ForceErrorBatchResponse{Processed: processed, Failed: failed}
}
