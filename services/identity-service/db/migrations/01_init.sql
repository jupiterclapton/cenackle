-- Active l'extension UUID pour que Postgres puisse générer des UUIDs si besoin
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Création de la table users
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY,
    email VARCHAR(255) UNIQUE NOT NULL,
    username VARCHAR(50) NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    full_name VARCHAR(100),
    is_active BOOLEAN DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Index pour accélérer le login (recherche par email)
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);

-- Index pour accélérer le refresh token (si on cherche par ID plus tard)
CREATE INDEX IF NOT EXISTS idx_users_created_at ON users(created_at);