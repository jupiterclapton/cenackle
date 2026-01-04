package ports

import (
	"context"
	"time"

	"github.com/jupiterclapton/cenackle/services/identity-service/internal/core/domain"
)

// --- INPUTS (Command Pattern) ---
// Utiliser des structs permet d'ajouter des champs optionnels plus tard sans casser la signature.

type RegisterCmd struct {
	Email    string
	Password string
	Username string
	FullName string
	// Plus tard, on pourra ajouter : IsTermsAccepted bool, ReferalCode string, etc.
}

type LoginCmd struct {
	Email    string
	Password string
	IP       string // Utile pour la sécurité / logs
	Device   string // Utile pour la sécurité
}

type UpdateProfileCmd struct {
	UserID   string
	Email    *string // Pointeur pour savoir si on veut update ou pas (nil = pas de changement)
	FullName *string
}

// --- OUTPUTS ---
// On groupe les tokens pour éviter de renvoyer (string, string) qui est ambigu.

type AuthResponse struct {
	User         *domain.User
	AccessToken  string
	RefreshToken string
	ExpiresIn    time.Duration
}

// --- PORT PRIMAIRE (Driving) ---
// C'est l'API que ton Hexagone expose au monde extérieur (gRPC, HTTP, CLI).

type IdentityService interface {
	// Authentification
	Register(ctx context.Context, cmd RegisterCmd) (*AuthResponse, error)
	Login(ctx context.Context, cmd LoginCmd) (*AuthResponse, error)

	// Token Management
	RefreshToken(ctx context.Context, refreshToken string) (*AuthResponse, error)
	ValidateToken(ctx context.Context, token string) (string, error) // Retourne UserID ou Claims

	// User Management
	GetUser(ctx context.Context, userID string) (*domain.User, error)
	UpdateProfile(ctx context.Context, cmd UpdateProfileCmd) (*domain.User, error)
	ChangePassword(ctx context.Context, userID, oldPass, newPass string) error
}
