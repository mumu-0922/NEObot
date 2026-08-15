package agentrunner

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"neo-chat/mm-chat/backend/internal/strictjson"
)

const (
	localTestRunnerID = "neo-runner-local-test"
	localTestCaller   = "spiffe://neo-chat/agent-runtime-local-test"
	localResultName   = "draft-local-test-smoke.json"
)

var localWorkloadChecks = []string{
	"nonzero_identity",
	"empty_capabilities",
	"no_new_privileges",
	"network_none",
	"readonly_rootfs",
	"readonly_workspace",
	"bounded_scratch",
	"no_secrets",
}

type LocalTestSmokeConfig struct {
	PodmanPath   string
	WorkloadPath string
	SeccompPath  string
	StateRoot    string
}

type LocalTestSmokeCleanup struct {
	ManagedSandboxes int  `json:"managedSandboxes"`
	ScratchResidue   bool `json:"scratchResidue"`
	ArtifactResidue  bool `json:"artifactResidue"`
}

type LocalTestSmokeReport struct {
	SchemaVersion      string                `json:"schemaVersion"`
	EvidenceClass      string                `json:"evidenceClass"`
	ProductionEligible bool                  `json:"productionEligible"`
	Outcome            string                `json:"outcome"`
	RuntimeVersion     string                `json:"runtimeVersion"`
	Checks             []string              `json:"checks"`
	Cleanup            LocalTestSmokeCleanup `json:"cleanup"`
	GeneratedAt        time.Time             `json:"generatedAt"`
}

type localPodmanInfo struct {
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
	Version struct {
		Version string `json:"Version"`
	} `json:"version"`
}

type localWorkloadResult struct {
	SchemaVersion string   `json:"schemaVersion"`
	Outcome       string   `json:"outcome"`
	Checks        []string `json:"checks"`
}

