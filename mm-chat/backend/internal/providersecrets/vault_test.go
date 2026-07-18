package providersecrets

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVaultEncryptDecryptBindsContextAndHidesPlaintext(t *testing.T) {
	vault := newTestVault(t, "active", testKey(1))
	plaintext := []byte("fixture-provider-secret")
	envelope, err := vault.Encrypt(plaintext, "provider:model:server-default")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if strings.Contains(string(encoded), string(plaintext)) {
		t.Fatal("persisted envelope contains plaintext")
	}
	if envelope.V != EnvelopeVersion || envelope.Alg != Algorithm || envelope.KID != "active" {
		t.Fatalf("envelope header = %#v", envelope)
	}

	decrypted, err := vault.Decrypt(envelope, "provider:model:server-default")
	if err != nil {
		t.Fatalf("Decrypt() error = %v", err)
	}
	if string(decrypted) != string(plaintext) {
		t.Fatalf("plaintext = %q", decrypted)
	}
	if _, err := vault.Decrypt(envelope, "provider:search:tavily"); !errors.Is(err, ErrContextMismatch) {
		t.Fatalf("context mismatch error = %v", err)
	}
}

func TestVaultRejectsTamperedAndUnknownKeyEnvelopes(t *testing.T) {
	vault := newTestVault(t, "active", testKey(2))
	envelope, err := vault.Encrypt([]byte("fixture"), "provider:search:tavily")
	if err != nil {
		t.Fatalf("Encrypt() error = %v", err)
	}

	tampered := envelope
	decoded, err := base64.RawURLEncoding.DecodeString(tampered.Ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	decoded[len(decoded)-1] ^= 1
	tampered.Ciphertext = base64.RawURLEncoding.EncodeToString(decoded)
	if _, err := vault.Decrypt(tampered, tampered.Context); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("tampered error = %v", err)
	}
	unknown := envelope
	unknown.KID = "unknown"
	if _, err := vault.Decrypt(unknown, unknown.Context); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("unknown kid error = %v", err)
	}
}

func TestParseEnvelopeIsClosedAndBounded(t *testing.T) {
	vault := newTestVault(t, "active", testKey(9))
	envelope, err := vault.Encrypt([]byte("fixture"), "provider:model:user:provider")
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseEnvelope(string(encoded))
	if err != nil || parsed != envelope {
		t.Fatalf("ParseEnvelope() = %#v, %v", parsed, err)
	}

	for name, value := range map[string]string{
		"empty":    "",
		"unknown":  strings.TrimSuffix(string(encoded), "}") + `,"extra":true}`,
		"trailing": string(encoded) + `{}`,
		"oversize": strings.Repeat("x", maxEnvelopeBytes+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseEnvelope(value); !errors.Is(err, ErrInvalidEnvelope) {
				t.Fatalf("ParseEnvelope() error = %v", err)
			}
		})
	}
}

func TestVaultRejectsInvalidPlaintextAndContext(t *testing.T) {
	vault := newTestVault(t, "active", testKey(8))
	if _, err := vault.Encrypt(nil, "provider:model:default"); !errors.Is(err, ErrInvalidPlaintext) {
		t.Fatalf("empty plaintext error = %v", err)
	}
	if _, err := vault.Encrypt([]byte("secret"), " provider:model:default"); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("untrimmed context error = %v", err)
	}
	if _, err := vault.Encrypt([]byte("secret"), "provider:\x00model"); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("NUL context error = %v", err)
	}
}

func TestVaultRotatesFromRetainedPreviousKey(t *testing.T) {
	oldVault := newTestVault(t, "old", testKey(3))
	oldEnvelope, err := oldVault.Encrypt([]byte("rotate-me"), "provider:rag:jina")
	if err != nil {
		t.Fatalf("old Encrypt() error = %v", err)
	}
	rotatingVault, err := NewVault(KeyringConfig{
		V:         KeyringVersion,
		ActiveKID: "new",
		Keys: []KeyConfig{
			{KID: "new", Key: testKey(4)},
			{KID: "old", Key: testKey(3)},
		},
	})
	if err != nil {
		t.Fatalf("NewVault() error = %v", err)
	}

	rotated, changed, err := rotatingVault.Rotate(oldEnvelope, oldEnvelope.Context)
	if err != nil {
		t.Fatalf("Rotate() error = %v", err)
	}
	if !changed || rotated.KID != "new" {
		t.Fatalf("rotation = changed:%v envelope:%#v", changed, rotated)
	}
	plaintext, err := rotatingVault.Decrypt(rotated, rotated.Context)
	if err != nil || string(plaintext) != "rotate-me" {
		t.Fatalf("rotated plaintext = %q, err = %v", plaintext, err)
	}
	if _, err := oldVault.Decrypt(rotated, rotated.Context); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("old keyring decrypt error = %v", err)
	}

	unchanged, changed, err := rotatingVault.Rotate(rotated, rotated.Context)
	if err != nil || changed || unchanged != rotated {
		t.Fatalf("current rotation = changed:%v envelope:%#v err:%v", changed, unchanged, err)
	}
}

