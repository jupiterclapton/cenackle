package domain

import "time"

// Relation représente un lien dirigé dans le graphe (User -> Follows -> User)
type Relation struct {
	ActorID   string // Celui qui fait l'action
	TargetID  string // Celui qui subit l'action
	Type      string // "FOLLOWS"
	CreatedAt time.Time
}

// RelationStatus est utilisé pour l'UI (CheckRelation)
type RelationStatus struct {
	IsFollowing  bool // Actor suit Target
	IsFollowedBy bool // Target suit Actor
}