// RunLocalTestSmoke runs one synthetic Skill through the real PodmanDriver,
// WorkspaceCatalog, artifact intake, signed launch/result/cancel, post-create
// inspection and cleanup paths. It deliberately uses StaticProbe and emits a
// different local-test schema, so it can never become production evidence.
func RunLocalTestSmoke(ctx context.Context, config LocalTestSmokeConfig) (LocalTestSmokeReport, error) {
	if ctx == nil || os.Geteuid() == 0 || validateLocalTestSmokeConfig(config) != nil {
		return LocalTestSmokeReport{}, ErrInvalidInput
	}
	commandRunner := ExecCommandRunner{}
	infoRaw, err := commandRunner.Run(ctx, config.PodmanPath, "info", "--format=json")
	if err != nil {
		return LocalTestSmokeReport{}, ErrIsolationUnavailable
	}
	info, err := validateLocalPodmanInfo(infoRaw)
	if err != nil {
		return LocalTestSmokeReport{}, err
	}
	stateRoot, err := prepareLocalSmokeState(config.StateRoot)
	if err != nil {
		return LocalTestSmokeReport{}, err
	}
	baseRoot := filepath.Dir(stateRoot)
	cleanedState := false
	defer func() {
		if !cleanedState {
			_ = safeRemoveTree(baseRoot, stateRoot)
		}
	}()

	driver, err := NewPodmanDriver(config.PodmanPath, commandRunner)
	if err != nil {
		return LocalTestSmokeReport{}, err
	}
	if err := reapLocalManagedSandboxes(ctx, driver); err != nil {
		return LocalTestSmokeReport{}, err
	}
	imageTag := "localhost/neo-agent-local-test-smoke:bounded"
	imageCreated := false
	defer func() {
		if imageCreated {
			_, _ = commandRunner.Run(context.Background(), config.PodmanPath, "rmi", "--force", imageTag)
		}
	}()
	imageRef, err := importLocalSmokeImage(ctx, commandRunner, config, stateRoot, imageTag)
	if err != nil {
		return LocalTestSmokeReport{}, err
	}
	imageCreated = true

	workspaceFiles := map[string][]byte{"input.txt": []byte("local-test-workspace\n")}
	workspaceID := "workspace_snapshot_1111111111111111"
	workspaceFingerprint := fingerprintWorkspaceFiles(workspaceFiles)
	catalog, err := NewWorkspaceCatalog(filepath.Join(stateRoot, "workspaces"))
	if err != nil {
		return LocalTestSmokeReport{}, err
	}
	if _, err = catalog.Register(ctx, workspaceID, workspaceFingerprint, bytes.NewReader(localWorkspaceTar(workspaceFiles))); err != nil {
		return LocalTestSmokeReport{}, err
	}
	defer func() { _ = catalog.Remove(workspaceID, workspaceFingerprint) }()

	seccompFingerprint, err := fingerprintLocalFile(config.SeccompPath, 1<<20)
	if err != nil {
		return LocalTestSmokeReport{}, err
	}
	broker, err := NewArtifactBroker(filepath.Join(stateRoot, "artifacts"))
	if err != nil {
		return LocalTestSmokeReport{}, err
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return LocalTestSmokeReport{}, ErrRuntimeUnavailable
	}
	authority, err := NewSignedAuthorityVerifier(publicKey)
	if err != nil {
		return LocalTestSmokeReport{}, err
	}
	evidence := ProbeEvidence{
		Ready: true, RunnerID: localTestRunnerID, RunnerVersion: "local-test-only",
		Runtime: "podman", RuntimeVersion: info.Version.Version, KernelClass: kernelClass(),
		StorageDriver: "overlay", NetworkMode: "none",
		Features:       []string{"cgroup_reap", "cgroup_v2", "network_none", "pidfd_kill", "readonly_rootfs", "rootless_runtime", "rootless_userns", "seccomp", "snapshot_workspace", "subordinate_ids"},
		FailureClasses: []string{}, ProbeFingerprint: fingerprintBytes("neo-agent-runner-local-test-probe-v1", infoRaw),
	}
	service, err := NewService(ServiceConfig{
		RunnerID: localTestRunnerID, StateRoot: stateRoot, SeccompPath: config.SeccompPath,
		SeccompFingerprint: seccompFingerprint, ReplayTTL: time.Hour, HeartbeatExtension: 30 * time.Second,
	}, StaticProbe{Evidence: evidence}, authority, NewMemoryReplayLedger(), driver, catalog, broker)
	if err != nil {
		return LocalTestSmokeReport{}, err
	}

	attempt := AttemptRef{
		RunID: "run_1111111111111111", StepID: "step_1111111111111111",
		AttemptID: "attempt_1111111111111111", LeaseGeneration: 1,
		LeaseOwner: localTestRunnerID, LeaseToken: "lease_local_test_111111111111111111111111",
	}
	launchBody := LaunchRequest{
		Attempt: attempt,
		Lineage: RunLineage{RootRunID: attempt.RunID, Depth: 0},
		GrantID: "grant_1111111111111111", GrantFingerprint: localFingerprint("grant"),
		SnapshotFingerprint: localFingerprint("snapshot"),
		Sandbox: SandboxSpec{
			RuntimeBundleFingerprint: localFingerprint("runtime"), PackageFingerprint: localFingerprint("package"),
			Image: imageRef, UID: 10001, GID: 10001, RootfsReadOnly: true, NoNewPrivileges: true,
			Capabilities: []string{}, SeccompProfileFingerprint: seccompFingerprint, NetworkMode: "none",
			WorkspaceSnapshotID: workspaceID, WorkspaceFingerprint: workspaceFingerprint,
			Resources: ResourceLimits{CPUMillis: 500, MemoryMiB: 128, PIDs: 32, WallSeconds: 60,
				OutputBytes: 64 << 10, ScratchBytes: 4 << 20},
		},
		ToolRegistry: ToolRegistry{Depth: 0, Tools: []string{}, RegistryFingerprint: localFingerprint("registry")},
		Argv:         []string{"/neo-skill-local-test-workload"},
	}
	launchRequest, err := signedLocalRequest(MethodLaunch, "rpc_1111111111111111", strings.Repeat("l", 32), launchBody, attempt, privateKey)
	if err != nil {
		return LocalTestSmokeReport{}, err
	}
	if response, handleErr := service.Handle(ctx, localTestCaller, launchRequest); handleErr != nil {
		return LocalTestSmokeReport{}, handleErr
	} else if result, ok := response.Body.(LaunchResult); !ok || !result.Accepted || result.Error != nil {
		return LocalTestSmokeReport{}, ErrRuntimeUnavailable
	}
	launched := true
	defer func() {
		if launched {
			_ = cancelLocalSmoke(context.Background(), service, attempt, privateKey, "kill")
		}
	}()

	workload, err := awaitLocalResult(ctx, service, attempt, launchBody.SnapshotFingerprint, privateKey)
	if err != nil {
		return LocalTestSmokeReport{}, err
	}
	if workload.SchemaVersion != "neo.agent-runner-local-test-workload/v1" || workload.Outcome != "passed" ||
		!sameStrings(workload.Checks, localWorkloadChecks) {
		return LocalTestSmokeReport{}, ErrIsolationUnavailable
	}
	if err := cancelLocalSmoke(ctx, service, attempt, privateKey, "kill"); err != nil {
		return LocalTestSmokeReport{}, err
	}
	launched = false
	if items, listErr := driver.List(ctx); listErr != nil || len(items) != 0 {
		return LocalTestSmokeReport{}, ErrIsolationUnavailable
	}
	if _, err := commandRunner.Run(ctx, config.PodmanPath, "rmi", "--force", imageTag); err != nil {
		return LocalTestSmokeReport{}, err
	}
	imageCreated = false
	if err := catalog.Remove(workspaceID, workspaceFingerprint); err != nil {
		return LocalTestSmokeReport{}, err
	}
	if residueInLocalState(stateRoot) {
		return LocalTestSmokeReport{}, ErrIsolationUnavailable
	}
	if err := safeRemoveTree(baseRoot, stateRoot); err != nil {
		return LocalTestSmokeReport{}, err
	}
	cleanedState = true
	return LocalTestSmokeReport{
		SchemaVersion: "neo.agent-runner-local-test-report/v1", EvidenceClass: "local_test",
		ProductionEligible: false, Outcome: "LOCAL_SKILL_SMOKE_PASSED", RuntimeVersion: info.Version.Version,
		Checks:      append([]string(nil), localWorkloadChecks...),
		Cleanup:     LocalTestSmokeCleanup{ManagedSandboxes: 0, ScratchResidue: false, ArtifactResidue: false},
		GeneratedAt: time.Now().UTC(),
	}, nil
}

