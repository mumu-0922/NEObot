package agentrunner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"neo-chat/mm-chat/backend/internal/strictjson"

	"golang.org/x/sys/unix"
)

type LaunchPlan struct {
	SandboxID           string          `json:"sandboxId"`
	ContainerName       string          `json:"containerName"`
	Attempt             AttemptIdentity `json:"attempt"`
	SnapshotFingerprint string          `json:"snapshotFingerprint"`
	SpecFingerprint     string          `json:"specFingerprint"`
	ProbeFingerprint    string          `json:"probeFingerprint"`
	Sandbox             SandboxSpec     `json:"sandbox"`
	WorkspacePath       string          `json:"workspacePath"`
	ScratchPath         string          `json:"scratchPath"`
	BrokerPath          string          `json:"brokerPath"`
	SeccompPath         string          `json:"seccompPath"`
	Argv                []string        `json:"argv"`
}

type IDMap struct {
	ContainerID int64 `json:"container_id"`
	HostID      int64 `json:"host_id"`
	Size        int64 `json:"size"`
}

type DriverMount struct {
	Type        string   `json:"Type"`
	Source      string   `json:"Source"`
	Destination string   `json:"Destination"`
	Options     []string `json:"Options"`
	RW          bool     `json:"RW"`
}

type DriverIsolation struct {
	Name           string
	Image          string
	User           string
	OCIRuntime     string
	UsernsMode     string
	UIDMap         []IDMap
	GIDMap         []IDMap
	ReadonlyRootfs bool
	NetworkMode    string
	PIDMode        string
	IPCMode        string
	UTSMode        string
	Privileged     bool
	EffectiveCaps  []string
	BoundingCaps   []string
	SecurityOpt    []string
	PIDsLimit      int64
	Memory         int64
	MemorySwap     int64
	NanoCPUs       int64
	Cgroups        string
	CgroupsMode    string
	CgroupManager  string
	CgroupParent   string
	Tmpfs          map[string]string
	Mounts         []DriverMount
	LogDriver      string
	PID            int
	CgroupPath     string
}

type DriverSandbox struct {
	ContainerID         string
	SandboxID           string
	Attempt             AttemptIdentity
	SnapshotFingerprint string
	SpecFingerprint     string
	ProbeFingerprint    string
	State               string
	Isolation           DriverIsolation
}

type SandboxDriver interface {
	Create(context.Context, LaunchPlan) (DriverSandbox, error)
	Inspect(context.Context, string) (DriverSandbox, error)
	Start(context.Context, string) error
	Reap(context.Context, string, bool) error
	List(context.Context) ([]DriverSandbox, error)
}

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type ExecCommandRunner struct{}

func (ExecCommandRunner) Run(ctx context.Context, path string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, path, args...)
	command.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	output, err := command.Output()
	if err != nil {
		return nil, ErrRuntimeUnavailable
	}
	if len(output) > 1<<20 {
		return nil, ErrRuntimeUnavailable
	}
	return output, nil
}

type PodmanDriver struct {
	Binary string
	Runner CommandRunner
}

func NewPodmanDriver(binary string, runner CommandRunner) (*PodmanDriver, error) {
	if !filepath.IsAbs(binary) || runner == nil {
		return nil, ErrInvalidInput
	}
	return &PodmanDriver{Binary: binary, Runner: runner}, nil
}

