package hindsightfixture

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"neo-chat/mm-chat/backend/internal/memoryeval"
)

func TestCheckedInDraftManifestBindsGoldenWithoutPromotion(t *testing.T) {
	manifestBody, err := os.ReadFile("../../../docs/contracts/memory-hindsight-fixture-draft.json")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := DecodeManifest(bytes.NewReader(manifestBody))
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	goldenBody, err := os.ReadFile("../../../docs/contracts/memory-benchmark-golden-draft-template.json")
	if err != nil {
		t.Fatal(err)
	}
	golden, err := memoryeval.DecodeGoldenSet(bytes.NewReader(goldenBody))
	if err != nil {
		t.Fatalf("DecodeGoldenSet() error = %v", err)
	}
	digest := sha256.Sum256(goldenBody)
	if err := ValidateGoldenBinding(
		manifest,
		golden,
		hex.EncodeToString(digest[:]),
	); err != nil {
		t.Fatalf("ValidateGoldenBinding() error = %v", err)
	}
	if manifest.PromotionEligible == nil || *manifest.PromotionEligible {
		t.Fatal("checked-in fixture unexpectedly permits promotion")
	}
	if err := memoryeval.ValidateGoldenAdmission(golden); err == nil {
		t.Fatal("checked-in ten-case draft unexpectedly admitted as frozen evidence")
	}
}

func TestDecodeManifestRejectsStrictJSONAndHashDrift(t *testing.T) {
	valid := validManifestJSON(t)
	for _, test := range []struct {
		name string
		body string
		want string
	}{
		{
			name: "duplicate",
			body: strings.Replace(valid, `"schemaVersion":`, `"schemaVersion":"duplicate","schemaVersion":`, 1),
			want: "duplicate JSON object key",
		},
		{
			name: "unknown",
			body: strings.Replace(valid, `"id":`, `"apiUrl":"http://forbidden","id":`, 1),
			want: "unknown field",
		},
		{name: "trailing", body: valid + `{}`, want: "trailing"},
		{
			name: "hash drift",
			body: strings.Replace(valid, "Synthetic fixture fact", "Changed fixture fact", 1),
			want: "content hash does not match",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := DecodeManifest(strings.NewReader(test.body))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("DecodeManifest() error = %v, want %q", err, test.want)
			}
		})
	}
	oversized := strings.Repeat(" ", maximumManifestBytes+1)
	if _, err := DecodeManifest(strings.NewReader(oversized)); err == nil ||
		!strings.Contains(err.Error(), "size") {
		t.Fatalf("oversized DecodeManifest() error = %v", err)
	}
}

func TestDecodeManifestRejectsFixturePolicyAndCallerControlledAuthority(t *testing.T) {
	valid := validManifestJSON(t)
	for _, replacement := range []struct{ old, new string }{
		{`"promotionEligible":false`, `"promotionEligible":true`},
		{`"syntheticOnly":true`, `"syntheticOnly":false`},
		{`"containsRealUserData":false`, `"containsRealUserData":true`},
		{`"containsSensitiveData":false`, `"containsSensitiveData":true`},
		{`"alias":"fixture-a"`, `"alias":"https://endpoint.invalid"`},
	} {
		body := strings.Replace(valid, replacement.old, replacement.new, 1)
		_, err := DecodeManifest(strings.NewReader(body))
		if err == nil {
			t.Fatalf("DecodeManifest() accepted replacement %q", replacement.new)
		}
	}
}

func TestDeriveBankIDIsStableScopedAndOpaque(t *testing.T) {
	key := strings.Repeat("k", 32)
	hash := strings.Repeat("a", 64)
	first, err := DeriveBankID(key, hash, ModeEndToEnd, "fixture-a", "user-a")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := DeriveBankID(key, hash, ModeEndToEnd, "fixture-a", "user-a")
	otherMode, _ := DeriveBankID(key, hash, ModeRetrievalOnly, "fixture-a", "user-a")
	otherUser, _ := DeriveBankID(key, hash, ModeEndToEnd, "fixture-a", "user-b")
	if first != again || first == otherMode || first == otherUser {
		t.Fatalf("derived bank IDs are not correctly scoped: %q %q %q %q", first, again, otherMode, otherUser)
	}
	if strings.Contains(first, "fixture") || strings.Contains(first, "user") || !strings.HasPrefix(first, "neo-") {
		t.Fatalf("derived bank ID is not opaque: %q", first)
	}
}

func validManifestJSON(t *testing.T) string {
	t.Helper()
	promotion := false
	manifest := Manifest{
		SchemaVersion:     ManifestSchemaVersion,
		ID:                "fixture-manifest",
		PromotionEligible: &promotion,
		DataPolicy:        DataPolicy{SyntheticOnly: true},
		Fixtures: []FixtureSet{{
			Alias: "fixture-a", UserAlias: "user-a",
			Memories: []Memory{{
				ID: "memory-a", CanonicalContent: "Synthetic fixture fact",
				RawEventContent: "The user stated a synthetic fixture fact.",
				OccurredAt:      "2026-01-01T00:00:00Z", State: StateActive,
			}},
		}},
	}
	digest, err := ManifestContentSHA256(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifest.ContentSHA256 = digest
	body, err := jsonMarshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func jsonMarshal(value any) ([]byte, error) {
	return json.Marshal(value)
}
