package executor

// Task 3 continuity wire-contract tests (sync branch sync/upstream-v7.2.141).
//
// These assertions pin the upstream v7.2.141 session-header wire contract on
// the continuity path:
//
//   - HTTP path: exactly one canonical `Session-Id` header equal to the
//     resolved continuity key (prompt_cache_key / execution-session metadata /
//     API-key SHA1), and NO `Session_id`/`session_id` outbound.
//   - WebSocket path: exactly one `session_id` header equal to the continuity
//     key. NOTE: OpenAI's websockets wire protocol uses the lowercase
//     `session_id` header on the upgrade request; upstream v7.2.141
//     deliberately emits lowercase there and deletes any `Session-Id`
//     (codex_websockets_request.go ensureCodexWebsocketSessionHeader). The
//     plan's AC wording "Session-Id on both HTTP and WebSocket paths" does not
//     match upstream's WS wire contract; the lowercase `session_id` IS the
//     canonical WS form. This is documented as a sync-decision note, not a
//     production change.
//   - macOS-only `Session_id` auto-generation fallback
//     (codex_executor_request.go:303-305, UA-override path) must never be hit
//     when the continuity key is resolved.
//   - Upstream preloaded headers (f43aad76): X-Codex-Window-Id, Thread-Id,
//     X-Openai-Internal-Codex-Responses-Lite are forwarded/present on the HTTP
//     path; X-Codex-Window-Id + Thread-Id are set on the WS identity-confuse
//     path (X-Openai-Internal-Codex-Responses-Lite is HTTP-only upstream).
//   - Legacy inbound `Session_id`/`session_id`/`Session-Id` are accepted and
//     normalized on the WS path; on the HTTP path they never leak outbound in
//     underscore form and never override the resolved canonical key.
//
// No production code is modified by this file. Any assertion that stays red
// against upstream is a documented sync-decision signal (see
// materials/task3-evidence.md), not a fix trigger.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
	sdktranslator "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	"github.com/tidwall/gjson"
)

const macOSUAOverrideModel = "gpt-5.6-continuity-wire-ua"

// registerMacOSUAOverrideModel registers a model whose override header carries a
// macOS User-Agent, so applyModelHeaderOverrides hits the UA-override branch
// (codex_executor_request.go:292-306) where the macOS-only `Session_id`
// auto-generation fallback lives.
func registerMacOSUAOverrideModel(t *testing.T) {
	t.Helper()
	reg := registry.GetGlobalRegistry()
	clientID := "continuity-wire-ua-override"
	reg.RegisterClient(clientID, "codex", []*registry.ModelInfo{{
		ID: macOSUAOverrideModel,
		Config: &registry.ModelConfig{
			OverrideHeader: map[string]string{
				"user-agent": "codex-tui/0.144.0 (Mac OS 26.5.1; arm64) iTerm.app/3.6.11 (codex-tui; 0.144.0)",
			},
		},
	}})
	t.Cleanup(func() { reg.UnregisterClient(clientID) })
}

// assertExactlyOneHeaderValue asserts the map holds the key with exactly one
// value equal to want.
func assertExactlyOneHeaderValue(t *testing.T, headers http.Header, key string, want string) {
	t.Helper()
	got := headers[key]
	if len(got) != 1 || got[0] != want {
		t.Fatalf("%s = %#v, want exactly one value [%q]", key, got, want)
	}
}

// assertNoHeaderValue asserts the key is entirely absent from the map.
func assertNoHeaderValue(t *testing.T, headers http.Header, key string) {
	t.Helper()
	if got := headers[key]; len(got) != 0 {
		t.Fatalf("%s = %#v, want absent", key, got)
	}
}

// assertNoSessionHeaderVariants asserts no other casing of the session header
// key exists on the map (used to pin the "exactly one session header" rule).
func assertNoSessionHeaderVariants(t *testing.T, headers http.Header, presentKey string) {
	t.Helper()
	for existingKey := range headers {
		if codexSessionHeaderKey(existingKey) && existingKey != presentKey {
			t.Fatalf("unexpected extra session header %q (values %#v); only %q may be present", existingKey, headers[existingKey], presentKey)
		}
	}
}

