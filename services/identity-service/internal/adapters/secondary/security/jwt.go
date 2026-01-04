package security

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jupiterclapton/cenackle/services/identity-service/internal/core/domain"
)

// UserClaims étend les claims standards JWT
type UserClaims struct {
	UserID   string `json:"user_id"`
	Email    string `json:"email"`
	Username string `json:"username"`
	Role     string `json:"role,omitempty"` // ex: "admin", "user"
	jwt.RegisteredClaims
}

type JWTProvider struct {
	privateKey    *rsa.PrivateKey
	publicKey     *rsa.PublicKey
	accessExpiry  time.Duration
	refreshExpiry time.Duration
	issuer        string
}

// NewJWTProvider charge les clés RSA depuis des chaînes PEM (ou bytes)
func NewJWTProvider(privateKeyPEM, publicKeyPEM []byte) (*JWTProvider, error) {
	// Parsing de la clé privée
	privKey, err := jwt.ParseRSAPrivateKeyFromPEM(privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Parsing de la clé publique
	pubKey, err := jwt.ParseRSAPublicKeyFromPEM(publicKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %w", err)
	}

	return &JWTProvider{
		privateKey:    privKey,
		publicKey:     pubKey,
		accessExpiry:  15 * time.Minute,   // Court
		refreshExpiry: 7 * 24 * time.Hour, // Long
		issuer:        "cenackle-identity",
	}, nil
}

// GenerateTokens crée la paire Access + Refresh
func (j *JWTProvider) GenerateTokens(user *domain.User) (string, string, error) {
	// 1. Access Token
	accessClaims := UserClaims{
		UserID:   user.ID,
		Email:    user.Email,
		Username: user.Username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(j.accessExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    j.issuer,
			Subject:   user.ID,
			ID:        fmt.Sprintf("%s-acc", user.ID), // JTI unique
		},
	}

	// Signature avec RS256 et clé privée
	accessTokenObj := jwt.NewWithClaims(jwt.SigningMethodRS256, accessClaims)
	accessToken, err := accessTokenObj.SignedString(j.privateKey)
	if err != nil {
		return "", "", err
	}

	// 2. Refresh Token
	// Le refresh token contient moins d'infos, sert juste à identifier l'user pour renouveler
	refreshClaims := jwt.RegisteredClaims{
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(j.refreshExpiry)),
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		Issuer:    j.issuer,
		Subject:   user.ID,
		ID:        fmt.Sprintf("%s-ref", user.ID),
	}

	refreshTokenObj := jwt.NewWithClaims(jwt.SigningMethodRS256, refreshClaims)
	refreshToken, err := refreshTokenObj.SignedString(j.privateKey)
	if err != nil {
		return "", "", err
	}

	return accessToken, refreshToken, nil
}

// Validate vérifie la signature et retourne l'UserID (Subject)
func (j *JWTProvider) Validate(tokenString string) (string, error) {
	// Parse avec validation de la méthode de signature
	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Sécurité critique : vérifier que l'alg est bien RS256
		// Empêche les attaques où l'attaquant force l'algo à "None" ou "HS256"
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		// On retourne la clé PUBLIQUE pour vérifier la signature
		return j.publicKey, nil
	})

	if err != nil {
		return "", err // Token expiré ou signature invalide
	}

	if claims, ok := token.Claims.(*UserClaims); ok && token.Valid {
		return claims.Subject, nil
	}

	return "", errors.New("invalid token claims")
}
