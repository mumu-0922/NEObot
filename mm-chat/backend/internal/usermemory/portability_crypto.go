package usermemory

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"filippo.io/age"
)

const (
	MinPortabilityPassphraseBytes = 12
	MaxPortabilityPassphraseBytes = 1024
)

func validatePortabilityPassphrase(passphrase string) error {
	if len(passphrase) < MinPortabilityPassphraseBytes ||
		len(passphrase) > MaxPortabilityPassphraseBytes ||
		strings.TrimSpace(passphrase) == "" {
		return validation(
			"MEMORY_PORTABILITY_PASSPHRASE_INVALID",
			"passphrase must be between 12 and 1024 bytes",
		)
	}
	return nil
}

// EncryptPortabilityStream writes one age v1 scrypt-authenticated stream.
// Closing the age writer is part of the authentication contract and is always
// attempted after writePlaintext returns.
func EncryptPortabilityStream(
	destination io.Writer,
	passphrase string,
	writePlaintext func(io.Writer) error,
) error {
	if destination == nil || writePlaintext == nil {
		return errors.New("memory portability encryption writer is required")
	}
	if err := validatePortabilityPassphrase(passphrase); err != nil {
		return err
	}
	recipient, err := age.NewScryptRecipient(passphrase)
	if err != nil {
		return fmt.Errorf("configure memory portability encryption: %w", err)
	}
	writer, err := age.Encrypt(destination, recipient)
	if err != nil {
		return fmt.Errorf("start memory portability encryption: %w", err)
	}
	writeErr := writePlaintext(writer)
	closeErr := writer.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return fmt.Errorf("finish memory portability encryption: %w", closeErr)
	}
	return nil
}

func DecryptPortabilityStream(source io.Reader, passphrase string) (io.Reader, error) {
	if source == nil {
		return nil, errors.New("memory portability encrypted stream is required")
	}
	if err := validatePortabilityPassphrase(passphrase); err != nil {
		return nil, err
	}
	identity, err := age.NewScryptIdentity(passphrase)
	if err != nil {
		return nil, fmt.Errorf("configure memory portability decryption: %w", err)
	}
	reader, err := age.Decrypt(source, identity)
	if err != nil {
		return nil, validation(
			"MEMORY_PORTABILITY_DECRYPT_FAILED",
			"memory package passphrase or authenticated stream is invalid",
		)
	}
	return reader, nil
}
