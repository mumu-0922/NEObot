package knowledge

import (
	"context"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxRoutingCatalogCollections       = 8
	maxRoutingCatalogQueryBytes        = 2048
	maxRoutingCatalogTerms             = 64
	maxRoutingCatalogDocuments         = 8
	maxRoutingCatalogRelevantDocuments = 5
	maxRoutingCatalogFallbackDocuments = 3
)

// BuildRoutingCatalog returns ACL-filtered metadata for routing only. The
// result intentionally contains no document IDs, object keys, chunk text, or
// retrieval evidence.
func (s *Service) BuildRoutingCatalog(
	ctx context.Context,
	input RoutingCatalogInput,
) (RoutingCatalog, error) {
	if err := s.requireRepository(); err != nil {
		return RoutingCatalog{}, err
	}
	actor, err := requireActor(ctx)
	if err != nil {
		return RoutingCatalog{}, err
	}
	query := strings.Join(strings.Fields(input.QueryText), " ")
	if query == "" || len(query) > maxRoutingCatalogQueryBytes {
		return RoutingCatalog{}, errors.New("routing catalog query is invalid")
	}
	collectionIDs, err := normalizeRoutingCatalogCollectionIDs(input.CollectionIDs)
	if err != nil {
		return RoutingCatalog{}, err
	}
	collections, err := s.repo.FetchRoutingCatalog(ctx, RoutingCatalogRepositoryInput{
		ActorUserID:   actor.ID,
		CollectionIDs: collectionIDs,
		QueryText:     query,
		QueryTerms:    routingCatalogTerms(query),
	})
	if err != nil {
		return RoutingCatalog{}, err
	}

	result := RoutingCatalog{Collections: make([]RoutingCatalogCollection, 0, len(collections))}
	for _, collection := range collections {
		bounded := collection
		bounded.Documents = selectRoutingCatalogDocuments(collection.Documents)
		for _, document := range bounded.Documents {
			if document.RelevanceScore > 0 {
				result.HasLexicalMatch = true
				break
			}
		}
		result.Collections = append(result.Collections, bounded)
	}
	return result, nil
}

func normalizeRoutingCatalogCollectionIDs(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > maxRoutingCatalogCollections {
		return nil, errors.New("routing catalog collections are invalid")
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		id, err := normalizeUUID(value, "collection id")
		if err != nil {
			return nil, err
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	if len(result) == 0 {
		return nil, errors.New("routing catalog collections are invalid")
	}
	return result, nil
}

func selectRoutingCatalogDocuments(
	documents []RoutingCatalogDocument,
) []RoutingCatalogDocument {
	selected := make([]RoutingCatalogDocument, 0, maxRoutingCatalogDocuments)
	seen := make(map[string]struct{}, maxRoutingCatalogDocuments)
	for _, document := range documents {
		if document.RelevanceScore <= 0 || len(selected) >= maxRoutingCatalogRelevantDocuments {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(document.Title))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		selected = append(selected, document)
	}
	fallbackCount := 0
	for _, document := range documents {
		if len(selected) >= maxRoutingCatalogDocuments ||
			fallbackCount >= maxRoutingCatalogFallbackDocuments {
			break
		}
		if !document.Representative {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(document.Title))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		selected = append(selected, document)
		fallbackCount++
	}
	return selected
}

// routingCatalogTerms mirrors the retrieval CJK normalization contract:
// bounded lowercase ASCII/Unicode words plus overlapping CJK ideograph
// bigrams. Generic Latin bigrams are deliberately not generated.
func routingCatalogTerms(query string) []string {
	seen := make(map[string]struct{}, maxRoutingCatalogTerms)
	terms := make([]string, 0, maxRoutingCatalogTerms)
	add := func(term string) {
		term = strings.ToLower(strings.TrimSpace(term))
		if term == "" || len(terms) >= maxRoutingCatalogTerms {
			return
		}
		if _, ok := seen[term]; ok {
			return
		}
		seen[term] = struct{}{}
		terms = append(terms, term)
	}

	for _, term := range strings.FieldsFunc(query, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	}) {
		if utf8.RuneCountInString(term) >= 2 {
			add(term)
		}
	}

	cjk := make([]rune, 0, len(query))
	flushCJK := func() {
		for index := 0; index+1 < len(cjk) && len(terms) < maxRoutingCatalogTerms; index++ {
			add(string(cjk[index : index+2]))
		}
		cjk = cjk[:0]
	}
	for _, r := range query {
		if r >= '\u4e00' && r <= '\u9fff' {
			cjk = append(cjk, r)
			continue
		}
		flushCJK()
	}
	flushCJK()
	return terms
}
