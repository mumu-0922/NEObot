package mcpclient

import (
	"bytes"
	"context"
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
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.MaxResponseHeaderBytes = 64 << 10
	return &RunnerConnector{
		baseURL: parsed,
		token:   token,
		client:  &http.Client{Transport: transport},
	}, nil
}

func (c *RunnerConnector) Connect(_ context.Context, server Server, _ string) (Session, error) {
	if c == nil || c.baseURL == nil || server.Ref.Source != SourceManifest ||
		server.Transport != TransportStdio || server.Command == nil {
		return nil, ErrServerUnavailable
	}
	return &runnerSession{connector: c, server: server}, nil
}

type runnerSession struct {
	connector *RunnerConnector
	server    Server
}

func (s *runnerSession) ListTools(ctx context.Context) ([]Tool, error) {
	var response struct {
		Tools []*protocol.Tool `json:"tools"`
	}
	if err := s.connector.post(ctx, "/internal/v1/tools/list", map[string]any{
		"serverId": s.server.Ref.ID,
	}, &response); err != nil {
		return nil, err
	}
	tools := make([]Tool, 0, len(response.Tools))
	for _, raw := range response.Tools {
		if raw == nil {
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
	var response struct {
		Result *protocol.CallToolResult `json:"result"`
	}
	if err := s.connector.post(ctx, "/internal/v1/tools/call", map[string]any{
		"serverId": s.server.Ref.ID,
		"name":     name, "arguments": arguments,
	}, &response); err != nil {
		return CallResult{}, err
	}
	return normalizeProtocolResult(response.Result)
}

func (s *runnerSession) Close() error { return nil }

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
