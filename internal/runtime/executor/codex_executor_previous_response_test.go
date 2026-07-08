package executor

import (
	"context"
	"testing"

	"github.com/tidwall/gjson"
)

func TestNormalizePreviousResponseID_DropsForFullTranscript(t *testing.T) {
	raw := []byte(`{
		"previous_response_id": "resp_123",
		"prompt_cache_key": "pc-abc",
		"input": [
			{"type": "message", "role": "assistant", "content": "Hello"},
			{"type": "message", "role": "user", "content": "Hi"}
		]
	}`)
	ctx := context.Background()
	result := normalizeCodexPreviousResponseIDForPromptCache(ctx, "openai-response", raw)
	// Should drop previous_response_id for full transcript
	if gjson.GetBytes(result, "previous_response_id").Exists() {
		t.Error("previous_response_id should be dropped for full transcript")
	}
	// prompt_cache_key should be preserved
	if !gjson.GetBytes(result, "prompt_cache_key").Exists() {
		t.Error("prompt_cache_key should be preserved")
	}
}

func TestNormalizePreviousResponseID_PreservesForIncremental(t *testing.T) {
	raw := []byte(`{
		"previous_response_id": "resp_123",
		"prompt_cache_key": "pc-abc",
		"input": [
			{"type": "text", "text": "Just some text input"}
		]
	}`)
	ctx := context.Background()
	result := normalizeCodexPreviousResponseIDForPromptCache(ctx, "openai-response", raw)
	// Should preserve previous_response_id for incremental (not full transcript)
	if !gjson.GetBytes(result, "previous_response_id").Exists() {
		t.Error("previous_response_id should be preserved for incremental input")
	}
}

func TestNormalizePreviousResponseID_NoOpWithoutPreviousResponseID(t *testing.T) {
	raw := []byte(`{
		"prompt_cache_key": "pc-abc",
		"input": [
			{"type": "message", "role": "assistant", "content": "Hello"}
		]
	}`)
	ctx := context.Background()
	result := normalizeCodexPreviousResponseIDForPromptCache(ctx, "openai-response", raw)
	// No previous_response_id to drop — result should be unchanged
	if gjson.GetBytes(result, "previous_response_id").Exists() {
		t.Error("previous_response_id should not appear in output when absent from input")
	}
}

func TestNormalizePreviousResponseID_NoOpWithoutPromptCacheKey(t *testing.T) {
	raw := []byte(`{
		"previous_response_id": "resp_123",
		"input": [
			{"type": "message", "role": "assistant", "content": "Hello"}
		]
	}`)
	ctx := context.Background()
	result := normalizeCodexPreviousResponseIDForPromptCache(ctx, "openai-response", raw)
	// No prompt_cache_key — should not drop previous_response_id
	if !gjson.GetBytes(result, "previous_response_id").Exists() {
		t.Error("previous_response_id should be preserved when prompt_cache_key absent")
	}
}

func TestNormalizePreviousResponseID_NoOpForNonOpenAI(t *testing.T) {
	raw := []byte(`{
		"previous_response_id": "resp_123",
		"prompt_cache_key": "pc-abc",
		"input": [
			{"type": "message", "role": "assistant", "content": "Hello"}
		]
	}`)
	ctx := context.Background()
	result := normalizeCodexPreviousResponseIDForPromptCache(ctx, "anthropic", raw)
	// Non-OpenAI format — should not modify
	if !gjson.GetBytes(result, "previous_response_id").Exists() {
		t.Error("previous_response_id should be preserved for non-OpenAI format")
	}
}

func TestCodexInputLooksLikeFullTranscript_WithAssistantRole(t *testing.T) {
	raw := []byte(`{
		"input": [
			{"type": "message", "role": "assistant", "content": "Hello"},
			{"type": "message", "role": "user", "content": "Hi"}
		]
	}`)
	input := gjson.GetBytes(raw, "input")
	if !codexInputLooksLikeFullTranscript(input) {
		t.Error("expected full transcript with assistant role")
	}
}

func TestCodexInputLooksLikeFullTranscript_WithFunctionCall(t *testing.T) {
	raw := []byte(`{
		"input": [
			{"type": "function_call", "function": "test"},
			{"type": "message", "role": "user", "content": "Hi"}
		]
	}`)
	input := gjson.GetBytes(raw, "input")
	if !codexInputLooksLikeFullTranscript(input) {
		t.Error("expected full transcript with function_call type")
	}
}

func TestCodexInputLooksLikeFullTranscript_WithCustomToolCall(t *testing.T) {
	raw := []byte(`{
		"input": [
			{"type": "custom_tool_call", "tool": "test"},
			{"type": "message", "role": "user", "content": "Hi"}
		]
	}`)
	input := gjson.GetBytes(raw, "input")
	if !codexInputLooksLikeFullTranscript(input) {
		t.Error("expected full transcript with custom_tool_call type")
	}
}

func TestCodexInputLooksLikeFullTranscript_NotFullTranscript(t *testing.T) {
	raw := []byte(`{
		"input": [
			{"type": "text", "text": "Just text"}
		]
	}`)
	input := gjson.GetBytes(raw, "input")
	if codexInputLooksLikeFullTranscript(input) {
		t.Error("expected NOT full transcript for plain text input")
	}
}

func TestCodexInputLooksLikeFullTranscript_NoInput(t *testing.T) {
	raw := []byte(`{}`)
	input := gjson.GetBytes(raw, "input")
	if codexInputLooksLikeFullTranscript(input) {
		t.Error("expected NOT full transcript for missing input")
	}
}
