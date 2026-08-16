package mcpclient

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	protocol "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestDirectSessionFiltersManifestToolsAndRejectsUnallowedCall(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	server := protocol.NewServer(
		&protocol.Implementation{Name: "fixture", Version: "1.0.0"}, nil,
	)
	var unsafeCalls atomic.Int32
	protocol.AddTool(server, &protocol.Tool{Name: "safe"}, func(
		context.Context, *protocol.CallToolRequest, struct{},
	) (*protocol.CallToolResult, any, error) {
		return &protocol.CallToolResult{Content: []protocol.Content{
			&protocol.TextContent{Text: "ok"},
		}}, nil, nil
	})
	protocol.AddTool(server, &protocol.Tool{Name: "unsafe"}, func(
		context.Context, *protocol.CallToolRequest, struct{},
	) (*protocol.CallToolResult, any, error) {
		unsafeCalls.Add(1)
		return &protocol.CallToolResult{}, nil, nil
	})

	serverTransport, clientTransport := protocol.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := protocol.NewClient(
		&protocol.Implementation{Name: "fixture-client", Version: "1.0.0"}, nil,
	)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	session := &directSession{
		server: Server{
			Ref: ServerRef{Source: SourceManifest, ID: "reviewed-remote"},
			Metadata: map[string]any{
				manifestAllowedTools: []string{"safe"},
				"toolPolicy":         map[string]string{"safe": ClassificationRead},
			},
		},
		connection:  clientSession,
		listChanged: &atomic.Bool{},
	}
	t.Cleanup(func() { _ = session.Close() })

	tools, err := session.ListTools(ctx)
	if err != nil || len(tools) != 1 || tools[0].Name != "safe" ||
		tools[0].Classification != ClassificationRead {
		t.Fatalf("filtered direct Tools = %#v error=%v", tools, err)
	}
	if _, err := session.CallTool(ctx, "unsafe", map[string]any{}); !errors.Is(err, ErrToolNotFound) {
		t.Fatalf("unsafe direct CallTool error = %v, want ErrToolNotFound", err)
	}
	if unsafeCalls.Load() != 0 {
		t.Fatalf("unsafe direct Tool executed %d time(s)", unsafeCalls.Load())
	}
	result, err := session.CallTool(ctx, "safe", map[string]any{})
	if err != nil || len(result.Content) != 1 || result.Content[0].Text != "ok" {
		t.Fatalf("safe direct result = %#v error=%v", result, err)
	}
}
