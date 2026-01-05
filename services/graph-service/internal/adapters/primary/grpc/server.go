package grpc

import (
	"context"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	graphv1 "github.com/jupiterclapton/cenackle/gen/graph/v1"
	"github.com/jupiterclapton/cenackle/services/graph-service/internal/core/ports"
)

type Server struct {
	graphv1.UnimplementedGraphServiceServer
	service ports.GraphService
}

func NewServer(service ports.GraphService) *Server {
	return &Server{service: service}
}

func (s *Server) Register(grpcServer *grpc.Server) {
	graphv1.RegisterGraphServiceServer(grpcServer, s)
}

func (s *Server) FollowUser(ctx context.Context, req *graphv1.FollowUserRequest) (*emptypb.Empty, error) {
	err := s.service.FollowUser(ctx, req.FollowerId, req.FolloweeId)
	if err != nil {
		slog.Error("Follow failed", "error", err)
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) UnfollowUser(ctx context.Context, req *graphv1.UnfollowUserRequest) (*emptypb.Empty, error) {
	err := s.service.UnfollowUser(ctx, req.FollowerId, req.FolloweeId)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &emptypb.Empty{}, nil
}

func (s *Server) CheckRelation(ctx context.Context, req *graphv1.CheckRelationRequest) (*graphv1.CheckRelationResponse, error) {
	statusRel, err := s.service.CheckRelation(ctx, req.ActorId, req.TargetId)
	if err != nil {
		return nil, status.Error(codes.Internal, "internal error")
	}
	return &graphv1.CheckRelationResponse{
		IsFollowing:  statusRel.IsFollowing,
		IsFollowedBy: statusRel.IsFollowedBy,
	}, nil
}

func (s *Server) StreamFollowers(req *graphv1.StreamFollowersRequest, stream graphv1.GraphService_StreamFollowersServer) error {
	const BatchSize = 1000

	err := s.service.StreamFollowers(stream.Context(), req.UserId, BatchSize, func(ids []string) error {
		return stream.Send(&graphv1.StreamFollowersResponse{
			FollowerIds: ids,
		})
	})

	if err != nil {
		slog.Error("Streaming failed", "error", err)
		return status.Error(codes.Internal, "streaming failed")
	}
	return nil
}

// GetFollowers (Not implemented yet - placeholder)
func (s *Server) GetFollowers(ctx context.Context, req *graphv1.GetFollowersRequest) (*graphv1.GetFollowersResponse, error) {
	return nil, status.Error(codes.Unimplemented, "use StreamFollowers")
}

// GetFollowing (Not implemented yet)
func (s *Server) GetFollowing(ctx context.Context, req *graphv1.GetFollowingRequest) (*graphv1.GetFollowingResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}
