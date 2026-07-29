package memorycapture

import (
	"testing"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
)

func TestBuildFixtureIndexCoversFixedRegressionPool(t *testing.T) {
	pool, err := memoryauthor.GenerateRegression()
	if err != nil {
		t.Fatal(err)
	}
	index, err := BuildFixtureIndex(pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(index.FixturesByAlias) != 500 || len(index.CasesByFixture) != 500 ||
		len(index.MemoryToUUID) != 625 || len(index.UUIDToMemory) != 625 ||
		len(index.UserToUUID) != 550 {
		t.Fatalf("index counts = fixtures:%d cases:%d memories:%d reverse:%d users:%d",
			len(index.FixturesByAlias), len(index.CasesByFixture),
			len(index.MemoryToUUID), len(index.UUIDToMemory), len(index.UserToUUID))
	}
	for opaque, databaseID := range index.MemoryToUUID {
		if databaseID != deterministicUUID("memory", opaque) || index.UUIDToMemory[databaseID] != opaque {
			t.Fatalf("Memory mapping drift for %q", opaque)
		}
	}
}

func TestOpaqueMemoryIDsRejectsUnknownAndDuplicate(t *testing.T) {
	index := FixtureIndex{UUIDToMemory: map[string]string{
		"11111111-1111-4111-8111-111111111111": "memory-one",
	}}
	if _, err := index.OpaqueMemoryIDs([]string{"22222222-2222-4222-8222-222222222222"}); err == nil {
		t.Fatal("unknown database Memory ID was accepted")
	}
	if _, err := index.OpaqueMemoryIDs([]string{
		"11111111-1111-4111-8111-111111111111",
		"11111111-1111-4111-8111-111111111111",
	}); err == nil {
		t.Fatal("duplicate database Memory ID was accepted")
	}
}
