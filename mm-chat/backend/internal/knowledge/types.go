package knowledge

import (
	"context"
	"time"
)

const (
	ScopePersonal = "personal"
	ScopeTeam     = "team"
)

type Repository interface {
	CreateCollection(context.Context, CreateCollectionRepositoryInput) (Collection, error)
	ListCollections(context.Context, ListCollectionsRepositoryInput) (CollectionPageResult, error)
	GetCollection(context.Context, CollectionLookupInput) (Collection, error)
	UpdateCollection(context.Context, UpdateCollectionRepositoryInput) (Collection, error)
	DeleteCollection(context.Context, DeleteCollectionRepositoryInput) error
}

type Permissions struct {
	Read          bool `json:"read"`
	Manage        bool `json:"manage"`
	ManageConsent bool `json:"manageConsent"`
}

type Collection struct {
	ID                           string      `json:"id"`
	Name                         string      `json:"name"`
	Description                  string      `json:"description"`
	Icon                         string      `json:"icon"`
	Color                        string      `json:"color"`
	Scope                        string      `json:"scope"`
	TeamID                       string      `json:"teamId,omitempty"`
	Permissions                  Permissions `json:"permissions"`
	ACLRevision                  int64       `json:"aclRevision"`
	VisibilityEpoch              int64       `json:"visibilityEpoch"`
	CollectionProcessingRevision int64       `json:"collectionProcessingRevision"`
	CreatedAt                    time.Time   `json:"createdAt"`
	UpdatedAt                    time.Time   `json:"updatedAt"`
}

type ApiPage[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
}

type CreateCollectionInput struct {
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Icon           string `json:"icon,omitempty"`
	Color          string `json:"color,omitempty"`
	Scope          string `json:"scope"`
	TeamID         string `json:"teamId,omitempty"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type UpdateCollectionInput struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Icon        *string `json:"icon,omitempty"`
	Color       *string `json:"color,omitempty"`
}

type ListCollectionsInput struct {
	Scope  string `json:"scope,omitempty"`
	TeamID string `json:"teamId,omitempty"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type CollectionPageCursor struct {
	CreatedAt time.Time
	ID        string
}

type CreateCollectionRepositoryInput struct {
	ID                string
	ActorUserID       string
	Name              string
	Description       string
	Icon              string
	Color             string
	Scope             string
	TeamID            string
	IdempotencyKey    string
	CreateRequestHash string
}

type ListCollectionsRepositoryInput struct {
	ActorUserID string
	Scope       string
	TeamID      string
	Limit       int
	After       *CollectionPageCursor
}

type CollectionPageResult struct {
	Items   []Collection
	HasMore bool
}

type CollectionLookupInput struct {
	CollectionID string
	ActorUserID  string
}

type UpdateCollectionRepositoryInput struct {
	CollectionID string
	ActorUserID  string
	Name         *string
	Description  *string
	Icon         *string
	Color        *string
}

type DeleteCollectionRepositoryInput struct {
	CollectionID string
	ActorUserID  string
}
