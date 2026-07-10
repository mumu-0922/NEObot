package teams

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"

	"neo-chat/mm-chat/backend/internal/auth"
)

const (
	inviteTokenBytes   = 32
	mailPayloadVersion = 1
	aes256KeyBytes     = 32
	gcmNonceBytes      = 12
)

type MailKeyring struct {
	ActiveKeyID string
	Keys        map[string][]byte
}

type EncryptedMailPayload struct {
	KeyID      string
	Version    int
	Nonce      []byte
	Ciphertext []byte
}

type MailCipher struct {
	activeKeyID string
	keys        map[string][]byte
}

func GenerateInviteToken() (string, error) {
	var token [inviteTokenBytes]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", fmt.Errorf("generate invite token: %w", err)
	}
	return hex.EncodeToString(token[:]), nil
}

func HashInviteToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func NormalizeInviteToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if len(token) != inviteTokenBytes*2 || strings.ToLower(token) != token {
		return "", errors.New("invite token must be 32-byte lowercase hex")
	}
	decoded, err := hex.DecodeString(token)
	if err != nil || len(decoded) != inviteTokenBytes {
		return "", errors.New("invite token must be 32-byte lowercase hex")
	}
	return token, nil
}

func NewMailCipher(ring MailKeyring) (*MailCipher, error) {
	activeKeyID := strings.TrimSpace(ring.ActiveKeyID)
	if activeKeyID == "" {
		return nil, errors.New("mail active key id is required")
	}
	if len(ring.Keys) == 0 {
		return nil, errors.New("mail keyring is required")
	}

	keys := make(map[string][]byte, len(ring.Keys))
	for keyID, rawKey := range ring.Keys {
		keyID = strings.TrimSpace(keyID)
		if keyID == "" {
			return nil, errors.New("mail key id is required")
		}
		if len(rawKey) != aes256KeyBytes {
			return nil, fmt.Errorf("mail key %q must be 32 bytes", keyID)
		}
		if _, exists := keys[keyID]; exists {
			return nil, fmt.Errorf("mail key id %q is duplicated after normalization", keyID)
		}
		keyCopy := make([]byte, len(rawKey))
		copy(keyCopy, rawKey)
		keys[keyID] = keyCopy
	}
	if _, ok := keys[activeKeyID]; !ok {
		return nil, fmt.Errorf("mail active key %q is not configured", activeKeyID)
	}

	return &MailCipher{activeKeyID: activeKeyID, keys: keys}, nil
}

