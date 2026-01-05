package domain

import "time"

type ContentType string

const (
	TypePost    ContentType = "post"
	TypeVideo   ContentType = "video"
	TypeArticle ContentType = "article"
)

type FeedItem struct {
	PostID    string
	AuthorID  string
	Type      ContentType
	CreatedAt time.Time
}

// FeedRequest encapsule les critères de recherche
type FeedRequest struct {
	UserID string
	Limit  int64
	Offset int64         // Pagination
	Types  []ContentType // Filtrage optionnel
}
