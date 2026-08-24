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

// collectTopLevelYAMLKeys returns every top-level YAML key accepted by Config,
// including keys from the inlined SDKConfig.
func collectTopLevelYAMLKeys(t *testing.T) map[string]bool {
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
	walk(reflect.TypeOf(Config{}))
	return keys
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
