package auth

import (
	"context"
	"net/http"
	"strings"

	identityv1 "github.com/jupiterclapton/cenackle/gen/identity/v1"
)

// Clé privée pour le contexte (évite les collisions)
type contextKey struct{ name string }

var userCtxKey = &contextKey{"user_id"}

// Middleware décode le header Authorization et valide le token via gRPC
func Middleware(identityClient identityv1.IdentityServiceClient) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")

			// 1. Pas de header ? On laisse passer (c'est peut-être une requête publique comme Login/Register)
			if header == "" {
				next.ServeHTTP(w, r)
				return
			}

			// 2. Validation du format "Bearer <token>"
			tokenStr := ""
			if strings.HasPrefix(header, "Bearer ") {
				tokenStr = strings.TrimPrefix(header, "Bearer ")
			} else {
				// Format invalide -> 401 direct
				http.Error(w, "Invalid token format", http.StatusUnauthorized)
				return
			}

			// 3. Appel gRPC vers Identity Service
			// On utilise le contexte de la requête pour gérer le timeout/annulation
			validateResp, err := identityClient.ValidateToken(r.Context(), &identityv1.ValidateTokenRequest{
				Token: tokenStr,
			})

			if err != nil || !validateResp.IsValid {
				// Token invalide ou expiré -> 401
				http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
				return
			}

			// 4. Succès : On injecte l'ID utilisateur dans le contexte
			ctx := context.WithValue(r.Context(), userCtxKey, validateResp.UserId)

			// On passe la main au resolver GraphQL avec le nouveau contexte enrichi
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ForContext est un helper pour récupérer l'ID utilisateur depuis un resolver
func ForContext(ctx context.Context) string {
	raw, _ := ctx.Value(userCtxKey).(string)
	return raw
}
