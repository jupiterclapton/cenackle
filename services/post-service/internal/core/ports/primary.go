package ports

import (
	"context"

	"github.com/jupiterclapton/cenackle/services/post-service/internal/core/domain"
)

type PostService interface {
	CreatePost(ctx context.Context, userID, content string, media []domain.Media) (*domain.Post, error)
	GetPost(ctx context.Context, postID string) (*domain.Post, error)
	UpdatePost(ctx context.Context, postID, userID, content string, media []domain.Media) (*domain.Post, error)
	DeletePost(ctx context.Context, postID, userID string) error

	// 👇 Méthodes de lecture avancées
	GetPosts(ctx context.Context, postIDs []string) ([]*domain.Post, error)
	ListPostsByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*domain.Post, string, error)
}
