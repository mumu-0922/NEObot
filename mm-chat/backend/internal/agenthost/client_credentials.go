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

// LoadClientCredentials reads Docker-mounted Host client identity files. The
// files may be owned by root (Compose secrets) or by the Backend uid, but must
// be regular, non-symlinked, bounded, and not group/other writable.
func LoadClientCredentials(tokenPath, runnerIDPath string) (string, string, error) {
	token, err := loadClientCredentialFile(tokenPath, maxTokenBytes)
	if err != nil || validateToken(token) != nil {
		return "", "", errors.New("agent Host client token is unavailable")
	}
	runnerID, err := loadClientCredentialFile(runnerIDPath, maxRunnerIDBytes)
	if err != nil || validateRunnerID(runnerID) != nil {
		return "", "", errors.New("agent Host client runner id is unavailable")
	}
	return token, runnerID, nil
}

func loadClientCredentialFile(path string, limit int) (string, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(path) {
		return "", errors.New("credential path is invalid")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || canonical != path {
		return "", errors.New("credential file is unavailable")
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return "", errors.New("credential file is unavailable")
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return "", errors.New("credential file is unavailable")
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 ||
		info.Size() > int64(limit+2) || info.Mode().Perm()&0o022 != 0 {
		return "", errors.New("credential file is invalid")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || (int(stat.Uid) != 0 && int(stat.Uid) != os.Geteuid()) {
		return "", errors.New("credential file owner is invalid")
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit+3)))
	if err != nil || len(data) > limit+2 {
		return "", errors.New("credential file is invalid")
	}
	defer clear(data)
	return strings.TrimSpace(string(data)), nil
}
