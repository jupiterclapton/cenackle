package graph

import (
	"time"

	feedv1 "github.com/jupiterclapton/cenackle/gen/feed/v1"
	identityv1 "github.com/jupiterclapton/cenackle/gen/identity/v1"
	postv1 "github.com/jupiterclapton/cenackle/gen/post/v1"
	"github.com/jupiterclapton/cenackle/services/api-gateway/graph/model"
)

// --- IDENTITY MAPPERS ---

func mapProtoUserToGraph(u *identityv1.User) *model.User {
	if u == nil {
		return nil
	}

	// Gestion sécurisée des dates
	var createdAt time.Time
	if u.CreatedAt != nil {
		createdAt = u.CreatedAt.AsTime()
	}

	var updatedAt time.Time
	if u.UpdatedAt != nil {
		updatedAt = u.UpdatedAt.AsTime()
	}

	return &model.User{
		ID:        u.Id,
		Email:     u.Email,
		Username:  u.Username,
		FullName:  u.FullName,
		IsActive:  u.IsActive,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
}

// --- CONTENT MAPPERS ---

// Map pour les médias du Post Service
func mapProtoMediaToGraph(protoMedia []*postv1.Media) []*model.Media {
	if protoMedia == nil {
		return []*model.Media{}
	}

	res := make([]*model.Media, len(protoMedia))
	for i, m := range protoMedia {
		res[i] = &model.Media{
			ID:   m.Id,
			URL:  m.Url,
			Type: m.Type,
		}
	}
	return res
}

// Helper optionnel si on veut mapper un FeedItem directement (si besoin plus tard)
func mapFeedItemToPostID(items []*feedv1.FeedItem) []string {
	ids := make([]string, len(items))
	for i, item := range items {
		ids[i] = item.PostId
	}
	return ids
}
