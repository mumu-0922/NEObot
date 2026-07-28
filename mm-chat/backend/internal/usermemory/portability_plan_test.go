package usermemory

import (
	"crypto/rand"
	"strings"
	"testing"
	"time"
)

func TestPortabilityPlanCodecBindsPayloadAndDomain(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand.Read() error = %v", err)
	}
	codec, err := NewPortabilityPlanCodec(PortabilityPlanKeyring{
		ActiveKeyID: "k1", Keys: map[string][]byte{"k1": key},
	})
	if err != nil {
		t.Fatalf("NewPortabilityPlanCodec() error = %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	token := PortabilityPlanToken{
		UserID:        "00000000-0000-4000-8000-000000000001",
		ImportID:      "00000000-0000-4000-8000-000000000002",
		PackageSHA256: strings.Repeat("1", 64), ManifestSHA256: strings.Repeat("2", 64),
		MappingsSHA256: strings.Repeat("3", 64), PlanSHA256: strings.Repeat("4", 64),
		AuthorityStateHash: strings.Repeat("5", 64),
		IssuedAt:           now.UnixMilli(), ExpiresAt: now.Add(PortabilityPlanTTL).UnixMilli(),
	}
	encoded, err := codec.Encode(token)
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	decoded, err := codec.Decode(encoded)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if decoded.ImportID != token.ImportID || decoded.KeyID != "k1" {
		t.Fatalf("decoded = %#v", decoded)
	}

	tampered := []byte(encoded)
	tampered[len(tampered)-1] ^= 1
	if _, err := codec.Decode(string(tampered)); errorCode(err) != "MEMORY_IMPORT_PLAN_TOKEN_INVALID" {
		t.Fatalf("tampered token error = %v", err)
	}
}

func TestNormalizeImportMappingsRejectsMixedAuthority(t *testing.T) {
	valid := ImportMappings{
		Projects: map[string]ImportProjectMapping{
			"project-000001": {Mode: "create"},
		},
		Conversations: map[string]ImportConversationMapping{
			"conversation-000001": {Mode: "project", ProjectRef: "project-000001"},
		},
	}
	if _, err := normalizeImportMappings(valid); err != nil {
		t.Fatalf("normalizeImportMappings() error = %v", err)
	}
	valid.Conversations["conversation-000001"] = ImportConversationMapping{
		Mode: "project", ProjectID: "00000000-0000-4000-8000-000000000001",
		ProjectRef: "project-000001",
	}
	if _, err := normalizeImportMappings(valid); errorCode(err) != "MEMORY_IMPORT_MAPPING_INVALID" {
		t.Fatalf("mixed mapping error = %v", err)
	}
}

func TestNormalizeImportMappingsRejectsNormalizedDuplicateRefs(t *testing.T) {
	tests := []struct {
		name     string
		mappings ImportMappings
	}{
		{
			name: "project",
			mappings: ImportMappings{Projects: map[string]ImportProjectMapping{
				"project-000001":   {Mode: "create"},
				" project-000001 ": {Mode: "skip"},
			}},
		},
		{
			name: "conversation",
			mappings: ImportMappings{Conversations: map[string]ImportConversationMapping{
				"conversation-000001":   {Mode: "global"},
				" conversation-000001 ": {Mode: "skip"},
			}},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := normalizeImportMappings(testCase.mappings); errorCode(err) != "MEMORY_IMPORT_MAPPING_INVALID" {
				t.Fatalf("normalizeImportMappings() error = %v", err)
			}
		})
	}
}

func TestDeterministicImportUUID(t *testing.T) {
	first, err := deterministicImportUUID(
		"00000000-0000-4000-8000-000000000001",
		"memory-000001",
		"memory",
	)
	if err != nil {
		t.Fatalf("deterministicImportUUID() error = %v", err)
	}
	second, _ := deterministicImportUUID(
		"00000000-0000-4000-8000-000000000001",
		"memory-000001",
		"memory",
	)
	if first != second || !uuidRE.MatchString(first) {
		t.Fatalf("UUIDs = %q %q", first, second)
	}
}