func (driver *PodmanDriver) Create(ctx context.Context, plan LaunchPlan) (DriverSandbox, error) {
	if driver == nil || validateLaunchPlan(plan) != nil {
		return DriverSandbox{}, ErrInvalidInput
	}
	limits := plan.Sandbox.Resources
	args := []string{
		"create", "--name", plan.ContainerName,
		"--label", "neo.runner.managed=true",
		"--label", "neo.runner.sandbox=" + plan.SandboxID,
		"--label", "neo.runner.run=" + plan.Attempt.RunID,
		"--label", "neo.runner.step=" + plan.Attempt.StepID,
		"--label", "neo.runner.attempt=" + plan.Attempt.AttemptID,
		"--label", "neo.runner.generation=" + strconv.FormatInt(plan.Attempt.LeaseGeneration, 10),
		"--label", "neo.runner.snapshot=" + plan.SnapshotFingerprint,
		"--label", "neo.runner.spec=" + plan.SpecFingerprint,
		"--label", "neo.runner.probe=" + plan.ProbeFingerprint,
		"--runtime", "crun", "--userns", "auto:size=65536",
		"--user", strconv.Itoa(plan.Sandbox.UID) + ":" + strconv.Itoa(plan.Sandbox.GID),
		"--read-only", "--read-only-tmpfs=false", "--cap-drop=all",
		"--security-opt", "no-new-privileges", "--security-opt", "seccomp=" + plan.SeccompPath,
		"--network=none", "--pid=private", "--ipc=private", "--uts=private",
		"--cgroups=enabled", "--cpus", formatCPUs(limits.CPUMillis),
		"--memory", strconv.FormatInt(limits.MemoryMiB, 10) + "m",
		"--memory-swap", strconv.FormatInt(limits.MemoryMiB, 10) + "m",
		"--pids-limit", strconv.FormatInt(limits.PIDs, 10),
		"--log-driver=none", "--stop-timeout=5",
		"--mount", "type=bind,src=" + plan.WorkspacePath + ",dst=/workspace,ro=true,nosuid,nodev,noexec",
		"--mount", "type=bind,src=" + plan.BrokerPath + ",dst=/run/neo-broker,rw=true,nosuid,nodev,noexec",
		"--tmpfs", scratchTmpfsOption(limits.ScratchBytes),
		plan.Sandbox.Image,
	}
	args = append(args, plan.Argv...)
	output, err := driver.Runner.Run(ctx, driver.Binary, args...)
	if err != nil {
		return DriverSandbox{}, err
	}
	containerID := strings.TrimSpace(string(output))
	if !lowerHexLength(containerID, 64) {
		return DriverSandbox{}, ErrRuntimeUnavailable
	}
	return DriverSandbox{ContainerID: containerID, SandboxID: plan.SandboxID, Attempt: plan.Attempt,
		SnapshotFingerprint: plan.SnapshotFingerprint, SpecFingerprint: plan.SpecFingerprint,
		ProbeFingerprint: plan.ProbeFingerprint, State: "created"}, nil
}

