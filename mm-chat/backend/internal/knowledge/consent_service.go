package knowledge

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

var collectionConsentPurposes = map[string]bool{
	"parse": true, "passage_embedding": true, "rerank": true, "answer": true,
}
var queryConsentPurposes = map[string]bool{
	"query_embedding": true, "rerank": true, "answer": true,
}

func (s *Service) ListQueryConsents(ctx context.Context) ([]ProcessingConsent, error) {
	repo, err := s.queryConsentRepository()
	if err != nil {
		return nil, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return nil, err
	}
	return repo.ListQueryConsents(ctx, QueryConsentLookupInput{ActorUserID: actor.ID})
}

func (s *Service) PutQueryConsent(ctx context.Context, processor string, input PutConsentInput) (ProcessingConsent, error) {
	return s.PutQueryConsentForModel(ctx, ProcessorModelIdentity{Processor: processor}, input)
}

func (s *Service) PutQueryConsentForModel(ctx context.Context, identity ProcessorModelIdentity, input PutConsentInput) (ProcessingConsent, error) {
	repo, err := s.queryConsentRepository()
	if err != nil {
		return ProcessingConsent{}, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return ProcessingConsent{}, err
	}
	identity, err = normalizeProcessorModelIdentity(identity, true)
	if err != nil {
		return ProcessingConsent{}, err
	}
	purposes, dataTypes, policyVersion, expiresAt, err := normalizePutConsent(input, queryConsentPurposes)
	if err != nil {
		return ProcessingConsent{}, err
	}
	return repo.PutQueryConsent(ctx, PutQueryConsentRepositoryInput{ActorUserID: actor.ID,
		Processor: identity.Processor, EndpointID: identity.EndpointID, ModelID: identity.ModelID,
		Purposes: purposes, DataTypes: dataTypes,
		PolicyVersion: policyVersion, ExpiresAt: expiresAt})
}

func (s *Service) RevokeQueryConsent(ctx context.Context, processor string) error {
	return s.RevokeQueryConsentForModel(ctx, ProcessorModelIdentity{Processor: processor})
}

func (s *Service) RevokeQueryConsentForModel(ctx context.Context, identity ProcessorModelIdentity) error {
	repo, err := s.queryConsentRepository()
	if err != nil {
		return err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return err
	}
	identity, err = normalizeProcessorModelIdentity(identity, true)
	if err != nil {
		return err
	}
	return repo.RevokeQueryConsent(ctx, QueryConsentLookupInput{ActorUserID: actor.ID,
		Processor: identity.Processor, EndpointID: identity.EndpointID, ModelID: identity.ModelID})
}

func (s *Service) ListCollectionConsents(ctx context.Context, collectionID string) ([]ProcessingConsent, error) {
	repo, err := s.collectionConsentRepository()
	if err != nil {
		return nil, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return nil, err
	}
	collectionID, err = normalizeUUID(collectionID, "collection id")
	if err != nil {
		return nil, invalidConsentPayload("collection id is invalid")
	}
	return repo.ListCollectionConsents(ctx, CollectionConsentLookupInput{
		CollectionID: collectionID, ActorUserID: actor.ID,
	})
}

func (s *Service) PutCollectionConsent(ctx context.Context, collectionID, processor string, input PutConsentInput) (ProcessingConsent, error) {
	return s.PutCollectionConsentForModel(ctx, collectionID, ProcessorModelIdentity{Processor: processor}, input)
}

func (s *Service) PutCollectionConsentForModel(ctx context.Context, collectionID string, identity ProcessorModelIdentity, input PutConsentInput) (ProcessingConsent, error) {
	repo, err := s.collectionConsentRepository()
	if err != nil {
		return ProcessingConsent{}, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return ProcessingConsent{}, err
	}
	collectionID, err = normalizeUUID(collectionID, "collection id")
	if err != nil {
		return ProcessingConsent{}, invalidConsentPayload("collection id is invalid")
	}
	identity, err = normalizeProcessorModelIdentity(identity, true)
	if err != nil {
		return ProcessingConsent{}, err
	}
	purposes, dataTypes, policyVersion, expiresAt, err := normalizePutConsent(input, collectionConsentPurposes)
	if err != nil {
		return ProcessingConsent{}, err
	}
	return repo.PutCollectionConsent(ctx, PutCollectionConsentRepositoryInput{
		CollectionID: collectionID, ActorUserID: actor.ID, Processor: identity.Processor,
		EndpointID: identity.EndpointID, ModelID: identity.ModelID,
		Purposes: purposes, DataTypes: dataTypes, PolicyVersion: policyVersion, ExpiresAt: expiresAt,
	})
}

