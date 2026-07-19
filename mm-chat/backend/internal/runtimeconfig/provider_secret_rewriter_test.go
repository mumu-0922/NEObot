package runtimeconfig

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"strings"
	"testing"
	"time"

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/providersecrets"
	"neo-chat/mm-chat/backend/internal/websearch"
)

func TestProviderSecretRewritePlanCoversLegacyRotationCurrentAndDeletedRows(t *testing.T) {
	privateKey, cfg := providerSecretRewriteBYOKConfig(t)
	oldVault := providerSecretRewriteVault(t, "old", map[string]byte{"old": 21})
	rotatingVault := providerSecretRewriteVault(t, "new", map[string]byte{
		"new": 22,
		"old": 21,
	})
	const userID = "00000000-0000-0000-0000-000000000001"
	rows := []providerSecretRewriteRow{
		providerSecretVaultRewriteRow(t, oldVault, "1", userID, "OLD", "old-secret", false),
		providerSecretLegacyRewriteRow(t, privateKey, "2", userID, "LEGACY", "legacy-secret"),
		providerSecretVaultRewriteRow(t, rotatingVault, "3", userID, "CURRENT", "current-secret", false),
		{
			id: "4", userID: userID, providerID: "EMPTY",
			configJSON: `{"type":"OpenAI Compatible","enabled":true}`,
		},
		providerSecretVaultRewriteRow(t, oldVault, "5", userID, "OLD", "deleted-secret", true),
	}
	rewriter := NewPostgresProviderSecretRewriter(nil, cfg, rotatingVault)

	plan, err := rewriter.buildProviderSecretRewritePlan(rows)
	if err != nil {
		t.Fatalf("buildProviderSecretRewritePlan() error = %v", err)
	}
	result := plan.result
	if result.TotalRows != 5 || result.SecretRows != 4 || result.ChangedRows != 3 ||
		result.LegacyRows != 1 || result.EnvRows != 0 || result.RotatedRows != 2 ||
		result.CurrentRows != 1 || result.EmptyRows != 1 || result.BlockedRows != 0 ||
		result.Executed ||
		!providerSecretRewritePlanPattern.MatchString(result.PlanSHA256) {
		t.Fatalf("rewrite result = %#v", result)
	}

	wantPlaintext := map[string]string{
		"1": "old-secret", "2": "legacy-secret", "5": "deleted-secret",
	}
	for _, row := range plan.rows {
		if row.action != providerSecretRewriteRotate &&
			row.action != providerSecretRewriteLegacy {
			continue
		}
		encoded, err := rewriter.rewriteProviderSecret(row)
		if err != nil {
			t.Fatalf("rewriteProviderSecret(%s) error = %v", row.id, err)
		}
		envelope, err := providersecrets.ParseEnvelope(encoded)
		if err != nil || envelope.KID != "new" {
			t.Fatalf("rewritten envelope %s = %#v, %v", row.id, envelope, err)
		}
		plaintext, err := rotatingVault.Decrypt(
			envelope,
			modelProviderSecretContext(row.userID, row.providerID),
		)
		if err != nil || string(plaintext) != wantPlaintext[row.id] {
			t.Fatalf("rewritten plaintext %s = %q, %v", row.id, plaintext, err)
		}
		clear(plaintext)
	}
}

func TestProviderSecretRewriteReportsUnrecoverableCustomLegacyRow(t *testing.T) {
	_, cfg := providerSecretRewriteBYOKConfig(t)
	stalePrivateKey, _ := providerSecretRewriteBYOKConfig(t)
	vault := providerSecretRewriteVault(t, "active", map[string]byte{"active": 27})
	row := providerSecretLegacyRewriteRow(
		t,
		stalePrivateKey,
		"1",
		"00000000-0000-0000-0000-000000000001",
		"CUSTOM",
		"unrecoverable-secret",
	)
	plan, err := NewPostgresProviderSecretRewriter(nil, cfg, vault).
		buildProviderSecretRewritePlan([]providerSecretRewriteRow{row})
	if err != nil {
		t.Fatalf("buildProviderSecretRewritePlan() error = %v", err)
	}
	if plan.result.BlockedRows != 1 || plan.result.SecretRows != 1 ||
		plan.result.ChangedRows != 0 ||
		plan.rows[0].action != providerSecretRewriteBlocked {
		t.Fatalf("blocked plan = %#v / %#v", plan.result, plan.rows[0])
	}
}