type podmanInspect struct {
	ID            string   `json:"Id"`
	Name          string   `json:"Name"`
	ImageName     string   `json:"ImageName"`
	OCIRuntime    string   `json:"OCIRuntime"`
	EffectiveCaps []string `json:"EffectiveCaps"`
	BoundingCaps  []string `json:"BoundingCaps"`
	State         struct {
		Status     string `json:"Status"`
		PID        int    `json:"Pid"`
		CgroupPath string `json:"CgroupPath"`
	} `json:"State"`
	Config struct {
		User   string            `json:"User"`
		Image  string            `json:"Image"`
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
	HostConfig struct {
		ReadonlyRootfs bool              `json:"ReadonlyRootfs"`
		NetworkMode    string            `json:"NetworkMode"`
		PidMode        string            `json:"PidMode"`
		IpcMode        string            `json:"IpcMode"`
		UTSMode        string            `json:"UTSMode"`
		Privileged     bool              `json:"Privileged"`
		SecurityOpt    []string          `json:"SecurityOpt"`
		PidsLimit      int64             `json:"PidsLimit"`
		Memory         int64             `json:"Memory"`
		MemorySwap     int64             `json:"MemorySwap"`
		NanoCpus       int64             `json:"NanoCpus"`
		Cgroups        string            `json:"Cgroups"`
		CgroupsMode    string            `json:"CgroupsMode"`
		CgroupManager  string            `json:"CgroupManager"`
		CgroupParent   string            `json:"CgroupParent"`
		UsernsMode     string            `json:"UsernsMode"`
		Tmpfs          map[string]string `json:"Tmpfs"`
		LogConfig      struct {
			Type string `json:"Type"`
		} `json:"LogConfig"`
	} `json:"HostConfig"`
	IDMappings struct {
		UIDMap []IDMap `json:"UidMap"`
		GIDMap []IDMap `json:"GidMap"`
	} `json:"IDMappings"`
	Mounts []DriverMount `json:"Mounts"`
}

func (driver *PodmanDriver) Inspect(ctx context.Context, containerID string) (DriverSandbox, error) {
	if driver == nil || !lowerHexLength(containerID, 64) {
		return DriverSandbox{}, ErrInvalidInput
	}
	value, err := driver.inspectRaw(ctx, containerID)
	if err != nil {
		return DriverSandbox{}, err
	}
	labels := value.Config.Labels
	generation, parseErr := strconv.ParseInt(labels["neo.runner.generation"], 10, 64)
	isolation := isolationFromInspect(value)
	result := DriverSandbox{ContainerID: value.ID, SandboxID: labels["neo.runner.sandbox"],
		Attempt: AttemptIdentity{RunID: labels["neo.runner.run"], StepID: labels["neo.runner.step"],
			AttemptID: labels["neo.runner.attempt"], LeaseGeneration: generation},
		SnapshotFingerprint: labels["neo.runner.snapshot"], SpecFingerprint: labels["neo.runner.spec"],
		ProbeFingerprint: labels["neo.runner.probe"], State: value.State.Status, Isolation: isolation}
	if parseErr != nil || !validDriverSandbox(result) || !validIsolationBaseline(isolation) {
		return DriverSandbox{}, ErrIsolationUnavailable
	}
	return result, nil
}

func (driver *PodmanDriver) inspectRaw(ctx context.Context, containerID string) (podmanInspect, error) {
	output, err := driver.Runner.Run(ctx, driver.Binary, "inspect", "--type=container", containerID)
	if err != nil {
		return podmanInspect{}, err
	}
	var raw json.RawMessage
	if strictjson.Decode(output, 1<<20, &raw) != nil {
		return podmanInspect{}, ErrRuntimeUnavailable
	}
	var values []podmanInspect
	if json.Unmarshal(raw, &values) != nil || len(values) != 1 {
		return podmanInspect{}, ErrRuntimeUnavailable
	}
	value := values[0]
	if value.ID != containerID || value.Config.Labels["neo.runner.managed"] != "true" {
		return podmanInspect{}, ErrIsolationUnavailable
	}
	return value, nil
}

func isolationFromInspect(value podmanInspect) DriverIsolation {
	return DriverIsolation{
		Name: value.Name, Image: firstNonEmpty(value.ImageName, value.Config.Image), User: value.Config.User,
		OCIRuntime: value.OCIRuntime, UsernsMode: value.HostConfig.UsernsMode,
		UIDMap: value.IDMappings.UIDMap, GIDMap: value.IDMappings.GIDMap,
		ReadonlyRootfs: value.HostConfig.ReadonlyRootfs, NetworkMode: value.HostConfig.NetworkMode,
		PIDMode: value.HostConfig.PidMode, IPCMode: value.HostConfig.IpcMode, UTSMode: value.HostConfig.UTSMode,
		Privileged: value.HostConfig.Privileged, EffectiveCaps: value.EffectiveCaps,
		BoundingCaps: value.BoundingCaps, SecurityOpt: value.HostConfig.SecurityOpt,
		PIDsLimit: value.HostConfig.PidsLimit, Memory: value.HostConfig.Memory,
		MemorySwap: value.HostConfig.MemorySwap, NanoCPUs: value.HostConfig.NanoCpus,
		Cgroups: value.HostConfig.Cgroups, CgroupsMode: value.HostConfig.CgroupsMode,
		CgroupManager: value.HostConfig.CgroupManager, CgroupParent: value.HostConfig.CgroupParent,
		Tmpfs: value.HostConfig.Tmpfs, Mounts: value.Mounts,
		LogDriver: value.HostConfig.LogConfig.Type, PID: value.State.PID,
		CgroupPath: value.State.CgroupPath,
	}
}

func (driver *PodmanDriver) Start(ctx context.Context, id string) error {
	if driver == nil || !lowerHexLength(id, 64) {
		return ErrInvalidInput
	}
	_, err := driver.Runner.Run(ctx, driver.Binary, "start", id)
	return err
}

func (driver *PodmanDriver) Reap(ctx context.Context, id string, force bool) error {
	if driver == nil || !lowerHexLength(id, 64) {
		return ErrInvalidInput
	}
	observed, err := driver.inspectRaw(ctx, id)
	if err != nil {
		return err
	}
	if force {
		if _, err = driver.Runner.Run(ctx, driver.Binary, "kill", "--signal=KILL", id); err != nil {
			return err
		}
	} else if _, err = driver.Runner.Run(ctx, driver.Binary, "stop", "--time=5", id); err != nil {
		if _, err = driver.Runner.Run(ctx, driver.Binary, "kill", "--signal=KILL", id); err != nil {
			return err
		}
	}
	if _, err = driver.Runner.Run(ctx, driver.Binary, "wait", "--condition=stopped", id); err != nil {
		return err
	}
	if _, err = driver.Runner.Run(ctx, driver.Binary, "rm", "--force", id); err != nil {
		return err
	}
	return waitRuntimeIdentityGone(ctx, isolationFromInspect(observed))
}

func (driver *PodmanDriver) List(ctx context.Context) ([]DriverSandbox, error) {
	if driver == nil {
		return nil, ErrInvalidInput
	}
	output, err := driver.Runner.Run(ctx, driver.Binary, "ps", "--all", "--filter", "label=neo.runner.managed=true", "--format=json")
	if err != nil {
		return nil, err
	}
	var raw json.RawMessage
	if strictjson.Decode(output, 1<<20, &raw) != nil {
		return nil, ErrRuntimeUnavailable
	}
	var entries []struct {
		ID string `json:"Id"`
	}
	if json.Unmarshal(raw, &entries) != nil || len(entries) > 1000 {
		return nil, ErrRuntimeUnavailable
	}
	result := make([]DriverSandbox, 0, len(entries))
	for _, entry := range entries {
		value, inspectErr := driver.inspectRaw(ctx, entry.ID)
		if inspectErr != nil {
			return nil, inspectErr
		}
		labels := value.Config.Labels
		generation, _ := strconv.ParseInt(labels["neo.runner.generation"], 10, 64)
		result = append(result, DriverSandbox{ContainerID: value.ID, SandboxID: labels["neo.runner.sandbox"],
			Attempt: AttemptIdentity{RunID: labels["neo.runner.run"], StepID: labels["neo.runner.step"],
				AttemptID: labels["neo.runner.attempt"], LeaseGeneration: generation},
			SnapshotFingerprint: labels["neo.runner.snapshot"], SpecFingerprint: labels["neo.runner.spec"],
			ProbeFingerprint: labels["neo.runner.probe"], State: value.State.Status,
			Isolation: isolationFromInspect(value)})
	}
	return result, nil
}

func validateLaunchPlan(plan LaunchPlan) error {
	if !validID(plan.SandboxID, "sandbox") || plan.ContainerName == "" || len(plan.ContainerName) > 128 ||
		!validID(plan.Attempt.RunID, "run") || !validID(plan.Attempt.StepID, "step") ||
		!validID(plan.Attempt.AttemptID, "attempt") || plan.Attempt.LeaseGeneration < 1 ||
		!validFingerprint(plan.SnapshotFingerprint) || !validFingerprint(plan.SpecFingerprint) ||
		!validFingerprint(plan.ProbeFingerprint) || validateSandbox(plan.Sandbox) != nil ||
		!secureAbsolutePath(plan.WorkspacePath) || !secureAbsolutePath(plan.ScratchPath) ||
		!secureAbsolutePath(plan.BrokerPath) || !pathWithin(plan.ScratchPath, plan.BrokerPath) ||
		!secureAbsolutePath(plan.SeccompPath) || len(plan.Argv) < 1 {
		return ErrInvalidInput
	}
	return nil
}

func matchesLaunchPlan(item DriverSandbox, plan LaunchPlan) bool {
	if !sameSandboxIdentity(item, DriverSandbox{ContainerID: item.ContainerID, SandboxID: plan.SandboxID,
		Attempt: plan.Attempt, SnapshotFingerprint: plan.SnapshotFingerprint,
		SpecFingerprint: plan.SpecFingerprint, ProbeFingerprint: plan.ProbeFingerprint}) {
		return false
	}
	isolation := item.Isolation
	expectedUser := strconv.Itoa(plan.Sandbox.UID) + ":" + strconv.Itoa(plan.Sandbox.GID)
	return strings.TrimPrefix(isolation.Name, "/") == plan.ContainerName && isolation.Image == plan.Sandbox.Image &&
		isolation.User == expectedUser && isolation.OCIRuntime == "crun" && isolation.UsernsMode == "private" &&
		validIDMappings(isolation.UIDMap, 65536) && validIDMappings(isolation.GIDMap, 65536) &&
		isolation.ReadonlyRootfs && isolation.NetworkMode == "none" && isolation.PIDMode == "private" &&
		isolation.IPCMode == "private" && isolation.UTSMode == "private" && !isolation.Privileged &&
		len(isolation.EffectiveCaps) == 0 && len(isolation.BoundingCaps) == 0 &&
		exactSecurityOptions(isolation.SecurityOpt, plan.SeccompPath) &&
		isolation.PIDsLimit == plan.Sandbox.Resources.PIDs &&
		isolation.Memory == plan.Sandbox.Resources.MemoryMiB<<20 &&
		isolation.MemorySwap == plan.Sandbox.Resources.MemoryMiB<<20 &&
		isolation.NanoCPUs == plan.Sandbox.Resources.CPUMillis*1_000_000 &&
		isolation.Cgroups == "enabled" && isolation.CgroupsMode == "private" &&
		isolation.CgroupManager == "systemd" && validCgroupParent(isolation.CgroupParent) &&
		isolation.LogDriver == "none" &&
		exactTmpfs(isolation.Tmpfs, plan.Sandbox.Resources.ScratchBytes) &&
		exactMounts(isolation.Mounts, plan.WorkspacePath, plan.BrokerPath)
}

func validIsolationBaseline(value DriverIsolation) bool {
	return validContainerUser(value.User) && value.ReadonlyRootfs && value.NetworkMode == "none" &&
		value.PIDMode != "host" && value.IPCMode != "host" && value.UTSMode != "host" &&
		!value.Privileged && len(value.EffectiveCaps) == 0 && len(value.BoundingCaps) == 0 &&
		value.PIDsLimit >= 8 && value.Memory >= 64<<20 && value.NanoCPUs >= 100_000_000 &&
		value.CgroupsMode == "private" && value.CgroupManager == "systemd"
}

func validDriverSandbox(item DriverSandbox) bool {
	return lowerHexLength(item.ContainerID, 64) && validID(item.SandboxID, "sandbox") &&
		validID(item.Attempt.RunID, "run") && validID(item.Attempt.StepID, "step") &&
		validID(item.Attempt.AttemptID, "attempt") && item.Attempt.LeaseGeneration >= 1 &&
		validFingerprint(item.SnapshotFingerprint) && validFingerprint(item.SpecFingerprint) &&
		validFingerprint(item.ProbeFingerprint)
}

func secureAbsolutePath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsRune(path, '\x00')
}

