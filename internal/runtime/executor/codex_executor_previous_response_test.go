package executor

// Ported fork regression tests for previous_response_id normalization.
//
// Fork sources:
//   - b50dba3c internal/runtime/executor/codex_executor_previous_response_test.go
//     (normalizeCodexPreviousResponseIDForPromptCache + codexInputLooksLikeFullTranscript)
//   - pr-3003 d52f2933 (compaction-as-transcript) + 53a73feb (tool-call continuations)
//
// Fork intent: drop previous_response_id ONLY when the input is a full
// transcript (assistant role / compaction / compaction_summary) AND a
// prompt_cache_key is present AND format is openai-response; preserve it for
// incremental tool-call continuations. Upstream policy (v7.2.141) is stricter:
// it deletes previous_response_id unconditionally on every Codex request path
// (codex_executor_execute.go:58, codex_executor_stream.go:59,
// codex_executor_tokens.go:32). Tests below assert the fork intent so any
// upstream drift is caught; failures are dispositioned in
// session materials/task2-evidence.md.

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

// newPortedExecuteServer records the upstream request body served by Execute.
func newPortedExecuteServer(t *testing.T) (*httptest.Server, *[]byte) {
	t.Helper()
	gotBody := new([]byte)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*gotBody = body
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_1\",\"object\":\"response\",\"created_at\":0,\"status\":\"completed\",\"background\":false,\"error\":null}}\n\n"))
	}))
	t.Cleanup(server.Close)
	return server, gotBody
}

func portedExecuteRequest(serverURL string, payload []byte) (cliproxyexecutor.Request, cliproxyexecutor.Options, *cliproxyauth.Auth) {
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": serverURL,
		"api_key":  "test",
	}}
	req := cliproxyexecutor.Request{Model: "gpt-5.4", Payload: payload}
	opts := cliproxyexecutor.Options{
		SourceFormat: sdktranslator.FromString("openai-response"),
		Stream:       false,
	}
	return req, opts, auth
}

func TestPortedFork_PreviousResponseID_DroppedForFullTranscript(t *testing.T) {
	// b50dba3c TestNormalizePreviousResponseID_DropsForFullTranscript
	server, gotBody := newPortedExecuteServer(t)
	executor := NewCodexExecutor(&config.Config{})
	req, opts, auth := portedExecuteRequest(server.URL, []byte(`{
		"model":"gpt-5.4",
		"previous_response_id":"resp_123",
		"prompt_cache_key":"pc-abc",
		"input":[
			{"type":"message","role":"assistant","content":"Hello"},
			{"type":"message","role":"user","content":"Hi"}
		]
	}`))

	if _, err := executor.Execute(context.Background(), auth, req, opts); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got := gjson.GetBytes(*gotBody, "previous_response_id"); got.Exists() {
		t.Fatalf("previous_response_id should be dropped for full transcript; body=%s", string(*gotBody))
	}
	if !gjson.GetBytes(*gotBody, "prompt_cache_key").Exists() {
		t.Fatalf("prompt_cache_key should be preserved; body=%s", string(*gotBody))
	}
}

func TestPortedFork_PreviousResponseID_DroppedForCompactionTranscript(t *testing.T) {
	// pr-3003 d52f2933 TestCodexExecutorCacheHelper_OpenAIResponses_DropsPreviousResponseIDForCompactionTranscript
	for _, typ := range []string{"compaction", "compaction_summary"} {
		t.Run(typ, func(t *testing.T) {
			server, gotBody := newPortedExecuteServer(t)
			executor := NewCodexExecutor(&config.Config{})
			req, opts, auth := portedExecuteRequest(server.URL, []byte(`{
				"model":"gpt-5.5",
				"previous_response_id":"resp-prev",
				"prompt_cache_key":"cpa:session",
				"input":[
					{"type":"message","role":"user","content":"hello"},
					{"type":"`+typ+`","encrypted_content":"summary"}
				]
			}`))

			if _, err := executor.Execute(context.Background(), auth, req, opts); err != nil {
				t.Fatalf("Execute error: %v", err)
			}
			if got := gjson.GetBytes(*gotBody, "previous_response_id"); got.Exists() {
				t.Fatalf("previous_response_id was not dropped for %s transcript; body=%s", typ, string(*gotBody))
			}
		})
	}
}

// DISPOSITION — fork intent: pr-3003 53a73feb + b50dba3c preserved
// previous_response_id for incremental tool-call/text inputs, when
// prompt_cache_key is absent, and for non-OpenAI formats; only full transcripts
// (assistant/compaction) with a cache key dropped it. Upstream v7.2.141 is
// STRICTER: it deletes previous_response_id unconditionally on every Codex path
// (codex_executor_execute.go:58, codex_executor_stream.go:59,
// codex_executor_tokens.go:32). The fork's conditional normalization is
// superseded by upstream's uniform policy — the fork code would be dead on this
// base. These tripwires assert the upstream contract so any future relaxation
// (or reintroduction of conditional preservation) is caught.