func TestProviderSecretRewriteUsesSearchVaultContextAndBlocksLegacySearchRows(t *testing.T) {
	privateKey, cfg := providerSecretRewriteBYOKConfig(t)
	oldVault := providerSecretRewriteVault(t, "old", map[string]byte{"old": 29})
	rotatingVault := providerSecretRewriteVault(t, "new", map[string]byte{
		"new": 30,
		"old": 29,
	})
	const userID = "00000000-0000-0000-0000-000000000001"
	old := providerSecretSearchVaultRewriteRow(
		t, oldVault, "1", userID, websearch.ProviderTavily, "old-search-secret",
	)
	current := providerSecretSearchVaultRewriteRow(
		t, rotatingVault, "2", userID, websearch.ProviderExa, "current-search-secret",
	)
	legacy := providerSecretLegacyRewriteRow(
		t, privateKey, "3", userID, searchProviderRecordID(websearch.ProviderBocha),
		"legacy-search-secret",
	)
	legacy.configJSON = `{"kind":"search","searchProvider":"bocha","enabled":true}`

	plan, err := NewPostgresProviderSecretRewriter(nil, cfg, rotatingVault).
		buildProviderSecretRewritePlan([]providerSecretRewriteRow{old, current, legacy})
	if err != nil {
		t.Fatalf("buildProviderSecretRewritePlan() error = %v", err)
	}
	if plan.result.RotatedRows != 1 || plan.result.CurrentRows != 1 ||
		plan.result.BlockedRows != 1 || plan.result.ChangedRows != 1 ||
		plan.rows[0].action != providerSecretRewriteRotate ||
		plan.rows[1].action != providerSecretRewriteCurrent ||
		plan.rows[2].action != providerSecretRewriteBlocked {
		t.Fatalf("search rewrite plan = %#v / %#v", plan.result, plan.rows)
	}

	encoded, err := NewPostgresProviderSecretRewriter(nil, cfg, rotatingVault).
		rewriteProviderSecret(plan.rows[0])
	if err != nil {
		t.Fatalf("rewriteProviderSecret() error = %v", err)
	}
	envelope, err := providersecrets.ParseEnvelope(encoded)
	if err != nil || envelope.KID != "new" {
		t.Fatalf("rewritten search envelope = %#v, %v", envelope, err)
	}
	plaintext, err := rotatingVault.Decrypt(
		envelope,
		searchProviderSecretContext(userID, searchProviderRecordID(websearch.ProviderTavily)),
	)
	if err != nil || string(plaintext) != "old-search-secret" {
		t.Fatalf("rewritten search plaintext = %q, %v", plaintext, err)
	}
	clear(plaintext)
	if _, err := rotatingVault.Decrypt(
		envelope,
		modelProviderSecretContext(userID, searchProviderRecordID(websearch.ProviderTavily)),
	); err == nil {
		t.Fatal("search envelope decrypted with model-provider context")
	}
}

