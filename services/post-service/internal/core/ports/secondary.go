package ports

import (
	"context"
	"time" // N'oubliez pas l'import

	"github.com/jupiterclapton/cenackle/services/post-service/internal/core/domain"
)

type PostRepository interface {
	Save(ctx context.Context, post *domain.Post) error
	FindByID(ctx context.Context, postID string) (*domain.Post, error)
	Delete(ctx context.Context, postID string) error

	// Utilisé pour l'hydratation du Feed (Batch)
	GetPosts(ctx context.Context, postIDs []string) ([]*domain.Post, error)

	// Utilisé pour la pagination Profil (Cursor-based)
	// Notez qu'ici on utilise time.Time, car le repo parle "Date", pas "Token string"
	ListByAuthor(ctx context.Context, authorID string, limit int, cursorTime time.Time) ([]*domain.Post, error)

	// Si vous avez Update dans le gRPC, il le faut aussi ici
	Update(ctx context.Context, post *domain.Post) error
}

type EventPublisher interface {
	PublishPostCreated(ctx context.Context, post *domain.Post) error
	PublishPostDeleted(ctx context.Context, postID string) error
}
