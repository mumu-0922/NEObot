package knowledge

import (
	"context"
	"time"
)

type PutConsentInput struct {
	Purposes      []string `json:"purposes"`
	DataTypes     []string `json:"dataTypes"`
	PolicyVersion string   `json:"policyVersion"`
	ExpiresAt     string   `json:"expiresAt,omitempty"`
}

type ProcessingConsent struct {
	Processor, EndpointID, ModelID, ProfileContractHash string
	Decision, EffectiveStatus, PolicyVersion            string
	Purposes, DataTypes                                 []string
	DecidedAt                                           time.Time
	ExpiresAt                                           *time.Time
	MaterializedAt                                      *time.Time
}

type ProcessorModelIdentity struct {
	Processor  string
	EndpointID string
	ModelID    string
}

type CollectionConsentRepository interface {
	ListCollectionConsents(context.Context, CollectionConsentLookupInput) ([]ProcessingConsent, error)
	PutCollectionConsent(context.Context, PutCollectionConsentRepositoryInput) (ProcessingConsent, error)
	RevokeCollectionConsent(context.Context, CollectionConsentLookupInput) error
}

type CollectionConsentLookupInput struct {
	CollectionID, ActorUserID, Processor, EndpointID, ModelID string
}

type PutCollectionConsentRepositoryInput struct {
	CollectionID, ActorUserID, Processor, EndpointID, ModelID string
	Purposes, DataTypes                                       []string
	PolicyVersion                                             string
	ExpiresAt                                                 *time.Time
}

type QueryConsentRepository interface {
	ListQueryConsents(context.Context, QueryConsentLookupInput) ([]ProcessingConsent, error)
	PutQueryConsent(context.Context, PutQueryConsentRepositoryInput) (ProcessingConsent, error)
	RevokeQueryConsent(context.Context, QueryConsentLookupInput) error
}

type QueryConsentLookupInput struct {
	ActorUserID, Processor, EndpointID, ModelID string
}

type PutQueryConsentRepositoryInput struct {
	ActorUserID, Processor, EndpointID, ModelID string
	Purposes, DataTypes                         []string
	PolicyVersion                               string
	ExpiresAt                                   *time.Time
}
