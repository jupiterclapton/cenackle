package ports

import (
	"context"

	"github.com/jupiterclapton/cenackle/services/graph-service/internal/core/domain"
)

// GraphService est le port Driving (API)
type GraphService interface {
	FollowUser(ctx context.Context, actorID, targetID string) error
	UnfollowUser(ctx context.Context, actorID, targetID string) error
	CheckRelation(ctx context.Context, actorID, targetID string) (*domain.RelationStatus, error)

	// StreamFollowers est crucial pour le Fan-out.
	// Il renvoie les followers par paquets via le callback 'yield'.
	StreamFollowers(ctx context.Context, userID string, batchSize int, yield func([]string) error) error
}