func TestProviderSecretRewriteUsesRAGVaultContextAndBlocksLegacyRAGRows(t *testing.T) {
	privateKey, cfg := providerSecretRewriteBYOKConfig(t)
	oldVault := providerSecretRewriteVault(t, "old", map[string]byte{"old": 36})
	rotatingVault := providerSecretRewriteVault(t, "new", map[string]byte{
		"new": 37,
		"old": 36,
	})
	const userID = "00000000-0000-0000-0000-000000000001"
	old := providerSecretRAGVaultRewriteRow(
		t, oldVault, "1", userID, RAGProviderMinerU, "old-rag-secret",
	)
	current := providerSecretRAGVaultRewriteRow(
		t, rotatingVault, "2", userID, RAGProviderJina, "current-rag-secret",
	)
	plan, err := NewPostgresProviderSecretRewriter(nil, cfg, rotatingVault).
		buildProviderSecretRewritePlan([]providerSecretRewriteRow{old, current})
	if err != nil {
		t.Fatalf("buildProviderSecretRewritePlan() error = %v", err)
	}
	if plan.result.RotatedRows != 1 || plan.result.CurrentRows != 1 ||
		plan.result.BlockedRows != 0 || plan.result.ChangedRows != 1 ||
		plan.rows[0].action != providerSecretRewriteRotate ||
		plan.rows[1].action != providerSecretRewriteCurrent {
		t.Fatalf("RAG rewrite plan = %#v / %#v", plan.result, plan.rows)
	}
	legacy := providerSecretLegacyRewriteRow(
		t, privateKey, "3", userID, ragProviderRecordID(RAGProviderJina),
		"legacy-rag-secret",
	)
	legacy.configJSON = `{"kind":"rag","ragProvider":"jina","enabled":true}`
	legacyPlan, err := NewPostgresProviderSecretRewriter(nil, cfg, rotatingVault).
		buildProviderSecretRewritePlan([]providerSecretRewriteRow{legacy})
	if err != nil || legacyPlan.result.BlockedRows != 1 ||
		legacyPlan.rows[0].action != providerSecretRewriteBlocked {
		t.Fatalf("legacy RAG rewrite plan = %#v / %#v / %v", legacyPlan.result, legacyPlan.rows, err)
	}

	encoded, err := NewPostgresProviderSecretRewriter(nil, cfg, rotatingVault).
		rewriteProviderSecret(plan.rows[0])
	if err != nil {
		t.Fatalf("rewriteProviderSecret() error = %v", err)
	}
	envelope, err := providersecrets.ParseEnvelope(encoded)
	if err != nil || envelope.KID != "new" {
		t.Fatalf("rewritten RAG envelope = %#v, %v", envelope, err)
	}
	plaintext, err := rotatingVault.Decrypt(
		envelope,
		ragProviderSecretContext(userID, ragProviderRecordID(RAGProviderMinerU)),
	)
	if err != nil || string(plaintext) != "old-rag-secret" {
		t.Fatalf("rewritten RAG plaintext = %q, %v", plaintext, err)
	}
	clear(plaintext)
	if _, err := rotatingVault.Decrypt(
		envelope,
		modelProviderSecretContext(userID, ragProviderRecordID(RAGProviderMinerU)),
	); err == nil {
		t.Fatal("RAG envelope decrypted with model-provider context")
	}
}

