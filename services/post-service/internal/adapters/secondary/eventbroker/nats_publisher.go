package eventbroker

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/jupiterclapton/cenackle/services/post-service/internal/core/domain"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type NatsPublisher struct {
	nc *nats.Conn
}

func NewNatsPublisher(nc *nats.Conn) *NatsPublisher {
	return &NatsPublisher{nc: nc}
}

// Structure de l'event (Contract implicite avec Feed-Service)
type PostCreatedEvent struct {
	ID        string    `json:"id"`
	AuthorID  string    `json:"author_id"`
	Content   string    `json:"content"` // Snippet optionnel
	Type      string    `json:"type"`    // "post", "video", "image"
	CreatedAt time.Time `json:"created_at"`
}

func (p *NatsPublisher) PublishPostCreated(ctx context.Context, post *domain.Post) error {
	contentType := "post"
	if len(post.Media) > 0 {
		contentType = string(post.Media[0].Type) // Simplification : type basé sur le 1er média
	}

	event := PostCreatedEvent{
		ID:        post.ID,
		AuthorID:  post.UserID,
		Content:   post.Content,
		Type:      contentType,
		CreatedAt: post.CreatedAt,
	}

	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshalling error: %w", err)
	}

	// Topic NATS
	topic := &nats.Msg{
		Subject: "post.created",
		Data:    data,
		Header:  nats.Header{},
	}
	// 👇 INJECTION DU TRACE ID DANS LES HEADERS NATS
	// Cela prend le contexte actuel (qui contient le TraceID du gRPC) et le met dans msg.Header
	otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(topic.Header))

	slog.Info("📢 Publishing event with trace context", "topic", topic.Subject, "post_id", post.ID)

	// On utilise PublishMsg au lieu de Publish
	return p.nc.PublishMsg(topic)
}

func (p *NatsPublisher) PublishPostDeleted(ctx context.Context, postID string) error {
	return p.nc.Publish("post.deleted", []byte(postID))
}
