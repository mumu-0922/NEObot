package chat

import "testing"

func TestExtractRAGSelectionParsesConfigAliasesAndDedupe(t *testing.T) {
	selection, err := extractRAGSelection(map[string]any{
		"knowledgeStrict": true,
		"knowledgeCollectionIds": []any{
			"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
			" AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA ",
			"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
		},
	}, nil)
	if err != nil {
		t.Fatalf("extractRAGSelection() error = %v", err)
	}
	if !selection.Enabled || !selection.Strict {
		t.Fatalf("selection flags = %#v", selection)
	}
	if len(selection.CollectionIDs) != 2 || selection.CollectionIDs[0] != "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa" || selection.CollectionIDs[1] != "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" {
		t.Fatalf("collection ids = %#v", selection.CollectionIDs)
	}
}

func TestExtractRAGSelectionFallsBackToMetadataSingleID(t *testing.T) {
	selection, err := extractRAGSelection(nil, map[string]any{
		"strictRag":             true,
		"knowledgeCollectionId": "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
	})
	if err != nil {
		t.Fatalf("extractRAGSelection() error = %v", err)
	}
	if !selection.Enabled || !selection.Strict || len(selection.CollectionIDs) != 1 {
		t.Fatalf("selection = %#v", selection)
	}
}

func TestExtractRAGSelectionRejectsInvalidCollectionID(t *testing.T) {
	_, err := extractRAGSelection(map[string]any{
		"ragCollectionIds": []any{"not-a-uuid"},
	}, nil)
	assertValidationCode(t, err, "INVALID_RAG_SELECTION")
}

func TestExtractRAGSelectionRejectsNonStringCollectionID(t *testing.T) {
	_, err := extractRAGSelection(map[string]any{
		"selectedCollectionIds": []any{"aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", 42},
	}, nil)
	assertValidationCode(t, err, "INVALID_RAG_SELECTION")
}

func TestExtractRAGSelectionRejectsTooManyCollections(t *testing.T) {
	ids := make([]any, 33)
	for i := range ids {
		ids[i] = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	}
	_, err := extractRAGSelection(map[string]any{"knowledgeCollectionIds": ids}, nil)
	assertValidationCode(t, err, "INVALID_RAG_SELECTION")
}

func assertValidationCode(t *testing.T, err error, want string) {
	t.Helper()
	validationError, ok := err.(ValidationError)
	if !ok {
		t.Fatalf("error = %#v, want ValidationError", err)
	}
	if validationError.Code != want {
		t.Fatalf("error code = %q, want %q", validationError.Code, want)
	}
}
