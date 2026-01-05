package ports

import (
	"context"

	"github.com/jupiterclapton/cenackle/services/feed-service/internal/core/domain"
)

// --- DRIVING (Ce que le service expose) ---

type FeedService interface {
	// DistributePost est appelé quand un event "PostCreated" arrive
	DistributePost(ctx context.Context, item *domain.FeedItem) error

	// GetTimeline est appelé par l'API Gateway pour l'affichage
	GetTimeline(ctx context.Context, req domain.FeedRequest) ([]*domain.FeedItem, error)
}
