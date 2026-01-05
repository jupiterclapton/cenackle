package ports

import (
	"context"

	"github.com/jupiterclapton/cenackle/services/feed-service/internal/core/domain"
)

// --- DRIVEN (Ce dont le service a besoin) ---

type FeedRepository interface {
	// AddToTimelines ajoute un post dans les feeds de PLUSIEURS utilisateurs (Batch)
	AddToTimelines(ctx context.Context, userIDs []string, item *domain.FeedItem) error

	// GetTimeline récupère les items bruts depuis Redis
	GetTimeline(ctx context.Context, req domain.FeedRequest) ([]*domain.FeedItem, error)
}

type GraphClient interface {
	// GetFollowers récupère les ID des abonnés (Stream ou Pagination pour la perf)
	GetFollowers(ctx context.Context, userID string) ([]string, error)
}