func validateLocalTestSmokeConfig(config LocalTestSmokeConfig) error {
	root := filepath.Dir(filepath.Dir(config.PodmanPath))
	if config.PodmanPath != filepath.Join(root, "bin", "podman") ||
		config.WorkloadPath != filepath.Join(root, "bin", "neo-skill-local-test-workload") ||
		config.SeccompPath != filepath.Join(root, "config", "seccomp-agent-v1.json") ||
		config.StateRoot != filepath.Join(root, "smoke-state") {
		return ErrInvalidInput
	}
	for _, path := range []string{root, filepath.Join(root, "bin"), filepath.Join(root, "config"), config.StateRoot} {
		if secureLocalDirectory(path) != nil {
			return ErrInvalidInput
		}
	}
	for _, path := range []string{config.PodmanPath, config.WorkloadPath, config.SeccompPath} {
		if !secureAbsolutePath(path) || secureLocalRegularFile(path, 512<<20) != nil {
			return ErrInvalidInput
		}
	}
	if !secureAbsolutePath(config.StateRoot) || config.StateRoot == string(filepath.Separator) {
		return ErrInvalidInput
	}
	return nil
}

func secureLocalDirectory(path string) error {
	resolved, err := filepath.EvalSymlinks(path)
	info, statErr := os.Lstat(path)
	if err != nil || statErr != nil || resolved != path || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return ErrInvalidInput
	}
	return nil
}

func secureLocalRegularFile(path string, maximum int64) error {
	resolved, resolveErr := filepath.EvalSymlinks(path)
	info, statErr := os.Lstat(path)
	if resolveErr != nil || statErr != nil || resolved != path || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || info.Size() < 1 || info.Size() > maximum {
		return ErrInvalidInput
	}
	return nil
}

