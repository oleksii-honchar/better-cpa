package executor

// Task 5 wire-capture verification harness (sync branch sync/upstream-v7.2.141).
//
// This is an INTEGRATION harness, distinct from the Task 3 unit-level wire
// tests: it runs the REAL CodexExecutor.Execute path (cacheHelper →
// applyCodexHeaders → the executor's actual httpClient.Do) against a local
// capture server that records the full outbound upstream HTTP request (headers
// + body). The assertions below pin the continuity contract on the HTTP
// continuity path for the BUILT code path:
//
//   - exactly one canonical `Session-Id` header equal to the resolved
//     continuity key (prompt_cache_key / derived ProviderSessionUUID key)
//   - upstream preloaded headers present: X-Codex-Window-Id, Thread-Id,
//     X-Openai-Internal-Codex-Responses-Lite
//   - `prompt_cache_key` present in the outbound request body
//   - NO `Session_id` / `session_id` outbound on the HTTP continuity path
//     (Task 3 note: the lowercase form is the WebSocket wire form only)
//   - ForceStablePromptCacheKey policy on the real Execute path: flag on +
//     execution_session_id metadata -> derived stable key overrides the random
//     client key; same affinity group -> identical key across turns; flag off
//     -> client key echoed unchanged.
//
// No production code is modified by this file.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/runtime/executor/helps"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

// capturedUpstreamRequest is the full outbound upstream HTTP request observed
// by the capture server.
type capturedUpstreamRequest struct {
	Method string
	Path   string
	Header http.Header
	Body   []byte
}

// runWireCapture executes the REAL CodexExecutor.Execute path against a local
// capture server with default config (ForceStablePromptCacheKey zero-value =
// false) and no metadata. The server returns a minimal valid responses SSE
// stream so Execute completes; the captured outbound request is returned for
// assertion.
func runWireCapture(t *testing.T, fixtureBody []byte, preloads http.Header) capturedUpstreamRequest {
	t.Helper()
	return runWireCaptureWithConfig(t, fixtureBody, preloads, &config.Config{}, nil)
}

// runWireCaptureWithConfig is runWireCapture with an explicit cfg and request
// metadata so the ForceStablePromptCacheKey policy can be exercised on the real
// Execute path.
func runWireCaptureWithConfig(t *testing.T, fixtureBody []byte, preloads http.Header, cfg *config.Config, metadata map[string]any) capturedUpstreamRequest {
	t.Helper()
	var captured capturedUpstreamRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		captured = capturedUpstreamRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Header: r.Header.Clone(),
			Body:   body,
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":0,\"status\":\"completed\",\"background\":false,\"error\":null}}\n\n"))
	}))
	defer server.Close()

	executor := NewCodexExecutor(cfg)
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL,
		"api_key":  "test",
	}}

	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:    "gpt-5.5",
		Payload:  fixtureBody,
		Metadata: metadata,
	}, cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-response"),
		Stream:       false,
		Headers:      preloads,
	})
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if captured.Path == "" {
		t.Fatal("capture server never received the outbound upstream request")
	}
	return captured
}

func TestWireCapture_HTTP_RandomClientKeyWithIdentity_DerivedStableKey(t *testing.T) {
	// New policy (spec §4) on the real Execute path: flag on + execution_session_id
	// metadata -> the random per-request client key is overridden by the derived
	// ProviderSessionUUID key. Before the override existed, this assertion failed:
	// the outbound key equaled the random client key (the zero-reuse bug).
	body := []byte(`{"model":"gpt-5.5","prompt_cache_key":"4f3b6f2a-1c2d-4e5f-8a9b-0c1d2e3f4a5b","input":"hello"}`)
	cfg := &config.Config{Codex: config.CodexConfig{ForceStablePromptCacheKey: true}}
	metadata := map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "ctx:v1:execution-root"}
	expectedKey := helps.ProviderSessionUUID("codex", metadata)
	if expectedKey == "" {
		t.Fatalf("test setup: expected a non-empty derived key")
	}

	captured := runWireCaptureWithConfig(t, body, nil, cfg, metadata)

	assertExactlyOneHeaderValue(t, captured.Header, "Session-Id", expectedKey)
	assertNoHeaderValue(t, captured.Header, "Session_id")
	assertNoHeaderValue(t, captured.Header, "session_id")
	assertNoSessionHeaderVariants(t, captured.Header, "Session-Id")

	if got := gjson.GetBytes(captured.Body, "prompt_cache_key").String(); got != expectedKey {
		t.Fatalf("outbound prompt_cache_key = %q, want derived %q", got, expectedKey)
	}
}