func (s *Service) queryConsentRepository() (QueryConsentRepository, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	repo, ok := s.repo.(QueryConsentRepository)
	if !ok {
		return nil, fmt.Errorf("query consent repository is required")
	}
	return repo, nil
}

func normalizePutConsent(input PutConsentInput, allowedPurposes map[string]bool) ([]string, []string, string, *time.Time, error) {
	purposes, err := normalizeConsentList(input.Purposes, allowedPurposes, "purposes")
	if err != nil {
		return nil, nil, "", nil, err
	}
	dataTypes, err := normalizeConsentDataTypes(input.DataTypes)
	if err != nil {
		return nil, nil, "", nil, err
	}
	policyVersion := strings.TrimSpace(input.PolicyVersion)
	if !governanceModelVersionPattern.MatchString(policyVersion) || len(policyVersion) > 128 {
		return nil, nil, "", nil, invalidConsentPayload("policy version is invalid")
	}
	var expiresAt *time.Time
	if strings.TrimSpace(input.ExpiresAt) != "" {
		parsed, parseErr := time.Parse(time.RFC3339, input.ExpiresAt)
		if parseErr != nil {
			return nil, nil, "", nil, invalidConsentPayload("expiry is invalid")
		}
		parsed = parsed.UTC().Truncate(time.Microsecond)
		expiresAt = &parsed
	}
	return purposes, dataTypes, policyVersion, expiresAt, nil
}

func (s *Service) RevokeCollectionConsent(ctx context.Context, collectionID, processor string) error {
	return s.RevokeCollectionConsentForModel(ctx, collectionID, ProcessorModelIdentity{Processor: processor})
}

func (s *Service) RevokeCollectionConsentForModel(ctx context.Context, collectionID string, identity ProcessorModelIdentity) error {
	repo, err := s.collectionConsentRepository()
	if err != nil {
		return err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return err
	}
	collectionID, err = normalizeUUID(collectionID, "collection id")
	if err != nil {
		return invalidConsentPayload("collection id is invalid")
	}
	identity, err = normalizeProcessorModelIdentity(identity, true)
	if err != nil {
		return err
	}
	return repo.RevokeCollectionConsent(ctx, CollectionConsentLookupInput{
		CollectionID: collectionID, ActorUserID: actor.ID, Processor: identity.Processor,
		EndpointID: identity.EndpointID, ModelID: identity.ModelID,
	})
}

func normalizeProcessorModelIdentity(input ProcessorModelIdentity, allowLegacy bool) (ProcessorModelIdentity, error) {
	var err error
	input.Processor, err = normalizeGovernanceAlias(input.Processor, "processor")
	if err != nil {
		return ProcessorModelIdentity{}, invalidConsentPayload("processor is invalid")
	}
	endpointPresent := strings.TrimSpace(input.EndpointID) != ""
	modelPresent := strings.TrimSpace(input.ModelID) != ""
	if endpointPresent != modelPresent || (!allowLegacy && !endpointPresent) {
		return ProcessorModelIdentity{}, invalidConsentPayload("endpointId and modelId must be provided together")
	}
	if !endpointPresent {
		return input, nil
	}
	input.EndpointID, err = normalizeGovernanceAlias(input.EndpointID, "endpoint id")
	if err != nil {
		return ProcessorModelIdentity{}, invalidConsentPayload("endpointId is invalid")
	}
	input.ModelID, err = normalizeGovernanceAlias(input.ModelID, "model id")
	if err != nil {
		return ProcessorModelIdentity{}, invalidConsentPayload("modelId is invalid")
	}
	return input, nil
}

func (s *Service) collectionConsentRepository() (CollectionConsentRepository, error) {
	if err := s.requireRepository(); err != nil {
		return nil, err
	}
	repo, ok := s.repo.(CollectionConsentRepository)
	if !ok {
		return nil, fmt.Errorf("collection consent repository is required")
	}
	return repo, nil
}

func normalizeConsentList(values []string, allowed map[string]bool, label string) ([]string, error) {
	if len(values) == 0 || len(values) > maximumGovernanceListItems {
		return nil, invalidConsentPayload(label + " are invalid")
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !allowed[value] {
			return nil, invalidConsentPayload(label + " are invalid")
		}
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result, nil
}

func normalizeConsentDataTypes(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > maximumGovernanceListItems {
		return nil, invalidConsentPayload("data types are invalid")
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if !governanceDataTypePattern.MatchString(value) {
			return nil, invalidConsentPayload("data types are invalid")
		}
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result, nil
}
