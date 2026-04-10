package config

import (
	"os"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	provider, model, err := cfg.ResolveModel("executive")
	if err != nil {
		t.Fatalf("ResolveModel: %v", err)
	}
	if provider != "claude-code" {
		t.Errorf("expected provider claude-code, got %s", provider)
	}
	if model != "claude-sonnet-4-20250514" {
		t.Errorf("expected model claude-sonnet-4-20250514, got %s", model)
	}
}

func TestLoadAndResolve(t *testing.T) {
	content := `
providers:
  claude-code:
    type: claude-code
  opencode:
    type: opencode-serve
    api_key_env: TEST_API_KEY
    models:
      opencode-go/glm-5.1:
        context_window: 128000
models:
  executive: opencode/opencode-go/glm-5.1
`
	f, err := os.CreateTemp("", "bud-test-*.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	f.WriteString(content)
	f.Close()

	os.Setenv("TEST_API_KEY", "test-key-123")
	defer os.Unsetenv("TEST_API_KEY")

	cfg, err := Load(f.Name())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	provider, model, err := cfg.ResolveModel("executive")
	if err != nil {
		t.Fatalf("ResolveModel: %v", err)
	}
	if provider != "opencode" {
		t.Errorf("expected provider opencode, got %s", provider)
	}
	if model != "opencode-go/glm-5.1" {
		t.Errorf("expected model opencode-go/glm-5.1, got %s", model)
	}

	key, err := cfg.APIKey("opencode")
	if err != nil {
		t.Fatalf("APIKey: %v", err)
	}
	if key != "test-key-123" {
		t.Errorf("expected api key test-key-123, got %s", key)
	}

	// claude-code has no api_key_env
	key, err = cfg.APIKey("claude-code")
	if err != nil {
		t.Fatalf("APIKey for claude-code: %v", err)
	}
	if key != "" {
		t.Errorf("expected empty api key for claude-code, got %s", key)
	}

	// Test ContextWindow
	cw := cfg.ContextWindow("opencode", "opencode-go/glm-5.1")
	if cw != 128000 {
		t.Errorf("expected context window 128000 for opencode/opencode-go/glm-5.1, got %d", cw)
	}
	cw = cfg.ContextWindow("claude-code", "claude-sonnet-4-20250514")
	if cw != 0 {
		t.Errorf("expected 0 context window for claude-code (not configured), got %d", cw)
	}
}

func TestValidation(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		errSubstr string
	}{
		{
			"unknown provider type",
			`providers:
  foo:
    type: unknown-type
models:
  executive: foo/bar`,
			"unknown type",
		},
		{
			"reference to missing provider",
			`providers:
  claude-code:
    type: claude-code
models:
  executive: nonexistent/model`,
			"unknown provider",
		},
		{
			"missing slash in model ref",
			`providers:
  claude-code:
    type: claude-code
models:
  executive: noslash`,
			"must be provider/model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.CreateTemp("", "bud-test-*.yaml")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(f.Name())
			f.WriteString(tt.yaml)
			f.Close()

			_, err = Load(f.Name())
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.errSubstr)
			}
			if !contains(err.Error(), tt.errSubstr) {
				t.Errorf("expected error containing %q, got %q", tt.errSubstr, err.Error())
			}
		})
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func TestSplitModelRef(t *testing.T) {
	tests := []struct {
		ref      string
		provider string
		model    string
		err      bool
	}{
		{"claude-code/claude-sonnet-4-20250514", "claude-code", "claude-sonnet-4-20250514", false},
		{"opencode/opencode-go/glm-5.1", "opencode", "opencode-go/glm-5.1", false},
		{"noslash", "", "", true},
	}
	for _, tt := range tests {
		p, m, err := SplitModelRef(tt.ref)
		if tt.err {
			if err == nil {
				t.Errorf("SplitModelRef(%q): expected error", tt.ref)
			}
		} else {
			if err != nil {
				t.Errorf("SplitModelRef(%q): unexpected error: %v", tt.ref, err)
			}
			if p != tt.provider {
				t.Errorf("SplitModelRef(%q): expected provider %q, got %q", tt.ref, tt.provider, p)
			}
			if m != tt.model {
				t.Errorf("SplitModelRef(%q): expected model %q, got %q", tt.ref, tt.model, m)
			}
		}
	}
}
