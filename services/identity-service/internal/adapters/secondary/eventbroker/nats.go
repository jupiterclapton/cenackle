package eventbroker

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream" // Le nouveau SDK JetStream
)

const (
	StreamName     = "IDENTITY"
	SubjectPattern = "identity.>" // Tous les events identity.*
)

type NatsBroker struct {
	js jetstream.JetStream
}

// NewNatsBroker initialise la connexion et s'assure que le Stream existe (Idempotent)
func NewNatsBroker(url string) (*NatsBroker, error) {
	nc, err := nats.Connect(url)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("jetstream init: %w", err)
	}

	// Optionnel : Créer le Stream au démarrage (ou via Terraform en prod)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     StreamName,
		Subjects: []string{SubjectPattern},
		Storage:  jetstream.FileStorage, // Persistance sur disque (Important !)
		Replicas: 1,                     // Mettre 3 en cluster
	})
	if err != nil {
		return nil, fmt.Errorf("create stream: %w", err)
	}

	return &NatsBroker{js: js}, nil
}

// Payload de l'événement (pourrait être généré par Protobuf)
type UserRegisteredEvent struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
}

func (n *NatsBroker) PublishUserRegistered(ctx context.Context, userID, email string) error {
	// 1. Préparation du payload
	eventData := UserRegisteredEvent{
		UserID: userID,
		Email:  email,
	}

	data, err := json.Marshal(eventData)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	// 2. Définition du Sujet (Topic)
	// identity.user.registered -> permet aux subscribers de filtrer facilement
	subject := "identity.user.registered"

	// 3. Publication Async avec confirmation
	// JetStream garantit que le serveur a bien reçu et persisté le message
	ack, err := n.js.Publish(ctx, subject, data)
	if err != nil {
		return fmt.Errorf("nats publish: %w", err)
	}

	// Optionnel : logger l'ACK sequence pour debug
	// fmt.Printf("Published event on %s, seq: %d\n", subject, ack.Sequence)
	_ = ack

	return nil
}
