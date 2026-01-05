package services

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jupiterclapton/cenackle/services/post-service/internal/core/domain"
	"github.com/jupiterclapton/cenackle/services/post-service/internal/core/ports"
)

type service struct {
	repo      ports.PostRepository
	publisher ports.EventPublisher
}

func NewPostService(repo ports.PostRepository, pub ports.EventPublisher) ports.PostService {
	return &service{repo: repo, publisher: pub}
}

func (s *service) CreatePost(ctx context.Context, userID, content string, media []domain.Media) (*domain.Post, error) {
	post := &domain.Post{
		ID:        uuid.New().String(),
		UserID:    userID,
		Content:   content,
		Media:     media,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}

	// 1. Sauvegarde DB (Source of Truth)
	if err := s.repo.Save(ctx, post); err != nil {
		return nil, err
	}

	// 2. Publication Événement (Fan-out Trigger)
	// Note: En architecture experte "Critique", on utiliserait le pattern "Transactional Outbox"
	// Pour l'instant, un appel direct suffit, mais gardez l'Outbox en tête pour la v2.
	if err := s.publisher.PublishPostCreated(ctx, post); err != nil {
		// Log error, mais ne pas faire échouer la requête utilisateur car la donnée est sauvée.
		// Idéalement : Background retry.
	}

	return post, nil
}

func (s *service) GetPost(ctx context.Context, postID string) (*domain.Post, error) {
	return s.repo.FindByID(ctx, postID)
}

func (s *service) DeletePost(ctx context.Context, postID, userID string) error {
	post, err := s.repo.FindByID(ctx, postID)
	if err != nil {
		return err
	}
	if post.UserID != userID {
		return errors.New("unauthorized")
	}

	if err := s.repo.Delete(ctx, postID); err != nil {
		return err
	}

	_ = s.publisher.PublishPostDeleted(ctx, postID)
	return nil
}

// Exemple à ajouter dans service.go plus tard :
// ListPostsByAuthor (Logique de Pagination Experte)
func (s *service) ListPostsByAuthor(ctx context.Context, authorID string, limit int, cursor string) ([]*domain.Post, string, error) {
	var cursorTime time.Time
	var err error

	// 1. Décodage du Token (String -> Time)
	// Le token est simplement la date formatée RFC3339Nano pour être précis à la nanoseconde
	if cursor != "" {
		cursorTime, err = time.Parse(time.RFC3339Nano, cursor)
		if err != nil {
			// Si le token est corrompu, on renvoie une erreur ou on repart du début.
			// Ici, on est strict.
			return nil, "", errors.New("invalid page token")
		}
	}

	// 2. Appel au Repository
	posts, err := s.repo.ListByAuthor(ctx, authorID, limit, cursorTime)
	if err != nil {
		return nil, "", err
	}

	// 3. Calcul du prochain Token (Time -> String)
	nextCursor := ""
	if len(posts) > 0 {
		// Le curseur pour la prochaine page est la date de création du DERNIER post récupéré.
		// La requête SQL suivante fera "WHERE created_at < last_post_date"
		lastPost := posts[len(posts)-1]
		nextCursor = lastPost.CreatedAt.Format(time.RFC3339Nano)
	}

	return posts, nextCursor, nil
}

// UpdatePost (Si demandé par le gRPC)
func (s *service) UpdatePost(ctx context.Context, postID, userID, content string, media []domain.Media) (*domain.Post, error) {
	// 1. Récupérer l'existant
	post, err := s.repo.FindByID(ctx, postID)
	if err != nil {
		return nil, err
	}

	// 2. Vérification de propriété (Seul l'auteur peut modifier)
	if post.UserID != userID {
		return nil, errors.New("unauthorized")
	}

	// 3. Mise à jour des champs
	post.Content = content
	post.Media = media
	post.UpdatedAt = time.Now().UTC()

	// 4. Sauvegarde
	if err := s.repo.Update(ctx, post); err != nil {
		return nil, err
	}

	// Optionnel: Publier un event PostUpdated si besoin

	return post, nil
}

// GetPosts (Batch pour le Feed)
func (s *service) GetPosts(ctx context.Context, postIDs []string) ([]*domain.Post, error) {
	// Petite optimisation : si vide, on ne dérange pas la DB
	if len(postIDs) == 0 {
		return []*domain.Post{}, nil
	}
	return s.repo.GetPosts(ctx, postIDs)
}
