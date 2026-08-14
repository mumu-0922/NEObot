package agentbrokercanary

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/sys/unix"

	"neo-chat/mm-chat/backend/internal/agentbroker"
	"neo-chat/mm-chat/backend/internal/mcpclient"
	"neo-chat/mm-chat/backend/internal/strictjson"
)

type ExactFileReader struct {
	action ActionPlan
}

func NewExactFileReader(action ActionPlan) (*ExactFileReader, error) {
	if (action.ID != ActionProjectRead && action.ID != ActionWorkspaceRead) ||
		action.File == nil || validateFilePlan(*action.File) != nil {
		return nil, ErrInvalidPlan
	}
	return &ExactFileReader{action: action}, nil
}

func (reader *ExactFileReader) Read(ctx context.Context, request agentbroker.ExecutionRequest) (json.RawMessage, error) {
	if reader == nil || reader.action.File == nil || !matchesExecution(reader.action, request) {
		return nil, agentbroker.ErrGrantDenied
	}
	file, err := openBeneath(reader.action.File.Root, reader.action.File.RelativePath, false)
	if err != nil {
		return nil, agentbroker.ErrExecutorUnavailable
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != reader.action.File.ExpectedBytes ||
		info.Size() > reader.action.File.MaxBytes {
		return nil, agentbroker.ErrExecutorUnavailable
	}
	payload, err := io.ReadAll(io.LimitReader(file, reader.action.File.MaxBytes+1))
	if err != nil || int64(len(payload)) != info.Size() || ctx.Err() != nil {
		clear(payload)
		return nil, agentbroker.ErrExecutorUnavailable
	}
	fingerprint := contentFingerprint(payload)
	clear(payload)
	if fingerprint != reader.action.File.ExpectedFingerprint {
		return nil, agentbroker.ErrSnapshotMismatch
	}
	result, _ := json.Marshal(struct {
		ByteCount   int64  `json:"byteCount"`
		Fingerprint string `json:"fingerprint"`
	}{info.Size(), fingerprint})
	return result, nil
}

type MCPReadExecutor struct {
	action    ActionPlan
	connector mcpclient.Connector
	server    mcpclient.Server
}

func NewMCPReadExecutor(action ActionPlan, connector mcpclient.Connector) (*MCPReadExecutor, error) {
	if action.ID != ActionMCPRead || action.MCP == nil || validateMCPPlan(*action.MCP) != nil || connector == nil {
		return nil, ErrInvalidPlan
	}
	server := mcpclient.Server{Ref: mcpclient.ServerRef{Source: mcpclient.SourceManifest, ID: action.MCP.ServerID},
		Name: action.MCP.ServerName, Transport: mcpclient.TransportStdio, AuthType: mcpclient.AuthNone,
		Status: mcpclient.ServerStatusReady, Command: &mcpclient.Command{
			Argv: append([]string(nil), action.MCP.CommandArgv...), Env: map[string]string{}, UserSecretEnv: []string{}},
		Metadata: map[string]any{"toolPolicy": clonePolicy(action.MCP.ToolPolicy)}}
	return &MCPReadExecutor{action: action, connector: connector, server: server}, nil
}

func (executor *MCPReadExecutor) Read(ctx context.Context, request agentbroker.ExecutionRequest) (json.RawMessage, error) {
	if executor == nil || executor.action.MCP == nil || executor.connector == nil ||
		!matchesExecution(executor.action, request) {
		return nil, agentbroker.ErrGrantDenied
	}
	arguments := map[string]any{}
	if strictjson.Decode(request.Arguments, 256<<10, &arguments) != nil {
		return nil, agentbroker.ErrGrantDenied
	}
	session, err := executor.connector.Connect(ctx, executor.server, "")
	if err != nil {
		return nil, agentbroker.ErrExecutorUnavailable
	}
	defer session.Close()
	tools, err := session.ListTools(ctx)
	if err != nil {
		return nil, agentbroker.ErrExecutorUnavailable
	}
	var selected *mcpclient.Tool
	for index := range tools {
		if tools[index].Name == executor.action.MCP.ToolName {
			selected = &tools[index]
			break
		}
	}
	if selected == nil || !selected.Supported || selected.Classification != mcpclient.ClassificationRead ||
		selected.ServerRef != executor.server.Ref || mcpclient.ValidateToolArguments(*selected, arguments) != nil {
		return nil, agentbroker.ErrGrantDenied
	}
	result, err := session.CallTool(ctx, selected.Name, arguments)
	if err != nil {
		return nil, &agentbroker.DispatchError{Cause: agentbroker.ErrExecutorUnavailable, PossibleSend: true}
	}
	fingerprint, byteCount, err := fingerprintMCPResult(result, executor.action.MCP.MaxResultBytes)
	if err != nil {
		return nil, &agentbroker.DispatchError{Cause: agentbroker.ErrExecutorUnavailable, PossibleSend: true}
	}
	receipt, _ := json.Marshal(struct {
		Fingerprint  string `json:"fingerprint"`
		ContentCount int    `json:"contentCount"`
		ByteCount    int64  `json:"byteCount"`
		IsError      bool   `json:"isError"`
	}{fingerprint, len(result.Content), byteCount, result.IsError})
	return receipt, nil
}

type PossibleSendExecutor struct {
	action ActionPlan
}

func NewPossibleSendExecutor(action ActionPlan) (*PossibleSendExecutor, error) {
	if action.ID != ActionPossibleSend || validateActionShape(action, Plan{}) != nil {
		return nil, ErrInvalidPlan
	}
	return &PossibleSendExecutor{action: action}, nil
}

func (executor *PossibleSendExecutor) Read(_ context.Context, request agentbroker.ExecutionRequest) (json.RawMessage, error) {
	if executor == nil || !matchesExecution(executor.action, request) {
		return nil, agentbroker.ErrGrantDenied
	}
	return nil, &agentbroker.DispatchError{Cause: agentbroker.ErrExecutorUnavailable, PossibleSend: true}
}

type ArtifactExecutor struct {
	action    ActionPlan
	policy    ArtifactPolicy
	publisher *agentbroker.ArtifactPublisher
}

func NewArtifactExecutor(action ActionPlan, policy ArtifactPolicy,
	publisher *agentbroker.ArtifactPublisher,
) (*ArtifactExecutor, error) {
	if action.ID != ActionArtifact || action.Artifact == nil || publisher == nil ||
		policy.MaxBytes < 1 || policy.MaxBytes > maximumArtifactBytes ||
		!containsString(policy.AllowedMediaTypes, action.Artifact.MediaType) ||
		action.Artifact.SizeBytes > policy.MaxBytes {
		return nil, ErrInvalidPlan
	}
	return &ArtifactExecutor{action: action, policy: policy, publisher: publisher}, nil
}

func (executor *ArtifactExecutor) Commit(ctx context.Context, request agentbroker.ExecutionRequest) (agentbroker.ExecutorReceipt, error) {
	if executor == nil || executor.action.Artifact == nil || !matchesExecution(executor.action, request) {
		return agentbroker.ExecutorReceipt{}, agentbroker.ErrGrantDenied
	}
	artifact := executor.action.Artifact
	candidate := agentbroker.ArtifactCandidate{ArtifactID: stablePlanID("artifact", request.IntentID, artifact.Name),
		IntentID: request.IntentID, UserID: request.UserID, RunID: request.RunID,
		AttemptID: request.AttemptID, Generation: request.Generation,
		SnapshotFingerprint: request.SnapshotFingerprint, GrantFingerprint: request.GrantFingerprint,
		RegistryFingerprint: request.RegistryFingerprint, QuarantineRef: request.AttemptID + "/" + artifact.Name,
		Name: artifact.Name, MediaType: artifact.MediaType, Size: artifact.SizeBytes,
		Fingerprint: artifact.Fingerprint}
	authority := agentbroker.ArtifactAuthority{UserID: request.UserID, RunID: request.RunID,
		AttemptID: request.AttemptID, Generation: request.Generation, MaxBytes: executor.policy.MaxBytes,
		AllowedMedia: append([]string(nil), executor.policy.AllowedMediaTypes...), Current: true,
		GrantAuthorized: true}
	objectKey, err := executor.publisher.Publish(ctx, authority, candidate)
	if err != nil {
		return agentbroker.ExecutorReceipt{}, err
	}
	return agentbroker.ExecutorReceipt{ReceiptFingerprint: domainFingerprint("neo-agent-artifact-receipt-v1", []byte(objectKey))}, nil
}

func (*ArtifactExecutor) Status(context.Context, agentbroker.ExecutionRequest) (agentbroker.ExecutorStatus, error) {
	return agentbroker.ExecutorStatus{}, agentbroker.ErrExecutorUnavailable
}

// DirectoryQuarantine exposes only exact Attempt/name entries under one
// private root. openat2 prevents parent symlink, magic-link and traversal
// substitution; Delete repeats the same parent walk before unlink.
type DirectoryQuarantine struct {
	root string
}

func NewDirectoryQuarantine(root string) (*DirectoryQuarantine, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if !filepath.IsAbs(root) {
		return nil, ErrInvalidPlan
	}
	file, err := openBeneath(root, ".", true)
	if err != nil {
		return nil, ErrInvalidPlan
	}
	info, statErr := file.Stat()
	closeErr := file.Close()
	if statErr != nil || closeErr != nil || !info.IsDir() || info.Mode().Perm()&0o022 != 0 {
		return nil, ErrInvalidPlan
	}
	return &DirectoryQuarantine{root: root}, nil
}

func (store *DirectoryQuarantine) Open(_ context.Context, reference string) (io.ReadCloser, error) {
	if store == nil || !validQuarantineReference(reference) {
		return nil, agentbroker.ErrArtifactDenied
	}
	return openBeneath(store.root, reference, false)
}

func (store *DirectoryQuarantine) Delete(_ context.Context, reference string) error {
	if store == nil || !validQuarantineReference(reference) {
		return agentbroker.ErrArtifactDenied
	}
	parts := strings.Split(reference, "/")
	parent, err := openBeneath(store.root, parts[0], true)
	if err != nil {
		return err
	}
	defer parent.Close()
	if err := unix.Unlinkat(int(parent.Fd()), parts[1], 0); err != nil && !errors.Is(err, unix.ENOENT) {
		return err
	}
	return nil
}

type CanaryArtifactScanner struct{}

func (CanaryArtifactScanner) Scan(_ context.Context, candidate agentbroker.ArtifactCandidate, reader io.Reader) error {
	payload, err := io.ReadAll(io.LimitReader(reader, candidate.Size+1))
	if err != nil || int64(len(payload)) != candidate.Size {
		clear(payload)
		return agentbroker.ErrArtifactDenied
	}
	defer clear(payload)
	switch candidate.MediaType {
	case "text/plain":
		if !utf8.Valid(payload) || bytes.IndexByte(payload, 0) >= 0 {
			return agentbroker.ErrArtifactDenied
		}
	case "application/json":
		var value any
		if strictjson.Decode(payload, int(candidate.Size), &value) != nil {
			return agentbroker.ErrArtifactDenied
		}
	default:
		return agentbroker.ErrArtifactDenied
	}
	return nil
}

func matchesExecution(action ActionPlan, request agentbroker.ExecutionRequest) bool {
	if request.ToolIdentity != action.ToolIdentity || request.Capability != action.Capability ||
		request.Action != action.Action || request.Resource != action.Resource || request.IntentID == "" {
		return false
	}
	fingerprint, err := agentbroker.ArgumentsFingerprint(request.Arguments)
	expected, expectedErr := agentbroker.ArgumentsFingerprint(action.Arguments)
	return err == nil && expectedErr == nil && fingerprint == expected &&
		request.ArgumentsFingerprint == expected
}

func openBeneath(root, relative string, directory bool) (*os.File, error) {
	rootFD, err := unix.Open(root, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	defer unix.Close(rootFD)
	flags := uint64(unix.O_RDONLY | unix.O_CLOEXEC | unix.O_NOFOLLOW)
	if directory {
		flags |= unix.O_DIRECTORY
	}
	fd, err := unix.Openat2(rootFD, relative, &unix.OpenHow{Flags: flags,
		Resolve: unix.RESOLVE_BENEATH | unix.RESOLVE_NO_MAGICLINKS | unix.RESOLVE_NO_SYMLINKS})
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), filepath.Join(root, relative)), nil
}

