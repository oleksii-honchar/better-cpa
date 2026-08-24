package executor

// Ported fork regression tests (b50dba3c + pr-3003 + pr-3141).
//
// These tripwires assert the fork's intended behavior through the UPSTREAM
// executor API surface (cacheHelper with the v7.2.141 signature). They are the
// regression proof for the "drop fork executor code" decision: green means
// upstream covers the fork behavior; red means the drop decision must be
// revisited for that specific behavior (see dispositions in
// session materials/task2-evidence.md).
//
// Fork source of truth:
//   - b50dba3c internal/runtime/executor/codex_executor_cache_key_test.go
//   - pr-3003  internal/runtime/executor/codex_executor_cache_test.go
//   - pr-3141  internal/runtime/executor/codex_continuity.go (resolve chain)

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

// newCodexCacheHelperContext is the ported fork test helper, adapted to the
// upstream gin-context convention (userApiKey is what helps.APIKeyFromContext
// reads on the sync branch).
func newCodexCacheHelperContext(apiKey string, headers map[string]string) context.Context {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	if apiKey != "" {
		ginCtx.Set("userApiKey", apiKey)
	}
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	for name, value := range headers {
		ginCtx.Request.Header.Set(name, value)
	}
	return context.WithValue(context.Background(), "gin", ginCtx)
}

// cacheHelperRequest runs the upstream cacheHelper and returns the final
// upstream body plus the outbound request.
func cacheHelperRequest(t *testing.T, ctx context.Context, from sdktranslator.Format, req cliproxyexecutor.Request, rawJSON []byte) ([]byte, *http.Request) {
	t.Helper()
	executor := &CodexExecutor{}
	url := "https://example.com/responses"
	httpReq, body, _, err := executor.cacheHelper(ctx, from, url, nil, req, req.Payload, rawJSON)
	if err != nil {
		t.Fatalf("cacheHelper error: %v", err)
	}
	if body == nil {
		readBody, errRead := io.ReadAll(httpReq.Body)
		if errRead != nil {
			t.Fatalf("read request body: %v", errRead)
		}
		body = readBody
	}
	return body, httpReq
}

func TestPortedFork_PromptCacheKeyFromPayload_OpenAIResponses(t *testing.T) {
	// b50dba3c TestCodexPromptCacheKeyFromClient_FromPayload
	ctx := newCodexCacheHelperContext("", nil)
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"gpt-5.5","prompt_cache_key":"from-payload"}`),
	}
	body, httpReq := cacheHelperRequest(t, ctx, sdktranslator.FromString("openai-response"), req, []byte(`{"model":"gpt-5.5","stream":true}`))

	if got := gjson.GetBytes(body, "prompt_cache_key").String(); got != "from-payload" {
		t.Fatalf("prompt_cache_key = %q, want from-payload; body=%s", got, string(body))
	}
	if got := httpReq.Header.Get("Session-Id"); got != "from-payload" {
		t.Fatalf("Session-Id = %q, want from-payload", got)
	}
	if got := httpReq.Header.Get("Session_id"); got != "" {
		t.Fatalf("Session_id = %q, want empty (canonical casing only)", got)
	}
}

func TestPortedFork_PromptCacheKeyFromPayload_OpenAIChatCompletions(t *testing.T) {
	// b50dba3c TestCodexPromptCacheKeyFromClient_FromPayload via openai branch
	// + pr-3003 TestCodexExecutorCacheHelper_OpenAIChatCompletions_PrefersPayloadPromptCacheKey
	ctx := newCodexCacheHelperContext("test-api-key", map[string]string{"X-Session-ID": "cpa:header"})
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"gpt-5.5","messages":[],"prompt_cache_key":"cpa:body"}`),
	}
	body, httpReq := cacheHelperRequest(t, ctx, sdktranslator.FromString("openai"), req, []byte(`{"model":"gpt-5.5","stream":true}`))

	if got := gjson.GetBytes(body, "prompt_cache_key").String(); got != "cpa:body" {
		t.Fatalf("prompt_cache_key = %q, want cpa:body; body=%s", got, string(body))
	}
	if got := httpReq.Header.Get("Session-Id"); got != "cpa:body" {
		t.Fatalf("Session-Id = %q, want cpa:body", got)
	}
}

