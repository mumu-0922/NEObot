package mcpclient

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxRunnerControlRequestBytes = 128 << 10
	maxRunnerResponseBytes       = int64(64 << 20)
	runnerResponseHeaderTimeout  = 2 * time.Minute
)

type RunnerConnector struct {
	baseURL *url.URL
	token   string
	client  *http.Client
}

type RoutingConnector struct {
	remote Connector
	runner Connector
}

func NewRoutingConnector(remote, runner Connector) *RoutingConnector {
	return &RoutingConnector{remote: remote, runner: runner}
}

func (c *RoutingConnector) Connect(ctx context.Context, server Server, credential string) (Session, error) {
	if server.Transport == TransportStdio {
		if c == nil || c.runner == nil {
			return nil, ErrServerUnavailable
		}
		return c.runner.Connect(ctx, server, credential)
	}
	if c == nil || c.remote == nil {
		return nil, ErrServerUnavailable
	}
	return c.remote.Connect(ctx, server, credential)
}

func NewRunnerConnector(rawURL, token string) (*RunnerConnector, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(rawURL), "/"))
	token = strings.TrimSpace(token)
	if err != nil || parsed.Scheme != "http" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || len(token) < 32 || len(token) > 4096 ||
		strings.ContainsAny(token, "\r\n") {
		return nil, ErrServerUnavailable
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.MaxIdleConns = 8
	transport.MaxIdleConnsPerHost = 8
	// A cold dynamic npm artifact must download before MCP initialize returns.
	// Keep this bounded but longer than an ordinary already-running Tool call.
	transport.ResponseHeaderTimeout = runnerResponseHeaderTimeout
	transport.MaxResponseHeaderBytes = 64 << 10
	return &RunnerConnector{
		baseURL: parsed,
		token:   token,
		client:  &http.Client{Transport: transport},
	}, nil
}

func (c *RunnerConnector) Connect(_ context.Context, server Server, credential string) (Session, error) {
	if c == nil || c.baseURL == nil || server.Transport != TransportStdio {
		return nil, ErrServerUnavailable
	}
	runnerServerID := ""
	switch server.Ref.Source {
	case SourceManifest:
		if server.Command != nil {
			runnerServerID = server.Ref.ID
		}
	case SourcePrivate:
		runnerServerID, _ = server.Metadata["runnerArtifactId"].(string)
		runnerServerID = strings.TrimSpace(runnerServerID)
	}
	if !manifestIDPattern.MatchString(runnerServerID) {
		return nil, ErrServerUnavailable
	}
	instanceID := runnerServerID
	if server.Ref.Source == SourceManifest {
		if scoped, ok := server.Metadata[runnerInstanceID].(string); ok {
			instanceID = strings.TrimSpace(scoped)
		}
		if !manifestIDPattern.MatchString(instanceID) {
			return nil, ErrServerUnavailable
		}
	}
	environment := map[string]string{}
	if server.Ref.Source == SourcePrivate && server.AuthType == AuthEnv {
		instanceID = server.Ref.ID
		if !validUUID(instanceID) || json.Unmarshal([]byte(credential), &environment) != nil || len(environment) == 0 {
			return nil, ErrCredentialRequired
		}
	}
	artifact, _ := server.Metadata["dynamicRunnerArtifactResolved"].(DynamicRunnerArtifact)
	return &runnerSession{
		connector: c, server: server, runnerServerID: runnerServerID,
		instanceID: instanceID, environment: environment, artifact: artifact,
	}, nil
}

type runnerSession struct {
	connector      *RunnerConnector
	server         Server
	runnerServerID string
	instanceID     string
	environment    map[string]string
	artifact       DynamicRunnerArtifact
}

func (s *runnerSession) ListTools(ctx context.Context) ([]Tool, error) {
	var response struct {
		Tools []*protocol.Tool `json:"tools"`
	}
	if err := s.connector.post(ctx, "/internal/v1/tools/list", map[string]any{
		"serverId": s.runnerServerID, "instanceId": s.instanceID,
		"environment": s.connector.sealEnvironment(s.runnerServerID, s.instanceID, s.environment),
		"artifact":    s.connector.sealArtifact(s.runnerServerID, s.instanceID, s.artifact),
	}, &response); err != nil {
		return nil, err
	}
	tools := make([]Tool, 0, len(response.Tools))
	for _, raw := range response.Tools {
		if raw == nil {
			continue
		}
		if !manifestToolAllowed(s.server, raw.Name) {
			continue
		}
		classification := ClassificationUnknown
		if policy, ok := s.server.Metadata["toolPolicy"].(map[string]string); ok {
			classification = normalizeClassification(policy[raw.Name])
		}
		tools = append(tools, normalizeTool(
			s.server.Ref, raw.Name, raw.Title, raw.Description, raw.InputSchema, classification,
		))
	}
	return sortedTools(tools), nil
}

func (s *runnerSession) CallTool(
	ctx context.Context,
	name string,
	arguments map[string]any,
) (CallResult, error) {
	if !manifestToolAllowed(s.server, name) {
		return CallResult{}, ErrToolNotFound
	}
	var response struct {
		Result *protocol.CallToolResult `json:"result"`
	}
	if err := s.connector.post(ctx, "/internal/v1/tools/call", map[string]any{
		"serverId": s.runnerServerID, "instanceId": s.instanceID,
		"environment": s.connector.sealEnvironment(s.runnerServerID, s.instanceID, s.environment),
		"artifact":    s.connector.sealArtifact(s.runnerServerID, s.instanceID, s.artifact),
		"name":        name, "arguments": arguments,
	}, &response); err != nil {
		return CallResult{}, err
	}
	return normalizeProtocolResult(response.Result)
}

