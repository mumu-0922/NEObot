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
	processor, err = normalizeGovernanceAlias(processor, "processor")
	if err != nil {
		return ProcessingConsent{}, invalidConsentPayload("processor is invalid")
	}
	purposes, err := normalizeConsentList(input.Purposes, collectionConsentPurposes, "purposes")
	if err != nil {
		return ProcessingConsent{}, err
	}
	dataTypes, err := normalizeConsentDataTypes(input.DataTypes)
	if err != nil {
		return ProcessingConsent{}, err
	}
	policyVersion := strings.TrimSpace(input.PolicyVersion)
	if !governanceModelVersionPattern.MatchString(policyVersion) || len(policyVersion) > 128 {
		return ProcessingConsent{}, invalidConsentPayload("policy version is invalid")
	}
	var expiresAt *time.Time
	if strings.TrimSpace(input.ExpiresAt) != "" {
		parsed, parseErr := time.Parse(time.RFC3339, input.ExpiresAt)
		if parseErr != nil {
			return ProcessingConsent{}, invalidConsentPayload("expiry is invalid")
		}
		parsed = parsed.UTC()
		expiresAt = &parsed
	}
	return repo.PutCollectionConsent(ctx, PutCollectionConsentRepositoryInput{
		CollectionID: collectionID, ActorUserID: actor.ID, Processor: processor,
		Purposes: purposes, DataTypes: dataTypes, PolicyVersion: policyVersion, ExpiresAt: expiresAt,
	})
}

func (s *Service) RevokeCollectionConsent(ctx context.Context, collectionID, processor string) error {
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
	processor, err = normalizeGovernanceAlias(processor, "processor")
	if err != nil {
		return invalidConsentPayload("processor is invalid")
	}
	return repo.RevokeCollectionConsent(ctx, CollectionConsentLookupInput{
		CollectionID: collectionID, ActorUserID: actor.ID, Processor: processor,
	})
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
