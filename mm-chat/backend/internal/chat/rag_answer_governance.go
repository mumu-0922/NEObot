package chat

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

var (
	ErrRAGAnswerGovernanceRequired         = errors.New("rag answer governance required")
	ErrRAGRerankGovernanceRequired         = errors.New("rag rerank governance required")
	ErrRAGRoutingCatalogGovernanceRequired = errors.New("rag routing catalog governance required")
)

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

type RAGRoutingCatalogGovernanceInput struct {
	ModelRef              ModelRef
	SelectedCollectionIDs []string
}

type RAGAnswerGovernanceGate interface {
	AuthorizeRAGAnswer(context.Context, RAGAnswerGovernanceInput) (RAGAnswerAuthority, error)
}

type RAGRerankGovernanceGate interface {
	AuthorizeRAGRerank(context.Context, []string) error
}

type RAGAnswerConsentReader interface {
	ListQueryConsents(context.Context) ([]knowledge.ProcessingConsent, error)
	ListCollectionConsents(context.Context, string) ([]knowledge.ProcessingConsent, error)
}

type KnowledgeConsentRAGAnswerGovernanceGate struct {
	Reader RAGAnswerConsentReader
}

type KnowledgeConsentRAGRerankGovernanceGate struct {
	Reader RAGAnswerConsentReader
}

type KnowledgeConsentRoutingCatalogGovernanceGate struct {
	Reader RAGAnswerConsentReader
}

func NewKnowledgeConsentRAGAnswerGovernanceGate(reader RAGAnswerConsentReader) *KnowledgeConsentRAGAnswerGovernanceGate {
	return &KnowledgeConsentRAGAnswerGovernanceGate{Reader: reader}
}

func NewKnowledgeConsentRAGRerankGovernanceGate(reader RAGAnswerConsentReader) *KnowledgeConsentRAGRerankGovernanceGate {
	return &KnowledgeConsentRAGRerankGovernanceGate{Reader: reader}
}

func NewKnowledgeConsentRoutingCatalogGovernanceGate(
	reader RAGAnswerConsentReader,
) *KnowledgeConsentRoutingCatalogGovernanceGate {
	return &KnowledgeConsentRoutingCatalogGovernanceGate{Reader: reader}
}

func (g *KnowledgeConsentRoutingCatalogGovernanceGate) AuthorizeRoutingCatalog(
	ctx context.Context,
	input RAGRoutingCatalogGovernanceInput,
) error {
	if g == nil || g.Reader == nil {
		return ErrRAGDependencyUnavailable
	}
	providerID := strings.TrimSpace(input.ModelRef.ProviderID)
	modelID := strings.TrimSpace(input.ModelRef.ModelID)
	if providerID == "" || modelID == "" || len(input.SelectedCollectionIDs) == 0 {
		return ErrRAGRoutingCatalogGovernanceRequired
	}
	queryConsents, err := g.Reader.ListQueryConsents(ctx)
	if err != nil {
		return fmt.Errorf("list routing catalog query consents: %w", err)
	}
	if _, ok := selectRAGAnswerConsent(queryConsents, providerID, modelID); !ok {
		return ErrRAGRoutingCatalogGovernanceRequired
	}
	for _, collectionID := range input.SelectedCollectionIDs {
		collectionID = strings.TrimSpace(collectionID)
		if collectionID == "" {
			return ErrRAGRoutingCatalogGovernanceRequired
		}
		consents, err := g.Reader.ListCollectionConsents(ctx, collectionID)
		if err != nil {
			if errors.Is(err, knowledge.ErrCollectionNotFound) ||
				errors.Is(err, knowledge.ErrUnauthenticated) {
				return ErrRAGRoutingCatalogGovernanceRequired
			}
			return fmt.Errorf("list routing catalog collection consents: %w", err)
		}
		if _, ok := selectRAGAnswerConsent(consents, providerID, modelID); !ok {
			return ErrRAGRoutingCatalogGovernanceRequired
		}
	}
	return nil
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
	return selectRAGPurposeConsent(consents, providerID, modelID, "answer")
}

func (g *KnowledgeConsentRAGRerankGovernanceGate) AuthorizeRAGRerank(
	ctx context.Context,
	selectedCollectionIDs []string,
) error {
	if g == nil || g.Reader == nil {
		return ErrRAGDependencyUnavailable
	}
	if len(selectedCollectionIDs) == 0 {
		return ErrRAGRerankGovernanceRequired
	}
	identity := knowledge.SingleUserRerankIdentity()
	queryConsents, err := g.Reader.ListQueryConsents(ctx)
	if err != nil {
		return fmt.Errorf("list rag rerank query consents: %w", err)
	}
	if _, ok := selectRAGPurposeConsent(
		queryConsents,
		identity.Processor,
		identity.ModelID,
		"rerank",
	); !ok {
		return ErrRAGRerankGovernanceRequired
	}
	for _, collectionID := range selectedCollectionIDs {
		collectionID = strings.TrimSpace(collectionID)
		if collectionID == "" {
			return ErrRAGRerankGovernanceRequired
		}
		collectionConsents, err := g.Reader.ListCollectionConsents(ctx, collectionID)
		if err != nil {
			if errors.Is(err, knowledge.ErrCollectionNotFound) || errors.Is(err, knowledge.ErrUnauthenticated) {
				return ErrRAGRerankGovernanceRequired
			}
			return fmt.Errorf("list rag rerank collection consents: %w", err)
		}
		if _, ok := selectRAGPurposeConsent(
			collectionConsents,
			identity.Processor,
			identity.ModelID,
			"rerank",
		); !ok {
			return ErrRAGRerankGovernanceRequired
		}
	}
	return nil
}

func selectRAGPurposeConsent(
	consents []knowledge.ProcessingConsent,
	providerID string,
	modelID string,
	purpose string,
) (knowledge.ProcessingConsent, bool) {
	providerID = strings.TrimSpace(providerID)
	modelID = strings.TrimSpace(modelID)
	for _, consent := range consents {
		if !ragConsentMatchesModel(consent, providerID, modelID) || !ragConsentAllowsPurpose(consent, purpose) {
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
	return ragConsentAllowsPurpose(consent, "answer")
}

func ragConsentAllowsPurpose(consent knowledge.ProcessingConsent, purpose string) bool {
	if consent.Decision != "granted" || consent.EffectiveStatus != "granted" {
		return false
	}
	return containsString(consent.Purposes, purpose) && containsString(consent.DataTypes, "text/plain")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}
