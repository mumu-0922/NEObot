package memorycapture

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"neo-chat/mm-chat/backend/internal/memoryauthor"
	"neo-chat/mm-chat/backend/internal/memoryeval"
)

const fixtureMappingVersion = "neo-chat.memory-regression-fixture-mapping.v1"

// FixtureIndex binds opaque benchmark aliases to deterministic ephemeral UUIDs
// and back. UUIDs never appear in retained observations.
type FixtureIndex struct {
	FixturesByAlias map[string]memoryauthor.Fixture
	CasesByFixture  map[string]memoryeval.GoldenCase
	MemoryToUUID    map[string]string
	UUIDToMemory    map[string]string
	UserToUUID      map[string]string
}

func BuildFixtureIndex(pool memoryauthor.RegressionPool) (FixtureIndex, error) {
	index := FixtureIndex{
		FixturesByAlias: make(map[string]memoryauthor.Fixture, len(pool.Fixtures.Fixtures)),
		CasesByFixture:  make(map[string]memoryeval.GoldenCase, len(pool.Corpus.Cases)),
		MemoryToUUID:    make(map[string]string),
		UUIDToMemory:    make(map[string]string),
		UserToUUID:      make(map[string]string),
	}
	for _, fixture := range pool.Fixtures.Fixtures {
		if _, duplicate := index.FixturesByAlias[fixture.Alias]; duplicate {
			return FixtureIndex{}, fmt.Errorf("%w: duplicate fixture alias", ErrCaptureInvalid)
		}
		index.FixturesByAlias[fixture.Alias] = fixture
		index.ensureUser(fixture.UserAlias)
		for _, memory := range fixture.Memories {
			index.ensureUser(memory.UserAlias)
			id := deterministicUUID("memory", memory.ID)
			if _, duplicate := index.MemoryToUUID[memory.ID]; duplicate {
				return FixtureIndex{}, fmt.Errorf("%w: duplicate fixture Memory ID", ErrCaptureInvalid)
			}
			if other, collision := index.UUIDToMemory[id]; collision && other != memory.ID {
				return FixtureIndex{}, fmt.Errorf("%w: fixture Memory UUID collision", ErrCaptureInvalid)
			}
			index.MemoryToUUID[memory.ID] = id
			index.UUIDToMemory[id] = memory.ID
		}
	}
	for _, item := range pool.Corpus.Cases {
		if _, duplicate := index.CasesByFixture[item.FixtureAlias]; duplicate {
			return FixtureIndex{}, fmt.Errorf("%w: fixture maps to multiple cases", ErrCaptureInvalid)
		}
		if _, ok := index.FixturesByAlias[item.FixtureAlias]; !ok {
			return FixtureIndex{}, fmt.Errorf("%w: case fixture is missing", ErrCaptureInvalid)
		}
		index.CasesByFixture[item.FixtureAlias] = item
		index.ensureUser(item.Scope.UserAlias)
	}
	if len(index.FixturesByAlias) != len(pool.Corpus.Cases) ||
		len(index.CasesByFixture) != len(pool.Fixtures.Fixtures) {
		return FixtureIndex{}, fmt.Errorf("%w: fixture/case cardinality differs", ErrCaptureInvalid)
	}
	return index, nil
}

func (index *FixtureIndex) ensureUser(alias string) {
	if index.UserToUUID == nil || strings.TrimSpace(alias) == "" {
		return
	}
	if _, ok := index.UserToUUID[alias]; !ok {
		index.UserToUUID[alias] = deterministicUUID("user", alias)
	}
}

func (index FixtureIndex) OpaqueMemoryIDs(values []string) ([]string, error) {
	result := make([]string, len(values))
	seen := make(map[string]struct{}, len(values))
	for position, value := range values {
		opaque, ok := index.UUIDToMemory[strings.ToLower(strings.TrimSpace(value))]
		if !ok {
			return nil, fmt.Errorf("%w: reader returned an unknown Memory ID", ErrCaptureStateConflict)
		}
		if _, duplicate := seen[opaque]; duplicate {
			return nil, fmt.Errorf("%w: reader returned a duplicate Memory ID", ErrCaptureStateConflict)
		}
		seen[opaque] = struct{}{}
		result[position] = opaque
	}
	return result, nil
}

func deterministicUUID(kind, value string) string {
	digest := sha256.Sum256([]byte(fixtureMappingVersion + "\x00" + kind + "\x00" + value))
	bytes := digest[:16]
	bytes[6] = (bytes[6] & 0x0f) | 0x50
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(bytes)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" +
		encoded[16:20] + "-" + encoded[20:32]
}
