CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS posts (
    id UUID PRIMARY KEY, -- UUID généré par le service (client-side generation)
    user_id TEXT NOT NULL, -- L'ID de l'auteur (provenant de Identity Service)
    content TEXT NOT NULL,
    media JSONB NOT NULL DEFAULT '[]', -- Stockage performant: [{url: "...", type: "video"}, ...]
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 🚀 INDEX CRITIQUE POUR "ListPostsByAuthor"
-- Permet de récupérer les X derniers posts d'un auteur en O(log N)
-- Le DESC est important pour éviter un tri mémoire lors du ORDER BY created_at DESC
CREATE INDEX IF NOT EXISTS idx_posts_author_timestamp 
ON posts (user_id, created_at DESC);

-- Index pour la suppression (Optionnel, utile si on supprime souvent par ID)
-- Note: La PK indexe déjà 'id', donc pas besoin d'index supplémentaire ici.