package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	maximumEmailBytes       = 254
	minimumPasswordRunes    = 15
	maximumPasswordBytes    = 256
	argon2Memory            = 64 * 1024
	argon2Time              = 3
	argon2Parallelism       = 2
	argon2SaltLength        = 16
	argon2HashLength        = 32
	maximumPasswordPHCBytes = 128
	maximumPasswordHashes   = 2
)

var passwordHashSemaphore = make(chan struct{}, maximumPasswordHashes)

var strictRawBase64 = base64.RawStdEncoding.Strict()

// CanonicalizeEmail is the shared identity boundary for every mailbox-backed
// auth and Team flow. Callers should map the returned validation error to their
// endpoint-specific public error code rather than reimplementing these rules.
func CanonicalizeEmail(value string) (string, error) {
	return canonicalizeEmail(value)
}

func canonicalizeEmail(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", invalidIdentityInput("email must be valid UTF-8")
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return "", invalidIdentityInput("email must not contain control or format characters")
		}
	}

	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", invalidIdentityInput("email is required")
	}
	if len(trimmed) > maximumEmailBytes {
		return "", invalidIdentityInput("email must not exceed 254 bytes")
	}
	canonical := strings.ToLower(trimmed)
	if len(canonical) > maximumEmailBytes {
		return "", invalidIdentityInput("email must not exceed 254 bytes")
	}
	if hasMailboxEnvelopeSyntax(canonical) {
		return "", invalidIdentityInput("email must be a mailbox without a display name")
	}

	address, err := mail.ParseAddress(canonical)
	if err != nil || address.Address == "" {
		return "", invalidIdentityInput("email is invalid")
	}
	if address.Name != "" {
		return "", invalidIdentityInput("email must not contain a display name")
	}

	return canonical, nil
}

// hasMailboxEnvelopeSyntax rejects address lists, groups, comments, and
// name-addr wrappers without rejecting punctuation inside a quoted local part.
func hasMailboxEnvelopeSyntax(value string) bool {
	inQuote := false
	escaped := false
	for _, r := range value {
		if inQuote {
			if escaped {
				escaped = false
				continue
			}
			if r == '\\' {
				escaped = true
				continue
			}
			if r == '"' {
				inQuote = false
			}
			continue
		}

		if r == '"' {
			inQuote = true
			continue
		}
		switch r {
		case '<', '>', '(', ')', ',', ':', ';':
			return true
		}
	}
	return false
}

func validatePassword(password string) error {
	if len(password) > maximumPasswordBytes {
		return invalidIdentityInput("password must not exceed 256 bytes")
	}
	if !utf8.ValidString(password) {
		return invalidIdentityInput("password must be valid UTF-8")
	}
	if utf8.RuneCountInString(password) < minimumPasswordRunes {
		return invalidIdentityInput("password must contain at least 15 characters")
	}
	return nil
}

func hashPassword(ctx context.Context, password string) (string, error) {
	if err := validatePassword(password); err != nil {
		return "", err
	}
	ctx = nonNilContext(ctx)
	release, err := acquirePasswordHashSlot(ctx)
	if err != nil {
		return "", err
	}
	defer release()

	var salt [argon2SaltLength]byte
	if _, err := rand.Read(salt[:]); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}

	hash := argon2.IDKey(
		[]byte(password),
		salt[:],
		argon2Time,
		argon2Memory,
		argon2Parallelism,
		argon2HashLength,
	)
	encodedHash := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		argon2Memory,
		argon2Time,
		argon2Parallelism,
		strictRawBase64.EncodeToString(salt[:]),
		strictRawBase64.EncodeToString(hash),
	)
	clear(hash)
	return encodedHash, nil
}

