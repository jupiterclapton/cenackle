package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jupiterclapton/cenackle/services/feed-service/internal/core/domain"
	"github.com/redis/go-redis/v9"
)

type RedisFeedRepo struct {
	client *redis.Client
	ttl    time.Duration // Ex: 30 jours (on ne garde pas l'infini en RAM)
}

func NewRedisFeedRepo(client *redis.Client) *RedisFeedRepo {
	return &RedisFeedRepo{
		client: client,
		ttl:    24 * 30 * time.Hour,
	}
}

// AddToTimelines implémente le Fan-out massif
func (r *RedisFeedRepo) AddToTimelines(ctx context.Context, userIDs []string, item *domain.FeedItem) error {
	pipe := r.client.Pipeline()

	// Format du membre : "VIDEO:user-uuid-123:post-uuid-456"
	member := fmt.Sprintf("%s:%s:%s", item.Type, item.AuthorID, item.PostID)
	score := float64(item.CreatedAt.Unix())

	// Batch operation : On ajoute l'entrée pour chaque follower
	for _, uid := range userIDs {
		key := fmt.Sprintf("timeline:%s", uid)

		// 1. Ajout au Sorted Set
		pipe.ZAdd(ctx, key, redis.Z{
			Score:  score,
			Member: member,
		})

		// 2. Capping (Optionnel mais recommandé) : On garde max 500 items pour économiser la RAM
		// pipe.ZRemRangeByRank(ctx, key, 0, -501)

		// 3. Refresh TTL
		pipe.Expire(ctx, key, r.ttl)
	}

	// Exécution atomique (ou presque) du pipeline
	_, err := pipe.Exec(ctx)
	return err
}

// GetTimeline lit et filtre
func (r *RedisFeedRepo) GetTimeline(ctx context.Context, req domain.FeedRequest) ([]*domain.FeedItem, error) {
	key := fmt.Sprintf("timeline:%s", req.UserID)

	// Pagination Redis (Inclusive)
	start := req.Offset
	stop := req.Offset + req.Limit - 1

	results, err := r.client.ZRevRangeWithScores(ctx, key, start, stop).Result()
	if err != nil {
		return nil, err
	}

	items := make([]*domain.FeedItem, 0, len(results))

	// Préparation du filtre (optimisation map)
	filterMap := make(map[domain.ContentType]bool)
	for _, t := range req.Types {
		filterMap[t] = true
	}
	hasFilter := len(filterMap) > 0

	for _, z := range results {
		member, ok := z.Member.(string)
		if !ok {
			continue
		}

		// 🟢 CORRECTION EXPERTE : Parsing robuste
		// On s'attend à "TYPE:AUTHOR_ID:POST_ID"
		parts := strings.Split(member, ":")

		// Gestion de la compatibilité (si jamais on a encore des vieilles données)
		var contentType domain.ContentType
		var postID, authorID string

		if len(parts) == 3 {
			// Nouveau format
			contentType = domain.ContentType(parts[0])
			authorID = parts[1]
			postID = parts[2]
		} else if len(parts) == 2 {
			// Ancien format (fallback pour ne pas crasher)
			contentType = domain.ContentType(parts[0])
			postID = parts[1]
			authorID = "" // On ne l'a pas
		} else {
			// Format inconnu (donnée corrompue ?)
			continue
		}

		// Filtrage Applicatif
		if hasFilter && !filterMap[contentType] {
			continue
		}

		items = append(items, &domain.FeedItem{
			PostID:    postID,
			AuthorID:  authorID, // ✅ Maintenant c'est rempli
			Type:      contentType,
			CreatedAt: time.Unix(int64(z.Score), 0),
		})
	}

	return items, nil
}
