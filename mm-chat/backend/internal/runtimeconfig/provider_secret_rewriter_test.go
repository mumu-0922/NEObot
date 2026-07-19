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

	"neo-chat/mm-chat/backend/internal/config"
	"neo-chat/mm-chat/backend/internal/providersecrets"
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
