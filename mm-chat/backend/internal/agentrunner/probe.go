package agentrunner

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"neo-chat/mm-chat/backend/internal/strictjson"

	"golang.org/x/sys/unix"
)

type HostProbe interface {
	Probe(context.Context) (ProbeEvidence, error)
}

type SystemProbe struct{ Manifest ReleaseManifest }

func LoadReleaseManifest(path string) (ReleaseManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 || len(data) > 64<<10 {
		return ReleaseManifest{}, ErrInvalidInput
	}
	return ParseReleaseManifest(data)
}

// ParseReleaseManifest validates one already-read release manifest. Security-
// sensitive callers can bind and parse the same bytes without reopening a path.
func ParseReleaseManifest(data []byte) (ReleaseManifest, error) {
	if len(data) == 0 || len(data) > 64<<10 {
		return ReleaseManifest{}, ErrInvalidInput
	}
	var manifest ReleaseManifest
	if err := strictjson.Decode(data, 64<<10, &manifest); err != nil || validateReleaseManifest(manifest) != nil {
		return ReleaseManifest{}, ErrInvalidInput
	}
	return manifest, nil
}

func (probe SystemProbe) Probe(ctx context.Context) (ProbeEvidence, error) {
	manifest := probe.Manifest
	failures := map[string]struct{}{}
	features := map[string]struct{}{}
	if validateReleaseManifest(manifest) != nil || !manifest.Approved {
		failures["release_manifest_unapproved"] = struct{}{}
	}
	if os.Geteuid() == 0 {
		failures["root_identity"] = struct{}{}
	}
	if runtime.GOOS != "linux" {
		failures["linux_required"] = struct{}{}
	}
	for _, binary := range manifest.Binaries {
		if verifyReleaseBinary(ctx, binary) != nil {
			failures["binary_drift_"+binary.Name] = struct{}{}
		}
	}
	if rootlessRuntimeAvailable(ctx, manifest) {
		features["rootless_runtime"] = struct{}{}
		features["cgroup_reap"] = struct{}{}
	} else {
		failures["rootless_runtime_unavailable"] = struct{}{}
	}
	if isolationAcceptanceAvailable(manifest, time.Now()) {
		features["isolation_acceptance"] = struct{}{}
	} else {
		failures["isolation_acceptance_unavailable"] = struct{}{}
	}
	if verifyReleaseFile(manifest.SeccompProfile) != nil {
		failures["seccomp_profile_drift"] = struct{}{}
	} else {
		features["seccomp"] = struct{}{}
	}
	if userNamespacesAvailable() {
		features["rootless_userns"] = struct{}{}
	} else {
		failures["rootless_userns_unavailable"] = struct{}{}
	}
	if subordinateIDsAvailable(os.Geteuid(), os.Getegid()) {
		features["subordinate_ids"] = struct{}{}
	} else {
		failures["subordinate_ids_unavailable"] = struct{}{}
	}
	if cgroupV2Available(manifest.RequiredControllers) {
		features["cgroup_v2"] = struct{}{}
	} else {
		failures["cgroup_v2_delegation_unavailable"] = struct{}{}
	}
	if pidfdAvailable() {
		features["pidfd_kill"] = struct{}{}
	} else {
		failures["pidfd_unavailable"] = struct{}{}
	}
	features["readonly_rootfs"] = struct{}{}
	features["snapshot_workspace"] = struct{}{}
	if manifest.NetworkMode == "none" {
		features["network_none"] = struct{}{}
	}
	evidence := ProbeEvidence{RunnerID: manifest.RunnerID, RunnerVersion: manifest.RunnerVersion,
		Runtime: "podman", RuntimeVersion: releaseVersion(manifest, "podman"),
		KernelClass: kernelClass(), StorageDriver: manifest.StorageDriver,
		NetworkMode: manifest.NetworkMode, Features: sortedKeys(features), FailureClasses: sortedKeys(failures)}
	evidence.Ready = len(failures) == 0
	evidence.ProbeFingerprint = fingerprintProbe(evidence, manifest.ProbeSuiteFingerprint)
	if !evidence.Ready {
		return evidence, ErrIsolationUnavailable
	}
	return evidence, nil
}

func verifyReleaseBinary(ctx context.Context, binary ReleaseBinary) error {
	if verifyReleaseFile(ReleaseFile{Path: binary.Path, SHA256: binary.SHA256}) != nil {
		return ErrIsolationUnavailable
	}
	command := exec.CommandContext(ctx, binary.Path, "--version")
	command.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	output, err := command.Output()
	if err != nil || len(output) > 4096 || !strings.Contains(string(output), binary.Version) {
		return ErrIsolationUnavailable
	}
	return nil
}

