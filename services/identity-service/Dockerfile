# --- STAGE 1: Builder ---
FROM golang:1.24.11-alpine AS builder

WORKDIR /app

# 1. On copie d'abord les définitions de modules (pour le cache Docker)
COPY go.work .
COPY go.work.sum .
COPY pkg/go.mod ./pkg/
COPY gen/go.mod gen/go.sum ./gen/
COPY services/identity-service/go.mod services/identity-service/go.sum ./services/identity-service/

# 2. Téléchargement des dépendances
RUN go mod download
RUN go work sync

# 3. Copie du code source complet
COPY pkg/ ./pkg/
COPY gen/ ./gen/
COPY services/identity-service/ ./services/identity-service/

# 4. Compilation Statique (pour Alpine/Scratch)
# On se place dans le dossier du service pour compiler son main
WORKDIR /app/services/identity-service
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/identity-service cmd/main.go

# --- STAGE 2: Runner (Production ready) ---
FROM alpine:3.19

WORKDIR /app

# Certificats SSL pour les appels HTTPS externes
RUN apk --no-cache add ca-certificates

# Création d'un user non-root pour la sécurité
RUN addgroup -S cenackle && adduser -S cenackle -G cenackle
USER cenackle

# Copie du binaire depuis le builder
COPY --from=builder /app/identity-service .

# Copie des clés RSA (Note: En vraie prod k8s, on utiliserait des Secrets montés, pas COPY)
# Pour le docker compose local, on suppose que les clés sont générées dans /keys
# COPY --from=builder /app/services/identity-service/keys ./keys 
# ^ Mieux : On montera les clés via un volume dans le compose

CMD ["./identity-service"]