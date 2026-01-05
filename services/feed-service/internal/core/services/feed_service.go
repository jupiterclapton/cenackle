package services

import (
	"context"
	"log/slog"

	"github.com/jupiterclapton/cenackle/services/feed-service/internal/core/domain"
	"github.com/jupiterclapton/cenackle/services/feed-service/internal/core/ports"
)

const BatchSize = 1000 // Taille des paquets pour Redis

type FeedService struct {
	repo        ports.FeedRepository
	graphClient ports.GraphClient
}

func NewFeedService(repo ports.FeedRepository, graph ports.GraphClient) *FeedService {
	return &FeedService{
		repo:        repo,
		graphClient: graph,
	}
}

func (s *FeedService) DistributePost(ctx context.Context, item *domain.FeedItem) error {
	slog.Info("📢 Fan-out starting", "post_id", item.PostID, "author_id", item.AuthorID)

	// 1. Récupérer les followers via gRPC (Graph Service)
	followers, err := s.graphClient.GetFollowers(ctx, item.AuthorID)
	if err != nil {
		return err
	}

	if len(followers) == 0 {
		return nil
	}

	// 2. Batch Processing (Chunking)
	// On découpe la liste des followers pour ne pas saturer Redis ou la RAM
	for i := 0; i < len(followers); i += BatchSize {
		end := i + BatchSize
		if end > len(followers) {
			end = len(followers)
		}

		batch := followers[i:end]

		// 3. Écriture Redis (Pipeline)
		err := s.repo.AddToTimelines(ctx, batch, item)
		if err != nil {
			slog.Error("❌ Failed to push batch to redis", "error", err, "batch_start", i)
			// En prod : On pourrait envoyer ça dans une Dead Letter Queue ou retenter
			continue
		}
	}

	slog.Info("✅ Fan-out complete", "count", len(followers))
	return nil
}

func (s *FeedService) GetTimeline(ctx context.Context, req domain.FeedRequest) ([]*domain.FeedItem, error) {
	return s.repo.GetTimeline(ctx, req)
}
