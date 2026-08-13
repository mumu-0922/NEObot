package mcpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
)

type Session interface {
	ListTools(context.Context) ([]Tool, error)
	CallTool(context.Context, string, map[string]any) (CallResult, error)
	Close() error
}

type Connector interface {
	Connect(context.Context, Server, string) (Session, error)
}

type DirectConnector struct {
	clientName    string
	clientVersion string
}

func NewDirectConnector(clientName, clientVersion string) *DirectConnector {
	return &DirectConnector{
		clientName:    nonEmpty(strings.TrimSpace(clientName), "neo-chat"),
		clientVersion: nonEmpty(strings.TrimSpace(clientVersion), "dev"),
	}
}

func (c *DirectConnector) Connect(
	ctx context.Context,
	server Server,
	credential string,
) (Session, error) {
	if server.Transport != TransportStreamableHTTP {
		return nil, ErrServerUnavailable
	}
	policy := NetworkPolicy{RequireHTTPS: server.Ref.Source == SourcePrivate, AllowPrivate: server.Ref.Source == SourceManifest}
	endpoint, err := ValidateEndpoint(ctx, server.EndpointURL, policy)
	if err != nil {
		return nil, err
	}
	headers := make(http.Header)
	switch server.AuthType {
	case AuthNone:
	case AuthHeader:
		if strings.TrimSpace(credential) == "" || server.HeaderAuth == nil ||
			!validHeaderName(server.HeaderAuth.Name) {
			return nil, ErrCredentialRequired
		}
		headers.Set(server.HeaderAuth.Name, server.HeaderAuth.Prefix+credential)
	case AuthOAuth:
		if strings.TrimSpace(credential) == "" {
			return nil, ErrCredentialRequired
		}
		headers.Set("Authorization", "Bearer "+credential)
	case AuthEnv:
		return nil, ErrCredentialInvalid
	default:
		return nil, ErrCredentialInvalid
	}
	httpClient := NewSafeHTTPClient(policy, endpoint, headers, 0)
	changed := &atomic.Bool{}
	client := protocol.NewClient(
		&protocol.Implementation{Name: c.clientName, Version: c.clientVersion},
		&protocol.ClientOptions{
			Capabilities: &protocol.ClientCapabilities{},
			ToolListChangedHandler: func(context.Context, *protocol.ToolListChangedRequest) {
				changed.Store(true)
			},
		},
	)
	transport := &protocol.StreamableClientTransport{
		Endpoint:   endpoint.String(),
		HTTPClient: httpClient,
		MaxRetries: -1,
	}
	connection, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("connect streamable mcp: %w", ErrServerUnavailable)
	}
	return &directSession{
		server:      server,
		connection:  connection,
		listChanged: changed,
	}, nil
}

type directSession struct {
	server      Server
	connection  *protocol.ClientSession
	listChanged *atomic.Bool
}

func (s *directSession) ListTools(ctx context.Context) ([]Tool, error) {
	if s == nil || s.connection == nil {
		return nil, ErrServerUnavailable
	}
	tools := []Tool{}
	for raw, err := range s.connection.Tools(ctx, nil) {
		if err != nil {
			return nil, fmt.Errorf("list mcp tools: %w", ErrServerUnavailable)
		}
		classification := ClassificationUnknown
		if s.server.Ref.Source == SourceManifest {
			if policy, ok := s.server.Metadata["toolPolicy"].(map[string]string); ok {
				classification = normalizeClassification(policy[raw.Name])
			}
		}
		tool := normalizeTool(
			s.server.Ref,
			raw.Name,
			raw.Title,
			raw.Description,
			raw.InputSchema,
			classification,
		)
		tools = append(tools, tool)
	}
	return sortedTools(tools), nil
}

func (s *directSession) CallTool(
	ctx context.Context,
	name string,
	arguments map[string]any,
) (CallResult, error) {
	if s == nil || s.connection == nil {
		return CallResult{}, ErrServerUnavailable
	}
	result, err := s.connection.CallTool(ctx, &protocol.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		if ctx.Err() != nil {
			return CallResult{}, ctx.Err()
		}
		return CallResult{}, fmt.Errorf("call mcp tool: %w", ErrServerUnavailable)
	}
	return normalizeProtocolResult(result)
}

func (s *directSession) Close() error {
	if s == nil || s.connection == nil {
		return nil
	}
	return s.connection.Close()
}

func normalizeProtocolResult(result *protocol.CallToolResult) (CallResult, error) {
	if result == nil {
		return CallResult{}, ErrServerUnavailable
	}
	normalized := CallResult{IsError: result.IsError}
	for _, item := range result.Content {
		switch value := item.(type) {
		case *protocol.TextContent:
			normalized.Content = append(normalized.Content, Content{Type: "text", Text: value.Text, ByteSize: int64(len(value.Text))})
		case *protocol.ImageContent:
			normalized.Content = append(normalized.Content, Content{Type: "image", Data: append([]byte(nil), value.Data...), MIMEType: value.MIMEType, ByteSize: int64(len(value.Data))})
		case *protocol.AudioContent:
			normalized.Content = append(normalized.Content, Content{Type: "audio", Data: append([]byte(nil), value.Data...), MIMEType: value.MIMEType, ByteSize: int64(len(value.Data))})
		case *protocol.ResourceLink:
			normalized.Content = append(normalized.Content, Content{Type: "resource_link", URI: value.URI, Name: value.Name, MIMEType: value.MIMEType})
		case *protocol.EmbeddedResource:
			if value.Resource == nil {
				continue
			}
			content := Content{Type: "resource", URI: value.Resource.URI, MIMEType: value.Resource.MIMEType}
			if value.Resource.Text != "" {
				content.Text = value.Resource.Text
				content.ByteSize = int64(len(value.Resource.Text))
			} else if len(value.Resource.Blob) > 0 {
				content.Data = append([]byte(nil), value.Resource.Blob...)
				content.ByteSize = int64(len(value.Resource.Blob))
			}
			normalized.Content = append(normalized.Content, content)
		default:
			return CallResult{}, ErrResponseTooLarge
		}
	}
	if result.StructuredContent != nil {
		encoded, err := json.Marshal(result.StructuredContent)
		if err != nil {
			return CallResult{}, ErrResponseTooLarge
		}
		var object map[string]any
		if err := json.Unmarshal(encoded, &object); err != nil {
			object = map[string]any{"value": result.StructuredContent}
		}
		normalized.Content = append(normalized.Content, Content{
			Type:     "json",
			JSON:     object,
			ByteSize: int64(len(encoded)),
		})
	}
	return normalized, nil
}

func nonEmpty(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