// TestWireCapture_HTTP_SameAffinityGroup_TwoTurns_IdenticalOutboundKey: two
// turns in the same session-affinity group (same execution_session_id, different
// random client keys) produce the identical derived outbound key and header.
// Before the override existed, the two random client keys were echoed verbatim
// and differed — cross-turn cache reuse was impossible.
func TestWireCapture_HTTP_SameAffinityGroup_TwoTurns_IdenticalOutboundKey(t *testing.T) {
	cfg := &config.Config{Codex: config.CodexConfig{ForceStablePromptCacheKey: true}}
	metadata := map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "ctx:v1:same-wire-affinity"}
	expectedKey := helps.ProviderSessionUUID("codex", metadata)
	if expectedKey == "" {
		t.Fatalf("test setup: expected a non-empty derived key")
	}

	first := runWireCaptureWithConfig(t, []byte(`{"model":"gpt-5.5","prompt_cache_key":"aaaaaaaa-1111-4111-8111-111111111111","input":"hello"}`), nil, cfg, metadata)
	second := runWireCaptureWithConfig(t, []byte(`{"model":"gpt-5.5","prompt_cache_key":"bbbbbbbb-2222-4222-8222-222222222222","input":"hello"}`), nil, cfg, metadata)

	if got := gjson.GetBytes(first.Body, "prompt_cache_key").String(); got != expectedKey {
		t.Fatalf("turn 1 outbound prompt_cache_key = %q, want %q", got, expectedKey)
	}
	if got := gjson.GetBytes(second.Body, "prompt_cache_key").String(); got != expectedKey {
		t.Fatalf("turn 2 outbound prompt_cache_key = %q, want %q", got, expectedKey)
	}
	assertExactlyOneHeaderValue(t, first.Header, "Session-Id", expectedKey)
	assertExactlyOneHeaderValue(t, second.Header, "Session-Id", expectedKey)
	assertNoSessionHeaderVariants(t, second.Header, "Session-Id")
}

// TestWireCapture_HTTP_ForceStableKeyOff_ClientKeyEchoedUnchanged: flag off +
// identity metadata present -> the client-supplied key is echoed unchanged on
// the real Execute path. The config gate, not the identity alone, drives the
// override (legacy preservation guard).
func TestWireCapture_HTTP_ForceStableKeyOff_ClientKeyEchoedUnchanged(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","prompt_cache_key":"wire-client-echo","input":"hello"}`)
	metadata := map[string]any{cliproxyexecutor.ExecutionSessionMetadataKey: "ctx:v1:execution-root"}

	captured := runWireCaptureWithConfig(t, body, nil, &config.Config{}, metadata)

	assertExactlyOneHeaderValue(t, captured.Header, "Session-Id", "wire-client-echo")
	if got := gjson.GetBytes(captured.Body, "prompt_cache_key").String(); got != "wire-client-echo" {
		t.Fatalf("outbound prompt_cache_key = %q, want wire-client-echo (flag off)", got)
	}
}

func TestWireCapture_HTTP_PreloadedHeadersPresentOnOutbound(t *testing.T) {
	preloads := http.Header{}
	preloads.Set("X-Codex-Window-Id", "window-wire")
	preloads.Set("Thread-Id", "thread-wire")
	preloads.Set("X-Openai-Internal-Codex-Responses-Lite", "true")

	body := []byte(`{"model":"gpt-5.5","prompt_cache_key":"wire-preload","input":"hello"}`)
	captured := runWireCapture(t, body, preloads)

	if got := captured.Header.Get("X-Codex-Window-Id"); got != "window-wire" {
		t.Fatalf("X-Codex-Window-Id = %q, want window-wire", got)
	}
	if got := captured.Header.Get("Thread-Id"); got != "thread-wire" {
		t.Fatalf("Thread-Id = %q, want thread-wire", got)
	}
	if got := captured.Header.Get("X-Openai-Internal-Codex-Responses-Lite"); got != "true" {
		t.Fatalf("X-Openai-Internal-Codex-Responses-Lite = %q, want true", got)
	}
	assertExactlyOneHeaderValue(t, captured.Header, "Session-Id", "wire-preload")
	assertNoHeaderValue(t, captured.Header, "Session_id")
	assertNoHeaderValue(t, captured.Header, "session_id")
}

func TestWireCapture_HTTP_NoUnderscoreSessionHeaderOnContinuityPath(t *testing.T) {
	// Legacy inbound underscore header must not leak outbound on the HTTP
	// continuity path and must not override the resolved continuity key.
	preloads := http.Header{}
	preloads.Set("Session_id", "legacy-in-wire")

	body := []byte(`{"model":"gpt-5.5","prompt_cache_key":"wire-canon","input":"hello"}`)
	captured := runWireCapture(t, body, preloads)

	assertExactlyOneHeaderValue(t, captured.Header, "Session-Id", "wire-canon")
	assertNoHeaderValue(t, captured.Header, "Session_id")
	assertNoHeaderValue(t, captured.Header, "session_id")
	assertNoSessionHeaderVariants(t, captured.Header, "Session-Id")
}
