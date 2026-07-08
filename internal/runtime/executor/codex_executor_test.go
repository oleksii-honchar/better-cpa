package executor

import (
	"net/http"
	"testing"
)

func TestApplyCodexContinuityHeaders_SetsSessionId(t *testing.T) {
	headers := http.Header{}
	continuity := codexContinuity{Key: "test-key-123"}

	applyCodexContinuityHeaders(headers, continuity)

	if got := headers.Get("Session-Id"); got != "test-key-123" {
		t.Fatalf("Session-Id = %q, want %q", got, "test-key-123")
	}
	if got := headers.Get("Session_id"); got != "" {
		t.Fatalf("Session_id = %q, want empty (should use Session-Id with hyphen)", got)
	}
}

func TestApplyCodexContinuityHeaders_EmptyKeySetsNothing(t *testing.T) {
	headers := http.Header{}
	continuity := codexContinuity{Key: ""}

	applyCodexContinuityHeaders(headers, continuity)

	if got := headers.Get("Session-Id"); got != "" {
		t.Fatalf("Session-Id = %q, want empty for empty continuity key", got)
	}
	if got := headers.Get("Session_id"); got != "" {
		t.Fatalf("Session_id = %q, want empty for empty continuity key", got)
	}
}
