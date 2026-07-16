package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

var ErrRAGAnswerGovernanceRequired = errors.New("rag answer governance required")

type RAGAnswerGovernanceInput struct {
	ModelRef              ModelRef
	SelectedCollectionIDs []string
	Citations             []RAGCitation
}

type RAGAnswerAuthority struct {
	Processor           string `json:"processor"`
	EndpointID          string `json:"endpointId,omitempty"`
	ModelID             string `json:"modelId"`
	ProfileContractHash string `json:"profileContractHash,omitempty"`
	PolicyVersion       string `json:"policyVersion,omitempty"`
	CollectionCount     int    `json:"collectionCount"`
}

type RAGAnswerGovernanceGate interface {
	AuthorizeRAGAnswer(context.Context, RAGAnswerGovernanceInput) (RAGAnswerAuthority, error)
}

type RAGAnswerConsentReader interface {
	ListQueryConsents(context.Context) ([]knowledge.ProcessingConsent, error)
	ListCollectionConsents(context.Context, string) ([]knowledge.ProcessingConsent, error)
}

type KnowledgeConsentRAGAnswerGovernanceGate struct {
	Reader RAGAnswerConsentReader
}

func NewKnowledgeConsentRAGAnswerGovernanceGate(reader RAGAnswerConsentReader) *KnowledgeConsentRAGAnswerGovernanceGate {
	return &KnowledgeConsentRAGAnswerGovernanceGate{Reader: reader}
}

func (g *KnowledgeConsentRAGAnswerGovernanceGate) AuthorizeRAGAnswer(
	ctx context.Context,
	input RAGAnswerGovernanceInput,
) (RAGAnswerAuthority, error) {
	if g == nil || g.Reader == nil {
		return RAGAnswerAuthority{}, ErrRAGDependencyUnavailable
	}
	providerID := strings.TrimSpace(input.ModelRef.ProviderID)
	modelID := strings.TrimSpace(input.ModelRef.ModelID)
	if providerID == "" || modelID == "" || len(input.SelectedCollectionIDs) == 0 || len(input.Citations) == 0 {
		return RAGAnswerAuthority{}, ErrRAGAnswerGovernanceRequired
	}

	queryConsents, err := g.Reader.ListQueryConsents(ctx)
	if err != nil {
		return RAGAnswerAuthority{}, fmt.Errorf("list rag answer query consents: %w", err)
	}
	queryConsent, ok := selectRAGAnswerConsent(queryConsents, providerID, modelID)
	if !ok {
		return RAGAnswerAuthority{}, ErrRAGAnswerGovernanceRequired
	}
	for _, collectionID := range input.SelectedCollectionIDs {
		collectionID = strings.TrimSpace(collectionID)
		if collectionID == "" {
			return RAGAnswerAuthority{}, ErrRAGAnswerGovernanceRequired
		}
		collectionConsents, err := g.Reader.ListCollectionConsents(ctx, collectionID)
		if err != nil {
			if errors.Is(err, knowledge.ErrCollectionNotFound) || errors.Is(err, knowledge.ErrUnauthenticated) {
				return RAGAnswerAuthority{}, ErrRAGAnswerGovernanceRequired
			}
			return RAGAnswerAuthority{}, fmt.Errorf("list rag answer collection consents: %w", err)
		}
		if _, ok := selectRAGAnswerConsent(collectionConsents, providerID, modelID); !ok {
			return RAGAnswerAuthority{}, ErrRAGAnswerGovernanceRequired
		}
	}

	return RAGAnswerAuthority{
		Processor:           queryConsent.Processor,
		EndpointID:          queryConsent.EndpointID,
		ModelID:             queryConsent.ModelID,
		ProfileContractHash: queryConsent.ProfileContractHash,
		PolicyVersion:       queryConsent.PolicyVersion,
		CollectionCount:     len(input.SelectedCollectionIDs),
	}, nil
}

func selectRAGAnswerConsent(consents []knowledge.ProcessingConsent, providerID string, modelID string) (knowledge.ProcessingConsent, bool) {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	for _, consent := range consents {
		if !ragConsentMatchesModel(consent, providerID, modelID) || !ragConsentAllowsAnswer(consent) {
			continue
		}
		return consent, true
	}
	return knowledge.ProcessingConsent{}, false
}

func ragConsentMatchesModel(consent knowledge.ProcessingConsent, providerID string, modelID string) bool {
	if strings.TrimSpace(consent.Processor) != providerID {
		return false
	}
	consentModel := strings.TrimSpace(consent.ModelID)
	if consentModel != "" && consentModel != modelID {
		return false
	}
	return true
}

func ragConsentAllowsAnswer(consent knowledge.ProcessingConsent) bool {
	if consent.Decision != "granted" || consent.EffectiveStatus != "granted" {
		return false
	}
	return containsString(consent.Purposes, "answer") && containsString(consent.DataTypes, "text/plain")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}