func TestProviderSecretRewriteReservesVoiceVaultContextWithoutAttestation(t *testing.T) {
	privateKey, cfg := providerSecretRewriteBYOKConfig(t)
	oldVault := providerSecretRewriteVault(t, "old", map[string]byte{"old": 40})
	rotatingVault := providerSecretRewriteVault(t, "new", map[string]byte{
		"new": 41,
		"old": 40,
	})
	const userID = "00000000-0000-0000-0000-000000000001"
	old := providerSecretVoiceVaultRewriteRow(
		t,
		oldVault,
		"1",
		userID,
		voiceProviderElevenLabs,
		"future-voice-secret",
	)
	rewriter := NewPostgresProviderSecretRewriter(nil, cfg, rotatingVault)
	plan, err := rewriter.buildProviderSecretRewritePlan(
		[]providerSecretRewriteRow{old},
	)
	if err != nil || plan.result.RotatedRows != 1 ||
		plan.result.ChangedRows != 1 || plan.result.BlockedRows != 0 ||
		plan.rows[0].action != providerSecretRewriteRotate {
		t.Fatalf("Voice rewrite plan = %#v / %#v / %v", plan.result, plan.rows, err)
	}

	row := plan.rows[0]
	encoded, err := rewriter.rewriteProviderSecret(row)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := providersecrets.ParseEnvelope(encoded)
	if err != nil || envelope.KID != "new" {
		t.Fatalf("rewritten Voice envelope = %#v, %v", envelope, err)
	}
	plaintext, err := rotatingVault.Decrypt(
		envelope,
		voiceProviderSecretContext(userID, row.providerID),
	)
	if err != nil || string(plaintext) != "future-voice-secret" {
		t.Fatalf("rewritten Voice plaintext = %q, %v", plaintext, err)
	}
	clear(plaintext)
	for _, wrongContext := range []string{
		modelProviderSecretContext(userID, row.providerID),
		searchProviderSecretContext(userID, row.providerID),
		ragProviderSecretContext(userID, row.providerID),
		voiceProviderSecretContext(userID, voiceProviderRecordID(voiceProviderMimo)),
	} {
		if _, err := rotatingVault.Decrypt(envelope, wrongContext); err == nil {
			t.Fatalf("Voice envelope decrypted with wrong context %q", wrongContext)
		}
	}
	fingerprint, preserve, err := rewriter.rewrittenProviderConnectionAttestation(
		row,
		encoded,
	)
	if err != nil || preserve || fingerprint != "" {
		t.Fatalf("reserved Voice attestation = %q, %t, %v", fingerprint, preserve, err)
	}

	legacy := providerSecretLegacyRewriteRow(
		t,
		privateKey,
		"2",
		userID,
		voiceProviderRecordID(voiceProviderMimo),
		"legacy-voice-secret",
	)
	legacy.configJSON = `{"kind":"voice","voiceProvider":"mimo","enabled":true}`
	legacyPlan, err := rewriter.buildProviderSecretRewritePlan(
		[]providerSecretRewriteRow{legacy},
	)
	if err != nil || legacyPlan.result.BlockedRows != 1 ||
		legacyPlan.result.ChangedRows != 0 ||
		legacyPlan.rows[0].action != providerSecretRewriteBlocked {
		t.Fatalf(
			"legacy Voice plan = %#v / %#v / %v",
			legacyPlan.result,
			legacyPlan.rows,
			err,
		)
	}
}

func TestVoiceProviderReservationRejectsReservedModelAndMismatchedIdentity(t *testing.T) {
	const userID = "00000000-0000-0000-0000-000000000001"
	for _, recordID := range []string{
		voiceProviderRecordID(voiceProviderElevenLabs),
		voiceProviderRecordID(voiceProviderMimo),
	} {
		if IsModelProviderConfig(StoredProviderConfig{ProviderID: recordID}) {
			t.Fatalf("reserved Voice record %s admitted as a model provider", recordID)
		}
	}
	valid := StoredProviderConfigPayload{
		Kind: providerConfigKindVoice, VoiceProvider: string(voiceProviderElevenLabs),
	}
	storedVoice := StoredProviderConfig{
		ProviderID: voiceProviderRecordID(voiceProviderElevenLabs),
		Config:     valid,
	}
	if IsModelProviderConfig(storedVoice) || isSearchProviderConfig(storedVoice) ||
		isRAGProviderConfig(storedVoice) {
		t.Fatal("reserved Voice record admitted by another provider reader")
	}
	context, ok := storedProviderSecretContext(
		userID,
		voiceProviderRecordID(voiceProviderElevenLabs),
		valid,
	)
	if !ok || context != voiceProviderSecretContext(
		userID,
		voiceProviderRecordID(voiceProviderElevenLabs),
	) {
		t.Fatalf("reserved Voice context = %q, %t", context, ok)
	}
	for _, invalid := range []StoredProviderConfigPayload{
		{Kind: providerConfigKindVoice, VoiceProvider: "unknown"},
		{Kind: providerConfigKindVoice, VoiceProvider: string(voiceProviderMimo)},
	} {
		if _, ok := storedProviderSecretContext(
			userID,
			voiceProviderRecordID(voiceProviderElevenLabs),
			invalid,
		); ok {
			t.Fatalf("invalid Voice identity admitted: %#v", invalid)
		}
	}
	if voiceProviderIngressContext(voiceProviderElevenLabs) !=
		"provider:voice:elevenlabs" {
		t.Fatal("Voice ingress context drifted")
	}
}

