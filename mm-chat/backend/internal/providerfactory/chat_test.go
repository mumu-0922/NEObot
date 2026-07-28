package providerfactory

import (
	"errors"
	"testing"

	"neo-chat/mm-chat/backend/internal/chat"
	"neo-chat/mm-chat/backend/internal/runtimeconfig"
)

func TestNewChatProviderSupportsServerProviderTypes(t *testing.T) {
	tests := []struct {
		name         string
		providerType runtimeconfig.ProviderType
		baseURL      string
		want         any
	}{
		{"openai", runtimeconfig.ProviderTypeOpenAI, "default", (*chat.OpenAIProvider)(nil)},
		{"compatible", runtimeconfig.ProviderTypeOpenAICompatible, "https://example.test/v1", (*chat.OpenAICompatibleProvider)(nil)},
		{"gemini", runtimeconfig.ProviderTypeGemini, "default", (*chat.GeminiProvider)(nil)},
		{"anthropic", runtimeconfig.ProviderTypeAnthropic, "default", (*chat.AnthropicProvider)(nil)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider, err := NewChatProvider(ChatConfig{
				ProviderID: "fixture", Type: test.providerType,
				BaseURL: test.baseURL, APIKey: "fixture-secret",
			})
			if err != nil {
				t.Fatal(err)
			}
			switch test.want.(type) {
			case *chat.OpenAIProvider:
				if _, ok := provider.(*chat.OpenAIProvider); !ok {
					t.Fatalf("provider = %T", provider)
				}
			case *chat.OpenAICompatibleProvider:
				if _, ok := provider.(*chat.OpenAICompatibleProvider); !ok {
					t.Fatalf("provider = %T", provider)
				}
			case *chat.GeminiProvider:
				if _, ok := provider.(*chat.GeminiProvider); !ok {
					t.Fatalf("provider = %T", provider)
				}
			case *chat.AnthropicProvider:
				if _, ok := provider.(*chat.AnthropicProvider); !ok {
					t.Fatalf("provider = %T", provider)
				}
			}
		})
	}
}

func TestNewChatProviderCanPromoteCompatibleToOpenAIResponses(t *testing.T) {
	provider, err := NewChatProvider(ChatConfig{
		ProviderID: "fixture", Type: runtimeconfig.ProviderTypeOpenAICompatible,
		BaseURL: "https://example.test", APIKey: "fixture-secret",
		UseOpenAIResponses: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := provider.(*chat.OpenAIProvider); !ok {
		t.Fatalf("provider = %T", provider)
	}
}

func TestNewChatProviderRejectsUnsupportedType(t *testing.T) {
	_, err := NewChatProvider(ChatConfig{
		ProviderID: "fixture", Type: "unsupported", APIKey: "fixture-secret",
	})
	if !errors.Is(err, ErrUnsupportedChatProvider) {
		t.Fatalf("error = %v", err)
	}
}