func TestPortedFork_PreviousResponseID_DroppedForToolCallIncrementalInput(t *testing.T) {
	// pr-3003 53a73feb: fork preserved for function_call/custom_tool_call
	// continuations; upstream drops uniformly. Tripwire asserts upstream contract.
	for _, tc := range []struct {
		name string
		item string
	}{
		{name: "function_call", item: `{"type":"function_call","id":"fc-1","call_id":"call-1","name":"tool"}`},
		{name: "custom_tool_call", item: `{"type":"custom_tool_call","id":"ctc-1","call_id":"call-1","name":"apply_patch"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, gotBody := newPortedExecuteServer(t)
			executor := NewCodexExecutor(&config.Config{})
			req, opts, auth := portedExecuteRequest(server.URL, []byte(`{
				"model":"gpt-5.5",
				"previous_response_id":"resp-prev",
				"prompt_cache_key":"cpa:session",
				"input":[`+tc.item+`,{"type":"message","id":"msg-1"}]
			}`))

			if _, err := executor.Execute(context.Background(), auth, req, opts); err != nil {
				t.Fatalf("Execute error: %v", err)
			}
			if got := gjson.GetBytes(*gotBody, "previous_response_id"); got.Exists() {
				t.Fatalf("previous_response_id = %q, want dropped (upstream unconditional delete); body=%s", got.String(), string(*gotBody))
			}
		})
	}
}

func TestPortedFork_PreviousResponseID_DroppedForIncrementalTextInput(t *testing.T) {
	// b50dba3c TestNormalizePreviousResponseID_PreservesForIncremental —
	// fork preserved; upstream deletes. Tripwire asserts upstream contract.
	server, gotBody := newPortedExecuteServer(t)
	executor := NewCodexExecutor(&config.Config{})
	req, opts, auth := portedExecuteRequest(server.URL, []byte(`{
		"model":"gpt-5.4",
		"previous_response_id":"resp_123",
		"prompt_cache_key":"pc-abc",
		"input":[{"type":"text","text":"Just some text input"}]
	}`))

	if _, err := executor.Execute(context.Background(), auth, req, opts); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got := gjson.GetBytes(*gotBody, "previous_response_id"); got.Exists() {
		t.Fatalf("previous_response_id = %q, want dropped (upstream unconditional delete); body=%s", got.String(), string(*gotBody))
	}
}

func TestPortedFork_PreviousResponseID_DroppedWhenPromptCacheKeyAbsent(t *testing.T) {
	// b50dba3c TestNormalizePreviousResponseID_NoOpWithoutPromptCacheKey —
	// fork preserved; upstream deletes regardless. Tripwire asserts upstream contract.
	server, gotBody := newPortedExecuteServer(t)
	executor := NewCodexExecutor(&config.Config{})
	req, opts, auth := portedExecuteRequest(server.URL, []byte(`{
		"model":"gpt-5.4",
		"previous_response_id":"resp_123",
		"input":[{"type":"message","role":"assistant","content":"Hello"}]
	}`))

	if _, err := executor.Execute(context.Background(), auth, req, opts); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got := gjson.GetBytes(*gotBody, "previous_response_id"); got.Exists() {
		t.Fatalf("previous_response_id = %q, want dropped (upstream unconditional delete); body=%s", got.String(), string(*gotBody))
	}
}

func TestPortedFork_PreviousResponseID_DroppedForNonOpenAIFormat(t *testing.T) {
	// b50dba3c TestNormalizePreviousResponseID_NoOpForNonOpenAI — fork preserved;
	// upstream deletes on the Codex path regardless of source format.
	server, gotBody := newPortedExecuteServer(t)
	executor := NewCodexExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL,
		"api_key":  "test",
	}}
	req := cliproxyexecutor.Request{Model: "gpt-5.4", Payload: []byte(`{
		"previous_response_id":"resp_123",
		"prompt_cache_key":"pc-abc",
		"input":[{"type":"message","role":"assistant","content":"Hello"}]
	}`)}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("anthropic"), Stream: false}

	if _, err := executor.Execute(context.Background(), auth, req, opts); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if got := gjson.GetBytes(*gotBody, "previous_response_id"); got.Exists() {
		t.Fatalf("previous_response_id = %q, want dropped (upstream unconditional delete); body=%s", got.String(), string(*gotBody))
	}
}

func TestPortedFork_PreviousResponseID_DroppedOnStreamPath(t *testing.T) {
	// pr-3003 TestCodexExecutorExecuteStream_PreservesPreviousResponseID — fork
	// preserved on the stream path; upstream deletes (codex_executor_stream.go:59).
	server, gotBody := newPortedExecuteServer(t)
	executor := NewCodexExecutor(&config.Config{})
	auth := &cliproxyauth.Auth{Attributes: map[string]string{
		"base_url": server.URL,
		"api_key":  "test",
	}}
	req := cliproxyexecutor.Request{Model: "gpt-5.4", Payload: []byte(`{
		"model":"gpt-5.4",
		"previous_response_id":"resp-prev",
		"prompt_cache_key":"pc-abc",
		"input":[{"type":"text","text":"hello"}]
	}`)}
	opts := cliproxyexecutor.Options{SourceFormat: sdktranslator.FromString("openai-response"), Stream: true}

	result, err := executor.ExecuteStream(context.Background(), auth, req, opts)
	if err != nil {
		t.Fatalf("ExecuteStream error: %v", err)
	}
	for range result.Chunks {
	}
	if got := gjson.GetBytes(*gotBody, "previous_response_id"); got.Exists() {
		t.Fatalf("previous_response_id = %q, want dropped on stream path; body=%s", got.String(), string(*gotBody))
	}
}

func TestPortedFork_PreviousResponseID_NoOpWhenAbsentFromInput(t *testing.T) {
	// b50dba3c TestNormalizePreviousResponseID_NoOpWithoutPreviousResponseID
	server, gotBody := newPortedExecuteServer(t)
	executor := NewCodexExecutor(&config.Config{})
	req, opts, auth := portedExecuteRequest(server.URL, []byte(`{
		"model":"gpt-5.4",
		"prompt_cache_key":"pc-abc",
		"input":[{"type":"message","role":"assistant","content":"Hello"}]
	}`))

	if _, err := executor.Execute(context.Background(), auth, req, opts); err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if gjson.GetBytes(*gotBody, "previous_response_id").Exists() {
		t.Fatalf("previous_response_id should not appear when absent from input; body=%s", string(*gotBody))
	}
}