func scratchTmpfsOption(size int64) string {
	return "/scratch:rw,noexec,nosuid,nodev,size=" + strconv.FormatInt(size, 10) + ",mode=0700"
}

func exactTmpfs(values map[string]string, size int64) bool {
	if len(values) != 1 {
		return false
	}
	value, ok := values["/scratch"]
	if !ok {
		return false
	}
	return sameOptionSet(value, "rw,noexec,nosuid,nodev,size="+strconv.FormatInt(size, 10)+",mode=0700")
}

func exactMounts(mounts []DriverMount, workspace, broker string) bool {
	if len(mounts) != 2 {
		return false
	}
	found := map[string]bool{}
	for _, mount := range mounts {
		if mount.Type != "bind" || !secureAbsolutePath(mount.Source) {
			return false
		}
		switch mount.Destination {
		case "/workspace":
			found["workspace"] = mount.Source == workspace && !mount.RW && containsMountOptions(mount.Options, "ro", "nosuid", "nodev", "noexec")
		case "/run/neo-broker":
			found["broker"] = mount.Source == broker && mount.RW && containsMountOptions(mount.Options, "rw", "nosuid", "nodev", "noexec")
		default:
			return false
		}
	}
	return found["workspace"] && found["broker"]
}

func containsMountOptions(options []string, required ...string) bool {
	set := stringSet(options...)
	for _, option := range required {
		if _, ok := set[option]; !ok {
			return false
		}
	}
	return true
}

