package executor

import (
	"context"
	"testing"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestCodexPromptCacheKeyFromClient_FromPayload(t *testing.T) {
	raw := []byte(`{}`)
	payload := []byte(`{"prompt_cache_key": "from-payload"}`)
	req := cliproxyexecutor.Request{
		Payload: payload,
	}
	key, source := codexPromptCacheKeyFromClient(context.Background(), req, raw)
	if key != "from-payload" {
		t.Errorf("expected key 'from-payload', got '%s'", key)
	}
	if source != "payload" {
		t.Errorf("expected source 'payload', got '%s'", source)
	}
}

func TestCodexPromptCacheKeyFromClient_FromBody(t *testing.T) {
	raw := []byte(`{"promptCacheKey": "from-body"}`)
	payload := []byte(`{}`)
	req := cliproxyexecutor.Request{
		Payload: payload,
	}
	key, source := codexPromptCacheKeyFromClient(context.Background(), req, raw)
	if key != "from-body" {
		t.Errorf("expected key 'from-body', got '%s'", key)
	}
	if source != "body" {
		t.Errorf("expected source 'body', got '%s'", source)
	}
}

func TestCodexPromptCacheKeyFromClient_FromProviderOptions(t *testing.T) {
	raw := []byte(`{"providerOptions": {"openai": {"promptCacheKey": "from-provider-options"}}}`)
	payload := []byte(`{}`)
	req := cliproxyexecutor.Request{
		Payload: payload,
	}
	key, source := codexPromptCacheKeyFromClient(context.Background(), req, raw)
	if key != "from-provider-options" {
		t.Errorf("expected key 'from-provider-options', got '%s'", key)
	}
	if source != "body" {
		t.Errorf("expected source 'body', got '%s'", source)
	}
}

func TestCodexPromptCacheKeyFromClient_FallbackToHeaders(t *testing.T) {
	raw := []byte(`{}`)
	payload := []byte(`{}`)
	req := cliproxyexecutor.Request{
		Payload: payload,
	}
	key, source := codexPromptCacheKeyFromClient(context.Background(), req, raw)
	// Falls through to headers → API key derivation (non-empty)
	if key == "" {
		t.Error("expected non-empty fallback cache key from API key derivation")
	}
	if source != "headers" {
		t.Errorf("expected source 'headers', got '%s'", source)
	}
}

func TestCodexPromptCacheKeyFromJSON_InvalidJSON(t *testing.T) {
	payload := []byte(`{invalid json}`)
	key, _ := codexPromptCacheKeyFromJSON(payload, "test")
	if key != "" {
		t.Errorf("expected empty key for invalid JSON, got '%s'", key)
	}
}

func TestCodexPromptCacheKeyFromJSON_EmptyPayload(t *testing.T) {
	key, _ := codexPromptCacheKeyFromJSON(nil, "test")
	if key != "" {
		t.Errorf("expected empty key for empty payload, got '%s'", key)
	}
}
