package response

type StartUploadResponse struct {
	UploadID string `json:"upload_id"`
	StreamID string `json:"stream_id"`
}
