package teams

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const (
	cursorVersion      = 1
	maximumCursorBytes = 1024

	cursorResourceTeams       = "teams"
	cursorResourceTeamMembers = "team_members"
	cursorResourceTeamInvites = "team_invites"

	cursorSortCreatedDesc = "created_at:desc,id:desc"
	cursorSortMemberAsc   = "created_at:asc,user_id:asc"
)

var cursorEncoding = base64.RawURLEncoding.Strict()

type CursorKeyring struct {
	ActiveKeyID string
	Keys        map[string][]byte
}

type CursorCodec struct {
	activeKeyID string
	keys        map[string][]byte
}

type CursorContext struct {
	Resource     string
	UserID       string
	TeamID       string
	FilterDigest string
	Sort         string
}

type Cursor struct {
	Version      int      `json:"v"`
	KeyID        string   `json:"k"`
	Resource     string   `json:"r"`
	UserID       string   `json:"u"`
	TeamID       string   `json:"t,omitempty"`
	FilterDigest string   `json:"f,omitempty"`
	Sort         string   `json:"s"`
	Values       []string `json:"p"`
}

func NewCursorCodec(ring CursorKeyring) (*CursorCodec, error) {
	activeKeyID := strings.TrimSpace(ring.ActiveKeyID)
	if activeKeyID == "" {
		return nil, errors.New("cursor active key id is required")
	}
	if len(ring.Keys) == 0 {
		return nil, errors.New("cursor keyring is required")
	}

	keys := make(map[string][]byte, len(ring.Keys))
	for keyID, rawKey := range ring.Keys {
		keyID = strings.TrimSpace(keyID)
		if keyID == "" {
			return nil, errors.New("cursor key id is required")
		}
		if len(rawKey) < sha256.Size {
			return nil, fmt.Errorf("cursor key %q must be at least 32 bytes", keyID)
		}
		if _, exists := keys[keyID]; exists {
			return nil, fmt.Errorf("cursor key id %q is duplicated after normalization", keyID)
		}
		keyCopy := make([]byte, len(rawKey))
		copy(keyCopy, rawKey)
		keys[keyID] = keyCopy
	}
	if _, ok := keys[activeKeyID]; !ok {
		return nil, fmt.Errorf("cursor active key %q is not configured", activeKeyID)
	}

	return &CursorCodec{activeKeyID: activeKeyID, keys: keys}, nil
}

func CursorFilterDigest(filter string) string {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(filter))
	return hex.EncodeToString(sum[:])
}

func (c *CursorCodec) Encode(cursor Cursor) (string, error) {
	if c == nil {
		return "", ErrCursorCodecRequired
	}
	cursor = normalizeCursor(cursor)
	if cursor.Version == 0 {
		cursor.Version = cursorVersion
	}
	if cursor.Version != cursorVersion {
		return "", fmt.Errorf("unsupported cursor version %d", cursor.Version)
	}
	if cursor.KeyID == "" {
		cursor.KeyID = c.activeKeyID
	}
	if cursor.KeyID != c.activeKeyID {
		return "", fmt.Errorf("cursor key %q is not the active signing key", cursor.KeyID)
	}
	if _, ok := c.keys[cursor.KeyID]; !ok {
		return "", fmt.Errorf("cursor key %q is not configured", cursor.KeyID)
	}
	if cursor.Resource == "" {
		return "", errors.New("cursor resource is required")
	}
	if !isUUID(cursor.UserID) {
		return "", errors.New("cursor user id must be a UUID")
	}
	if cursor.TeamID != "" && !isUUID(cursor.TeamID) {
		return "", errors.New("cursor team id must be a UUID")
	}
	if strings.TrimSpace(cursor.Sort) == "" {
		return "", errors.New("cursor sort is required")
	}
	if len(cursor.Values) == 0 {
		return "", errors.New("cursor sort values are required")
	}

	payload, err := json.Marshal(cursor)
	if err != nil {
		return "", fmt.Errorf("marshal cursor: %w", err)
	}
	signature, err := c.sign(cursor.KeyID, payload)
	if err != nil {
		return "", err
	}

	encoded := cursorEncoding.EncodeToString(payload) + "." + cursorEncoding.EncodeToString(signature)
	if len(encoded) > maximumCursorBytes {
		return "", fmt.Errorf("cursor exceeds %d bytes", maximumCursorBytes)
	}
	return encoded, nil
}