func validQuarantineReference(reference string) bool {
	parts := strings.Split(reference, "/")
	return len(parts) == 2 && strings.HasPrefix(parts[0], "attempt_") && len(parts[0]) >= 24 &&
		parts[1] != "" && parts[1] != "." && parts[1] != ".." &&
		!strings.ContainsAny(parts[1], "\\\x00\r\n")
}

func contentFingerprint(payload []byte) string {
	digest := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func domainFingerprint(domain string, payload []byte) string {
	digest := sha256.Sum256(append(append([]byte(nil), domain...), append([]byte{0}, payload...)...))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func fingerprintMCPResult(result mcpclient.CallResult, maximum int64) (string, int64, error) {
	hash := sha256.New()
	_, _ = io.WriteString(hash, "neo-agent-mcp-read-result-v1\x00")
	var total int64
	for _, content := range result.Content {
		_, _ = io.WriteString(hash, content.Type+"\x00"+content.MIMEType+"\x00"+content.URI+"\x00"+content.Name+"\x00")
		var payload []byte
		switch {
		case len(content.Data) > 0:
			payload = content.Data
		case content.Text != "":
			payload = []byte(content.Text)
		case content.JSON != nil:
			payload, _ = json.Marshal(content.JSON)
		}
		total += int64(len(payload))
		if total > maximum {
			clear(payload)
			return "", 0, fmt.Errorf("mcp result too large")
		}
		_, _ = hash.Write(payload)
		_, _ = hash.Write([]byte{0})
		if len(content.Data) == 0 {
			clear(payload)
		}
	}
	if result.IsError {
		_, _ = io.WriteString(hash, "error")
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), total, nil
}

func clonePolicy(policy map[string]string) map[string]string {
	result := make(map[string]string, len(policy))
	for key, value := range policy {
		result[key] = value
	}
	return result
}