func TestPortedFork_PromptCacheKey_FallbackToAPIKeySHA1(t *testing.T) {
	// b50dba3c TestCodexPromptCacheKeyFromClient_FallbackToHeaders (API key derivation)
	// + upstream TestCodexExecutorCacheHelper_OpenAIChatCompletions_StablePromptCacheKeyFromAPIKey
	ctx := newCodexCacheHelperContext("test-api-key", nil)
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.3-codex",
		Payload: []byte(`{"model":"gpt-5.3-codex"}`),
	}
	body, httpReq := cacheHelperRequest(t, ctx, sdktranslator.FromString("openai"), req, []byte(`{"model":"gpt-5.3-codex","stream":true}`))

	expectedKey := uuid.NewSHA1(uuid.NameSpaceOID, []byte("cli-proxy-api:codex:prompt-cache:test-api-key")).String()
	if got := gjson.GetBytes(body, "prompt_cache_key").String(); got != expectedKey {
		t.Fatalf("prompt_cache_key = %q, want %q", got, expectedKey)
	}
	if got := httpReq.Header.Get("Session-Id"); got != expectedKey {
		t.Fatalf("Session-Id = %q, want %q", got, expectedKey)
	}
	if got := httpReq.Header.Get("Session_id"); got != "" {
		t.Fatalf("Session_id = %q, want empty", got)
	}
}

func TestPortedFork_PromptCacheKey_FallbackToProviderSessionUUIDMetadata(t *testing.T) {
	// pr-3141 resolve chain step 2: payload prompt_cache_key -> metadata
	// (ProviderSessionUUID/ExecutionSessionMetadataKey). Fork expressed this as
	// X-Session-ID header; upstream expresses it as session metadata.
	ctx := newCodexCacheHelperContext("", nil)
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.4",
		Payload: []byte(`{"model":"gpt-5.4","messages":[{"role":"user","content":"hello"}]}`),
		Metadata: map[string]any{
			cliproxyexecutor.ExecutionSessionMetadataKey: "ctx:v1:execution-root",
		},
	}
	expectedKey := helps.ProviderSessionUUID("codex", req.Metadata)
	body, httpReq := cacheHelperRequest(t, ctx, sdktranslator.FromString("openai"), req, []byte(`{"model":"gpt-5.4","stream":true}`))

	if got := gjson.GetBytes(body, "prompt_cache_key").String(); got != expectedKey {
		t.Fatalf("prompt_cache_key = %q, want %q", got, expectedKey)
	}
	if got := httpReq.Header.Get("Session-Id"); got != expectedKey {
		t.Fatalf("Session-Id = %q, want %q", got, expectedKey)
	}
}

func TestPortedFork_PromptCacheKey_InvalidJSONProducesNoKey(t *testing.T) {
	// b50dba3c TestCodexPromptCacheKeyFromJSON_InvalidJSON: invalid JSON must
	// not panic and must not fabricate a payload-derived key. Upstream gjson
	// parsing is tolerant; a payload key absent means no key is derived from it.
	ctx := newCodexCacheHelperContext("", nil)
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{invalid json}`),
	}
	body, httpReq := cacheHelperRequest(t, ctx, sdktranslator.FromString("openai-response"), req, []byte(`{"model":"gpt-5.5","stream":true}`))

	if got := gjson.GetBytes(body, "prompt_cache_key").String(); got != "" {
		t.Fatalf("prompt_cache_key = %q, want empty for invalid payload", got)
	}
	if got := httpReq.Header.Get("Session-Id"); got != "" {
		t.Fatalf("Session-Id = %q, want empty for invalid payload", got)
	}
}

func TestPortedFork_PromptCacheKey_EmptyPayloadProducesNoKey(t *testing.T) {
	// b50dba3c TestCodexPromptCacheKeyFromJSON_EmptyPayload
	ctx := newCodexCacheHelperContext("", nil)
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: nil,
	}
	body, httpReq := cacheHelperRequest(t, ctx, sdktranslator.FromString("openai-response"), req, []byte(`{"model":"gpt-5.5","stream":true}`))

	if got := gjson.GetBytes(body, "prompt_cache_key").String(); got != "" {
		t.Fatalf("prompt_cache_key = %q, want empty for nil payload", got)
	}
	if got := httpReq.Header.Get("Session-Id"); got != "" {
		t.Fatalf("Session-Id = %q, want empty for nil payload", got)
	}
}

// DISPOSITION-TRIPWIRE: the fork read prompt_cache_key from the raw translated
// body (promptCacheKey camelCase / providerOptions) and from X-Session-ID
// headers. Upstream only reads req.Payload.prompt_cache_key, then falls back to
// session metadata / API key. Those fork body/header sources are superseded by
// upstream's resolve chain. These tests document the upstream boundary instead
// of asserting the fork source.
func TestPortedFork_ResolveChain_DoesNotReadXSessionIDHeader(t *testing.T) {
	ctx := newCodexCacheHelperContext("", map[string]string{"X-Session-ID": "cpa:session-123"})
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"gpt-5.5","input":[]}`),
	}
	body, httpReq := cacheHelperRequest(t, ctx, sdktranslator.FromString("openai-response"), req, []byte(`{"model":"gpt-5.5","stream":true}`))

	if got := gjson.GetBytes(body, "prompt_cache_key").String(); got != "" {
		t.Fatalf("prompt_cache_key = %q, want empty (upstream does not read X-Session-ID)", got)
	}
	if got := httpReq.Header.Get("Session-Id"); got != "" {
		t.Fatalf("Session-Id = %q, want empty (upstream does not read X-Session-ID)", got)
	}
}

