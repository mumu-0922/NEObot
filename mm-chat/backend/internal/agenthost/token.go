package agenthost

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func LoadTokenFile(path string) (string, error) {
	return loadTokenFile(path, os.Geteuid())
}

func loadTokenFile(path string, expectedUID int) (string, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(path) {
		return "", errors.New("agent Host token path is invalid")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || canonical != path {
		return "", errors.New("agent Host token file is unavailable")
	}
	fileDescriptor, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return "", errors.New("agent Host token file is unavailable")
	}
	file := os.NewFile(uintptr(fileDescriptor), path)
	if file == nil {
		_ = unix.Close(fileDescriptor)
		return "", errors.New("agent Host token file is unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 ||
		info.Size() < minTokenBytes || info.Size() > maxTokenBytes {
		return "", errors.New("agent Host token file is invalid")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || int(stat.Uid) != expectedUID {
		return "", errors.New("agent Host token file owner is invalid")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxTokenBytes+1))
	if err != nil || len(data) > maxTokenBytes {
		return "", errors.New("agent Host token file is invalid")
	}
	defer clear(data)
	token := strings.TrimSpace(string(data))
	if err := validateToken(token); err != nil {
		return "", err
	}
	return token, nil
}
