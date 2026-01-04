package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jupiterclapton/cenackle/services/identity-service/internal/core/domain"
)

// SQLUser est un DTO (Data Transfer Object) interne.
// Il sert de tampon entre la base et le domaine pour gérer les différences de types (NULLs, etc.)
type sqlUser struct {
	ID           string    `db:"id"`
	Email        string    `db:"email"`
	Username     string    `db:"username"`
	PasswordHash string    `db:"password_hash"`
	FullName     string    `db:"full_name"`
	IsActive     bool      `db:"is_active"`
	CreatedAt    time.Time `db:"created_at"`
	UpdatedAt    time.Time `db:"updated_at"`
}

type PostgresRepo struct {
	db *pgxpool.Pool
}

// NewPostgresRepo initialise le pool de connexions.
// Dans un vrai main.go, on passerait *pgxpool.Pool déjà configuré.
func NewPostgresRepo(pool *pgxpool.Pool) *PostgresRepo {
	return &PostgresRepo{db: pool}
}

// Save insère un utilisateur.
func (r *PostgresRepo) Save(ctx context.Context, user *domain.User) error {
	q := `
		INSERT INTO users (id, email, username, password_hash, full_name, is_active, created_at, updated_at)
		VALUES (@id, @email, @username, @password_hash, @full_name, @is_active, @created_at, @updated_at)
	`

	// Utilisation de pgx.NamedArgs pour la clarté
	args := pgx.NamedArgs{
		"id":            user.ID,
		"email":         user.Email,
		"username":      user.Username,
		"password_hash": user.PasswordHash,
		"full_name":     user.FullName,
		"is_active":     user.IsActive,
		"created_at":    user.CreatedAt,
		"updated_at":    user.UpdatedAt,
	}

	_, err := r.db.Exec(ctx, q, args)
	if err != nil {
		return r.handleError(err)
	}

	return nil
}

// GetByEmail récupère un utilisateur.
func (r *PostgresRepo) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	q := `SELECT id, email, username, password_hash, full_name, is_active, created_at, updated_at FROM users WHERE email = $1`

	var u sqlUser
	// CollectOneRow est un helper de pgx v5 pour mapper struct automatiquement
	//err := pgxscan.Get(ctx, r.db, &u, q, email) // Note: nécessite "github.com/georgysavva/scany/v2/pgxscan" ou scan manuel

	// Alternative manuelle "Pure pgx" (sans scany) pour plus de contrôle :

	row := r.db.QueryRow(ctx, q, email)
	err := row.Scan(&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.FullName, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound // Traduction technique -> Domaine
		}
		return nil, fmt.Errorf("db: get by email: %w", err)
	}

	return r.toDomain(&u), nil
}

func (r *PostgresRepo) GetByID(ctx context.Context, id string) (*domain.User, error) {
	q := `SELECT id, email, username, password_hash, full_name, is_active, created_at, updated_at FROM users WHERE id = $1`

	var u sqlUser
	// Scan manuel pour l'exemple
	err := r.db.QueryRow(ctx, q, id).Scan(
		&u.ID, &u.Email, &u.Username, &u.PasswordHash, &u.FullName, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domain.ErrUserNotFound
		}
		return nil, fmt.Errorf("db: get by id: %w", err)
	}

	return r.toDomain(&u), nil
}

func (r *PostgresRepo) Update(ctx context.Context, user *domain.User) error {
	q := `
		UPDATE users 
		SET email = @email, full_name = @full_name, password_hash = @password_hash, updated_at = @updated_at 
		WHERE id = @id
	`
	args := pgx.NamedArgs{
		"id":            user.ID,
		"email":         user.Email,
		"full_name":     user.FullName,
		"password_hash": user.PasswordHash,
		"updated_at":    user.UpdatedAt,
	}

	tag, err := r.db.Exec(ctx, q, args)
	if err != nil {
		return r.handleError(err)
	}

	if tag.RowsAffected() == 0 {
		return domain.ErrUserNotFound
	}

	return nil
}

// --- HELPERS ---

// toDomain convertit le DTO SQL en entité Domaine
func (r *PostgresRepo) toDomain(u *sqlUser) *domain.User {
	return &domain.User{
		ID:           u.ID,
		Email:        u.Email,
		Username:     u.Username,
		PasswordHash: u.PasswordHash,
		FullName:     u.FullName,
		IsActive:     u.IsActive,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}
}

// handleError traduit les codes d'erreur PostgreSQL en erreurs du Domaine
func (r *PostgresRepo) handleError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		// Code 23505 = Unique Violation
		if pgErr.Code == "23505" {
			// On pourrait parser pgErr.Detail pour savoir si c'est l'email ou le username
			return domain.ErrEmailAlreadyExists
		}
	}
	return err
}
