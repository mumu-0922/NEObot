package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const localTempPattern = ".tmp-object-*"

type LocalStore struct {
	root string
}

type localObjectMetadata struct {
	ContentType string `json:"contentType,omitempty"`
}

func NewLocalStore(root string) (*LocalStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, errors.New("local storage root is required")
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve local storage root: %w", err)
	}
	if err := os.MkdirAll(absRoot, 0o700); err != nil {
		return nil, fmt.Errorf("create local storage root: %w", err)
	}

	return &LocalStore{root: absRoot}, nil
}

func (s *LocalStore) CheckReady(ctx context.Context) error {
	if s == nil || s.root == "" {
		return errors.New("local store is not initialized")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	stat, err := os.Stat(s.root)
	if err != nil {
		return fmt.Errorf("stat local storage root: %w", err)
	}
	if !stat.IsDir() {
		return errors.New("local storage root is not a directory")
	}

	return nil
}

func (s *LocalStore) Put(
	ctx context.Context,
	key string,
	body io.Reader,
	size int64,
	contentType string,
) error {
	if body == nil {
		return errors.New("object body is required")
	}
	if size < 0 {
		return errors.New("object size must be non-negative")
	}

	objectPath, err := s.objectPath(key)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(objectPath), 0o700); err != nil {
		return fmt.Errorf("create object directory: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(objectPath), localTempPattern)
	if err != nil {
		return fmt.Errorf("create temp object: %w", err)
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	written, err := copyWithContext(ctx, tmp, body, size+1)
	if err != nil {
		return fmt.Errorf("write temp object: %w", err)
	}
	if written != size {
		return fmt.Errorf("object size mismatch: wrote %d bytes, expected %d", written, size)
	}
	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("chmod temp object: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp object: %w", err)
	}
	if err := os.Rename(tmpPath, objectPath); err != nil {
		return fmt.Errorf("commit object: %w", err)
	}
	committed = true

	if err := s.writeMetadata(key, localObjectMetadata{ContentType: strings.TrimSpace(contentType)}); err != nil {
		_ = os.Remove(objectPath)
		return err
	}

	return nil
}

func (s *LocalStore) Get(ctx context.Context, key string) (io.ReadCloser, ObjectInfo, error) {
	objectPath, err := s.objectPath(key)
	if err != nil {
		return nil, ObjectInfo{}, err
	}
	if err := ctx.Err(); err != nil {
		return nil, ObjectInfo{}, err
	}

	file, err := os.Open(objectPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ObjectInfo{}, ErrObjectNotFound
	}
	if err != nil {
		return nil, ObjectInfo{}, fmt.Errorf("open object: %w", err)
	}

	stat, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, ObjectInfo{}, fmt.Errorf("stat object: %w", err)
	}
	metadata := s.readMetadata(key)

	return file, ObjectInfo{
		Key:         key,
		Size:        stat.Size(),
		ContentType: metadata.ContentType,
		UpdatedAt:   stat.ModTime(),
	}, nil
}

func (s *LocalStore) Delete(ctx context.Context, key string) error {
	objectPath, err := s.objectPath(key)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := os.Remove(objectPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete object: %w", err)
	}
	if metadataPath, err := s.metadataPath(key); err == nil {
		if err := os.Remove(metadataPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("delete object metadata: %w", err)
		}
	}

	return nil
}

func (s *LocalStore) objectPath(key string) (string, error) {
	if s == nil || s.root == "" {
		return "", errors.New("local store is not initialized")
	}
	if err := validateObjectKey(key); err != nil {
		return "", err
	}

	candidate := filepath.Join(s.root, filepath.FromSlash(key))
	absCandidate, err := filepath.Abs(candidate)
	if err != nil {
		return "", fmt.Errorf("resolve object path: %w", err)
	}
	rel, err := filepath.Rel(s.root, absCandidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", ErrInvalidObjectKey
	}

	return absCandidate, nil
}

func (s *LocalStore) metadataPath(key string) (string, error) {
	objectPath, err := s.objectPath(key)
	if err != nil {
		return "", err
	}

	return objectPath + ".meta.json", nil
}

func (s *LocalStore) writeMetadata(key string, metadata localObjectMetadata) error {
	metadataPath, err := s.metadataPath(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(metadataPath), 0o700); err != nil {
		return fmt.Errorf("create metadata directory: %w", err)
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("marshal object metadata: %w", err)
	}
	if err := os.WriteFile(metadataPath, payload, 0o600); err != nil {
		return fmt.Errorf("write object metadata: %w", err)
	}

	return nil
}

func (s *LocalStore) readMetadata(key string) localObjectMetadata {
	metadataPath, err := s.metadataPath(key)
	if err != nil {
		return localObjectMetadata{}
	}
	payload, err := os.ReadFile(metadataPath)
	if err != nil {
		return localObjectMetadata{}
	}

	var metadata localObjectMetadata
	if err := json.Unmarshal(payload, &metadata); err != nil {
		return localObjectMetadata{}
	}
	return metadata
}

func validateObjectKey(key string) error {
	if key != strings.TrimSpace(key) || key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") || strings.Contains(key, ":") {
		return ErrInvalidObjectKey
	}
	if path.Clean(key) != key || key == "." || strings.HasPrefix(key, "../") || strings.Contains(key, "/../") || strings.HasSuffix(key, "/..") {
		return ErrInvalidObjectKey
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return ErrInvalidObjectKey
		}
	}

	return nil
}

func copyWithContext(ctx context.Context, dst io.Writer, src io.Reader, limit int64) (int64, error) {
	buffer := make([]byte, 32*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		maxRead := len(buffer)
		if remaining := limit - written; remaining <= 0 {
			return written, nil
		} else if remaining < int64(maxRead) {
			maxRead = int(remaining)
		}

		n, readErr := src.Read(buffer[:maxRead])
		if n > 0 {
			writeN, writeErr := dst.Write(buffer[:n])
			written += int64(writeN)
			if writeErr != nil {
				return written, writeErr
			}
			if writeN != n {
				return written, io.ErrShortWrite
			}
		}
		if errors.Is(readErr, io.EOF) {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}
