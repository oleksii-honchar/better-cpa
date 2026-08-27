package config

import (
	"testing"
)

// TestParseConfigBytes_CodexForceStablePromptCacheKey_DefaultsTrueWhenCodexBlockAbsent
// is the TDD red condition for the cutover default: an absent codex block must yield
// ForceStablePromptCacheKey == true so the stable per-session prompt_cache_key override
// is active out of the box (spec §10.1; rollback = flip false or run an old image).
func TestParseConfigBytes_CodexForceStablePromptCacheKey_DefaultsTrueWhenCodexBlockAbsent(t *testing.T) {
	cfg, errParse := ParseConfigBytes([]byte("port: 8317\n"))
	if errParse != nil {
		t.Fatalf("ParseConfigBytes() error = %v", errParse)
	}
	if !cfg.Codex.ForceStablePromptCacheKey {
		t.Errorf("absent codex block: ForceStablePromptCacheKey = false, want true")
	}
}

// TestParseConfigBytes_CodexForceStablePromptCacheKey_DefaultsTrueWhenCodexBlockEmpty
// covers the empty codex: {} case explicitly so an empty block cannot silently flip the
// override off.
func TestParseConfigBytes_CodexForceStablePromptCacheKey_DefaultsTrueWhenCodexBlockEmpty(t *testing.T) {
	cfg, errParse := ParseConfigBytes([]byte("codex: {}\n"))
	if errParse != nil {
		t.Fatalf("ParseConfigBytes() error = %v", errParse)
	}
	if !cfg.Codex.ForceStablePromptCacheKey {
		t.Errorf("empty codex block: ForceStablePromptCacheKey = false, want true")
	}
}

// TestParseConfigBytes_CodexForceStablePromptCacheKey_ExplicitFalseHonored is the
// instant-rollback path: an explicit false must win over the cutover default.
func TestParseConfigBytes_CodexForceStablePromptCacheKey_ExplicitFalseHonored(t *testing.T) {
	cfg, errParse := ParseConfigBytes([]byte("codex:\n  force-stable-prompt-cache-key: false\n"))
	if errParse != nil {
		t.Fatalf("ParseConfigBytes() error = %v", errParse)
	}
	if cfg.Codex.ForceStablePromptCacheKey {
		t.Errorf("explicit force-stable-prompt-cache-key: false not honored; got true")
	}
}

// TestParseConfigBytes_CodexForceStablePromptCacheKey_ExplicitTrueHonored guards that an
// explicit true survives parsing verbatim.
func TestParseConfigBytes_CodexForceStablePromptCacheKey_ExplicitTrueHonored(t *testing.T) {
	cfg, errParse := ParseConfigBytes([]byte("codex:\n  force-stable-prompt-cache-key: true\n"))
	if errParse != nil {
		t.Fatalf("ParseConfigBytes() error = %v", errParse)
	}
	if !cfg.Codex.ForceStablePromptCacheKey {
		t.Errorf("explicit force-stable-prompt-cache-key: true not honored; got false")
	}
}
