package grpc

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/gen/go/stream"
	"github.com/mrhumster/stream-service/internal/domain/models"
	"github.com/mrhumster/stream-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type StreamGRPCServer struct {
	stream.UnimplementedStreamServiceServer
	streamService service.StreamService
}

func NewStreamGRPCServer(svc service.StreamService) *StreamGRPCServer {
	if svc == nil {
		log.Fatal("❌ Stream srevice is nil in NewStreamGRPCServer")
	}
	return &StreamGRPCServer{streamService: svc}
}

func mapProtoStatusToDomain(pbStatus stream.Status) models.StreamStatus {
	switch pbStatus {
	case stream.Status_STATUS_DRAFT:
		return models.StatusDraft
	case stream.Status_STATUS_PROCESSING:
		return models.StatusProcessing
	case stream.Status_STATUS_READY:
		return models.StatusReady
	case stream.Status_STATUS_PUBLISHED:
		return models.StatusPublished
	case stream.Status_STATUS_ERROR:
		return models.StatusError
	case stream.Status_STATUS_UPLOADING:
		return models.StatusUploading
	default:
		return ""
	}
}

func (s *StreamGRPCServer) UpdateStreamStatus(ctx context.Context, req *stream.UpdateStreamStatusRequest) (*stream.UpdateStreamStatusResponse, error) {
	streamID, err := uuid.Parse(req.StreamUuid)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	streamStatus := mapProtoStatusToDomain(req.Status)
	err = s.streamService.UpdateStreamStatus(ctx, streamID, streamStatus)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &stream.UpdateStreamStatusResponse{
		Updated: true,
	}, nil
}

func protoMetadataReqToService(req *stream.UpdateStreamMetadataRequest) (*service.UpdateStreamMetadataRequest, error) {
	streamUUID, err := uuid.Parse(req.StreamUuid)
	if err != nil {
		return nil, fmt.Errorf("error parse stream uuid: %w", err)
	}

	meta := models.StreamMetadata{
		Duration:   int(req.Duration),
		Format:     req.Format,
		Resolution: req.Resolution,
		Size:       req.Size,
		RecordedAt: parseOptionalTime(req.RecordedAt),
		Location:   parseOptionalString(req.Location),
		Camera:     parseOptionalString(req.Camera),
	}
	return &service.UpdateStreamMetadataRequest{
		StreamUUID: streamUUID,
		Metadata:   meta,
	}, nil
}

// parseOptionalTime parses an RFC3339 timestamp into a *time.Time, returning
// nil for empty input. Values that fail to parse are ignored so a malformed
// payload never fails the metadata update entirely.
func parseOptionalTime(v string) *time.Time {
	if v == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
		t = t.UTC()
		return &t
	}
	return nil
}

func parseOptionalString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func (s *StreamGRPCServer) UpdateStreamMetadata(ctx context.Context, req *stream.UpdateStreamMetadataRequest) (*stream.UpdateStreamMetadataResponse, error) {
	serviceReq, err := protoMetadataReqToService(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := s.streamService.UpdateStreamMetadata(ctx, serviceReq); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &stream.UpdateStreamMetadataResponse{Updated: true}, nil
}

func protoProcessingReqToService(req *stream.UpdateStreamProcessingRequest) (*service.UpdateStreamProcessingRequest, error) {
	streamUUID, err := uuid.Parse(req.StreamUuid)
	if err != nil {
		return nil, fmt.Errorf("error parse stream uuid: %w", err)
	}
	processing := models.StreamProcessingTask{
		TaskType: req.Task,
		Progress: int(req.Progress),
		Steps:    req.Steps,
		Error:    &req.Error,
	}
	return &service.UpdateStreamProcessingRequest{
		StreamUUID: streamUUID,
		Processing: processing,
	}, nil
}

func (s *StreamGRPCServer) UpdateStreamProcessing(ctx context.Context, req *stream.UpdateStreamProcessingRequest) (*stream.UpdateStreamProcessingResponse, error) {
	serviceReq, err := protoProcessingReqToService(req)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	if err := s.streamService.UpdateStreamProcessing(ctx, serviceReq); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &stream.UpdateStreamProcessingResponse{Updated: true}, nil
}
