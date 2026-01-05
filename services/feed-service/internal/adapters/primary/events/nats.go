package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jupiterclapton/cenackle/services/feed-service/internal/core/domain"
	"github.com/jupiterclapton/cenackle/services/feed-service/internal/core/ports"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

type EventHandler struct {
	service ports.FeedService
}

// ✅ C'est cette fonction qui manquait pour que main.go fonctionne !
func NewEventHandler(service ports.FeedService) *EventHandler {
	return &EventHandler{service: service}
}

func (h *EventHandler) HandlePostCreated(msg *nats.Msg) {
	// 🟢 1. EXTRACTION DU CONTEXTE DE TRACE (Le lien avec le Post Service)
	// On crée un contexte vide, et on le remplit avec les infos trouvées dans les headers NATS
	ctx := context.Background()
	ctx = otel.GetTextMapPropagator().Extract(ctx, propagation.HeaderCarrier(msg.Header))

	// 🟢 2. DÉMARRAGE DU SPAN (La mesure du temps de traitement)
	tracer := otel.Tracer("feed-service")
	// On crée un span nommé "process_post_created"
	ctx, span := tracer.Start(ctx, "process_post_created", trace.WithSpanKind(trace.SpanKindConsumer))
	defer span.End() // On s'assure que le span se ferme à la fin de la fonction

	// --- VOTRE CODE EXISTANT ---
	type PostCreatedEvent struct {
		ID        string    `json:"id"`
		AuthorID  string    `json:"author_id"`
		Type      string    `json:"type"`
		CreatedAt time.Time `json:"created_at"`
	}

	var event PostCreatedEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		// 🟢 Astuce : On peut attacher l'erreur au span pour la voir dans Jaeger
		span.RecordError(err)
		slog.Error("❌ Invalid event format", "error", err)
		return
	}

	slog.Info("📨 Feed Service received event", "post_id", event.ID, "type", event.Type)

	item := &domain.FeedItem{
		PostID:    event.ID,
		AuthorID:  event.AuthorID,
		Type:      domain.ContentType(event.Type),
		CreatedAt: event.CreatedAt,
	}

	// --- LANCEMENT EN BACKGROUND ---
	go func() {
		// 🟢 3. PROPAGATION DU CONTEXTE
		// AU LIEU DE : context.Background()
		// ON UTILISE : ctx (celui qui contient la trace)
		// Cela permet à DistributePost -> Redis d'hériter du TraceID

		childCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		if err := h.service.DistributePost(childCtx, item); err != nil {
			slog.Error("❌ Fan-out failed", "post_id", event.ID, "error", err)
		} else {
			slog.Debug("✅ Fan-out success", "post_id", event.ID)
		}
	}()
}
