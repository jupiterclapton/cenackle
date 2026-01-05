package services

import (
	"context"
	"errors"

	"github.com/jupiterclapton/cenackle/services/graph-service/internal/core/domain"
	"github.com/jupiterclapton/cenackle/services/graph-service/internal/core/ports"
)

type graphService struct {
	repo ports.GraphRepository
}

func NewGraphService(repo ports.GraphRepository) ports.GraphService {
	return &graphService{repo: repo}
}

func (s *graphService) FollowUser(ctx context.Context, actorID, targetID string) error {
	if actorID == "" || targetID == "" {
		return errors.New("ids cannot be empty")
	}
	if actorID == targetID {
		return errors.New("cannot follow yourself")
	}
	return s.repo.CreateRelation(ctx, actorID, targetID)
}

func (s *graphService) UnfollowUser(ctx context.Context, actorID, targetID string) error {
	return s.repo.DeleteRelation(ctx, actorID, targetID)
}

func (s *graphService) CheckRelation(ctx context.Context, actorID, targetID string) (*domain.RelationStatus, error) {
	return s.repo.GetRelationStatus(ctx, actorID, targetID)
}

func (s *graphService) StreamFollowers(ctx context.Context, userID string, batchSize int, yield func([]string) error) error {
	return s.repo.StreamFollowersIDs(ctx, userID, batchSize, yield)
}
