package auth

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

const testPassword = "correct horse battery staple"

func TestCanonicalizeEmail(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "trims and lowercases",
			input: "  User.Name+Tag@Example.COM  ",
			want:  "user.name+tag@example.com",
		},
		{
			name:  "valid UTF-8 mailbox",
			input: "用户@例子.公司",
			want:  "用户@例子.公司",
		},
		{
			name: "exactly 254 bytes",
			input: strings.Repeat("a", 64) + "@" +
				strings.Repeat("b", 63) + "." +
				strings.Repeat("c", 63) + "." +
				strings.Repeat("d", 61),
			want: strings.Repeat("a", 64) + "@" +
				strings.Repeat("b", 63) + "." +
				strings.Repeat("c", 63) + "." +
				strings.Repeat("d", 61),
		},
		{name: "empty", input: "   ", wantErr: true},
		{name: "malformed", input: "not-an-email", wantErr: true},
		{
			name:    "multiple mailboxes",
			input:   "first@example.com, second@example.com",
			wantErr: true,
		},
		{
			name:    "display name",
			input:   "User <user@example.com>",
			wantErr: true,
		},
		{
			name:    "empty display wrapper",
			input:   "<user@example.com>",
			wantErr: true,
		},
		{
			name:    "control character",
			input:   "user@example.com\n",
			wantErr: true,
		},
		{
			name:    "Unicode format control",
			input:   "user\u200b@example.com",
			wantErr: true,
		},
		{
			name:    "invalid UTF-8",
			input:   string([]byte{'u', 's', 'e', 'r', '@', 0xff}),
			wantErr: true,
		},
		{
			name:    "over 254 bytes",
			input:   strings.Repeat("a", 243) + "@example.com",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := canonicalizeEmail(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("canonicalizeEmail(%q) error = nil, want error", tt.input)
				}
				if !errors.Is(err, ErrInvalidIdentityInput) {
					t.Fatalf("canonicalizeEmail(%q) error = %v, want ErrInvalidIdentityInput", tt.input, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("canonicalizeEmail(%q) error = %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("canonicalizeEmail(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCanonicalizeEmailDoesNotFoldDotsOrPlusTags(t *testing.T) {
	got, err := canonicalizeEmail("First.Last+Alerts@Gmail.COM")
	if err != nil {
		t.Fatalf("canonicalizeEmail() error = %v", err)
	}
	if got != "first.last+alerts@gmail.com" {
		t.Fatalf("canonicalizeEmail() = %q, want dots and plus tag preserved", got)
	}
}

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "exactly 15 runes", value: "123456789012345"},
		{name: "counts Unicode runes", value: strings.Repeat("界", 15)},
		{name: "preserves spaces", value: strings.Repeat(" ", 15)},
		{name: "exactly 256 bytes", value: strings.Repeat("a", 256)},
		{name: "fewer than 15 runes", value: "12345678901234", wantErr: true},
		{name: "over 256 bytes", value: strings.Repeat("a", 257), wantErr: true},
		{
			name:    "invalid UTF-8",
			value:   string(append([]byte(strings.Repeat("a", 15)), 0xff)),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validatePassword(tt.value)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("validatePassword() error = nil, want error")
				}
				if !errors.Is(err, ErrInvalidIdentityInput) {
					t.Fatalf("validatePassword() error = %v, want ErrInvalidIdentityInput", err)
				}
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validatePassword() error = %v", err)
			}
		})
	}
}

func TestHashAndVerifyPassword(t *testing.T) {
	password := "  correct horse battery staple  "
	first, err := hashPassword(context.Background(), password)
	if err != nil {
		t.Fatalf("hashPassword() first error = %v", err)
	}
	second, err := hashPassword(context.Background(), password)
	if err != nil {
		t.Fatalf("hashPassword() second error = %v", err)
	}
	if first == second {
		t.Fatal("hashPassword() reused a salt")
	}
	assertPasswordPHC(t, first)
	assertPasswordPHC(t, second)

	verified, err := verifyPassword(context.Background(), password, first)
	if err != nil {
		t.Fatalf("verifyPassword() correct password error = %v", err)
	}
	if !verified {
		t.Fatal("verifyPassword() correct password = false, want true")
	}

	verified, err = verifyPassword(
		context.Background(),
		strings.TrimSpace(password),
		first,
	)
	if err != nil {
		t.Fatalf("verifyPassword() trimmed password error = %v", err)
	}
	if verified {
		t.Fatal("verifyPassword() trimmed password = true, want spaces preserved")
	}
}

