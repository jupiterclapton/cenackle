package services

import (
	"context"
	"fmt"
	"time"

	"github.com/jupiterclapton/cenackle/services/identity-service/internal/core/domain"
	"github.com/jupiterclapton/cenackle/services/identity-service/internal/core/ports"
)

// IdentityService implémente ports.IdentityService (Primary Port)
// Il contient la logique applicative (Application Business Rules).
type IdentityService struct {
	repo          ports.UserRepository
	hasher        ports.PasswordHasher
	tokenProvider ports.TokenProvider
	broker        ports.EventPublisher
	// On pourrait ajouter ici un LoggerPort pour le logging structuré
}

// NewIdentityService est le constructeur avec injection de dépendances.
func NewIdentityService(
	repo ports.UserRepository,
	hasher ports.PasswordHasher,
	token ports.TokenProvider,
	broker ports.EventPublisher,
) *IdentityService {
	return &IdentityService{
		repo:          repo,
		hasher:        hasher,
		tokenProvider: token,
		broker:        broker,
	}
}

// --- AUTHENTIFICATION ---

func (s *IdentityService) Register(ctx context.Context, cmd ports.RegisterCmd) (*ports.AuthResponse, error) {
	// 1. Fail Fast : Vérifier l'unicité de l'email
	// Note: C'est une vérification "soft". La contrainte UNIQUE de la DB est la sécurité ultime (Race condition).
	existingUser, err := s.repo.GetByEmail(ctx, cmd.Email)
	if err == nil && existingUser != nil {
		return nil, domain.ErrEmailAlreadyExists
	}

	// 2. Sécurité : Hachage du mot de passe
	hashedPassword, err := s.hasher.Hash(cmd.Password)
	if err != nil {
		return nil, fmt.Errorf("hashing failed: %w", err)
	}

	// 3. Domaine : Création de l'agrégat User (Validation des invariants ici via NewUser)
	user, err := domain.NewUser(cmd.Email, cmd.Username, hashedPassword, cmd.FullName)
	if err != nil {
		return nil, err // Retourne l'erreur du domaine (ex: ErrInvalidEmail)
	}

	// 4. Persistance : Sauvegarde atomique
	if err := s.repo.Save(ctx, user); err != nil {
		return nil, fmt.Errorf("repository save failed: %w", err)
	}

	// 5. Side Effects : Génération des tokens + Publication événement
	// Note : Idéalement, utiliser le pattern "Transactional Outbox" pour garantir que l'event part si la DB commit.
	accessToken, refreshToken, err := s.tokenProvider.GenerateTokens(user)
	if err != nil {
		// Cas critique : User créé mais tokens échoués.
		// On renvoie une erreur, le client devra retry le login (le user existe maintenant).
		return nil, fmt.Errorf("token generation failed: %w", err)
	}

	// Publication asynchrone (Best effort)
	// On ne bloque pas le retour utilisateur si le broker est lent/down (on loguerait l'erreur ici)
	_ = s.broker.PublishUserRegistered(ctx, user.ID, user.Email)

	return &ports.AuthResponse{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    24 * time.Hour, // Exemple, devrait venir de la config
	}, nil
}

func (s *IdentityService) Login(ctx context.Context, cmd ports.LoginCmd) (*ports.AuthResponse, error) {
	// 1. Récupération
	user, err := s.repo.GetByEmail(ctx, cmd.Email)
	if err != nil {
		// Pour la sécurité, on évite de dire si c'est l'email ou le mdp qui est faux
		// Mais en interne (logs), on veut savoir. Ici on retourne une erreur générique au client.
		return nil, domain.ErrInvalidCredentials
	}

	// 2. Vérification Mot de passe
	if err := s.hasher.Compare(user.PasswordHash, cmd.Password); err != nil {
		return nil, domain.ErrInvalidCredentials
	}

	// 3. Génération Tokens
	accessToken, refreshToken, err := s.tokenProvider.GenerateTokens(user)
	if err != nil {
		return nil, fmt.Errorf("login token gen failed: %w", err)
	}

	return &ports.AuthResponse{
		User:         user,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    15 * time.Minute,
	}, nil
}

// --- GESTION UTILISATEUR ---

func (s *IdentityService) UpdateProfile(ctx context.Context, cmd ports.UpdateProfileCmd) (*domain.User, error) {
	// 1. Charger l'user
	user, err := s.repo.GetByID(ctx, cmd.UserID)
	if err != nil {
		return nil, domain.ErrUserNotFound
	}

	// 2. Appliquer les modifications (Domaine)
	// On utilise les pointeurs pour savoir quels champs mettre à jour
	isUpdated := false

	if cmd.FullName != nil {
		// La méthode du domaine gère le UpdatedAt automatiquement
		user.UpdateProfile(*cmd.FullName)
		isUpdated = true
	}

	if cmd.Email != nil && *cmd.Email != user.Email {
		// Si changement d'email, vérifier l'unicité à nouveau !
		if _, err := s.repo.GetByEmail(ctx, *cmd.Email); err == nil {
			return nil, domain.ErrEmailAlreadyExists
		}
		// Logique métier : User doit peut-être re-valider son email ?
		// Pour l'instant on update direct.
		user.Email = *cmd.Email
		isUpdated = true
	}

	// 3. Persister uniquement si nécessaire
	if isUpdated {
		if err := s.repo.Update(ctx, user); err != nil {
			return nil, fmt.Errorf("update profile failed: %w", err)
		}
	}

	return user, nil
}

func (s *IdentityService) ChangePassword(ctx context.Context, userID, oldPass, newPass string) error {
	user, err := s.repo.GetByID(ctx, userID)
	if err != nil {
		return domain.ErrUserNotFound
	}

	// Vérifier l'ancien mot de passe
	if err := s.hasher.Compare(user.PasswordHash, oldPass); err != nil {
		return fmt.Errorf("old password incorrect: %w", domain.ErrInvalidCredentials)
	}

	// Hasher le nouveau
	newHash, err := s.hasher.Hash(newPass)
	if err != nil {
		return err
	}

	// Mise à jour Domaine
	user.UpdatePassword(newHash)

	// Sauvegarde
	return s.repo.Update(ctx, user)
}

// --- TOKEN MANAGEMENT (Boilerplate) ---

func (s *IdentityService) ValidateToken(ctx context.Context, token string) (string, error) {
	return s.tokenProvider.Validate(token)
}

func (s *IdentityService) RefreshToken(ctx context.Context, refreshToken string) (*ports.AuthResponse, error) {
	// TODO: Implémenter la logique de refresh token (vérifier validité, blacklist, récupérer user, etc.)
	// C'est un peu plus complexe, souvent on stocke les RefreshToken en DB pour pouvoir les révoquer.
	return nil, nil
}

func (s *IdentityService) GetUser(ctx context.Context, userID string) (*domain.User, error) {
	return s.repo.GetByID(ctx, userID)
}