func verifyPassword(ctx context.Context, password string, encodedHash string) (bool, error) {
	if err := validatePassword(password); err != nil {
		return false, err
	}
	parsed, err := parsePasswordPHC(encodedHash)
	if err != nil {
		return false, err
	}

	ctx = nonNilContext(ctx)
	release, err := acquirePasswordHashSlot(ctx)
	if err != nil {
		return false, err
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return false, err
	}

	actual := argon2.IDKey(
		[]byte(password),
		parsed.salt[:],
		argon2Time,
		argon2Memory,
		argon2Parallelism,
		argon2HashLength,
	)
	verified := subtle.ConstantTimeCompare(actual, parsed.hash[:]) == 1
	clear(actual)
	return verified, nil
}

func invalidIdentityInput(message string) error {
	return fmt.Errorf("%w: %s", ErrInvalidIdentityInput, message)
}

type parsedPasswordPHC struct {
	salt [argon2SaltLength]byte
	hash [argon2HashLength]byte
}

func parsePasswordPHC(encodedHash string) (parsedPasswordPHC, error) {
	var parsed parsedPasswordPHC
	if len(encodedHash) == 0 || len(encodedHash) > maximumPasswordPHCBytes {
		return parsed, errors.New("password hash has an invalid length")
	}

	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return parsed, errors.New("password hash has an invalid PHC format")
	}
	if parts[2] != "v="+strconv.Itoa(argon2.Version) {
		return parsed, errors.New("password hash uses an unsupported Argon2 version")
	}
	if err := validateArgon2Parameters(parts[3]); err != nil {
		return parsed, err
	}

	if err := decodePHCField(parts[4], parsed.salt[:]); err != nil {
		return parsed, fmt.Errorf("password hash salt: %w", err)
	}
	if err := decodePHCField(parts[5], parsed.hash[:]); err != nil {
		return parsed, fmt.Errorf("password hash digest: %w", err)
	}
	return parsed, nil
}

func validateArgon2Parameters(value string) error {
	memoryField, remainder, ok := strings.Cut(value, ",")
	if !ok {
		return errors.New("password hash has invalid Argon2 parameters")
	}
	timeField, parallelismField, ok := strings.Cut(remainder, ",")
	if !ok || strings.Contains(parallelismField, ",") {
		return errors.New("password hash has invalid Argon2 parameters")
	}

	memory, err := parseBoundedPHCParameter(memoryField, "m=", argon2Memory)
	if err != nil {
		return err
	}
	timeCost, err := parseBoundedPHCParameter(timeField, "t=", argon2Time)
	if err != nil {
		return err
	}
	parallelism, err := parseBoundedPHCParameter(
		parallelismField,
		"p=",
		argon2Parallelism,
	)
	if err != nil {
		return err
	}
	if memory != argon2Memory ||
		timeCost != argon2Time ||
		parallelism != argon2Parallelism {
		return errors.New("password hash uses unsupported Argon2 parameters")
	}
	return nil
}

func parseBoundedPHCParameter(field string, prefix string, maximum uint32) (uint32, error) {
	if !strings.HasPrefix(field, prefix) {
		return 0, errors.New("password hash has invalid Argon2 parameters")
	}
	digits := strings.TrimPrefix(field, prefix)
	if digits == "" || (len(digits) > 1 && digits[0] == '0') {
		return 0, errors.New("password hash has invalid Argon2 parameters")
	}
	value, err := strconv.ParseUint(digits, 10, 32)
	if err != nil {
		return 0, errors.New("password hash has invalid Argon2 parameters")
	}
	if value > uint64(maximum) {
		return 0, errors.New("password hash Argon2 parameters exceed limits")
	}
	return uint32(value), nil
}

func decodePHCField(encoded string, destination []byte) error {
	if len(encoded) != strictRawBase64.EncodedLen(len(destination)) {
		return errors.New("has an invalid length")
	}
	n, err := strictRawBase64.Decode(destination, []byte(encoded))
	if err != nil || n != len(destination) {
		return errors.New("is not canonical raw base64")
	}
	if strictRawBase64.EncodeToString(destination) != encoded {
		return errors.New("is not canonical raw base64")
	}
	return nil
}

func acquirePasswordHashSlot(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case passwordHashSemaphore <- struct{}{}:
		return func() { <-passwordHashSemaphore }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}
