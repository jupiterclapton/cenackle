package graph

import identityv1 "github.com/jupiterclapton/cenackle/gen/identity/v1"

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

type Resolver struct {
	IdentityClient identityv1.IdentityServiceClient
}
