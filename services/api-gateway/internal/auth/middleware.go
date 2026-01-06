package auth

import (
	"context"
	"net/http"
	"strings"

	identityv1 "github.com/jupiterclapton/cenackle/gen/identity/v1"
)

// Clé privée pour le contexte
type contextKey struct{ name string }

var userCtxKey = &contextKey{"user"}

// ✅ AMÉLIORATION : On définit une struct User.
// Cela résout votre erreur "user.ID undefined" et permet d'ajouter "Role" plus tard.
type User struct {
	ID string
}

// Middleware décode le header Authorization et valide le token via gRPC
func Middleware(identityClient identityv1.IdentityServiceClient) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")

			// 1. Pas de header ? On laisse passer (c'est le Resolver qui décidera si c'est grave)
			if header == "" {
				next.ServeHTTP(w, r)
				return
			}

			// 2. Validation du format "Bearer <token>"
			tokenStr := ""
			if strings.HasPrefix(header, "Bearer ") {
				tokenStr = strings.TrimPrefix(header, "Bearer ")
			} else {
				http.Error(w, "Invalid token format", http.StatusUnauthorized)
				return
			}

			// 3. Appel gRPC vers Identity Service
			validateResp, err := identityClient.ValidateToken(r.Context(), &identityv1.ValidateTokenRequest{
				Token: tokenStr,
			})

			if err != nil || !validateResp.IsValid {
				http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
				return
			}

			// 4. Succès : On crée l'objet User
			user := &User{
				ID: validateResp.UserId,
			}

			// 5. Injection dans le contexte
			ctx := context.WithValue(r.Context(), userCtxKey, user)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ✅ ForContext renvoie maintenant un Pointeur vers User (*User)
// Cela permet de faire "if user == nil" dans le resolver.
func ForContext(ctx context.Context) *User {
	raw, _ := ctx.Value(userCtxKey).(*User)
	return raw
}