func TestProviderSecretRewriteRebindsOnlyExistingValidConnectionAttestations(t *testing.T) {
	oldVault := providerSecretRewriteVault(t, "old", map[string]byte{"old": 38})
	rotatingVault := providerSecretRewriteVault(t, "new", map[string]byte{
		"new": 39,
		"old": 38,
	})
	const userID = "00000000-0000-0000-0000-000000000001"
	testedAt := time.Now().UTC().Format(time.RFC3339Nano)

	model := providerSecretVaultRewriteRow(
		t, oldVault, "1", userID, "MODEL", "model-secret", false,
	)
	model.configJSON = providerSecretRewriteConfigJSON(t, model, func(
		payload *StoredProviderConfigPayload,
	) {
		payload.Type = ProviderTypeOpenAICompatible
		payload.BaseURL = "https://model.example.test/v1"
		payload.ConnectionTestedAt = testedAt
		payload.ConnectionTestSHA256 = providerConnectionFingerprint(
			model.providerID,
			payload.Type,
			payload.BaseURL,
			model.encryptedSecretRef,
		)
	})

	search := providerSecretSearchVaultRewriteRow(
		t, oldVault, "2", userID, websearch.ProviderTavily, "search-secret",
	)
	search.configJSON = providerSecretRewriteConfigJSON(t, search, func(
		payload *StoredProviderConfigPayload,
	) {
		payload.BaseURL = "https://api.tavily.com"
		payload.ConnectionTestedAt = testedAt
		payload.ConnectionTestSHA256 = searchProviderConnectionFingerprint(
			search.providerID,
			websearch.ProviderTavily,
			payload.BaseURL,
			search.encryptedSecretRef,
		)
	})

	rag := providerSecretRAGVaultRewriteRow(
		t, oldVault, "3", userID, RAGProviderJina, "rag-secret",
	)
	rag.configJSON = providerSecretRewriteConfigJSON(t, rag, func(
		payload *StoredProviderConfigPayload,
	) {
		payload.ConnectionTestedAt = testedAt
		payload.ConnectionTestSHA256 = ragProviderConnectionFingerprint(
			rag.providerID,
			RAGProviderJina,
			rag.encryptedSecretRef,
		)
	})

	invalid := providerSecretRAGVaultRewriteRow(
		t, oldVault, "4", userID, RAGProviderMinerU, "invalid-secret",
	)
	invalid.configJSON = providerSecretRewriteConfigJSON(t, invalid, func(
		payload *StoredProviderConfigPayload,
	) {
		payload.ConnectionTestedAt = testedAt
		payload.ConnectionTestSHA256 = strings.Repeat("0", 64)
	})

	rewriter := NewPostgresProviderSecretRewriter(nil, config.Config{}, rotatingVault)
	tests := []struct {
		row  providerSecretRewriteRow
		want func(string) string
	}{
		{model, func(secretRef string) string {
			return providerConnectionFingerprint(
				model.providerID,
				ProviderTypeOpenAICompatible,
				"https://model.example.test/v1",
				secretRef,
			)
		}},
		{search, func(secretRef string) string {
			return searchProviderConnectionFingerprint(
				search.providerID,
				websearch.ProviderTavily,
				"https://api.tavily.com",
				secretRef,
			)
		}},
		{rag, func(secretRef string) string {
			return ragProviderConnectionFingerprint(
				rag.providerID,
				RAGProviderJina,
				secretRef,
			)
		}},
	}
	for _, test := range tests {
		test.row.action = providerSecretRewriteRotate
		rewrittenSecretRef, err := rewriter.rewriteProviderSecret(test.row)
		if err != nil {
			t.Fatalf("rewriteProviderSecret(%s) error = %v", test.row.providerID, err)
		}
		fingerprint, preserve, err := rewriter.rewrittenProviderConnectionAttestation(
			test.row,
			rewrittenSecretRef,
		)
		if err != nil || !preserve || fingerprint != test.want(rewrittenSecretRef) {
			t.Fatalf(
				"rewrittenProviderConnectionAttestation(%s) = %q, %t, %v",
				test.row.providerID,
				fingerprint,
				preserve,
				err,
			)
		}
	}

	invalid.action = providerSecretRewriteRotate
	rewrittenSecretRef, err := rewriter.rewriteProviderSecret(invalid)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, preserve, err := rewriter.rewrittenProviderConnectionAttestation(
		invalid,
		rewrittenSecretRef,
	)
	if err != nil || preserve || fingerprint != "" {
		t.Fatalf("invalid attestation = %q, %t, %v", fingerprint, preserve, err)
	}
}

