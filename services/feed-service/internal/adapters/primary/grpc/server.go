package grpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	feedv1 "github.com/jupiterclapton/cenackle/gen/feed/v1"
	"github.com/jupiterclapton/cenackle/services/feed-service/internal/core/domain"
	"github.com/jupiterclapton/cenackle/services/feed-service/internal/core/ports"
)

type Server struct {
	feedv1.UnimplementedFeedServiceServer
	service ports.FeedService
}

func NewServer(service ports.FeedService) *Server {
	return &Server{service: service}
}

func (s *Server) Register(grpcServer *grpc.Server) {
	feedv1.RegisterFeedServiceServer(grpcServer, s)
}

func (s *Server) GetTimeline(ctx context.Context, req *feedv1.GetTimelineRequest) (*feedv1.GetTimelineResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20
	}
	offset := int(req.Offset)
	if offset < 0 {
		offset = 0
	}

	// Appel du Core Domain
	domainReq := domain.FeedRequest{
		UserID: req.UserId,
		Limit:  int64(limit),
		Offset: int64(offset),
	}

	items, err := s.service.GetTimeline(ctx, domainReq)
	if err != nil {
		slog.Error("Failed to get timeline", "error", err)
		return nil, status.Error(codes.Internal, "failed to fetch timeline")
	}

	// Mapping Domain -> Proto
	protoItems := make([]*feedv1.FeedItem, len(items))
	for i, item := range items {
		protoItems[i] = &feedv1.FeedItem{
			PostId:    item.PostID,
			AuthorId:  item.AuthorID, // Note: stocké dans Redis ou à déduire si manquant
			Type:      string(item.Type),
			CreatedAt: timestamppb.New(item.CreatedAt),
		}
	}

	return &feedv1.GetTimelineResponse{
		Items: protoItems,
	}, nil
}