func TestContinuityWire_HTTP_ExactlyOneCanonicalSessionIdEqualsContinuityKey(t *testing.T) {
	ctx := newCodexCacheHelperContext("", nil)
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"gpt-5.5","prompt_cache_key":"cont-key-http"}`),
	}
	body, httpReq := cacheHelperRequest(t, ctx, sdktranslator.FromString("openai-response"), req, []byte(`{"model":"gpt-5.5","stream":true}`))
	if got := gjson.GetBytes(body, "prompt_cache_key").String(); got != "cont-key-http" {
		t.Fatalf("prompt_cache_key = %q, want cont-key-http", got)
	}

	// Full outbound pipeline as in codex_executor_execute.go:82-83 and
	// codex_executor_stream.go:88-89.
	applyCodexHeaders(httpReq, nil, "oauth-token", true, nil)
	applyModelHeaderOverrides(httpReq.Header, "gpt-5.5")

	assertExactlyOneHeaderValue(t, httpReq.Header, "Session-Id", "cont-key-http")
	assertNoHeaderValue(t, httpReq.Header, "Session_id")
	assertNoHeaderValue(t, httpReq.Header, "session_id")
	assertNoSessionHeaderVariants(t, httpReq.Header, "Session-Id")
}

func TestContinuityWire_WebSocket_ExactlyOneSessionHeaderEqualsContinuityKey(t *testing.T) {
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"gpt-5.5","prompt_cache_key":"cont-key-ws"}`),
	}
	body, headers := applyCodexPromptCacheHeaders("openai-response", req, []byte(`{"model":"gpt-5.5","stream":true}`))
	if got := gjson.GetBytes(body, "prompt_cache_key").String(); got != "cont-key-ws" {
		t.Fatalf("prompt_cache_key = %q, want cont-key-ws", got)
	}

	// Full outbound WS pipeline as in codex_websockets_execute.go:80-89 and
	// codex_websockets_stream.go:77-86.
	headers = applyCodexWebsocketHeaders(context.Background(), headers, nil, "oauth-token", nil)
	applyModelHeaderOverrides(headers, "gpt-5.5")

	// WS wire contract: the upgrade request carries exactly one lowercase
	// `session_id` header (OpenAI websockets protocol). `Session-Id` is
	// deliberately deleted by ensureCodexWebsocketSessionHeader — see file
	// header note; this is the sync-decision nuance vs the plan AC wording.
	assertExactlyOneHeaderValue(t, headers, "session_id", "cont-key-ws")
	assertNoHeaderValue(t, headers, "Session-Id")
	assertNoHeaderValue(t, headers, "Session_id")
	assertNoSessionHeaderVariants(t, headers, "session_id")
}