func rootlessRuntimeAvailable(ctx context.Context, manifest ReleaseManifest) bool {
	podman := ""
	for _, binary := range manifest.Binaries {
		if binary.Name == "podman" {
			podman = binary.Path
			break
		}
	}
	if podman == "" || verifyReleaseFile(releaseFileFor(manifest, "podman")) != nil {
		return false
	}
	command := exec.CommandContext(ctx, podman, "info", "--format=json")
	command.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	output, err := command.Output()
	if err != nil || len(output) == 0 || len(output) > 1<<20 {
		return false
	}
	var raw json.RawMessage
	if strictjson.Decode(output, 1<<20, &raw) != nil {
		return false
	}
	var info struct {
		Host struct {
			Security struct {
				Rootless       bool   `json:"rootless"`
				SeccompEnabled bool   `json:"seccompEnabled"`
				CgroupVersion  string `json:"cgroupVersion"`
			} `json:"security"`
			OCIRuntime struct {
				Name string `json:"name"`
			} `json:"ociRuntime"`
			CgroupManager string `json:"cgroupManager"`
		} `json:"host"`
		Store struct {
			GraphDriverName string `json:"graphDriverName"`
		} `json:"store"`
	}
	return json.Unmarshal(raw, &info) == nil && info.Host.Security.Rootless &&
		info.Host.Security.SeccompEnabled && info.Host.Security.CgroupVersion == "v2" &&
		info.Host.OCIRuntime.Name == "crun" && info.Host.CgroupManager == "systemd" &&
		info.Store.GraphDriverName == manifest.StorageDriver
}

func releaseFileFor(manifest ReleaseManifest, name string) ReleaseFile {
	for _, binary := range manifest.Binaries {
		if binary.Name == name {
			return ReleaseFile{Path: binary.Path, SHA256: binary.SHA256}
		}
	}
	return ReleaseFile{}
}

func isolationAcceptanceAvailable(manifest ReleaseManifest, now time.Time) bool {
	if verifyReleaseFile(manifest.IsolationAcceptance) != nil {
		return false
	}
	data, err := os.ReadFile(manifest.IsolationAcceptance.Path)
	if err != nil || len(data) == 0 || len(data) > 64<<10 {
		return false
	}
	var report IsolationAcceptanceReport
	if strictjson.Decode(data, 64<<10, &report) != nil {
		return false
	}
	required := []string{"artifact_socket", "cgroup_reap", "cgroup_v2", "kill_reap",
		"network_none", "pidfd_kill", "readonly_rootfs", "restart_reconcile",
		"rootless_runtime", "rootless_userns", "scratch_quota", "seccomp", "snapshot_workspace"}
	return report.SchemaVersion == "neo.agent-runner-isolation-report/v1" && report.Approved &&
		report.RunnerID == manifest.RunnerID && report.RunnerUID == os.Geteuid() && report.RunnerGID == os.Getegid() &&
		report.RunnerUID > 0 && report.RunnerGID > 0 && report.RunnerVersion == manifest.RunnerVersion &&
		report.ProtocolVersion == manifest.ProtocolVersion && report.PodmanVersion == releaseVersion(manifest, "podman") &&
		report.CrunVersion == releaseVersion(manifest, "crun") && report.StorageDriver == manifest.StorageDriver &&
		report.NetworkMode == manifest.NetworkMode && report.SeccompProfileFingerprint == manifest.SeccompProfile.SHA256 &&
		report.ProbeSuiteFingerprint == manifest.ProbeSuiteFingerprint && sameStrings(report.PassedFeatures, required) &&
		!report.GeneratedAt.IsZero() && report.ExpiresAt.After(report.GeneratedAt) &&
		report.ExpiresAt.Sub(report.GeneratedAt) <= 24*time.Hour && !now.Before(report.GeneratedAt) && now.Before(report.ExpiresAt)
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	leftCopy := append([]string(nil), left...)
	rightCopy := append([]string(nil), right...)
	sort.Strings(leftCopy)
	sort.Strings(rightCopy)
	for index := range leftCopy {
		if leftCopy[index] != rightCopy[index] || index > 0 && leftCopy[index] == leftCopy[index-1] {
			return false
		}
	}
	return true
}