func TestActiveKeyBindingIsStableAndBindsKeyMaterial(t *testing.T) {
	first := newTestVault(t, "same-kid", testKey(10))
	same := newTestVault(t, "same-kid", testKey(10))
	different := newTestVault(t, "same-kid", testKey(11))
	firstBinding, err := first.ActiveKeyBinding("provider:model:rewrite:v1")
	if err != nil {
		t.Fatal(err)
	}
	sameBinding, err := same.ActiveKeyBinding("provider:model:rewrite:v1")
	if err != nil {
		t.Fatal(err)
	}
	differentBinding, err := different.ActiveKeyBinding("provider:model:rewrite:v1")
	if err != nil {
		t.Fatal(err)
	}
	if firstBinding == "" || firstBinding != sameBinding ||
		firstBinding == differentBinding {
		t.Fatal("active key binding is not stable or did not bind key material")
	}
	if _, err := first.ActiveKeyBinding(" bad"); !errors.Is(err, ErrInvalidContext) {
		t.Fatalf("invalid purpose error = %v", err)
	}
}

func TestLoadVaultFileIsStrictAndBounded(t *testing.T) {
	dir := t.TempDir()
	validPath := filepath.Join(dir, "keyring.json")
	writeKeyringFile(t, validPath, map[string]any{
		"v":         KeyringVersion,
		"activeKid": "active",
		"keys": []map[string]any{{
			"kid": "active",
			"key": testKey(5),
		}},
	})
	vault, err := LoadVaultFile(validPath)
	if err != nil || vault.ActiveKID() != "active" {
		t.Fatalf("LoadVaultFile() = %#v, %v", vault, err)
	}

	unknownFieldPath := filepath.Join(dir, "unknown.json")
	writeKeyringFile(t, unknownFieldPath, map[string]any{
		"v": KeyringVersion, "activeKid": "active",
		"keys":  []map[string]any{{"kid": "active", "key": testKey(5)}},
		"extra": true,
	})
	if _, err := LoadVaultFile(unknownFieldPath); !errors.Is(err, ErrInvalidKeyring) {
		t.Fatalf("unknown field error = %v", err)
	}

	oversizedPath := filepath.Join(dir, "oversized.json")
	if err := os.WriteFile(oversizedPath, []byte(strings.Repeat("x", maxKeyringBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadVaultFile(oversizedPath); !errors.Is(err, ErrInvalidKeyring) {
		t.Fatalf("oversized error = %v", err)
	}
	if _, err := LoadVaultFile(filepath.Join(dir, "missing")); !errors.Is(err, ErrKeyringUnavailable) {
		t.Fatalf("missing error = %v", err)
	}
}

func TestNewVaultRejectsInvalidKeyrings(t *testing.T) {
	tests := []KeyringConfig{
		{},
		{V: KeyringVersion, ActiveKID: "missing", Keys: []KeyConfig{{KID: "other", Key: testKey(6)}}},
		{
			V: KeyringVersion, ActiveKID: "same",
			Keys: []KeyConfig{{KID: "same", Key: testKey(6)}, {KID: "same", Key: testKey(7)}},
		},
		{V: KeyringVersion, ActiveKID: "bad key", Keys: []KeyConfig{{KID: "bad key", Key: testKey(6)}}},
		{V: KeyringVersion, ActiveKID: "short", Keys: []KeyConfig{{KID: "short", Key: "c2hvcnQ"}}},
	}
	for index, config := range tests {
		if _, err := NewVault(config); !errors.Is(err, ErrInvalidKeyring) {
			t.Fatalf("case %d error = %v", index, err)
		}
	}
}

func newTestVault(t *testing.T, kid string, key string) *Vault {
	t.Helper()
	vault, err := NewVault(KeyringConfig{
		V:         KeyringVersion,
		ActiveKID: kid,
		Keys:      []KeyConfig{{KID: kid, Key: key}},
	})
	if err != nil {
		t.Fatalf("NewVault() error = %v", err)
	}
	return vault
}

func testKey(fill byte) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat(string([]byte{fill}), keyBytes)))
}

func writeKeyringFile(t *testing.T, path string, value any) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}
