package graph

import (
	feedv1 "github.com/jupiterclapton/cenackle/gen/feed/v1"
	identityv1 "github.com/jupiterclapton/cenackle/gen/identity/v1"
	postv1 "github.com/jupiterclapton/cenackle/gen/post/v1"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require
// here.

type Resolver struct {
	IdentityClient identityv1.IdentityServiceClient
	PostClient     postv1.PostServiceClient
	FeedClient     feedv1.FeedServiceClient
}