func verifyReleaseFile(file ReleaseFile) error {
	info, err := os.Lstat(file.Path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 {
		return ErrIsolationUnavailable
	}
	handle, err := os.Open(file.Path)
	if err != nil {
		return ErrIsolationUnavailable
	}
	defer handle.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, io.LimitReader(handle, 512<<20)); err != nil {
		return ErrIsolationUnavailable
	}
	actual := "sha256:" + hex.EncodeToString(digest.Sum(nil))
	if actual != file.SHA256 {
		return ErrIsolationUnavailable
	}
	return nil
}

func userNamespacesAvailable() bool {
	data, err := os.ReadFile("/proc/sys/user/max_user_namespaces")
	if err != nil {
		return false
	}
	value, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	return err == nil && value > 0
}

func subordinateIDsAvailable(uid, gid int) bool {
	identity, err := user.Current()
	if err != nil || identity.Username == "" {
		return false
	}
	return hasSubordinateRange("/etc/subuid", identity.Username) && hasSubordinateRange("/etc/subgid", identity.Username) && uid > 0 && gid > 0
}

func hasSubordinateRange(path, username string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, 1<<20))
	for scanner.Scan() {
		parts := strings.Split(scanner.Text(), ":")
		if len(parts) != 3 || parts[0] != username {
			continue
		}
		count, err := strconv.ParseInt(parts[2], 10, 64)
		if err == nil && count >= 65536 {
			return true
		}
	}
	return false
}

func cgroupV2Available(required []string) bool {
	var stat syscall.Statfs_t
	if syscall.Statfs("/sys/fs/cgroup", &stat) != nil || uint64(stat.Type) != 0x63677270 {
		return false
	}
	data, err := os.ReadFile("/sys/fs/cgroup/cgroup.controllers")
	if err != nil {
		return false
	}
	controllers := stringSet(strings.Fields(string(data))...)
	for _, controller := range required {
		if _, ok := controllers[controller]; !ok {
			return false
		}
	}
	// The exact Runner account must receive a delegated subtree. Writable root
	// cgroup files or mere controller visibility are not sufficient evidence.
	data, err = os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 || parts[0] != "0" || parts[1] != "" || parts[2] == "/" {
			continue
		}
		path := filepath.Join("/sys/fs/cgroup", filepath.Clean(parts[2]), "cgroup.procs")
		if file, err := os.OpenFile(path, os.O_WRONLY, 0); err == nil {
			file.Close()
			return true
		}
	}
	return false
}

func pidfdAvailable() bool {
	fd, err := unix.PidfdOpen(os.Getpid(), 0)
	if err != nil {
		return false
	}
	unix.Close(fd)
	return true
}

func kernelClass() string {
	var info syscall.Utsname
	if syscall.Uname(&info) != nil {
		return "linux-unknown"
	}
	bytes := make([]byte, 0, len(info.Release))
	for _, value := range info.Release {
		if value == 0 {
			break
		}
		bytes = append(bytes, byte(value))
	}
	release := string(bytes)
	if index := strings.IndexByte(release, '-'); index >= 0 {
		release = release[:index]
	}
	parts := strings.Split(release, ".")
	if len(parts) >= 2 {
		release = parts[0] + "." + parts[1]
	}
	return "linux-" + release
}

func releaseVersion(manifest ReleaseManifest, name string) string {
	for _, binary := range manifest.Binaries {
		if binary.Name == name {
			return binary.Version
		}
	}
	return "unavailable"
}

func fingerprintProbe(evidence ProbeEvidence, suiteFingerprint string) string {
	evidence.ProbeFingerprint = ""
	body, _ := json.Marshal(struct {
		Evidence ProbeEvidence `json:"evidence"`
		Suite    string        `json:"suite"`
	}{evidence, suiteFingerprint})
	digest := sha256.Sum256(append([]byte("neo-runner-probe-v1\x00"), body...))
	return "sha256:" + hex.EncodeToString(digest[:])
}

type StaticProbe struct {
	Evidence ProbeEvidence
	Err      error
}

func (probe StaticProbe) Probe(context.Context) (ProbeEvidence, error) {
	return probe.Evidence, probe.Err
}

func sanitizedProbeJSON(evidence ProbeEvidence) ([]byte, error) {
	if !validFingerprint(evidence.ProbeFingerprint) {
		return nil, ErrInvalidInput
	}
	sort.Strings(evidence.Features)
	sort.Strings(evidence.FailureClasses)
	return json.Marshal(evidence)
}

func probeError(evidence ProbeEvidence, err error) error {
	if errors.Is(err, ErrIsolationUnavailable) {
		return ErrIsolationUnavailable
	}
	return fmt.Errorf("probe unavailable (%d classes): %w", len(evidence.FailureClasses), ErrIsolationUnavailable)
}
