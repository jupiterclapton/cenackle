package ports

import (
	"context"

	"github.com/jupiterclapton/cenackle/services/graph-service/internal/core/domain"
)

// GraphRepository est le port Driven (Database Neo4j)
type GraphRepository interface {
	// EnsureSchema crée les contraintes et index (Idempotent)
	EnsureSchema(ctx context.Context) error

	CreateRelation(ctx context.Context, actorID, targetID string) error
	DeleteRelation(ctx context.Context, actorID, targetID string) error
	GetRelationStatus(ctx context.Context, actorID, targetID string) (*domain.RelationStatus, error)

	// StreamFollowersIDs doit utiliser le curseur natif de Neo4j pour la performance
	StreamFollowersIDs(ctx context.Context, userID string, batchSize int, yield func([]string) error) error
}