// DISPOSITION — pr-3003 also normalized the OpenCode developer env "Current
// time:" line to midnight (normalizeCodexDeveloperCurrentTimeForPromptCache) so
// repeated requests share a stable prompt prefix. Upstream v7.2.141 has NO such
// normalization (only Claude cloaking exists upstream); spec non-goal says "no
// prompt-rendering changes". The behavior is genuinely absent upstream — the
// sync does NOT carry it. These tripwires assert the upstream boundary: env
// content passes through unchanged. If upstream later adds timestamp
// normalization, these fail and the sync decision should be revisited.

func TestPortedFork_DeveloperCurrentTime_PassesThroughUnchanged(t *testing.T) {
	rawJSON := []byte(`{"model":"gpt-5.5","stream":true,"prompt_cache_key":"cpa:session","input":[{"role":"developer","content":"You are powered by gpt-5.5.\n<env>\n  Working directory: /repo\n  Workspace root folder: /repo\n  Platform: linux\n  Today's date: Fri Apr 24 2026\n  Current time: 2026-04-24T05:55:01.054Z\n</env>"},{"role":"user","content":"hello"}]}`)
	ctx := newCodexCacheHelperContext("", nil)
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"gpt-5.5","prompt_cache_key":"cpa:session","input":[]}`),
	}
	body, _ := cacheHelperRequest(t, ctx, sdktranslator.FromString("openai-response"), req, rawJSON)

	content := gjson.GetBytes(body, "input.0.content").String()
	if !strings.Contains(content, "Current time: 2026-04-24T05:55:01.054Z") {
		t.Fatalf("developer current time was altered by upstream: %q", content)
	}
}

func TestPortedFork_DeveloperCurrentTime_StructuredContentPassesThrough(t *testing.T) {
	rawJSON := []byte(`{"model":"gpt-5.5","stream":true,"prompt_cache_key":"cpa:session","input":[{"role":"developer","content":[{"type":"input_text","text":"prefix\n  Current time: 2026-04-24T05:55:01.054Z\nsuffix"}]},{"role":"user","content":"hello"}]}`)
	ctx := newCodexCacheHelperContext("", nil)
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"gpt-5.5","prompt_cache_key":"cpa:session","input":[]}`),
	}
	body, _ := cacheHelperRequest(t, ctx, sdktranslator.FromString("openai-response"), req, rawJSON)

	content := gjson.GetBytes(body, "input.0.content")
	if !content.IsArray() {
		t.Fatalf("developer content type changed: %s", string(body))
	}
	if gotText := content.Get("0.text").String(); !strings.Contains(gotText, "2026-04-24T05:55:01.054Z") {
		t.Fatalf("structured developer content was altered by upstream: %q", gotText)
	}
}
