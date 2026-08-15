// Command neo-skill-local-test-workload is the inert payload used by the
// explicitly non-production rootless Runner smoke.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	resultName = "draft-local-test-smoke.json"
	socketPath = "/run/neo-broker/artifact.sock"
)

type artifactFrame struct {
	Name      string `json:"name"`
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
}

type artifactReceipt struct {
	Name        string `json:"name"`
	MediaType   string `json:"mediaType"`
	Size        int64  `json:"size"`
	Fingerprint string `json:"fingerprint"`
}

type workloadResult struct {
	SchemaVersion string   `json:"schemaVersion"`
	Outcome       string   `json:"outcome"`
	Checks        []string `json:"checks"`
}

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
	// Keep the process alive long enough for the Runner to prove the started
	// state and then exercise its signed reap path.
	time.Sleep(30 * time.Second)
}

func run() error {
	checks := make([]string, 0, 8)
	if os.Geteuid() != 10001 || os.Getegid() != 10001 {
		return errors.New("unexpected identity")
	}
	checks = append(checks, "nonzero_identity")
	status, err := os.ReadFile("/proc/self/status")
	if err != nil || !statusFieldIsZero(status, "CapEff") || !statusFieldEquals(status, "NoNewPrivs", "1") {
		return errors.New("process isolation unavailable")
	}
	checks = append(checks, "empty_capabilities", "no_new_privileges")
	if err := assertNoNetwork(); err != nil {
		return err
	}
	checks = append(checks, "network_none")
	if err := assertReadOnly("/", ".neo-local-test-root-write"); err != nil {
		return err
	}
	workspace, err := os.ReadFile("/workspace/input.txt")
	if err != nil || string(workspace) != "local-test-workspace\n" {
		return errors.New("workspace unavailable")
	}
	if err := assertReadOnly("/workspace", ".neo-local-test-workspace-write"); err != nil {
		return err
	}
	checks = append(checks, "readonly_rootfs", "readonly_workspace")
	if err := os.WriteFile("/scratch/probe", []byte("scratch-ok\n"), 0o600); err != nil {
		return errors.New("scratch unavailable")
	}
	if body, err := os.ReadFile("/scratch/probe"); err != nil || string(body) != "scratch-ok\n" {
		return errors.New("scratch readback failed")
	}
	if err := os.Remove("/scratch/probe"); err != nil {
		return errors.New("scratch cleanup failed")
	}
	checks = append(checks, "bounded_scratch")
	if _, err := os.Stat("/run/secrets"); !os.IsNotExist(err) {
		return errors.New("secret path exposed")
	}
	for _, item := range os.Environ() {
		key, _, _ := strings.Cut(item, "=")
		upper := strings.ToUpper(key)
		if strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "TOKEN") ||
			strings.Contains(upper, "SECRET") || strings.Contains(upper, "PROXY") || strings.HasSuffix(upper, "_KEY") {
			return errors.New("sensitive environment exposed")
		}
	}
	checks = append(checks, "no_secrets")
	result := workloadResult{
		SchemaVersion: "neo.agent-runner-local-test-workload/v1",
		Outcome:       "passed",
		Checks:        checks,
	}
	body, err := json.Marshal(result)
	if err != nil || len(body) > 8<<10 {
		return errors.New("result unavailable")
	}
	return publishResult(body)
}

func statusFieldEquals(status []byte, name, expected string) bool {
	prefix := name + ":"
	for _, line := range strings.Split(string(status), "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix)) == expected
		}
	}
	return false
}

func statusFieldIsZero(status []byte, name string) bool {
	prefix := name + ":"
	for _, line := range strings.Split(string(status), "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		return value != "" && strings.Trim(value, "0") == ""
	}
	return false
}

func assertNoNetwork() error {
	interfaces, err := net.Interfaces()
	if err != nil || len(interfaces) > 1 {
		return errors.New("network namespace unavailable")
	}
	for _, item := range interfaces {
		if item.Flags&net.FlagLoopback == 0 {
			return errors.New("network interface exposed")
		}
	}
	routes, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return errors.New("network route state unavailable")
	}
	lines := strings.Split(strings.TrimSpace(string(routes)), "\n")
	if len(lines) > 1 {
		return errors.New("network route exposed")
	}
	return nil
}

func assertReadOnly(root, name string) error {
	path := filepath.Join(root, name)
	err := os.WriteFile(path, []byte("forbidden"), 0o600)
	if err == nil {
		_ = os.Remove(path)
		return errors.New("read-only boundary writable")
	}
	if !errors.Is(err, syscall.EROFS) && !errors.Is(err, syscall.EACCES) && !errors.Is(err, syscall.EPERM) {
		return errors.New("read-only boundary ambiguous")
	}
	return nil
}

func publishResult(body []byte) error {
	digest := sha256.Sum256(body)
	frame := artifactFrame{Name: resultName, MediaType: "application/json", Size: int64(len(body))}
	header, _ := json.Marshal(frame)
	for attempt := 0; attempt < 100; attempt++ {
		receipt, err := sendFrame(header, body)
		if err == nil && receipt.Name == resultName && receipt.MediaType == "application/json" &&
			receipt.Size == int64(len(body)) && receipt.Fingerprint == "sha256:"+hex.EncodeToString(digest[:]) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("artifact intake unavailable")
}

func sendFrame(header, body []byte) (artifactReceipt, error) {
	connection, err := net.DialTimeout("unix", socketPath, time.Second)
	if err != nil {
		return artifactReceipt{}, err
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(time.Second))
	length := make([]byte, 4)
	binary.BigEndian.PutUint32(length, uint32(len(header)))
	if _, err := connection.Write(append(append(length, header...), body...)); err != nil {
		return artifactReceipt{}, err
	}
	reader := bufio.NewReaderSize(connection, 4<<10)
	if _, err := io.ReadFull(reader, length); err != nil {
		return artifactReceipt{}, err
	}
	size := binary.BigEndian.Uint32(length)
	if size < 2 || size > 1024 {
		return artifactReceipt{}, fmt.Errorf("invalid receipt size")
	}
	encoded := make([]byte, size)
	if _, err := io.ReadFull(reader, encoded); err != nil {
		return artifactReceipt{}, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	decoder.DisallowUnknownFields()
	var receipt artifactReceipt
	if err := decoder.Decode(&receipt); err != nil {
		return artifactReceipt{}, err
	}
	return receipt, nil
}