func providerSecretRewriteConfigJSON(
	t *testing.T,
	row providerSecretRewriteRow,
	mutate func(*StoredProviderConfigPayload),
) string {
	t.Helper()
	var payload StoredProviderConfigPayload
	if err := json.Unmarshal([]byte(row.configJSON), &payload); err != nil {
		t.Fatal(err)
	}
	mutate(&payload)
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestProviderSecretRewriteBlocksUnrecoverableServerDefaultWithoutEnvFallback(t *testing.T) {
	_, cfg := providerSecretRewriteBYOKConfig(t)
	stalePrivateKey, _ := providerSecretRewriteBYOKConfig(t)
	vault := providerSecretRewriteVault(t, "active", map[string]byte{"active": 26})
	row := providerSecretLegacyRewriteRow(
		t,
		stalePrivateKey,
		"1",
		"00000000-0000-0000-0000-000000000001",
		serverDefaultProviderID,
		"unrecoverable-secret",
	)
	plan, err := NewPostgresProviderSecretRewriter(nil, cfg, vault).
		buildProviderSecretRewritePlan([]providerSecretRewriteRow{row})
	if err != nil {
		t.Fatalf("buildProviderSecretRewritePlan() error = %v", err)
	}
	if plan.result.EnvRows != 0 || plan.result.BlockedRows != 1 ||
		plan.result.ChangedRows != 0 || plan.rows[0].action != providerSecretRewriteBlocked {
		t.Fatalf("blocked plan = %#v / %#v", plan.result, plan.rows[0])
	}
}

func TestProviderSecretRewritePlanRejectsAmbiguousOrInvalidRows(t *testing.T) {
	_, cfg := providerSecretRewriteBYOKConfig(t)
	vault := providerSecretRewriteVault(t, "active", map[string]byte{"active": 23})
	rewriter := NewPostgresProviderSecretRewriter(nil, cfg, vault)
	const userID = "00000000-0000-0000-0000-000000000001"

	duplicate := []providerSecretRewriteRow{
		{id: "1", userID: userID, providerID: "DUP", configJSON: `{}`},
		{id: "2", userID: userID, providerID: "DUP", configJSON: `{}`},
	}
	if _, err := rewriter.buildProviderSecretRewritePlan(duplicate); !errors.Is(err, ErrProviderSecretRewriteInvalid) {
		t.Fatalf("duplicate error = %v", err)
	}

	copied := providerSecretVaultRewriteRow(
		t, vault, "3", userID, "SOURCE", "copied-secret", false,
	)
	copied.providerID = "TARGET"
	if _, err := rewriter.buildProviderSecretRewritePlan(
		[]providerSecretRewriteRow{copied},
	); !errors.Is(err, ErrProviderSecretRewriteInvalid) {
		t.Fatalf("copied context error = %v", err)
	}

	untrimmed := providerSecretRewriteRow{
		id: "4", userID: userID, providerID: " BAD ", configJSON: `{}`,
	}
	if _, err := rewriter.buildProviderSecretRewritePlan(
		[]providerSecretRewriteRow{untrimmed},
	); !errors.Is(err, ErrProviderSecretRewriteInvalid) {
		t.Fatalf("untrimmed provider error = %v", err)
	}

	reserved := providerSecretVaultRewriteRow(
		t, vault, "5", userID, ragProviderRecordID(RAGProviderJina),
		"reserved-model-context-secret", false,
	)
	if _, err := rewriter.buildProviderSecretRewritePlan(
		[]providerSecretRewriteRow{reserved},
	); !errors.Is(err, ErrProviderSecretRewriteInvalid) {
		t.Fatalf("reserved RAG model context error = %v", err)
	}
}

func TestProviderSecretRewritePlanHashBindsActiveKeyAndSourceState(t *testing.T) {
	_, cfg := providerSecretRewriteBYOKConfig(t)
	firstVault := providerSecretRewriteVault(t, "first", map[string]byte{"first": 24})
	secondVault := providerSecretRewriteVault(t, "second", map[string]byte{
		"second": 25,
		"first":  24,
	})
	replacedVault := providerSecretRewriteVault(t, "first", map[string]byte{
		"first": 28,
	})
	rows := []providerSecretRewriteRow{{
		id: "1", userID: "00000000-0000-0000-0000-000000000001",
		providerID: "EMPTY", configJSON: `{}`,
	}}
	first, err := NewPostgresProviderSecretRewriter(nil, cfg, firstVault).
		buildProviderSecretRewritePlan(append([]providerSecretRewriteRow(nil), rows...))
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewPostgresProviderSecretRewriter(nil, cfg, secondVault).
		buildProviderSecretRewritePlan(append([]providerSecretRewriteRow(nil), rows...))
	if err != nil {
		t.Fatal(err)
	}
	replaced, err := NewPostgresProviderSecretRewriter(nil, cfg, replacedVault).
		buildProviderSecretRewritePlan(append([]providerSecretRewriteRow(nil), rows...))
	if err != nil {
		t.Fatal(err)
	}
	changedRows := append([]providerSecretRewriteRow(nil), rows...)
	changedRows[0].configJSON = `{"models":["changed"]}`
	changed, err := NewPostgresProviderSecretRewriter(nil, cfg, firstVault).
		buildProviderSecretRewritePlan(changedRows)
	if err != nil {
		t.Fatal(err)
	}
	if first.result.PlanSHA256 == second.result.PlanSHA256 ||
		first.result.PlanSHA256 == replaced.result.PlanSHA256 ||
		first.result.PlanSHA256 == changed.result.PlanSHA256 {
		t.Fatal("plan hash did not bind active key and source state")
	}
}

func TestProviderSecretRewriteRequiresDatabaseAndVault(t *testing.T) {
	result, err := NewPostgresProviderSecretRewriter(nil, config.Config{}, nil).Rewrite(
		context.Background(), ProviderSecretRewriteOptions{},
	)
	if !errors.Is(err, ErrProviderSecretRewriteUnavailable) || result != (ProviderSecretRewriteResult{}) {
		t.Fatalf("Rewrite() = %#v, %v", result, err)
	}
}

func providerSecretRewriteBYOKConfig(t *testing.T) (*rsa.PrivateKey, config.Config) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemValue := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	}))
	return privateKey, config.Config{BYOK: config.BYOKConfig{
		PrivateKeyPEM: pemValue,
	}}
}

