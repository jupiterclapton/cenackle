package domain

import (
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid" // go get github.com/google/uuid
)

// --- ERREURS DU DOMAINE ---
var (
	ErrUserNotFound       = errors.New("user not found")
	ErrEmailAlreadyExists = errors.New("email already exists")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidToken       = errors.New("invalid token")
	ErrInvalidEmail       = errors.New("invalid email format")
	ErrInvalidUsername    = errors.New("username must be at least 3 characters")
)

// --- ENTITÉ ---

type User struct {
	ID           string
	Email        string
	Username     string
	PasswordHash string
	FullName     string
	IsActive     bool // Utile pour le "soft delete" ou ban
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// --- FACTORY (CONSTRUCTEUR) ---

// NewUser crée une nouvelle instance valide.
// C'est le SEUL moyen de créer un user proprement (avec ID et Validation).
func NewUser(email, username, passwordHash, fullName string) (*User, error) {
	// 1. Validation des invariants (Règles métier bloquantes)
	if err := validateEmail(email); err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(username)) < 3 {
		return nil, ErrInvalidUsername
	}

	// 2. Création avec génération d'ID (UUID v7 est mieux pour les DB, v4 est standard)
	return &User{
		ID:           uuid.NewString(), // L'identité est générée ICI, pas en DB
		Email:        strings.ToLower(strings.TrimSpace(email)),
		Username:     strings.TrimSpace(username),
		PasswordHash: passwordHash,
		FullName:     strings.TrimSpace(fullName),
		IsActive:     true,
		CreatedAt:    time.Now().UTC(), // Toujours utiliser UTC
		UpdatedAt:    time.Now().UTC(),
	}, nil
}

// --- COMPORTEMENTS (MÉTHODES MÉTIER) ---

// UpdatePassword change le hash et met à jour le timestamp
func (u *User) UpdatePassword(newHash string) {
	u.PasswordHash = newHash
	u.touch()
}

// UpdateProfile permet de changer les infos non-critiques
func (u *User) UpdateProfile(fullName string) {
	u.FullName = strings.TrimSpace(fullName)
	u.touch()
}

// touch met à jour la date de modification (interne)
func (u *User) touch() {
	u.UpdatedAt = time.Now().UTC()
}

// --- VALIDATEURS INTERNES ---

func validateEmail(email string) error {
	_, err := mail.ParseAddress(email)
	if err != nil {
		return ErrInvalidEmail
	}
	return nil
}