func validateLocalPodmanInfo(raw []byte) (localPodmanInfo, error) {
	var info localPodmanInfo
	if strictjson.Decode(raw, 1<<20, &info) != nil || !info.Host.Security.Rootless ||
		!info.Host.Security.SeccompEnabled || info.Host.Security.CgroupVersion != "v2" ||
		info.Host.OCIRuntime.Name != "crun" || info.Host.CgroupManager != "systemd" ||
		info.Store.GraphDriverName != "overlay" || info.Version.Version != "6.1.0" {
		return localPodmanInfo{}, ErrIsolationUnavailable
	}
	return info, nil
}

func prepareLocalSmokeState(base string) (string, error) {
	if secureLocalDirectory(base) != nil {
		return "", ErrRuntimeUnavailable
	}
	root, err := os.MkdirTemp(base, "run-")
	if err != nil {
		return "", ErrRuntimeUnavailable
	}
	if err := os.Chmod(root, 0o700); err != nil {
		return "", ErrRuntimeUnavailable
	}
	for _, name := range []string{"sandboxes", "scratch", "workspaces", "artifacts"} {
		path := filepath.Join(root, name)
		if err := os.Mkdir(path, 0o700); err != nil {
			return "", ErrRuntimeUnavailable
		}
	}
	return root, nil
}

func reapLocalManagedSandboxes(ctx context.Context, driver *PodmanDriver) error {
	items, err := driver.List(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if err := driver.Reap(ctx, item.ContainerID, true); err != nil {
			return err
		}
	}
	return nil
}

func importLocalSmokeImage(ctx context.Context, runner CommandRunner, config LocalTestSmokeConfig, root, tag string) (string, error) {
	archivePath := filepath.Join(root, "rootfs.tar")
	archive, err := os.OpenFile(archivePath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", ErrRuntimeUnavailable
	}
	writer := tar.NewWriter(archive)
	workload, err := os.Open(config.WorkloadPath)
	if err != nil {
		writer.Close()
		archive.Close()
		return "", ErrRuntimeUnavailable
	}
	info, err := workload.Stat()
	if err == nil {
		err = writer.WriteHeader(&tar.Header{Name: "neo-skill-local-test-workload", Mode: 0o555, Size: info.Size(), Typeflag: tar.TypeReg})
	}
	if err == nil {
		_, err = io.CopyN(writer, workload, info.Size())
	}
	workload.Close()
	closeErr := writer.Close()
	syncErr := archive.Sync()
	fileCloseErr := archive.Close()
	if err != nil || closeErr != nil || syncErr != nil || fileCloseErr != nil {
		return "", ErrRuntimeUnavailable
	}
	defer os.Remove(archivePath)
	if _, err := runner.Run(ctx, config.PodmanPath, "import", archivePath, tag); err != nil {
		return "", err
	}
	raw, err := runner.Run(ctx, config.PodmanPath, "image", "inspect", "--format=json", tag)
	if err != nil {
		return "", err
	}
	var images []struct {
		Digest      string   `json:"Digest"`
		RepoDigests []string `json:"RepoDigests"`
	}
	if strictjson.Decode(raw, 1<<20, &images) != nil || len(images) != 1 {
		return "", ErrRuntimeUnavailable
	}
	for _, digest := range images[0].RepoDigests {
		if strings.HasPrefix(digest, strings.TrimSuffix(tag, ":bounded")+"@sha256:") && imagePattern.MatchString(digest) {
			return digest, nil
		}
	}
	if validFingerprint(images[0].Digest) {
		candidate := strings.TrimSuffix(tag, ":bounded") + "@" + images[0].Digest
		if imagePattern.MatchString(candidate) {
			return candidate, nil
		}
	}
	return "", ErrRuntimeUnavailable
}

func localWorkspaceTar(files map[string][]byte) []byte {
	var output bytes.Buffer
	writer := tar.NewWriter(&output)
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		body := files[path]
		_ = writer.WriteHeader(&tar.Header{Name: path, Mode: 0o400, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = writer.Write(body)
	}
	_ = writer.Close()
	return output.Bytes()
}

func fingerprintLocalFile(path string, maximum int64) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", ErrInvalidInput
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 1 || info.Size() > maximum {
		return "", ErrInvalidInput
	}
	digest := sha256.New()
	if _, err := io.CopyN(digest, file, info.Size()); err != nil {
		return "", ErrRuntimeUnavailable
	}
	return "sha256:" + hex.EncodeToString(digest.Sum(nil)), nil
}

