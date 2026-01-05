package grpc

import (
	"context"
	"errors"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	postv1 "github.com/jupiterclapton/cenackle/gen/post/v1"
	"github.com/jupiterclapton/cenackle/services/post-service/internal/core/domain"
	"github.com/jupiterclapton/cenackle/services/post-service/internal/core/ports"
)

type Server struct {
	postv1.UnimplementedPostServiceServer
	service ports.PostService
}

func NewServer(service ports.PostService) *Server {
	return &Server{service: service}
}

func (s *Server) Register(grpcServer *grpc.Server) {
	postv1.RegisterPostServiceServer(grpcServer, s)
}

// --- COMMANDS (Write) ---

func (s *Server) CreatePost(ctx context.Context, req *postv1.CreatePostRequest) (*postv1.CreatePostResponse, error) {
	if req.UserId == "" || req.Content == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and content are required")
	}

	// Mapping Proto -> Domain
	domainMedia := mapProtoMediaToDomain(req.Media)

	post, err := s.service.CreatePost(ctx, req.UserId, req.Content, domainMedia)
	if err != nil {
		slog.Error("Failed to create post", "error", err)
		return nil, status.Error(codes.Internal, "failed to create post")
	}

	return &postv1.CreatePostResponse{
		Post: mapDomainToProto(post),
	}, nil
}

func (s *Server) UpdatePost(ctx context.Context, req *postv1.UpdatePostRequest) (*postv1.UpdatePostResponse, error) {
	if req.PostId == "" {
		return nil, status.Error(codes.InvalidArgument, "post_id required")
	}

	domainMedia := mapProtoMediaToDomain(req.Media)

	post, err := s.service.UpdatePost(ctx, req.PostId, req.UserId, req.Content, domainMedia)
	if err != nil {
		// Gestion fine des erreurs (si on avait des erreurs typées dans le domain)
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &postv1.UpdatePostResponse{
		Post: mapDomainToProto(post),
	}, nil
}

func (s *Server) DeletePost(ctx context.Context, req *postv1.DeletePostRequest) (*emptypb.Empty, error) {
	if req.PostId == "" || req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "ids required")
	}

	err := s.service.DeletePost(ctx, req.PostId, req.UserId)
	if err != nil {
		// Exemple de gestion d'erreur d'autorisation
		if errors.Is(err, errors.New("unauthorized")) { // À remplacer par var sentinel
			return nil, status.Error(codes.PermissionDenied, "unauthorized")
		}
		return nil, status.Error(codes.Internal, "failed to delete")
	}

	return &emptypb.Empty{}, nil
}

// --- QUERIES (Read) ---

func (s *Server) GetPost(ctx context.Context, req *postv1.GetPostRequest) (*postv1.GetPostResponse, error) {
	post, err := s.service.GetPost(ctx, req.PostId)
	if err != nil {
		return nil, status.Error(codes.NotFound, "post not found")
	}
	return &postv1.GetPostResponse{Post: mapDomainToProto(post)}, nil
}

// GetPosts : BATCH FETCH (Pour le Feed Service)
func (s *Server) GetPosts(ctx context.Context, req *postv1.GetPostsRequest) (*postv1.GetPostsResponse, error) {
	if len(req.PostIds) == 0 {
		return &postv1.GetPostsResponse{Posts: []*postv1.Post{}}, nil
	}

	posts, err := s.service.GetPosts(ctx, req.PostIds)
	if err != nil {
		slog.Error("Batch fetch failed", "error", err)
		return nil, status.Error(codes.Internal, "failed to fetch posts")
	}

	// Mapping de la liste
	protoPosts := make([]*postv1.Post, len(posts))
	for i, p := range posts {
		protoPosts[i] = mapDomainToProto(p)
	}

	return &postv1.GetPostsResponse{Posts: protoPosts}, nil
}

// ListPostsByAuthor : PAGINATION
func (s *Server) ListPostsByAuthor(ctx context.Context, req *postv1.ListPostsByAuthorRequest) (*postv1.ListPostsByAuthorResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 20 // Défaut
	}
	if limit > 100 {
		limit = 100 // Protection
	}

	// On passe le token (string) au service qui gérera le décodage vers time.Time
	// Ou on le fait ici. Dans l'architecture hexagonale pure, l'adapter (ici) gère le format protocolaire.
	// Mais pour simplifier l'interface service, passons le string.

	posts, nextCursor, err := s.service.ListPostsByAuthor(ctx, req.AuthorId, limit, req.PageToken)
	if err != nil {
		return nil, err
	}

	protoPosts := make([]*postv1.Post, len(posts))
	for i, p := range posts {
		protoPosts[i] = mapDomainToProto(p)
	}

	return &postv1.ListPostsByAuthorResponse{
		Posts:         protoPosts,
		NextPageToken: nextCursor,
	}, nil
}

// --- HELPERS (Mappers) ---

func mapDomainToProto(p *domain.Post) *postv1.Post {
	if p == nil {
		return nil
	}

	protoMedia := make([]*postv1.Media, len(p.Media))
	for i, m := range p.Media {
		protoMedia[i] = &postv1.Media{
			Id:   m.ID,
			Url:  m.URL,
			Type: string(m.Type),
		}
	}

	return &postv1.Post{
		Id:        p.ID,
		AuthorId:  p.UserID,
		Content:   p.Content,
		Media:     protoMedia, // On renvoie l'objet riche (ID+URL+Type)
		CreatedAt: timestamppb.New(p.CreatedAt),
		UpdatedAt: timestamppb.New(p.UpdatedAt),
	}
}

func mapProtoMediaToDomain(protoMedia []*postv1.Media) []domain.Media {
	domainMedia := make([]domain.Media, len(protoMedia))
	for i, m := range protoMedia {
		domainMedia[i] = domain.Media{
			ID:   m.Id,
			URL:  m.Url,
			Type: domain.MediaType(m.Type),
		}
	}
	return domainMedia
}
