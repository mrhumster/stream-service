package request

type StartUploadRequest struct {
	FileName    string `json:"file_name" binding:"required,min=3"`
	TotalSize   int64  `json:"total_size" binding:"required,gt=0"`
	ContentType string `json:"content_type" binding:"required"`
}