func TestVerifyPasswordRejectsMalformedOrUnsafePHC(t *testing.T) {
	valid := testPasswordPHC()
	tests := map[string]string{
		"empty":                  "",
		"oversized":              strings.Repeat("$", maximumPasswordPHCBytes+1),
		"wrong algorithm":        strings.Replace(valid, "argon2id", "argon2i", 1),
		"wrong version":          strings.Replace(valid, "v=19", "v=18", 1),
		"memory over limit":      strings.Replace(valid, "m=65536", "m=99999", 1),
		"time over limit":        strings.Replace(valid, "t=3", "t=4", 1),
		"parallelism over limit": strings.Replace(valid, "p=2", "p=3", 1),
		"memory below fixed":     strings.Replace(valid, "m=65536", "m=32768", 1),
		"leading zero":           strings.Replace(valid, "t=3", "t=03", 1),
		"reordered parameters": strings.Replace(
			valid,
			"m=65536,t=3,p=2",
			"t=3,m=65536,p=2",
			1,
		),
		"short salt":   strings.Replace(valid, strings.Repeat("A", 22), strings.Repeat("A", 21), 1),
		"padded salt":  strings.Replace(valid, strings.Repeat("A", 22), strings.Repeat("A", 21)+"=", 1),
		"short digest": strings.TrimSuffix(valid, "A"),
		"extra field":  valid + "$extra",
	}

	for name, encodedHash := range tests {
		t.Run(name, func(t *testing.T) {
			verified, err := verifyPassword(context.Background(), testPassword, encodedHash)
			if err == nil {
				t.Fatalf("verifyPassword() error = nil, verified = %v, want parse error", verified)
			}
			if verified {
				t.Fatal("verifyPassword() = true for malformed PHC")
			}
		})
	}
}

func TestPasswordHashingHonorsContextWhileWaitingForSemaphore(t *testing.T) {
	for i := 0; i < cap(passwordHashSemaphore); i++ {
		passwordHashSemaphore <- struct{}{}
	}
	defer func() {
		for i := 0; i < cap(passwordHashSemaphore); i++ {
			<-passwordHashSemaphore
		}
	}()

	t.Run("hash", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, err := hashPassword(ctx, testPassword)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("hashPassword() error = %v, want context deadline exceeded", err)
		}
	})

	t.Run("verify", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		_, err := verifyPassword(ctx, testPassword, testPasswordPHC())
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("verifyPassword() error = %v, want context deadline exceeded", err)
		}
	})
}

func assertPasswordPHC(t *testing.T, encodedHash string) {
	t.Helper()
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		t.Fatalf("hash parts = %d, want 6: %q", len(parts), encodedHash)
	}
	if parts[1] != "argon2id" || parts[2] != "v=19" || parts[3] != "m=65536,t=3,p=2" {
		t.Fatalf("hash parameters = %q/%q/%q, want fixed Argon2id parameters", parts[1], parts[2], parts[3])
	}
	salt, err := base64.RawStdEncoding.Strict().DecodeString(parts[4])
	if err != nil || len(salt) != argon2SaltLength {
		t.Fatalf("salt decode = %d bytes, %v; want %d bytes", len(salt), err, argon2SaltLength)
	}
	digest, err := base64.RawStdEncoding.Strict().DecodeString(parts[5])
	if err != nil || len(digest) != argon2HashLength {
		t.Fatalf("digest decode = %d bytes, %v; want %d bytes", len(digest), err, argon2HashLength)
	}
}

func testPasswordPHC() string {
	return fmt.Sprintf(
		"$argon2id$v=19$m=65536,t=3,p=2$%s$%s",
		base64.RawStdEncoding.EncodeToString(make([]byte, argon2SaltLength)),
		base64.RawStdEncoding.EncodeToString(make([]byte, argon2HashLength)),
	)
}