func exactSecurityOptions(values []string, seccompPath string) bool {
	if len(values) != 2 {
		return false
	}
	set := stringSet(values...)
	_, nnp := set["no-new-privileges"]
	_, seccomp := set["seccomp="+seccompPath]
	return nnp && seccomp
}

func validIDMappings(values []IDMap, minimum int64) bool {
	if len(values) < 1 || len(values) > 8 {
		return false
	}
	copyValues := append([]IDMap(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i].ContainerID < copyValues[j].ContainerID })
	var covered int64
	var previousContainerEnd int64 = -1
	var hostRanges [][2]int64
	for _, value := range copyValues {
		if value.ContainerID < 0 || value.HostID < 1 || value.Size < 1 ||
			(previousContainerEnd >= 0 && value.ContainerID < previousContainerEnd) {
			return false
		}
		end := value.HostID + value.Size
		if end <= value.HostID {
			return false
		}
		for _, hostRange := range hostRanges {
			if value.HostID < hostRange[1] && end > hostRange[0] {
				return false
			}
		}
		hostRanges = append(hostRanges, [2]int64{value.HostID, end})
		previousContainerEnd = value.ContainerID + value.Size
		covered += value.Size
	}
	return copyValues[0].ContainerID == 0 && covered >= minimum
}

func validCgroupParent(value string) bool {
	value = filepath.Clean(value)
	return value != "." && value != "/" && !strings.Contains(value, "..") && len(value) <= 512
}

