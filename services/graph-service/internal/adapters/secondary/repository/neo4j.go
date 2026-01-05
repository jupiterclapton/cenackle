package repository

import (
	"context"

	"github.com/jupiterclapton/cenackle/services/graph-service/internal/core/domain"
	"github.com/neo4j/neo4j-go-driver/v5/neo4j"
)

type Neo4jRepo struct {
	driver neo4j.DriverWithContext
}

func NewNeo4jRepo(driver neo4j.DriverWithContext) *Neo4jRepo {
	return &Neo4jRepo{driver: driver}
}

// EnsureSchema crée les index pour que les lookups par ID soient O(1)
func (r *Neo4jRepo) EnsureSchema(ctx context.Context) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Contrainte d'unicité sur User.id (crée aussi un index)
		query := `CREATE CONSTRAINT user_id_unique IF NOT EXISTS FOR (u:User) REQUIRE u.id IS UNIQUE`
		_, err := tx.Run(ctx, query, nil)
		return nil, err
	})
	return err
}

func (r *Neo4jRepo) CreateRelation(ctx context.Context, actorID, targetID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// MERGE est idempotent : Crée les noeuds s'ils n'existent pas, crée la flèche si elle n'existe pas
		query := `
			MERGE (a:User {id: $actorId})
			MERGE (b:User {id: $targetId})
			MERGE (a)-[r:FOLLOWS]->(b)
			ON CREATE SET r.created_at = datetime()
		`
		_, err := tx.Run(ctx, query, map[string]any{
			"actorId":  actorID,
			"targetId": targetID,
		})
		return nil, err
	})
	return err
}

func (r *Neo4jRepo) DeleteRelation(ctx context.Context, actorID, targetID string) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeWrite})
	defer session.Close(ctx)

	_, err := session.ExecuteWrite(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		query := `
			MATCH (a:User {id: $actorId})-[r:FOLLOWS]->(b:User {id: $targetId})
			DELETE r
		`
		_, err := tx.Run(ctx, query, map[string]any{"actorId": actorID, "targetId": targetID})
		return nil, err
	})
	return err
}

func (r *Neo4jRepo) GetRelationStatus(ctx context.Context, actorID, targetID string) (*domain.RelationStatus, error) {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	result, err := session.ExecuteRead(ctx, func(tx neo4j.ManagedTransaction) (any, error) {
		// Une seule requête pour checker les deux sens !
		query := `
			MATCH (a:User {id: $actorId}), (b:User {id: $targetId})
			RETURN EXISTS((a)-[:FOLLOWS]->(b)) as following, 
			       EXISTS((b)-[:FOLLOWS]->(a)) as followedBy
		`
		res, err := tx.Run(ctx, query, map[string]any{"actorId": actorID, "targetId": targetID})
		if err != nil {
			return nil, err
		}

		if res.Next(ctx) {
			rec := res.Record()
			following, _ := rec.Get("following")
			followedBy, _ := rec.Get("followedBy")
			return &domain.RelationStatus{
				IsFollowing:  following.(bool),
				IsFollowedBy: followedBy.(bool),
			}, nil
		}
		// Si aucun noeud trouvé, on considère false/false
		return &domain.RelationStatus{}, nil
	})

	if err != nil {
		return nil, err
	}
	return result.(*domain.RelationStatus), nil
}

// StreamFollowersIDs : La méthode pour le Fan-out
func (r *Neo4jRepo) StreamFollowersIDs(ctx context.Context, userID string, batchSize int, yield func([]string) error) error {
	session := r.driver.NewSession(ctx, neo4j.SessionConfig{AccessMode: neo4j.AccessModeRead})
	defer session.Close(ctx)

	// Note: On n'utilise pas ExecuteRead ici car on veut streamer le résultat manuellement
	// La requête cherche tous les noeuds 'f' qui ont une flèche FOLLOWS vers 'u'
	query := `MATCH (u:User {id: $userId})<-[:FOLLOWS]-(f:User) RETURN f.id as followerId`

	res, err := session.Run(ctx, query, map[string]any{"userId": userID})
	if err != nil {
		return err
	}

	batch := make([]string, 0, batchSize)

	for res.Next(ctx) {
		id, _ := res.Record().Get("followerId")
		batch = append(batch, id.(string))

		if len(batch) >= batchSize {
			if err := yield(batch); err != nil {
				return err
			}
			batch = batch[:0]
		}
	}

	if len(batch) > 0 {
		if err := yield(batch); err != nil {
			return err
		}
	}

	return res.Err()
}
