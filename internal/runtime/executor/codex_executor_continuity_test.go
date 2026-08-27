package executor

// Ported fork regression tests for the pr-3141 continuity resolve chain.
//
// pr-3141 (1e87caba) introduced codex_continuity.go with resolveCodexContinuity:
//   payload prompt_cache_key -> execution-session metadata
//   -> API-key SHA1 fallback -> auth.ID fallback -> identity-confuse isolation.
//
// Upstream v7.2.141 implements the same chain inside cacheHelper
// (codex_executor_request.go:118-156) plus applyCodexIdentityConfuseBody
// (codex_executor_request.go:159-183) and ProviderSessionUUID
// (helps/derived_session.go:30-47). These tests assert that chain through the
// upstream API surface so any drift from the fork's continuity intent is caught.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

func TestPortedFork_ContinuityResolveChain_PayloadKeyPolicy_OpenAIResponses(t *testing.T) {
	// pr-3141 resolveCodexContinuity step 1, updated for the
	// ForceStablePromptCacheKey policy (spec §4): on openai-response the payload
	// prompt_cache_key wins only when the flag is off (or no stable session
	// identity resolves). With the flag on + execution_session_id metadata, the
	// derived ProviderSessionUUID key wins over the payload key (zero-reuse fix).
	ctx := newCodexCacheHelperContext("api-key-a", nil)
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"gpt-5.5","prompt_cache_key":"payload-key"}`),
		Metadata: map[string]any{
			cliproxyexecutor.ExecutionSessionMetadataKey: "metadata-session",
		},
	}
	expectedKey := helps.ProviderSessionUUID("codex", req.Metadata)
	if expectedKey == "" {
		t.Fatalf("test setup: expected a non-empty derived key")
	}
	from := sdktranslator.FromString("openai-response")
	rawJSON := []byte(`{"model":"gpt-5.5","stream":true}`)

	t.Run("flag_off_payload_key_wins", func(t *testing.T) {
		body, httpReq := cacheHelperRequestWithConfig(t, ctx, &config.Config{}, from, req, rawJSON)
		if got := gjson.GetBytes(body, "prompt_cache_key").String(); got != "payload-key" {
			t.Fatalf("prompt_cache_key = %q, want payload-key (flag off)", got)
		}
		if got := httpReq.Header.Get("Session-Id"); got != "payload-key" {
			t.Fatalf("Session-Id = %q, want payload-key (flag off)", got)
		}
	})

	t.Run("flag_on_identity_derived_key_wins", func(t *testing.T) {
		cfg := &config.Config{Codex: config.CodexConfig{ForceStablePromptCacheKey: true}}
		body, httpReq := cacheHelperRequestWithConfig(t, ctx, cfg, from, req, rawJSON)
		if got := gjson.GetBytes(body, "prompt_cache_key").String(); got != expectedKey {
			t.Fatalf("prompt_cache_key = %q, want derived %q", got, expectedKey)
		}
		if got := httpReq.Header.Get("Session-Id"); got != expectedKey {
			t.Fatalf("Session-Id = %q, want derived %q", got, expectedKey)
		}
	})
}

func TestPortedFork_ContinuityResolveChain_MetadataBeatsAPIKeyFallback(t *testing.T) {
	// pr-3141 step 2: execution-session metadata used before API-key SHA1.
	ctx := newCodexCacheHelperContext("api-key-a", nil)
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"gpt-5.5","messages":[]}`),
		Metadata: map[string]any{
			cliproxyexecutor.ExecutionSessionMetadataKey: "exec-session-1",
		},
	}
	expectedKey := helps.ProviderSessionUUID("codex", req.Metadata)
	body, httpReq := cacheHelperRequest(t, ctx, sdktranslator.FromString("openai"), req, []byte(`{"model":"gpt-5.5","stream":true}`))

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

