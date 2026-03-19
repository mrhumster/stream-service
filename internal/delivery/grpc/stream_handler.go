package grpc

import (
	"github.com/mrhumster/stream-service/gen/go/stream"
	"github.com/mrhumster/stream-service/internal/service"
)

type StreamGRPCServer struct {
	stream.UnimplementedStreamServiceServer
	service *service.StreamService
}