func localFingerprint(label string) string {
	digest := sha256.Sum256([]byte("neo-agent-runner-local-test-v1\x00" + label))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func signedLocalRequest(method, requestID, nonce string, body any, attempt AttemptRef, privateKey ed25519.PrivateKey) (Request, error) {
	fingerprint := AuthorityRequestFingerprint(method, body)
	if fingerprint == "" {
		return Request{}, ErrInvalidInput
	}
	now := time.Now().UTC()
	claims := NewAuthorityClaims(localTestCaller, localTestRunnerID, method, requestID, nonce,
		fingerprint, localSnapshotFingerprint(body), attempt, 0, now, now.Add(10*time.Second))
	ticket, err := SignAuthority(privateKey, claims)
	if err != nil {
		return Request{}, err
	}
	switch value := body.(type) {
	case LaunchRequest:
		value.Authority = ticket
		body = value
	case ResultRequest:
		value.Authority = ticket
		body = value
	case CancelRequest:
		value.Authority = ticket
		body = value
	default:
		return Request{}, ErrInvalidInput
	}
	bodyRaw, _ := json.Marshal(body)
	wire, _ := json.Marshal(Envelope{SchemaVersion: ProtocolVersion, Method: method, RequestID: requestID,
		SentAt: now, Nonce: nonce, Body: bodyRaw})
	return DecodeRequest(wire, now, time.Second)
}

func localSnapshotFingerprint(body any) string {
	switch value := body.(type) {
	case LaunchRequest:
		return value.SnapshotFingerprint
	case ResultRequest:
		return value.SnapshotFingerprint
	case CancelRequest:
		return value.Authority.SnapshotFingerprint
	default:
		return ""
	}
}

func awaitLocalResult(ctx context.Context, service *Service, attempt AttemptRef, snapshot string, privateKey ed25519.PrivateKey) (localWorkloadResult, error) {
	for index := 0; index < 100; index++ {
		requestID := fmt.Sprintf("rpc_%016x", index+2)
		nonce := fmt.Sprintf("localtestresult%018x", index)
		body := ResultRequest{Attempt: attempt, SnapshotFingerprint: snapshot, Name: localResultName}
		request, err := signedLocalRequest(MethodResult, requestID, nonce, body, attempt, privateKey)
		if err != nil {
			return localWorkloadResult{}, err
		}
		response, handleErr := service.Handle(ctx, localTestCaller, request)
		if handleErr == nil {
			result, ok := response.Body.(ResultResult)
			if ok && result.Ready && result.Error == nil {
				var workload localWorkloadResult
				if strictjson.Decode(result.Payload, 8<<10, &workload) != nil {
					return localWorkloadResult{}, ErrArtifactDenied
				}
				return workload, nil
			}
		} else if !errors.Is(handleErr, ErrNotFound) {
			return localWorkloadResult{}, handleErr
		}
		select {
		case <-ctx.Done():
			return localWorkloadResult{}, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return localWorkloadResult{}, ErrRuntimeUnavailable
}

func cancelLocalSmoke(ctx context.Context, service *Service, attempt AttemptRef, privateKey ed25519.PrivateKey, mode string) error {
	body := CancelRequest{Attempt: attempt, Mode: mode, ReasonCode: "local_smoke_complete"}
	// Cancel's snapshot authority is carried only by its ticket. Seed it while
	// deriving the unsigned request fingerprint and replace it with the signed
	// ticket in signedLocalRequest.
	body.Authority.SnapshotFingerprint = localFingerprint("snapshot")
	request, err := signedLocalRequest(MethodCancel, "rpc_ffffffffffffffff", strings.Repeat("c", 32), body, attempt, privateKey)
	if err != nil {
		return err
	}
	response, err := service.Handle(ctx, localTestCaller, request)
	if err != nil {
		return err
	}
	result, ok := response.Body.(CancelResult)
	if !ok || !result.Accepted || result.Error != nil {
		return ErrRuntimeUnavailable
	}
	return nil
}

func residueInLocalState(root string) bool {
	for _, name := range []string{"sandboxes", "scratch", "artifacts"} {
		entries, err := os.ReadDir(filepath.Join(root, name))
		if err != nil || len(entries) != 0 {
			return true
		}
	}
	return false
}
