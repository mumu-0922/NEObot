package agenthost

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const maxUnixSocketPathBytes = 100

var (
	ErrSocketInvalid       = errors.New("agent Host socket path is invalid")
	ErrSocketDirectory     = errors.New("agent Host socket directory is invalid")
	ErrSocketAlreadyActive = errors.New("agent Host is already active")
)

type UnixListener struct {
	listener *net.UnixListener
	path     string
	identity os.FileInfo
	once     sync.Once
	err      error
}

func ListenUnix(path string) (*UnixListener, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if !filepath.IsAbs(path) || len(path) > maxUnixSocketPathBytes || containsControl(path) {
		return nil, ErrSocketInvalid
	}
	directory := filepath.Dir(path)
	canonicalDirectory, err := filepath.EvalSymlinks(directory)
	if err != nil || canonicalDirectory != directory {
		return nil, ErrSocketDirectory
	}
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 || !ownedByEffectiveUser(info) {
		return nil, ErrSocketDirectory
	}
	if existing, statErr := os.Lstat(path); statErr == nil {
		if existing.Mode()&os.ModeSymlink != 0 || existing.Mode()&os.ModeSocket == 0 ||
			!ownedByEffectiveUser(existing) {
			return nil, ErrSocketInvalid
		}
		connection, dialErr := net.DialTimeout("unix", path, 250*time.Millisecond)
		if dialErr == nil {
			_ = connection.Close()
			return nil, ErrSocketAlreadyActive
		}
		current, currentErr := os.Lstat(path)
		if currentErr != nil || !os.SameFile(existing, current) ||
			current.Mode()&os.ModeSocket == 0 || !ownedByEffectiveUser(current) {
			return nil, ErrSocketInvalid
		}
		if err := os.Remove(path); err != nil {
			return nil, ErrSocketInvalid
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, ErrSocketInvalid
	}
	address := &net.UnixAddr{Name: path, Net: "unix"}
	listener, err := net.ListenUnix("unix", address)
	if err != nil {
		return nil, ErrSocketInvalid
	}
	listener.SetUnlinkOnClose(false)
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, ErrSocketInvalid
	}
	identity, err := os.Lstat(path)
	if err != nil || identity.Mode()&os.ModeSocket == 0 || !ownedByEffectiveUser(identity) {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, ErrSocketInvalid
	}
	return &UnixListener{listener: listener, path: path, identity: identity}, nil
}

func (listener *UnixListener) Accept() (net.Conn, error) {
	return listener.listener.Accept()
}

func (listener *UnixListener) Addr() net.Addr {
	return listener.listener.Addr()
}

func (listener *UnixListener) Close() error {
	if listener == nil {
		return nil
	}
	listener.once.Do(func() {
		listener.err = listener.listener.Close()
		current, err := os.Lstat(listener.path)
		if err == nil && os.SameFile(listener.identity, current) &&
			current.Mode()&os.ModeSocket != 0 && ownedByEffectiveUser(current) {
			if removeErr := os.Remove(listener.path); listener.err == nil {
				listener.err = removeErr
			}
		} else if err != nil && !errors.Is(err, os.ErrNotExist) && listener.err == nil {
			listener.err = err
		}
	})
	return listener.err
}

func ownedByEffectiveUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid()
}
