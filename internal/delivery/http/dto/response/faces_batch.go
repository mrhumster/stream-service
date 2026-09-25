package response

type FacesBatchResponse struct {
	Processed []string               `json:"processed"`
	Failed    []FacesBatchFailureDTO `json:"failed"`
}

type FacesBatchFailureDTO struct {
	StreamID string `json:"stream_id"`
	Reason   string `json:"reason"`
}

func NewFacesBatchResponse(processed []string, failed []FacesBatchFailureDTO) FacesBatchResponse {
	return FacesBatchResponse{Processed: processed, Failed: failed}
}