func (s *runnerSession) Close() error { return nil }

type RunnerEnvironmentEnvelope struct {
	Nonce      string `json:"nonce,omitempty"`
	Ciphertext string `json:"ciphertext,omitempty"`
}

type RunnerArtifactEnvelope struct {
	Nonce      string `json:"nonce,omitempty"`
	Ciphertext string `json:"ciphertext,omitempty"`
}

func (c *RunnerConnector) sealEnvironment(serverID, instanceID string, environment map[string]string) RunnerEnvironmentEnvelope {
	if len(environment) == 0 {
		return RunnerEnvironmentEnvelope{}
	}
	plaintext, err := json.Marshal(environment)
	if err != nil {
		return RunnerEnvironmentEnvelope{}
	}
	defer clear(plaintext)
	nonce, sealed, err := sealRunnerPayload(c.token, "env", serverID, instanceID, plaintext)
	if err != nil {
		return RunnerEnvironmentEnvelope{}
	}
	return RunnerEnvironmentEnvelope{Nonce: nonce, Ciphertext: sealed}
}

func (c *RunnerConnector) sealArtifact(serverID, instanceID string, artifact DynamicRunnerArtifact) RunnerArtifactEnvelope {
	if artifact.ID == "" {
		return RunnerArtifactEnvelope{}
	}
	plaintext, err := json.Marshal(artifact)
	if err != nil {
		return RunnerArtifactEnvelope{}
	}
	defer clear(plaintext)
	nonce, sealed, err := sealRunnerPayload(c.token, "artifact", serverID, instanceID, plaintext)
	if err != nil {
		return RunnerArtifactEnvelope{}
	}
	return RunnerArtifactEnvelope{Nonce: nonce, Ciphertext: sealed}
}

func sealRunnerPayload(token, purpose, serverID, instanceID string, plaintext []byte) (string, string, error) {
	key := sha256.Sum256([]byte("neo-chat:mcp-runner-" + purpose + ":" + token))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", "", err
	}
	sealed := gcm.Seal(nil, nonce, plaintext, []byte(purpose+"\x00"+serverID+"\x00"+instanceID))
	return base64.RawURLEncoding.EncodeToString(nonce), base64.RawURLEncoding.EncodeToString(sealed), nil
}

func OpenRunnerEnvironment(token, serverID, instanceID string, envelope RunnerEnvironmentEnvelope) (map[string]string, error) {
	if envelope.Nonce == "" && envelope.Ciphertext == "" {
		return map[string]string{}, nil
	}
	nonce, nonceErr := base64.RawURLEncoding.DecodeString(envelope.Nonce)
	sealed, sealedErr := base64.RawURLEncoding.DecodeString(envelope.Ciphertext)
	key := sha256.Sum256([]byte("neo-chat:mcp-runner-env:" + token))
	block, err := aes.NewCipher(key[:])
	if nonceErr != nil || sealedErr != nil || err != nil {
		return nil, ErrCredentialInvalid
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(nonce) != gcm.NonceSize() {
		return nil, ErrCredentialInvalid
	}
	plaintext, err := gcm.Open(nil, nonce, sealed, []byte("env\x00"+serverID+"\x00"+instanceID))
	if err != nil {
		return nil, ErrCredentialInvalid
	}
	defer clear(plaintext)
	environment := map[string]string{}
	if json.Unmarshal(plaintext, &environment) != nil {
		return nil, ErrCredentialInvalid
	}
	return environment, nil
}

func OpenRunnerArtifact(token, serverID, instanceID string, envelope RunnerArtifactEnvelope) (DynamicRunnerArtifact, error) {
	if envelope.Nonce == "" && envelope.Ciphertext == "" {
		return DynamicRunnerArtifact{}, nil
	}
	nonce, nonceErr := base64.RawURLEncoding.DecodeString(envelope.Nonce)
	sealed, sealedErr := base64.RawURLEncoding.DecodeString(envelope.Ciphertext)
	key := sha256.Sum256([]byte("neo-chat:mcp-runner-artifact:" + token))
	block, err := aes.NewCipher(key[:])
	if nonceErr != nil || sealedErr != nil || err != nil {
		return DynamicRunnerArtifact{}, ErrCredentialInvalid
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(nonce) != gcm.NonceSize() {
		return DynamicRunnerArtifact{}, ErrCredentialInvalid
	}
	plaintext, err := gcm.Open(nil, nonce, sealed, []byte("artifact\x00"+serverID+"\x00"+instanceID))
	if err != nil {
		return DynamicRunnerArtifact{}, ErrCredentialInvalid
	}
	defer clear(plaintext)
	var artifact DynamicRunnerArtifact
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&artifact) != nil || artifact.ID == "" {
		return DynamicRunnerArtifact{}, ErrCredentialInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return DynamicRunnerArtifact{}, ErrCredentialInvalid
	}
	return artifact, nil
}

func (c *RunnerConnector) post(ctx context.Context, path string, input any, output any) error {
	encoded, err := json.Marshal(input)
	if err != nil || len(encoded) > maxRunnerControlRequestBytes {
		return ErrServerUnavailable
	}
	endpoint := *c.baseURL
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + path
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(encoded))
	if err != nil {
		return ErrServerUnavailable
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := c.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrServerUnavailable
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxRunnerResponseBytes+1))
	if err != nil || int64(len(data)) > maxRunnerResponseBytes {
		return ErrResponseTooLarge
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ErrServerUnavailable
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(output); err != nil {
		return ErrServerUnavailable
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return ErrServerUnavailable
	}
	return nil
}