func (c *MailCipher) EncryptInvitePayload(
	outboxID string,
	inviteID string,
	teamID string,
	payload InviteMailPayload,
) (EncryptedMailPayload, error) {
	if c == nil {
		return EncryptedMailPayload{}, ErrInviteDeliveryUnavailable
	}

	outboxID = strings.TrimSpace(outboxID)
	inviteID = strings.TrimSpace(inviteID)
	teamID = strings.TrimSpace(teamID)
	if !isUUID(outboxID) || !isUUID(inviteID) || !isUUID(teamID) {
		return EncryptedMailPayload{}, errors.New("mail payload identifiers must be UUIDs")
	}

	normalized, err := normalizeInviteMailPayload(payload, teamID)
	if err != nil {
		return EncryptedMailPayload{}, err
	}
	plaintext, err := json.Marshal(normalized)
	if err != nil {
		return EncryptedMailPayload{}, fmt.Errorf("marshal invite mail payload: %w", err)
	}

	block, err := aes.NewCipher(c.keys[c.activeKeyID])
	if err != nil {
		return EncryptedMailPayload{}, fmt.Errorf("initialize mail cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedMailPayload{}, fmt.Errorf("initialize mail gcm: %w", err)
	}

	nonce := make([]byte, gcmNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return EncryptedMailPayload{}, fmt.Errorf("generate mail nonce: %w", err)
	}

	aad := inviteMailAAD(mailPayloadVersion, outboxID, inviteID, teamID)
	ciphertext := gcm.Seal(nil, nonce, plaintext, aad)

	return EncryptedMailPayload{
		KeyID:      c.activeKeyID,
		Version:    mailPayloadVersion,
		Nonce:      append([]byte(nil), nonce...),
		Ciphertext: append([]byte(nil), ciphertext...),
	}, nil
}

func (c *MailCipher) DecryptInvitePayload(
	outboxID string,
	inviteID string,
	teamID string,
	encrypted EncryptedMailPayload,
) (InviteMailPayload, error) {
	var payload InviteMailPayload
	if c == nil {
		return payload, ErrInviteDeliveryUnavailable
	}

	outboxID = strings.TrimSpace(outboxID)
	inviteID = strings.TrimSpace(inviteID)
	teamID = strings.TrimSpace(teamID)
	if !isUUID(outboxID) || !isUUID(inviteID) || !isUUID(teamID) {
		return payload, errors.New("mail payload identifiers must be UUIDs")
	}
	if encrypted.Version != mailPayloadVersion {
		return payload, fmt.Errorf("unsupported mail payload version %d", encrypted.Version)
	}
	if len(encrypted.Nonce) != gcmNonceBytes {
		return payload, errors.New("mail nonce must be 12 bytes")
	}
	keyID := strings.TrimSpace(encrypted.KeyID)
	key, ok := c.keys[keyID]
	if !ok {
		return payload, fmt.Errorf("mail key %q is not configured", keyID)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return payload, fmt.Errorf("initialize mail cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return payload, fmt.Errorf("initialize mail gcm: %w", err)
	}

	aad := inviteMailAAD(mailPayloadVersion, outboxID, inviteID, teamID)
	plaintext, err := gcm.Open(nil, encrypted.Nonce, encrypted.Ciphertext, aad)
	if err != nil {
		return payload, errors.New("decrypt invite mail payload: authentication failed")
	}
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return payload, fmt.Errorf("unmarshal invite mail payload: %w", err)
	}
	return normalizeInviteMailPayload(payload, teamID)
}

func MaskEmail(email string) (string, error) {
	canonical, err := canonicalizeEmail(email)
	if err != nil {
		return "", err
	}
	local, domain, ok := strings.Cut(canonical, "@")
	if !ok {
		return "", errors.New("email is invalid")
	}

	domainName := domain
	domainSuffix := ""
	if lastDot := strings.LastIndex(domain, "."); lastDot > 0 && lastDot < len(domain)-1 {
		domainName = domain[:lastDot]
		domainSuffix = domain[lastDot:]
	}

	return maskSegment(local) + "@" + maskSegment(domainName) + domainSuffix, nil
}

func normalizeInviteMailPayload(payload InviteMailPayload, teamID string) (InviteMailPayload, error) {
	var err error
	payload.Version = mailPayloadVersion
	payload.Email, err = canonicalizeEmail(payload.Email)
	if err != nil {
		return InviteMailPayload{}, err
	}
	payload.InviteToken, err = NormalizeInviteToken(payload.InviteToken)
	if err != nil {
		return InviteMailPayload{}, err
	}
	payload.AcceptanceURL, err = normalizeInviteAcceptanceURL(
		payload.AcceptanceURL,
		payload.InviteToken,
	)
	if err != nil {
		return InviteMailPayload{}, err
	}
	payload.TeamID = strings.TrimSpace(payload.TeamID)
	if payload.TeamID == "" {
		payload.TeamID = teamID
	}
	if payload.TeamID != teamID {
		return InviteMailPayload{}, errors.New("mail payload team id does not match")
	}
	payload.InvitedByUserID = strings.TrimSpace(payload.InvitedByUserID)
	if payload.InvitedByUserID != "" && !isUUID(payload.InvitedByUserID) {
		return InviteMailPayload{}, errors.New("mail payload inviter user id must be a UUID")
	}
	role, err := normalizeTeamRoleValue(payload.TeamRole)
	if err != nil {
		return InviteMailPayload{}, err
	}
	payload.TeamRole = role
	payload.InvitedByDisplayName = strings.TrimSpace(payload.InvitedByDisplayName)
	payload.ExpiresAt = payload.ExpiresAt.UTC()
	if payload.ExpiresAt.IsZero() {
		return InviteMailPayload{}, errors.New("mail payload expiration is required")
	}
	return payload, nil
}

func normalizeInviteAcceptanceURL(value string, token string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("acceptance URL is required")
	}
	parsed, err := url.Parse(value)
	scheme := ""
	if parsed != nil {
		scheme = strings.ToLower(parsed.Scheme)
	}
	if err != nil || parsed == nil || !parsed.IsAbs() || parsed.Host == "" ||
		(scheme != "https" && scheme != "http") || parsed.User != nil {
		return "", errors.New("acceptance URL is invalid")
	}
	parsed.Scheme = scheme
	if strings.Contains(parsed.Path, token) ||
		strings.Contains(parsed.RawPath, token) ||
		strings.Contains(parsed.RawQuery, token) {
		return "", errors.New("acceptance URL must not place the invite token in the path or query")
	}
	for _, values := range parsed.Query() {
		for _, queryValue := range values {
			if strings.Contains(queryValue, token) {
				return "", errors.New("acceptance URL must not place the invite token in the path or query")
			}
		}
	}
	fragment, err := url.ParseQuery(parsed.Fragment)
	if err != nil || len(fragment) != 1 || len(fragment["token"]) != 1 ||
		fragment.Get("token") != token {
		return "", errors.New("acceptance URL must carry the invite token in the URL fragment")
	}
	return parsed.String(), nil
}

func inviteMailAAD(version int, outboxID string, inviteID string, teamID string) []byte {
	return []byte(fmt.Sprintf("v=%d\x00outbox=%s\x00invite=%s\x00team=%s", version, outboxID, inviteID, teamID))
}

func canonicalizeEmail(value string) (string, error) {
	canonical, err := auth.CanonicalizeEmail(value)
	if err != nil {
		return "", invalidInvitePayload("email is invalid")
	}
	return canonical, nil
}

func maskSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "***"
	}
	first, _ := utf8.DecodeRuneInString(value)
	if first == utf8.RuneError {
		return "***"
	}
	return string(first) + "***"
}

func containsControlOrFormatRune(value string) bool {
	for _, r := range value {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return true
		}
	}
	return false
}
