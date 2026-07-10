package teams

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestGenerateInviteTokenAndHash(t *testing.T) {
	token, err := GenerateInviteToken()
	if err != nil {
		t.Fatalf("GenerateInviteToken() error = %v", err)
	}
	if len(token) != inviteTokenBytes*2 || strings.ToLower(token) != token {
		t.Fatalf("GenerateInviteToken() = %q", token)
	}
	if _, err := NormalizeInviteToken(token); err != nil {
		t.Fatalf("NormalizeInviteToken() error = %v", err)
	}

	hash := HashInviteToken(token)
	if len(hash) != 64 || hash == token {
		t.Fatalf("HashInviteToken() = %q", hash)
	}
}

func TestMailCipherRoundTripAADAndRotation(t *testing.T) {
	oldCipher, err := NewMailCipher(MailKeyring{
		ActiveKeyID: "old",
		Keys: map[string][]byte{
			"old": repeatedKey('m'),
		},
	})
	if err != nil {
		t.Fatalf("NewMailCipher(old) error = %v", err)
	}

	payload := InviteMailPayload{
		Email:                "Owner@Example.Test",
		InviteToken:          testInviteToken('a'),
		AcceptanceURL:        "https://example.test/accept#token=" + testInviteToken('a'),
		TeamID:               "11111111-1111-4111-8111-111111111111",
		InvitedByUserID:      "22222222-2222-4222-8222-222222222222",
		InvitedByDisplayName: "Owner",
		TeamRole:             TeamRoleAdmin,
		ExpiresAt:            time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
	}

	encrypted, err := oldCipher.EncryptInvitePayload(
		"33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444",
		"11111111-1111-4111-8111-111111111111",
		payload,
	)
	if err != nil {
		t.Fatalf("EncryptInvitePayload() error = %v", err)
	}
	if encrypted.KeyID != "old" || len(encrypted.Nonce) != gcmNonceBytes || len(encrypted.Ciphertext) == 0 {
		t.Fatalf("encrypted payload = %#v", encrypted)
	}

	rotatedCipher, err := NewMailCipher(MailKeyring{
		ActiveKeyID: "new",
		Keys: map[string][]byte{
			"new": repeatedKey('n'),
			"old": repeatedKey('m'),
		},
	})
	if err != nil {
		t.Fatalf("NewMailCipher(rotated) error = %v", err)
	}

	decrypted, err := rotatedCipher.DecryptInvitePayload(
		"33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444",
		"11111111-1111-4111-8111-111111111111",
		encrypted,
	)
	if err != nil {
		t.Fatalf("DecryptInvitePayload() error = %v", err)
	}
	if decrypted.Email != "owner@example.test" || decrypted.InviteToken != payload.InviteToken {
		t.Fatalf("decrypted payload = %#v", decrypted)
	}

	encryptedAgain, err := rotatedCipher.EncryptInvitePayload(
		"33333333-3333-4333-8333-333333333334",
		"44444444-4444-4444-8444-444444444445",
		"11111111-1111-4111-8111-111111111111",
		payload,
	)
	if err != nil {
		t.Fatalf("EncryptInvitePayload(second) error = %v", err)
	}
	if encryptedAgain.KeyID != "new" {
		t.Fatalf("EncryptInvitePayload(second) key = %q, want new", encryptedAgain.KeyID)
	}
	if string(encryptedAgain.Nonce) == string(encrypted.Nonce) {
		t.Fatal("EncryptInvitePayload() reused nonce")
	}

	if _, err := rotatedCipher.DecryptInvitePayload(
		"33333333-3333-4333-8333-333333333333",
		"44444444-4444-4444-8444-444444444444",
		"99999999-9999-4999-8999-999999999999",
		encrypted,
	); err == nil {
		t.Fatal("DecryptInvitePayload() wrong team error = nil, want error")
	}
}

func TestInviteAcceptanceURLKeepsRawTokenInFragmentOnly(t *testing.T) {
	token := testInviteToken('b')
	good := "https://example.test/invites/accept?source=email#token=" + token
	if got, err := normalizeInviteAcceptanceURL(good, token); err != nil || got != good {
		t.Fatalf("normalizeInviteAcceptanceURL(good) = (%q, %v)", got, err)
	}

	for _, unsafe := range []string{
		"https://example.test/invites/" + token + "/accept#token=" + token,
		"https://example.test/invites/accept?value=" + token + "#token=" + token,
		"https://example.test/invites/accept?token=" + token,
		"https://example.test/invites/accept#other=" + token,
	} {
		_, err := normalizeInviteAcceptanceURL(unsafe, token)
		if err == nil {
			t.Fatalf("normalizeInviteAcceptanceURL(%q) error = nil", unsafe)
		}
		if strings.Contains(err.Error(), token) {
			t.Fatalf("acceptance URL error leaked raw token: %v", err)
		}
	}
}

func TestMaskEmailAndCanonicalValidation(t *testing.T) {
	masked, err := MaskEmail(" Owner@Example.Test ")
	if err != nil {
		t.Fatalf("MaskEmail() error = %v", err)
	}
	if masked != "o***@e***.test" {
		t.Fatalf("MaskEmail() = %q, want %q", masked, "o***@e***.test")
	}

	_, err = MaskEmail("not-an-email")
	var validationErr ValidationError
	if !errors.As(err, &validationErr) || validationErr.Code != ErrorCodeInvalidInvitePayload {
		t.Fatalf("MaskEmail(invalid) error = %v", err)
	}
}

func TestMailKeyringRejectsNormalizedDuplicateIDs(t *testing.T) {
	_, err := NewMailCipher(MailKeyring{
		ActiveKeyID: "active",
		Keys: map[string][]byte{
			"active":   repeatedKey('a'),
			" active ": repeatedKey('b'),
		},
	})
	if err == nil {
		t.Fatal("NewMailCipher() accepted duplicate normalized key ids")
	}
}
