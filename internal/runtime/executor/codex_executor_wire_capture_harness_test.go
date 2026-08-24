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
//     continuity key (prompt_cache_key)
//   - upstream preloaded headers present: X-Codex-Window-Id, Thread-Id,
//     X-Openai-Internal-Codex-Responses-Lite
//   - `prompt_cache_key` present in the outbound request body
//   - NO `Session_id` / `session_id` outbound on the HTTP continuity path
//     (Task 3 note: the lowercase form is the WebSocket wire form only)
//
// No production code is modified by this file.

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
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
// capture server. The server returns a minimal valid responses SSE stream so
// Execute completes; the captured outbound request is returned for assertion.
func runWireCapture(t *testing.T, fixtureBody []byte, preloads http.Header) capturedUpstreamRequest {
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

	executor := NewCodexExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL,
		"api_key":  "test",
	}}

	_, err := executor.Execute(context.Background(), auth, cliproxyexecutor.Request{
		Model:   "gpt-5.5",
		Payload: fixtureBody,
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

func TestWireCapture_HTTP_CanonicalSessionIdEqualsContinuityKey(t *testing.T) {
	body := []byte(`{"model":"gpt-5.5","prompt_cache_key":"wire-cont-key","input":"hello"}`)
	captured := runWireCapture(t, body, nil)

	assertExactlyOneHeaderValue(t, captured.Header, "Session-Id", "wire-cont-key")
	assertNoHeaderValue(t, captured.Header, "Session_id")
	assertNoHeaderValue(t, captured.Header, "session_id")
	assertNoSessionHeaderVariants(t, captured.Header, "Session-Id")

	if got := gjson.GetBytes(captured.Body, "prompt_cache_key").String(); got != "wire-cont-key" {
		t.Fatalf("outbound prompt_cache_key = %q, want wire-cont-key", got)
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
