# --- STAGE 1: Builder ---
FROM golang:1.24.11-alpine AS builder

WORKDIR /app

# 1. On copie d'abord les définitions de modules (pour le cache Docker)
COPY go.work .
COPY go.work.sum .
COPY pkg/go.mod pkg/go.sum* ./pkg/
COPY gen/go.mod gen/go.sum ./gen/
# 3. CORRECTION : Il faut copier TOUS les go.mod déclarés dans le go.work
# Même ceux qu'on ne compile pas, sinon "go work" plante.
COPY services/identity-service/go.mod services/identity-service/go.sum ./services/identity-service/
COPY services/api-gateway/go.mod services/api-gateway/go.sum ./services/api-gateway/
COPY services/graph-service/go.mod services/graph-service/go.sum ./services/graph-service/
COPY services/post-service/go.mod services/post-service/go.sum ./services/post-service/
COPY services/feed-service/go.mod services/feed-service/go.sum ./services/feed-service/

# 2. Téléchargement des dépendances
RUN go mod download
RUN go work sync

# 3. Copie du code source complet
COPY pkg/ ./pkg/
COPY gen/ ./gen/
COPY services/graph-service/ ./services/graph-service/

# 4. Compilation Statique (pour Alpine/Scratch)
# On se place dans le dossier du service pour compiler son main
WORKDIR /app/services/graph-service
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/graph-service cmd/main.go

# --- STAGE 2: Runner (Production ready) ---
FROM alpine:3.19

WORKDIR /app

# Certificats SSL pour les appels HTTPS externes
RUN apk --no-cache add ca-certificates

# Création d'un user non-root pour la sécurité
RUN addgroup -S cenackle && adduser -S cenackle -G cenackle
USER cenackle

# Copie du binaire depuis le builder
COPY --from=builder /app/graph-service .



CMD ["./graph-service"]