func TestPortedFork_ContinuityResolveChain_IdentityConfuseIsolatesPerAuth(t *testing.T) {
	// pr-3141 step 4: identity-confuse auth isolation. Upstream:
	// applyCodexIdentityConfuseBody (codex_executor_request.go:159-183).
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx := context.WithValue(context.Background(), "gin", ginCtx)

	executor := &CodexExecutor{cfg: &config.Config{
		Routing: config.RoutingConfig{Strategy: "fill-first"},
		Codex:   config.CodexConfig{IdentityConfuse: true},
	}}
	url := "https://example.com/responses"
	userPayload := []byte(`{"model":"gpt-5.5","prompt_cache_key":"shared-key"}`)

	authA := &cliproxyauth.Auth{ID: "auth-a", Provider: "codex"}
	authB := &cliproxyauth.Auth{ID: "auth-b", Provider: "codex"}

	httpReqA, bodyA, _, errA := executor.cacheHelper(ctx, sdktranslator.FromString("openai-response"), url, authA, cliproxyexecutor.Request{Model: "gpt-5.5", Payload: userPayload}, userPayload, []byte(`{"model":"gpt-5.5","prompt_cache_key":"shared-key","stream":true}`))
	if errA != nil {
		t.Fatalf("cacheHelper authA error: %v", errA)
	}
	httpReqB, bodyB, _, errB := executor.cacheHelper(ctx, sdktranslator.FromString("openai-response"), url, authB, cliproxyexecutor.Request{Model: "gpt-5.5", Payload: userPayload}, userPayload, []byte(`{"model":"gpt-5.5","prompt_cache_key":"shared-key","stream":true}`))
	if errB != nil {
		t.Fatalf("cacheHelper authB error: %v", errB)
	}
	if bodyA == nil {
		bodyA, _ = io.ReadAll(httpReqA.Body)
	}
	if bodyB == nil {
		bodyB, _ = io.ReadAll(httpReqB.Body)
	}

	keyA := gjson.GetBytes(bodyA, "prompt_cache_key").String()
	keyB := gjson.GetBytes(bodyB, "prompt_cache_key").String()
	if keyA == "" || keyB == "" {
		t.Fatalf("identity-confuse keys must be non-empty: A=%q B=%q", keyA, keyB)
	}
	if keyA == keyB {
		t.Fatalf("identity-confuse keys must differ per auth: A=%q B=%q", keyA, keyB)
	}
	expectedA := codexIdentityConfuseUUID("auth-a", "prompt-cache", "shared-key")
	if keyA != expectedA {
		t.Fatalf("authA prompt_cache_key = %q, want %q", keyA, expectedA)
	}
}

func TestPortedFork_ContinuityResolveChain_NoIdentityConfuseWithoutConfig(t *testing.T) {
	// pr-3141: without identity-confuse enabled the key passes through as-is.
	recorder := httptest.NewRecorder()
	ginCtx, _ := gin.CreateTestContext(recorder)
	ginCtx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx := context.WithValue(context.Background(), "gin", ginCtx)

	executor := &CodexExecutor{cfg: &config.Config{}}
	url := "https://example.com/responses"
	auth := &cliproxyauth.Auth{ID: "auth-a", Provider: "codex"}
	userPayload := []byte(`{"model":"gpt-5.5","prompt_cache_key":"plain-key"}`)

	_, body, _, err := executor.cacheHelper(ctx, sdktranslator.FromString("openai-response"), url, auth, cliproxyexecutor.Request{Model: "gpt-5.5", Payload: userPayload}, userPayload, []byte(`{"model":"gpt-5.5","prompt_cache_key":"plain-key","stream":true}`))
	if err != nil {
		t.Fatalf("cacheHelper error: %v", err)
	}
	if got := gjson.GetBytes(body, "prompt_cache_key").String(); got != "plain-key" {
		t.Fatalf("prompt_cache_key = %q, want plain-key (no identity confuse)", got)
	}
}

func TestPortedFork_ContinuityResolveChain_APIKeySHA1StableAcrossRequests(t *testing.T) {
	// pr-3141 step 3 + b50dba3c fallback: same API key => same key on repeat calls.
	ctx := newCodexCacheHelperContext("stable-api-key", nil)
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.3-codex",
		Payload: []byte(`{"model":"gpt-5.3-codex"}`),
	}
	first, httpReqFirst := cacheHelperRequest(t, ctx, sdktranslator.FromString("openai"), req, []byte(`{"model":"gpt-5.3-codex","stream":true}`))
	second, _ := cacheHelperRequest(t, ctx, sdktranslator.FromString("openai"), req, []byte(`{"model":"gpt-5.3-codex","stream":true}`))

	expectedKey := uuid.NewSHA1(uuid.NameSpaceOID, []byte("cli-proxy-api:codex:prompt-cache:stable-api-key")).String()
	firstKey := gjson.GetBytes(first, "prompt_cache_key").String()
	secondKey := gjson.GetBytes(second, "prompt_cache_key").String()
	if firstKey != expectedKey || secondKey != expectedKey {
		t.Fatalf("keys not stable SHA1 of API key: first=%q second=%q want=%q", firstKey, secondKey, expectedKey)
	}
	if got := httpReqFirst.Header.Get("Session-Id"); got != expectedKey {
		t.Fatalf("Session-Id = %q, want %q", got, expectedKey)
	}
}

func TestPortedFork_ContinuityResolveChain_CanonicalSessionIdNoUnderscore(t *testing.T) {
	// pr-3141 applyCodexContinuityHeaders set "session_id"; upstream canonical is
	// "Session-Id". Assert no underscore-cased outbound header on continuity path.
	ctx := newCodexCacheHelperContext("key-auth", nil)
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"gpt-5.5","prompt_cache_key":"cont-key"}`),
	}
	_, httpReq := cacheHelperRequest(t, ctx, sdktranslator.FromString("openai-response"), req, []byte(`{"model":"gpt-5.5","stream":true}`))

	if got := httpReq.Header.Get("Session-Id"); got != "cont-key" {
		t.Fatalf("Session-Id = %q, want cont-key", got)
	}
	for _, legacy := range []string{"Session_id", "session_id"} {
		if got := httpReq.Header.Get(legacy); got != "" {
			t.Fatalf("%s = %q, want empty", legacy, got)
		}
	}
}
