package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

// fixturePath returns the snapshot of the production cpa-codex config.yaml used to
// validate v7.2.141 compatibility (Task 4). The snapshot mirrors the production file's
// keys and values; only the remote-management secret-key bcrypt hash is redacted.
func fixturePath() string {
	return filepath.Join("testdata", "cpa-codex-config-v7.2.141.fixture.yaml")
}

func readFixture(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(fixturePath())
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	return raw
}

// collectYAMLKeys returns every YAML key accepted by typ, following inlined structs.
func collectYAMLKeys(t *testing.T, typ reflect.Type) map[string]bool {
	t.Helper()
	keys := make(map[string]bool)
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			tag := f.Tag.Get("yaml")
			if tag == "-" || tag == "" {
				continue
			}
			if strings.Contains(tag, "inline") {
				walk(f.Type)
				continue
			}
			name := strings.Split(tag, ",")[0]
			if name != "" {
				keys[name] = true
			}
		}
	}
	walk(typ)
	return keys
}

// collectTopLevelYAMLKeys returns every top-level YAML key accepted by Config,
// including keys from the inlined SDKConfig.
func collectTopLevelYAMLKeys(t *testing.T) map[string]bool {
	t.Helper()
	return collectYAMLKeys(t, reflect.TypeOf(Config{}))
}

// collectCodexYAMLKeys returns every YAML key accepted by CodexConfig.
func collectCodexYAMLKeys(t *testing.T) map[string]bool {
	t.Helper()
	return collectYAMLKeys(t, reflect.TypeOf(CodexConfig{}))
}

// TestCPAProductionConfig_LoadsWithoutErrorOnV7_2_141 is the config-load validation:
// the production config.yaml snapshot must parse without error on the v7.2.141 schema.
// RED condition: any value type rejected by the v7.2.141 parser or any validation
// failure during load.
func TestCPAProductionConfig_LoadsWithoutErrorOnV7_2_141(t *testing.T) {
	cfg, err := ParseConfigBytes(readFixture(t))
	if err != nil {
		t.Fatalf("production config.yaml rejected by v7.2.141 build: %v", err)
	}
	if cfg == nil {
		t.Fatal("ParseConfigBytes returned nil config")
	}
}

// TestCPAProductionConfig_AllTopLevelKeysKnown is the static key-drift check: every
// top-level key present in the production config.yaml must map to a yaml tag accepted
// by the v7.2.141 Config struct. Because yaml.Unmarshal silently ignores unknown keys,
// this test is the guard that would go RED if a key were renamed or removed upstream.
func TestCPAProductionConfig_AllTopLevelKeysKnown(t *testing.T) {
	var doc map[string]any
	if err := yaml.Unmarshal(readFixture(t), &doc); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	known := collectTopLevelYAMLKeys(t)
	var unknown []string
	for key := range doc {
		if !known[key] {
			unknown = append(unknown, key)
		}
	}
	if len(unknown) > 0 {
		t.Fatalf("top-level keys in production config.yaml not accepted by v7.2.141: %v", unknown)
	}
}

// TestCPAProductionConfig_KeyValuesBehaveAsIntended asserts the cache-relevant keys in
// use behave as intended on v7.2.141: routing round-robin, no session affinity, 1h TTL,
// request-log enabled, passthrough-headers disabled, codex identity-confuse disabled,
// ws-auth enabled.
func TestCPAProductionConfig_KeyValuesBehaveAsIntended(t *testing.T) {
	cfg, err := ParseConfigBytes(readFixture(t))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	if got := cfg.Routing.Strategy; got != "round-robin" {
		t.Errorf("routing.strategy = %q, want %q", got, "round-robin")
	}
	if cfg.Routing.SessionAffinity {
		t.Errorf("routing.session-affinity = true, want false")
	}
	if got := cfg.Routing.SessionAffinityTTL; got != "1h" {
		t.Errorf("routing.session-affinity-ttl = %q, want %q", got, "1h")
	}
	if _, errParse := time.ParseDuration(cfg.Routing.SessionAffinityTTL); errParse != nil {
		t.Errorf("routing.session-affinity-ttl %q is not a valid duration: %v", cfg.Routing.SessionAffinityTTL, errParse)
	}
	if cfg.Codex.IdentityConfuse {
		t.Errorf("codex.identity-confuse = true, want false")
	}
	if !cfg.WebsocketAuth {
		t.Errorf("ws-auth = false, want true")
	}
	if !cfg.RequestLog {
		t.Errorf("request-log = false, want true")
	}
	if cfg.PassthroughHeaders {
		t.Errorf("passthrough-headers = true, want false")
	}
	if cfg.Port != 8317 {
		t.Errorf("port = %d, want 8317", cfg.Port)
	}
	if len(cfg.APIKeys) != 1 || cfg.APIKeys[0] != "sk-cliproxy" {
		t.Errorf("api-keys = %v, want [sk-cliproxy]", cfg.APIKeys)
	}
}

// TestCPAProductionConfig_ForceStablePromptCacheKeyKnown is the schema-drift guard for the
// new codex.force-stable-prompt-cache-key key: the CodexConfig struct must accept the key
// on the v7.2.141 schema, otherwise the YAML key would be silently ignored by yaml.Unmarshal.
func TestCPAProductionConfig_ForceStablePromptCacheKeyKnown(t *testing.T) {
	keys := collectCodexYAMLKeys(t)
	if !keys["force-stable-prompt-cache-key"] {
		t.Fatalf("codex.force-stable-prompt-cache-key not a known key in CodexConfig (schema drift)")
	}
}

// TestCPAProductionConfig_ForceStablePromptCacheKey_DefaultsTrue asserts the cutover
// default: the production fixture does not set the key, so parsing it must yield true.
func TestCPAProductionConfig_ForceStablePromptCacheKey_DefaultsTrue(t *testing.T) {
	cfg, errParse := ParseConfigBytes(readFixture(t))
	if errParse != nil {
		t.Fatalf("parse fixture: %v", errParse)
	}
	if !cfg.Codex.ForceStablePromptCacheKey {
		t.Errorf("absent codex.force-stable-prompt-cache-key: default = false, want true")
	}
}

// TestCPAProductionConfig_ForceStablePromptCacheKey_ExplicitFalseHonored asserts the
// instant-rollback path stays honored under the compat schema.
func TestCPAProductionConfig_ForceStablePromptCacheKey_ExplicitFalseHonored(t *testing.T) {
	cfg, errParse := ParseConfigBytes([]byte("codex:\n  force-stable-prompt-cache-key: false\n"))
	if errParse != nil {
		t.Fatalf("parse inline config: %v", errParse)
	}
	if cfg.Codex.ForceStablePromptCacheKey {
		t.Errorf("explicit codex.force-stable-prompt-cache-key: false not honored; got true")
	}
}