func waitRuntimeIdentityGone(ctx context.Context, isolation DriverIsolation) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		processGone := isolation.PID <= 0 || exactProcessIdentityGone(isolation.PID)
		cgroupGone := isolation.CgroupPath == "" && isolation.PID <= 0 || cgroupPathGone(isolation.CgroupPath)
		if processGone && cgroupGone {
			return nil
		}
		if time.Now().After(deadline) {
			return ErrIsolationUnavailable
		}
		select {
		case <-ctx.Done():
			return ErrIsolationUnavailable
		case <-time.After(25 * time.Millisecond):
		}
	}
}

func exactProcessIdentityGone(pid int) bool {
	fd, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return errors.Is(err, syscall.ESRCH)
	}
	defer unix.Close(fd)
	var info unix.Siginfo
	err = unix.Waitid(unix.P_PIDFD, fd, &info, unix.WEXITED|unix.WNOHANG, nil)
	return err == nil && info.Signo != 0 || errors.Is(err, syscall.ECHILD)
}

func cgroupPathGone(value string) bool {
	if value == "" || strings.ContainsRune(value, '\x00') {
		return false
	}
	clean := filepath.Clean("/" + value)
	if clean == "/" || strings.Contains(clean, "..") {
		return false
	}
	_, err := os.Stat(filepath.Join("/sys/fs/cgroup", clean))
	return errors.Is(err, os.ErrNotExist)
}

func formatCPUs(millis int64) string {
	return strconv.FormatFloat(float64(millis)/1000, 'f', 3, 64)
}

func lowerHexLength(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for _, character := range value {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}

func validContainerUser(value string) bool {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return false
	}
	uid, uidErr := strconv.Atoi(parts[0])
	gid, gidErr := strconv.Atoi(parts[1])
	return uidErr == nil && gidErr == nil && uid > 0 && gid > 0
}

