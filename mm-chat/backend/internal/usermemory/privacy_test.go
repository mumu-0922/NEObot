package usermemory

import (
	"strings"
	"testing"
)

func TestRedactMemoryProviderTextRemovesCredentialAssignments(t *testing.T) {
	for _, fixture := range []string{
		"token=fixture-token-value",
		"cookie: fixture-cookie-value",
		"session_id is fixture-session-value",
		"api token：fixture-api-token",
	} {
		if got := RedactMemoryProviderText(fixture, true); got != "" {
			t.Fatalf("credential fixture was not removed: input=%q output=%q", fixture, got)
		}
		if sensitivity := ClassifyMemorySensitivity(fixture); sensitivity != SensitivitySecret {
			t.Fatalf("credential fixture classified as %q: %q", sensitivity, fixture)
		}
	}

	redacted := RedactMemoryProviderText(
		"token=fixture-token-value. Keep answers concise.",
		true,
	)
	if strings.Contains(redacted, "fixture-token-value") ||
		!strings.Contains(redacted, "Keep answers concise") {
		t.Fatalf("mixed provider text redaction = %q", redacted)
	}
}
