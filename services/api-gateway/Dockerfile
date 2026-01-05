# --- STAGE 1: Builder ---
FROM golang:1.24.11-alpine AS builder

WORKDIR /app

# Copie des définitions de modules
COPY go.work .
COPY go.work.sum .

# Astuce du wildcard pour gérer pkg/go.sum s'il n'existe pas encore
COPY pkg/go.mod pkg/go.sum* ./pkg/
COPY gen/go.mod gen/go.sum ./gen/
# 3. CORRECTION : On copie les définitions de TOUS les services du workspace
COPY services/identity-service/go.mod services/identity-service/go.sum ./services/identity-service/
COPY services/api-gateway/go.mod services/api-gateway/go.sum ./services/api-gateway/
COPY services/graph-service/go.mod services/graph-service/go.sum ./services/graph-service/
COPY services/post-service/go.mod services/post-service/go.sum ./services/post-service/
COPY services/feed-service/go.mod services/feed-service/go.sum ./services/feed-service/

# Téléchargement des dépendances
RUN go mod download
RUN go work sync

# Copie des sources
COPY pkg/ ./pkg/
COPY gen/ ./gen/
COPY services/api-gateway/ ./services/api-gateway/

# Compilation
WORKDIR /app/services/api-gateway
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/api-gateway cmd/main.go

# --- STAGE 2: Runner ---
FROM alpine:3.19

WORKDIR /app
RUN apk --no-cache add ca-certificates

# User non-root
RUN addgroup -S cenackle && adduser -S cenackle -G cenackle
USER cenackle

COPY --from=builder /app/api-gateway .

# Port par défaut
EXPOSE 8080

CMD ["./api-gateway"]