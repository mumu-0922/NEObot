package chat

import (
	"context"
	"encoding/json"
	"strings"

	"neo-chat/mm-chat/backend/internal/knowledge"
)

const (
	maxKnowledgeRoutingCatalogBytes       = 4 << 10
	maxKnowledgeRoutingCollectionName     = 128
	maxKnowledgeRoutingDescription        = 512
	maxKnowledgeRoutingDocumentTitle      = 256
	maxKnowledgeRoutingCatalogCollections = 8
)

type KnowledgeRoutingCatalogSource interface {
	BuildRoutingCatalog(
		context.Context,
		knowledge.RoutingCatalogInput,
	) (knowledge.RoutingCatalog, error)
}

type KnowledgeRoutingCatalogGovernanceGate interface {
	AuthorizeRoutingCatalog(
		context.Context,
		RAGRoutingCatalogGovernanceInput,
	) error
}

type routingCatalogPromptCollection struct {
	Name                string   `json:"name"`
	Description         string   `json:"description,omitempty"`
	ActiveDocumentCount int      `json:"activeDocumentCount"`
	CandidateDocuments  []string `json:"candidateDocuments,omitempty"`
	Truncated           bool     `json:"truncated"`
}

func prepareKnowledgeRoutingCatalog(
	ctx context.Context,
	source KnowledgeRoutingCatalogSource,
	gate KnowledgeRoutingCatalogGovernanceGate,
	runtime *knowledgeToolRuntime,
) {
	if source == nil || gate == nil || !runtime.enabled() {
		return
	}
	if err := gate.AuthorizeRoutingCatalog(ctx, RAGRoutingCatalogGovernanceInput{
		ModelRef:              runtime.GovernanceModelRef,
		SelectedCollectionIDs: append([]string(nil), runtime.SelectedCollectionIDs...),
	}); err != nil {
		return
	}
	catalog, err := source.BuildRoutingCatalog(ctx, knowledge.RoutingCatalogInput{
		CollectionIDs: append([]string(nil), runtime.SelectedCollectionIDs...),
		QueryText:     runtime.OriginalQueryText,
	})
	if err != nil {
		return
	}
	serialized := serializeKnowledgeRoutingCatalog(catalog)
	if serialized == "" {
		return
	}
	runtime.RoutingCatalog = serialized
	runtime.StrongCatalogMatch = catalog.HasLexicalMatch
}

func serializeKnowledgeRoutingCatalog(catalog knowledge.RoutingCatalog) string {
	collections := catalog.Collections
	if len(collections) > maxKnowledgeRoutingCatalogCollections {
		collections = collections[:maxKnowledgeRoutingCatalogCollections]
	}
	items := make([]routingCatalogPromptCollection, 0, len(collections))
	titles := make([][]string, 0, len(collections))
	for _, collection := range collections {
		name := truncateProcessUTF8(
			strings.Join(strings.Fields(collection.Name), " "),
			maxKnowledgeRoutingCollectionName,
		)
		if name == "" {
			continue
		}
		description := truncateProcessUTF8(
			strings.Join(strings.Fields(collection.Description), " "),
			maxKnowledgeRoutingDescription,
		)
		itemTitles := make([]string, 0, len(collection.Documents))
		seen := make(map[string]struct{}, len(collection.Documents))
		for _, document := range collection.Documents {
			title := truncateProcessUTF8(
				strings.Join(strings.Fields(document.Title), " "),
				maxKnowledgeRoutingDocumentTitle,
			)
			key := strings.ToLower(title)
			if title == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			itemTitles = append(itemTitles, title)
		}
		items = append(items, routingCatalogPromptCollection{
			Name:                name,
			Description:         description,
			ActiveDocumentCount: max(0, collection.ActiveDocumentCount),
			CandidateDocuments:  []string{},
			Truncated:           collection.ActiveDocumentCount > len(itemTitles),
		})
		titles = append(titles, itemTitles)
	}
	if len(items) == 0 {
		return ""
	}

	// Preserve every visible collection before spending the remaining budget
	// on descriptions and titles. Description reduction is fair across the
	// selected set and always UTF-8 safe.
	for _, limit := range []int{maxKnowledgeRoutingDescription, 384, 256, 128, 64, 0} {
		for index := range items {
			items[index].Description = truncateProcessUTF8(
				items[index].Description,
				limit,
			)
		}
		if encodedCatalogFits(items) {
			break
		}
	}
	if !encodedCatalogFits(items) {
		for index := range items {
			items[index].Name = truncateProcessUTF8(items[index].Name, 64)
		}
		if !encodedCatalogFits(items) {
			return ""
		}
	}

	// Ensure the first title round is fair across visible collections. Without
	// this reservation, an early collection can consume the remaining budget
	// before later selected collections receive any routing hint.
	if !encodedCatalogTitleRoundFits(items, titles, 0) {
		for _, limit := range []int{384, 256, 128, 64, 0} {
			for index := range items {
				items[index].Description = truncateProcessUTF8(
					items[index].Description,
					limit,
				)
			}
			if encodedCatalogTitleRoundFits(items, titles, 0) {
				break
			}
		}
	}
	if !encodedCatalogTitleRoundFits(items, titles, 0) {
		for _, limit := range []int{224, 192, 160, 128, 96, 64} {
			for index := range titles {
				if len(titles[index]) > 0 {
					titles[index][0] = truncateProcessUTF8(titles[index][0], limit)
				}
			}
			if encodedCatalogTitleRoundFits(items, titles, 0) {
				break
			}
		}
	}

	for titleIndex := 0; ; titleIndex++ {
		added := false
		for collectionIndex := range items {
			if titleIndex >= len(titles[collectionIndex]) {
				continue
			}
			candidate := titles[collectionIndex][titleIndex]
			items[collectionIndex].CandidateDocuments = append(
				items[collectionIndex].CandidateDocuments,
				candidate,
			)
			if !encodedCatalogFits(items) {
				items[collectionIndex].CandidateDocuments =
					items[collectionIndex].CandidateDocuments[:len(items[collectionIndex].CandidateDocuments)-1]
				items[collectionIndex].Truncated = true
				continue
			}
			added = true
		}
		if !added {
			break
		}
	}
	encoded, err := json.Marshal(items)
	if err != nil || len(encoded) > maxKnowledgeRoutingCatalogBytes {
		return ""
	}
	return string(encoded)
}

func encodedCatalogFits(items []routingCatalogPromptCollection) bool {
	encoded, err := json.Marshal(items)
	return err == nil && len(encoded) <= maxKnowledgeRoutingCatalogBytes
}

func encodedCatalogTitleRoundFits(
	items []routingCatalogPromptCollection,
	titles [][]string,
	titleIndex int,
) bool {
	probe := make([]routingCatalogPromptCollection, len(items))
	for index := range items {
		probe[index] = items[index]
		probe[index].CandidateDocuments = append(
			[]string(nil),
			items[index].CandidateDocuments...,
		)
		if index < len(titles) && titleIndex < len(titles[index]) {
			probe[index].CandidateDocuments = append(
				probe[index].CandidateDocuments,
				titles[index][titleIndex],
			)
		}
	}
	return encodedCatalogFits(probe)
}