func sameOptionSet(left, right string) bool {
	leftParts := strings.Split(left, ",")
	rightParts := strings.Split(right, ",")
	if len(leftParts) != len(rightParts) {
		return false
	}
	sort.Strings(leftParts)
	sort.Strings(rightParts)
	return strings.Join(leftParts, ",") == strings.Join(rightParts, ",")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

type MemoryDriver struct {
	mu        sync.Mutex
	Sandboxes map[string]DriverSandbox
	Plans     []LaunchPlan
	Fail      error
}

func NewMemoryDriver() *MemoryDriver { return &MemoryDriver{Sandboxes: map[string]DriverSandbox{}} }

func (driver *MemoryDriver) Create(_ context.Context, plan LaunchPlan) (DriverSandbox, error) {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	if driver.Fail != nil {
		return DriverSandbox{}, driver.Fail
	}
	if validateLaunchPlan(plan) != nil {
		return DriverSandbox{}, ErrInvalidInput
	}
	id := strings.Repeat("a", 64)
	item := DriverSandbox{ContainerID: id, SandboxID: plan.SandboxID, Attempt: plan.Attempt,
		SnapshotFingerprint: plan.SnapshotFingerprint, SpecFingerprint: plan.SpecFingerprint,
		ProbeFingerprint: plan.ProbeFingerprint, State: "created",
		Isolation: testableIsolationForPlan(plan)}
	driver.Plans = append(driver.Plans, plan)
	driver.Sandboxes[id] = item
	return item, nil
}

func (driver *MemoryDriver) Inspect(_ context.Context, id string) (DriverSandbox, error) {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	item, ok := driver.Sandboxes[id]
	if !ok {
		return DriverSandbox{}, ErrNotFound
	}
	return item, nil
}

func (driver *MemoryDriver) Start(_ context.Context, id string) error {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	item, ok := driver.Sandboxes[id]
	if !ok {
		return ErrNotFound
	}
	item.State = "running"
	driver.Sandboxes[id] = item
	return nil
}

func (driver *MemoryDriver) Reap(_ context.Context, id string, _ bool) error {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	if _, ok := driver.Sandboxes[id]; !ok {
		return ErrNotFound
	}
	delete(driver.Sandboxes, id)
	return nil
}

func (driver *MemoryDriver) List(context.Context) ([]DriverSandbox, error) {
	driver.mu.Lock()
	defer driver.mu.Unlock()
	result := make([]DriverSandbox, 0, len(driver.Sandboxes))
	for _, item := range driver.Sandboxes {
		result = append(result, item)
	}
	return result, nil
}

func testableIsolationForPlan(plan LaunchPlan) DriverIsolation {
	return DriverIsolation{Name: plan.ContainerName, Image: plan.Sandbox.Image,
		User:       strconv.Itoa(plan.Sandbox.UID) + ":" + strconv.Itoa(plan.Sandbox.GID),
		OCIRuntime: "crun", UsernsMode: "private",
		UIDMap:         []IDMap{{ContainerID: 0, HostID: 100000, Size: 65536}},
		GIDMap:         []IDMap{{ContainerID: 0, HostID: 200000, Size: 65536}},
		ReadonlyRootfs: true, NetworkMode: "none", PIDMode: "private", IPCMode: "private", UTSMode: "private",
		SecurityOpt: []string{"no-new-privileges", "seccomp=" + plan.SeccompPath},
		PIDsLimit:   plan.Sandbox.Resources.PIDs, Memory: plan.Sandbox.Resources.MemoryMiB << 20,
		MemorySwap: plan.Sandbox.Resources.MemoryMiB << 20, NanoCPUs: plan.Sandbox.Resources.CPUMillis * 1_000_000,
		Cgroups: "enabled", CgroupsMode: "private", CgroupManager: "systemd", CgroupParent: "user.slice/neo-runner.scope",
		LogDriver: "none",
		Tmpfs:     map[string]string{"/scratch": strings.TrimPrefix(scratchTmpfsOption(plan.Sandbox.Resources.ScratchBytes), "/scratch:")},
		Mounts: []DriverMount{
			{Type: "bind", Source: plan.WorkspacePath, Destination: "/workspace", Options: []string{"ro", "nosuid", "nodev", "noexec"}},
			{Type: "bind", Source: plan.BrokerPath, Destination: "/run/neo-broker", Options: []string{"rw", "nosuid", "nodev", "noexec"}, RW: true},
		}}
}
