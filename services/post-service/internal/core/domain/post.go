package domain

import "time"

type MediaType string

const (
	MediaTypeImage MediaType = "image"
	MediaTypeVideo MediaType = "video"
)

type Media struct {
	ID   string
	URL  string
	Type MediaType
}

type Post struct {
	ID        string
	UserID    string
	Content   string
	Media     []Media
	CreatedAt time.Time
	UpdatedAt time.Time
}
