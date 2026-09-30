package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mrhumster/stream-service/gen/go/stream"
	"github.com/mrhumster/stream-service/internal/service"
	servicemock "github.com/mrhumster/stream-service/internal/service/mock"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestProtoMetadataReqToService(t *testing.T) {
	req := &stream.UpdateStreamMetadataRequest{
		StreamUuid: "0f6bd119-9e4c-4f3b-9f4a-2a4f2b1e7c1e",
		Duration:   3,
		Size:       1048576,
		Format:     "hls",
		Resolution: "1280x720",
		RecordedAt: "2024-02-29T12:34:56Z",
		Location:   "55.75580,37.61760",
		Camera:     "Apple iPhone 13 Pro Max",
	}

	got, err := protoMetadataReqToService(req)
	require.NoError(t, err)

	require.NotNil(t, got.Metadata.RecordedAt, "recorded_at must be parsed")
	require.True(t, got.Metadata.RecordedAt.Equal(time.Date(2024, 2, 29, 12, 34, 56, 0, time.UTC)))
	require.NotNil(t, got.Metadata.Location)
	require.Equal(t, "55.75580,37.61760", *got.Metadata.Location)
	require.NotNil(t, got.Metadata.Camera)
	require.Equal(t, "Apple iPhone 13 Pro Max", *got.Metadata.Camera)

	require.Equal(t, 3, got.Metadata.Duration)
	require.Equal(t, int64(1048576), got.Metadata.Size)
	require.Equal(t, "hls", got.Metadata.Format)
	require.Equal(t, "1280x720", got.Metadata.Resolution)
}

func TestProtoMetadataReqToService_EmptyOptionalFields(t *testing.T) {
	req := &stream.UpdateStreamMetadataRequest{
		StreamUuid: "0f6bd119-9e4c-4f3b-9f4a-2a4f2b1e7c1e",
		Duration:   3,
	}

	got, err := protoMetadataReqToService(req)
	require.NoError(t, err)

	require.Nil(t, got.Metadata.RecordedAt)
	require.Nil(t, got.Metadata.Location)
	require.Nil(t, got.Metadata.Camera)
}

func TestProtoMetadataReqToService_InvalidRecordedAtIgnored(t *testing.T) {
	req := &stream.UpdateStreamMetadataRequest{
		StreamUuid: "0f6bd119-9e4c-4f3b-9f4a-2a4f2b1e7c1e",
		RecordedAt: "not-a-date",
	}

	got, err := protoMetadataReqToService(req)
	require.NoError(t, err)

	require.Nil(t, got.Metadata.RecordedAt, "unparseable recorded_at must be ignored")
}

func TestProtoMetadataReqToService_InvalidUUID(t *testing.T) {
	req := &stream.UpdateStreamMetadataRequest{StreamUuid: "nope"}

	_, err := protoMetadataReqToService(req)
	require.Error(t, err)
}

func TestParseOptionalString(t *testing.T) {
	require.Nil(t, parseOptionalString(""))
	require.Equal(t, "x", *parseOptionalString("x"))
}
func TestCompleteStreamExport(t *testing.T) {
	streamID := "0f6bd119-9e4c-4f3b-9f4a-2a4f2b1e7c1e"

	t.Run("success forwards size to the service", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockSvc := servicemock.NewMockStreamService(ctrl)
		mockSvc.EXPECT().CompleteStreamExport(gomock.Any(), uuid.MustParse(streamID), int64(4096), "").Return(nil)
		srv := NewStreamGRPCServer(mockSvc)

		resp, err := srv.CompleteStreamExport(context.Background(), &stream.CompleteStreamExportRequest{
			StreamUuid: streamID,
			Success:    true,
			Size:       4096,
		})

		require.NoError(t, err)
		require.True(t, resp.Updated)
	})

	t.Run("failure is a business outcome, not an rpc error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockSvc := servicemock.NewMockStreamService(ctrl)
		mockSvc.EXPECT().CompleteStreamExport(gomock.Any(), uuid.MustParse(streamID), int64(0), "ffmpeg exited 1").Return(nil)
		srv := NewStreamGRPCServer(mockSvc)

		resp, err := srv.CompleteStreamExport(context.Background(), &stream.CompleteStreamExportRequest{
			StreamUuid: streamID,
			Success:    false,
			Error:      "ffmpeg exited 1",
		})

		require.NoError(t, err)
		require.True(t, resp.Updated)
	})

	t.Run("unknown stream maps to NotFound", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockSvc := servicemock.NewMockStreamService(ctrl)
		mockSvc.EXPECT().CompleteStreamExport(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(service.ErrExportNotFound)
		srv := NewStreamGRPCServer(mockSvc)

		_, err := srv.CompleteStreamExport(context.Background(), &stream.CompleteStreamExportRequest{
			StreamUuid: streamID, Success: true, Size: 1,
		})

		require.Error(t, err)
		require.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("a success without a size is rejected", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		srv := NewStreamGRPCServer(servicemock.NewMockStreamService(ctrl))

		_, err := srv.CompleteStreamExport(context.Background(), &stream.CompleteStreamExportRequest{
			StreamUuid: streamID, Success: true, Size: 0,
		})

		require.Error(t, err)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("bad uuid is rejected before the service is called", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		srv := NewStreamGRPCServer(servicemock.NewMockStreamService(ctrl))

		_, err := srv.CompleteStreamExport(context.Background(), &stream.CompleteStreamExportRequest{
			StreamUuid: "not-a-uuid", Success: true, Size: 1,
		})

		require.Error(t, err)
		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}
