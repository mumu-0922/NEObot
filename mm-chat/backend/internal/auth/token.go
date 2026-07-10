package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const sessionTokenBytes = 32

func GenerateSessionToken() (string, error) {
	var token [sessionTokenBytes]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return hex.EncodeToString(token[:]), nil
}

func HashSessionToken(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func normalizeOneTimeToken(token string) (string, error) {
	token = strings.TrimSpace(token)
	if len(token) != sessionTokenBytes*2 || strings.ToLower(token) != token {
		return "", errors.New("token must be 32-byte lowercase hex")
	}
	decoded, err := hex.DecodeString(token)
	if err != nil || len(decoded) != sessionTokenBytes {
		return "", errors.New("token must be 32-byte lowercase hex")
	}
	return token, nil
}
