package clients

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	graphv1 "github.com/jupiterclapton/cenackle/gen/graph/v1"
)

type GraphClient struct {
	client graphv1.GraphServiceClient
	conn   *grpc.ClientConn
}

// NewGraphClient initialise la connexion gRPC
// Note: En prod, on injecterait des options pour le retry, le load balancing, etc.
func NewGraphClient(targetURL string) (*GraphClient, error) {
	conn, err := grpc.NewClient(targetURL, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, err
	}

	return &GraphClient{
		client: graphv1.NewGraphServiceClient(conn),
		conn:   conn,
	}, nil
}

func (c *GraphClient) Close() error {
	return c.conn.Close()
}

// GetFollowers implémente la logique de STREAMING côté client
// C'est ici qu'on consomme le flux envoyé par le Graph Service
func (c *GraphClient) GetFollowers(ctx context.Context, userID string) ([]string, error) {
	slog.Debug("Asking graph-service for followers", "user_id", userID)

	// 1. Ouverture du Stream
	stream, err := c.client.StreamFollowers(ctx, &graphv1.StreamFollowersRequest{
		UserId: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start stream: %w", err)
	}

	var allFollowers []string

	// 2. Lecture en boucle des paquets
	for {
		resp, err := stream.Recv()
		if err == io.EOF {
			// Fin du stream
			break
		}
		if err != nil {
			return nil, fmt.Errorf("error while streaming: %w", err)
		}

		// Ajout du batch reçu à la liste totale
		// Note : Pour une optimisation ultime "Zero-Allocation", on pourrait
		// passer un channel à cette fonction au lieu de retourner un slice géant.
		allFollowers = append(allFollowers, resp.FollowerIds...)
	}

	slog.Debug("Retrieved followers", "count", len(allFollowers))
	return allFollowers, nil
}