func TestContinuityWire_HTTP_PreloadedHeadersPresent(t *testing.T) {
	ctx := newCodexCacheHelperContext("", map[string]string{
		"X-Codex-Window-Id":                 "window-1",
		"Thread-Id":                         "thread-1",
		"X-Openai-Internal-Codex-Responses-Lite": "true",
	})
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"gpt-5.5","prompt_cache_key":"cont-key-preload"}`),
	}
	_, httpReq := cacheHelperRequest(t, ctx, sdktranslator.FromString("openai-response"), req, []byte(`{"model":"gpt-5.5","stream":true}`))
	applyCodexHeaders(httpReq, nil, "oauth-token", true, nil)

	if got := httpReq.Header.Get("X-Codex-Window-Id"); got != "window-1" {
		t.Fatalf("X-Codex-Window-Id = %q, want window-1", got)
	}
	if got := httpReq.Header.Get("Thread-Id"); got != "thread-1" {
		t.Fatalf("Thread-Id = %q, want thread-1", got)
	}
	if got := httpReq.Header.Get("X-Openai-Internal-Codex-Responses-Lite"); got != "true" {
		t.Fatalf("X-Openai-Internal-Codex-Responses-Lite = %q, want true", got)
	}
	assertExactlyOneHeaderValue(t, httpReq.Header, "Session-Id", "cont-key-preload")
}

func TestContinuityWire_WebSocket_PreloadsPresentViaIdentityConfuse(t *testing.T) {
	cfg := &config.Config{
		Routing: config.RoutingConfig{SessionAffinity: true},
		Codex:   config.CodexConfig{IdentityConfuse: true},
	}
	auth := &cliproxyauth.Auth{ID: "auth-ws-preload", Provider: "codex"}
	req := cliproxyexecutor.Request{
		Model:   "gpt-5-codex",
		Payload: []byte(`{"prompt_cache_key":"cache-ws-preload"}`),
	}
	body, headers := applyCodexPromptCacheHeaders("openai-response", req, []byte(`{"model":"gpt-5-codex"}`))
	_, identityState := applyCodexIdentityConfuseBody(cfg, auth, req.Payload, body)
	headers = applyCodexWebsocketHeaders(context.Background(), headers, auth, "oauth-token", cfg)
	applyCodexIdentityConfuseHeaders(headers, &identityState)

	expectedKey := codexIdentityConfuseUUID("auth-ws-preload", "prompt-cache", "cache-ws-preload")
	assertExactlyOneHeaderValue(t, headers, "session_id", expectedKey)
	if got := headers.Get("Thread-Id"); got != expectedKey {
		t.Fatalf("Thread-Id = %q, want %q", got, expectedKey)
	}
	if got := headers.Get("X-Codex-Window-Id"); got != expectedKey+":0" {
		t.Fatalf("X-Codex-Window-Id = %q, want %q", got, expectedKey+":0")
	}
	// X-Openai-Internal-Codex-Responses-Lite is preloaded on the HTTP path
	// only (f43aad76); upstream does not emit it on the WS path.
	if got := headers.Get("X-Openai-Internal-Codex-Responses-Lite"); got != "" {
		t.Fatalf("X-Openai-Internal-Codex-Responses-Lite unexpectedly set on WS path: %q", got)
	}
}

func TestContinuityWire_WebSocket_LegacyInboundAllCasingsAccepted(t *testing.T) {
	auth := &cliproxyauth.Auth{Provider: "codex", Metadata: map[string]any{"email": "user@example.com"}}
	for _, inbound := range []string{"Session-Id", "Session_id", "session_id"} {
		t.Run(inbound, func(t *testing.T) {
			ctx := contextWithGinHeaders(map[string]string{inbound: "legacy-in-1"})
			headers := applyCodexWebsocketHeaders(ctx, http.Header{}, auth, "", nil)

			assertExactlyOneHeaderValue(t, headers, "session_id", "legacy-in-1")
			assertNoHeaderValue(t, headers, "Session-Id")
			assertNoHeaderValue(t, headers, "Session_id")
			assertNoSessionHeaderVariants(t, headers, "session_id")
		})
	}
}

func TestContinuityWire_HTTP_LegacyInboundDoesNotLeakOrOverride(t *testing.T) {
	// HTTP continuity is body/metadata-driven upstream; inbound session headers
	// must neither leak in underscore form nor override the resolved key.
	ctx := newCodexCacheHelperContext("", map[string]string{
		"Session_id": "legacy-http-in",
	})
	req := cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: []byte(`{"model":"gpt-5.5","prompt_cache_key":"cont-key-http-legacy"}`),
	}
	_, httpReq := cacheHelperRequest(t, ctx, sdktranslator.FromString("openai-response"), req, []byte(`{"model":"gpt-5.5","stream":true}`))
	applyCodexHeaders(httpReq, nil, "oauth-token", true, nil)

	assertExactlyOneHeaderValue(t, httpReq.Header, "Session-Id", "cont-key-http-legacy")
	assertNoHeaderValue(t, httpReq.Header, "Session_id")
	assertNoHeaderValue(t, httpReq.Header, "session_id")
}

func TestContinuityWire_HTTP_MacOSUAFallbackNotHitWhenContinuityResolved(t *testing.T) {
	registerMacOSUAOverrideModel(t)
	ctx := newCodexCacheHelperContext("", nil)
	req := cliproxyexecutor.Request{
		Model:   macOSUAOverrideModel,
		Payload: []byte(`{"model":"` + macOSUAOverrideModel + `","prompt_cache_key":"cont-key-ua"}`),
	}
	_, httpReq := cacheHelperRequest(t, ctx, sdktranslator.FromString("openai-response"), req, []byte(`{"model":"`+macOSUAOverrideModel+`","stream":true}`))
	applyCodexHeaders(httpReq, nil, "oauth-token", true, nil)
	applyModelHeaderOverrides(httpReq.Header, macOSUAOverrideModel)

	assertExactlyOneHeaderValue(t, httpReq.Header, "Session-Id", "cont-key-ua")
	// The macOS-only `Session_id` auto-generation fallback must not fire when a
	// continuity key was resolved (codex_executor_request.go:303-305).
	assertNoHeaderValue(t, httpReq.Header, "Session_id")
	assertNoHeaderValue(t, httpReq.Header, "session_id")
	assertNoSessionHeaderVariants(t, httpReq.Header, "Session-Id")
}

func TestContinuityWire_HTTP_MacOSUAFallbackActiveWithoutContinuityKey(t *testing.T) {
	// Companion proving the fallback really would fire when no continuity key
	// is resolved — the test above is only meaningful because this branch exists.
	registerMacOSUAOverrideModel(t)
	httpReq := httptest.NewRequest(http.MethodPost, "https://example.com/responses", nil)
	applyCodexHeaders(httpReq, nil, "oauth-token", true, nil)
	applyModelHeaderOverrides(httpReq.Header, macOSUAOverrideModel)

	if got := httpReq.Header.Get("Session_id"); got == "" {
		t.Fatal("expected macOS UA-override Session_id auto-generation fallback when no continuity key is resolved")
	}
}

func TestContinuityWire_WebSocket_MacOSFallbackNotUsedWhenKeyResolved(t *testing.T) {
	registerMacOSUAOverrideModel(t)
	req := cliproxyexecutor.Request{
		Model:   macOSUAOverrideModel,
		Payload: []byte(`{"model":"` + macOSUAOverrideModel + `","prompt_cache_key":"cont-key-ws-fb"}`),
	}
	body, headers := applyCodexPromptCacheHeaders("openai-response", req, []byte(`{"model":"`+macOSUAOverrideModel+`","stream":true}`))
	// No inbound session header; UA is macOS after cloaking/override, so
	// applyCodexWebsocketHeaders would generate a random fallback UUID — unless
	// the resolved continuity key already satisfies the session header.
	headers = applyCodexWebsocketHeaders(context.Background(), headers, nil, "oauth-token", nil)
	applyModelHeaderOverrides(headers, macOSUAOverrideModel)

	assertExactlyOneHeaderValue(t, headers, "session_id", "cont-key-ws-fb")
	if got := gjson.GetBytes(body, "prompt_cache_key").String(); got != "cont-key-ws-fb" {
		t.Fatalf("prompt_cache_key = %q, want cont-key-ws-fb", got)
	}
	assertNoHeaderValue(t, headers, "Session-Id")
	assertNoHeaderValue(t, headers, "Session_id")
}