func (c *CursorCodec) Decode(encoded string, expect CursorContext) (Cursor, error) {
	var cursor Cursor
	if c == nil {
		return cursor, ErrCursorCodecRequired
	}

	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return cursor, errors.New("cursor is required")
	}
	if len(encoded) > maximumCursorBytes {
		return cursor, fmt.Errorf("cursor exceeds %d bytes", maximumCursorBytes)
	}
	left, right, ok := strings.Cut(encoded, ".")
	if !ok || left == "" || right == "" || strings.Contains(right, ".") {
		return cursor, errors.New("cursor format is invalid")
	}

	payload, err := cursorEncoding.DecodeString(left)
	if err != nil {
		return cursor, errors.New("cursor payload is invalid")
	}
	signature, err := cursorEncoding.DecodeString(right)
	if err != nil {
		return cursor, errors.New("cursor signature is invalid")
	}
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return cursor, errors.New("cursor payload is invalid")
	}
	cursor = normalizeCursor(cursor)
	if cursor.Version != cursorVersion {
		return cursor, fmt.Errorf("unsupported cursor version %d", cursor.Version)
	}
	key, ok := c.keys[cursor.KeyID]
	if !ok {
		return cursor, fmt.Errorf("cursor key %q is not configured", cursor.KeyID)
	}

	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	expectedSignature := mac.Sum(nil)
	if subtle.ConstantTimeCompare(signature, expectedSignature) != 1 {
		return cursor, errors.New("cursor signature does not match")
	}
	if err := validateCursorContext(cursor, expect); err != nil {
		return cursor, err
	}
	return cursor, nil
}

func (c *CursorCodec) sign(keyID string, payload []byte) ([]byte, error) {
	key, ok := c.keys[keyID]
	if !ok {
		return nil, fmt.Errorf("cursor key %q is not configured", keyID)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write(payload)
	return mac.Sum(nil), nil
}

func normalizeCursor(cursor Cursor) Cursor {
	cursor.KeyID = strings.TrimSpace(cursor.KeyID)
	cursor.Resource = strings.TrimSpace(cursor.Resource)
	cursor.UserID = strings.TrimSpace(cursor.UserID)
	cursor.TeamID = strings.TrimSpace(cursor.TeamID)
	cursor.FilterDigest = strings.TrimSpace(cursor.FilterDigest)
	cursor.Sort = strings.TrimSpace(cursor.Sort)
	if len(cursor.Values) == 0 {
		return cursor
	}
	values := make([]string, len(cursor.Values))
	for i, value := range cursor.Values {
		values[i] = strings.TrimSpace(value)
	}
	cursor.Values = values
	return cursor
}

func validateCursorContext(cursor Cursor, expect CursorContext) error {
	expect.Resource = strings.TrimSpace(expect.Resource)
	expect.UserID = strings.TrimSpace(expect.UserID)
	expect.TeamID = strings.TrimSpace(expect.TeamID)
	expect.FilterDigest = strings.TrimSpace(expect.FilterDigest)
	expect.Sort = strings.TrimSpace(expect.Sort)

	if cursor.Resource != expect.Resource {
		return errors.New("cursor resource does not match request")
	}
	if cursor.UserID != expect.UserID {
		return errors.New("cursor user does not match request")
	}
	if cursor.TeamID != expect.TeamID {
		return errors.New("cursor team does not match request")
	}
	if cursor.FilterDigest != expect.FilterDigest {
		return errors.New("cursor filter does not match request")
	}
	if cursor.Sort != expect.Sort {
		return errors.New("cursor sort does not match request")
	}
	return nil
}
