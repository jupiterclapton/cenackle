package ports

import (
	"context"

	"github.com/jupiterclapton/cenackle/services/identity-service/internal/core/domain"
)

// --- PERSISTANCE (DB) ---

// UserRepository est un Port Secondaire (Driven).
// C'est le Service qui appelle le Repo pour sauvegarder/lire les données.
type UserRepository interface {
	Save(ctx context.Context, user *domain.User) error
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByID(ctx context.Context, id string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
}

// --- MESSAGERIE (BROKER) ---

// EventPublisher est le port vers Nats/Kafka.
// Il permet de notifier les autres microservices (Feed, Notif) qu'un événement a eu lieu.
type EventPublisher interface {
	PublishUserRegistered(ctx context.Context, userID, email string) error
}

// --- SÉCURITÉ (CRYPTO) ---

// PasswordHasher abstrait l'algorithme de hachage (Argon2, Bcrypt)
type PasswordHasher interface {
	Hash(password string) (string, error)
	Compare(hash, password string) error
}

// TokenProvider abstrait la génération de JWT/PASETO
type TokenProvider interface {
	GenerateTokens(user *domain.User) (access string, refresh string, err error)
	Validate(token string) (userID string, err error)
}