func providerSecretRewriteVault(
	t *testing.T,
	activeKID string,
	keys map[string]byte,
) *providersecrets.Vault {
	t.Helper()
	items := make([]providersecrets.KeyConfig, 0, len(keys))
	for kid, fill := range keys {
		items = append(items, providersecrets.KeyConfig{
			KID: kid,
			Key: base64.RawURLEncoding.EncodeToString(
				[]byte(strings.Repeat(string([]byte{fill}), 32)),
			),
		})
	}
	vault, err := providersecrets.NewVault(providersecrets.KeyringConfig{
		V: providersecrets.KeyringVersion, ActiveKID: activeKID, Keys: items,
	})
	if err != nil {
		t.Fatalf("NewVault() error = %v", err)
	}
	return vault
}

func providerSecretVaultRewriteRow(
	t *testing.T,
	vault *providersecrets.Vault,
	id string,
	userID string,
	providerID string,
	plaintext string,
	deleted bool,
) providerSecretRewriteRow {
	t.Helper()
	envelope, err := vault.Encrypt(
		[]byte(plaintext), modelProviderSecretContext(userID, providerID),
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return providerSecretRewriteRow{
		id: id, userID: userID, providerID: providerID,
		encryptedSecretRef: string(encoded),
		configJSON:         `{"type":"OpenAI Compatible","enabled":true}`,
		deleted:            deleted,
	}
}

func providerSecretLegacyRewriteRow(
	t *testing.T,
	privateKey *rsa.PrivateKey,
	id string,
	userID string,
	providerID string,
	plaintext string,
) providerSecretRewriteRow {
	t.Helper()
	envelope := encryptedSecretEnvelope(
		t, privateKey, plaintext, "provider:OpenAI Compatible",
	)
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	return providerSecretRewriteRow{
		id: id, userID: userID, providerID: providerID,
		encryptedSecretRef: string(encoded),
		configJSON:         `{"type":"OpenAI Compatible","enabled":true}`,
	}
}

func providerSecretSearchVaultRewriteRow(
	t *testing.T,
	vault *providersecrets.Vault,
	id string,
	userID string,
	providerID websearch.ProviderID,
	plaintext string,
) providerSecretRewriteRow {
	t.Helper()
	recordID := searchProviderRecordID(providerID)
	envelope, err := vault.Encrypt(
		[]byte(plaintext), searchProviderSecretContext(userID, recordID),
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	configJSON, err := json.Marshal(StoredProviderConfigPayload{
		Kind: providerConfigKindSearch, SearchProvider: string(providerID), Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return providerSecretRewriteRow{
		id: id, userID: userID, providerID: recordID,
		encryptedSecretRef: string(encoded), configJSON: string(configJSON),
	}
}

func providerSecretRAGVaultRewriteRow(
	t *testing.T,
	vault *providersecrets.Vault,
	id string,
	userID string,
	providerID RAGProviderID,
	plaintext string,
) providerSecretRewriteRow {
	t.Helper()
	recordID := ragProviderRecordID(providerID)
	envelope, err := vault.Encrypt(
		[]byte(plaintext), ragProviderSecretContext(userID, recordID),
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	configJSON, err := json.Marshal(StoredProviderConfigPayload{
		Kind: providerConfigKindRAG, RAGProvider: string(providerID), Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return providerSecretRewriteRow{
		id: id, userID: userID, providerID: recordID,
		encryptedSecretRef: string(encoded), configJSON: string(configJSON),
	}
}

func providerSecretVoiceVaultRewriteRow(
	t *testing.T,
	vault *providersecrets.Vault,
	id string,
	userID string,
	providerID voiceProviderID,
	plaintext string,
) providerSecretRewriteRow {
	t.Helper()
	recordID := voiceProviderRecordID(providerID)
	envelope, err := vault.Encrypt(
		[]byte(plaintext),
		voiceProviderSecretContext(userID, recordID),
	)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	configJSON, err := json.Marshal(StoredProviderConfigPayload{
		Kind: providerConfigKindVoice, VoiceProvider: string(providerID), Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return providerSecretRewriteRow{
		id: id, userID: userID, providerID: recordID,
		encryptedSecretRef: string(encoded), configJSON: string(configJSON),
	}
}
