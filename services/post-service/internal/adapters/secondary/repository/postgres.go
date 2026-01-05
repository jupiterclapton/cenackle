package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jupiterclapton/cenackle/services/post-service/internal/core/domain"
	"github.com/jupiterclapton/cenackle/services/post-service/internal/core/ports"
)

// DTO interne pour mapper le JSONB proprement sans polluer le Domain avec des tags JSON
type mediaDTO struct {
	ID   string `json:"id"`
	URL  string `json:"url"`
	Type string `json:"type"`
}

type PostgresRepo struct {
	db *pgxpool.Pool
}

func NewPostgresRepo(db *pgxpool.Pool) ports.PostRepository {
	return &PostgresRepo{db: db}
}

// Save : Insertion simple
func (r *PostgresRepo) Save(ctx context.Context, post *domain.Post) error {
	query := `
		INSERT INTO posts (id, user_id, content, media, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	// Mapping Domain -> JSONB DTO
	medias := make([]mediaDTO, len(post.Media))
	for i, m := range post.Media {
		medias[i] = mediaDTO{ID: m.ID, URL: m.URL, Type: string(m.Type)}
	}

	// Postgres driver gère auto le marshalling si on passe un []byte ou string,
	// mais pgx préfère souvent qu'on encode avant pour être sûr.
	mediaJSON, err := json.Marshal(medias)
	if err != nil {
		return fmt.Errorf("failed to marshal media: %w", err)
	}

	_, err = r.db.Exec(ctx, query,
		post.ID,
		post.UserID,
		post.Content,
		mediaJSON,
		post.CreatedAt,
		post.UpdatedAt,
	)
	return err
}

// FindByID : Récupération unitaire
func (r *PostgresRepo) FindByID(ctx context.Context, postID string) (*domain.Post, error) {
	query := `SELECT id, user_id, content, media, created_at, updated_at FROM posts WHERE id = $1`

	row := r.db.QueryRow(ctx, query, postID)
	return r.scanPost(row)
}

// GetPosts : BATCH FETCH (Hydratation Feed)
// Utilise WHERE id = ANY($1) pour récupérer plusieurs posts en une seule requête SQL
func (r *PostgresRepo) GetPosts(ctx context.Context, postIDs []string) ([]*domain.Post, error) {
	query := `
		SELECT id, user_id, content, media, created_at, updated_at 
		FROM posts 
		WHERE id = ANY($1)
	`

	rows, err := r.db.Query(ctx, query, postIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []*domain.Post
	for rows.Next() {
		post, err := r.scanPostRows(rows)
		if err != nil {
			return nil, err
		}
		posts = append(posts, post)
	}
	return posts, nil
}

// ListByAuthor : PAGINATION KEYSET (Cursor-based)
// C'est la méthode experte pour éviter "OFFSET 50000" qui tue la DB.
// cursorTime est la date du dernier post vu.
func (r *PostgresRepo) ListByAuthor(ctx context.Context, authorID string, limit int, cursorTime time.Time) ([]*domain.Post, error) {
	// Cas 1: Première page (pas de curseur)
	if cursorTime.IsZero() {
		query := `
			SELECT id, user_id, content, media, created_at, updated_at 
			FROM posts 
			WHERE user_id = $1 
			ORDER BY created_at DESC 
			LIMIT $2
		`
		rows, err := r.db.Query(ctx, query, authorID, limit)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		return r.collectRows(rows)
	}

	// Cas 2: Page suivante (on cherche ce qui est plus vieux que le curseur)
	query := `
		SELECT id, user_id, content, media, created_at, updated_at 
		FROM posts 
		WHERE user_id = $1 AND created_at < $2
		ORDER BY created_at DESC 
		LIMIT $3
	`
	rows, err := r.db.Query(ctx, query, authorID, cursorTime, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return r.collectRows(rows)
}

func (r *PostgresRepo) Update(ctx context.Context, post *domain.Post) error {
	query := `
		UPDATE posts 
		SET content = $1, media = $2, updated_at = $3 
		WHERE id = $4
	`
	// Réutilisation de la logique de marshalling JSON des médias
	medias := make([]mediaDTO, len(post.Media))
	for i, m := range post.Media {
		medias[i] = mediaDTO{ID: m.ID, URL: m.URL, Type: string(m.Type)}
	}
	mediaJSON, _ := json.Marshal(medias)

	cmdTag, err := r.db.Exec(ctx, query, post.Content, mediaJSON, post.UpdatedAt, post.ID)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return fmt.Errorf("post not found")
	}
	return nil
}

func (r *PostgresRepo) Delete(ctx context.Context, postID string) error {
	_, err := r.db.Exec(ctx, "DELETE FROM posts WHERE id = $1", postID)
	return err
}

// --- Helpers pour éviter la duplication de code ---

func (r *PostgresRepo) scanPost(row pgx.Row) (*domain.Post, error) {
	var p domain.Post
	var mediaJSON []byte

	if err := row.Scan(&p.ID, &p.UserID, &p.Content, &mediaJSON, &p.CreatedAt, &p.UpdatedAt); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("post not found") // Ou une erreur sentinel domain.ErrNotFound
		}
		return nil, err
	}
	p.Media = r.unmarshalMedia(mediaJSON)
	return &p, nil
}

func (r *PostgresRepo) scanPostRows(rows pgx.Rows) (*domain.Post, error) {
	var p domain.Post
	var mediaJSON []byte
	if err := rows.Scan(&p.ID, &p.UserID, &p.Content, &mediaJSON, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	p.Media = r.unmarshalMedia(mediaJSON)
	return &p, nil
}

func (r *PostgresRepo) collectRows(rows pgx.Rows) ([]*domain.Post, error) {
	var posts []*domain.Post
	for rows.Next() {
		p, err := r.scanPostRows(rows)
		if err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, nil
}

func (r *PostgresRepo) unmarshalMedia(data []byte) []domain.Media {
	var dtos []mediaDTO
	if err := json.Unmarshal(data, &dtos); err != nil {
		return []domain.Media{} // Fallback safe
	}

	domainMedias := make([]domain.Media, len(dtos))
	for i, d := range dtos {
		domainMedias[i] = domain.Media{
			ID:   d.ID,
			URL:  d.URL,
			Type: domain.MediaType(d.Type),
		}
	}
	return domainMedias
}
