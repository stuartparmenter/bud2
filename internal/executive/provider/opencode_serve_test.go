package provider

import (
	"testing"
)

func TestSplitOpenCodeModel(t *testing.T) {
	tests := []struct {
		model      string
		providerID string
		modelID    string
	}{
		{"opencode-go/glm-5.1", "opencode-go", "glm-5.1"},
		{"glm-5.1", "", "glm-5.1"},
		{"anthropic/claude-sonnet-4", "anthropic", "claude-sonnet-4"},
	}
	for _, tt := range tests {
		pid, mid := splitOpenCodeModel(tt.model)
		if pid != tt.providerID {
			t.Errorf("splitOpenCodeModel(%q): providerID = %q, want %q", tt.model, pid, tt.providerID)
		}
		if mid != tt.modelID {
			t.Errorf("splitOpenCodeModel(%q): modelID = %q, want %q", tt.model, mid, tt.modelID)
		}
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		chars  int
		tokens int
	}{
		{0, 1},
		{1, 1},
		{3, 1},
		{4, 1},
		{100, 25},
		{1000, 250},
	}
	for _, tt := range tests {
		result := estimateTokens(tt.chars)
		if result != tt.tokens {
			t.Errorf("estimateTokens(%d) = %d, want %d", tt.chars, result, tt.tokens)
		}
	}
}

func TestOpenCodeSessionShouldReset(t *testing.T) {
	s := &OpenCodeSession{
		sessionID:     "test",
		contextWindow: 128000,
	}
	if s.ShouldReset() {
		t.Error("new session should not need reset")
	}

	s.mu.Lock()
	s.lastUsage = &SessionUsage{
		InputTokens:          80000,
		CacheReadInputTokens: 29000,
	}
	s.mu.Unlock()

	if s.ShouldReset() {
		t.Error("context within window should not reset")
	}

	s.mu.Lock()
	s.lastUsage = &SessionUsage{
		InputTokens:          100000,
		CacheReadInputTokens: 40000,
	}
	s.mu.Unlock()

	if !s.ShouldReset() {
		t.Error("context exceeding window should reset")
	}
}

func TestOpenCodeSessionResetClearsState(t *testing.T) {
	s := &OpenCodeSession{
		sessionID:     "test-1",
		ocSessionID:   "oc-123",
		contextWindow: 128000,
		lastUsage:     &SessionUsage{InputTokens: 100},
		isResuming:    true,
	}
	s.Reset()

	if s.ocSessionID != "" {
		t.Error("Reset should clear ocSessionID")
	}
	if s.lastUsage != nil {
		t.Error("Reset should clear lastUsage")
	}
	if s.isResuming {
		t.Error("Reset should clear isResuming")
	}
	if s.sessionID == "test-1" {
		t.Error("Reset should generate new sessionID")
	}
}

func TestOpenCodeServeProviderConfig(t *testing.T) {
	p := NewOpenCodeServeProvider("", "test-key", "opencode-go/glm-5.1", "")
	if p.Name() != "opencode-serve" {
		t.Errorf("Name() = %q, want %q", p.Name(), "opencode-serve")
	}
	if p.baseURL != "http://127.0.0.1:4096" {
		t.Errorf("default baseURL = %q, want %q", p.baseURL, "http://127.0.0.1:4096")
	}
	if p.binPath != "opencode" {
		t.Errorf("default binPath = %q, want %q", p.binPath, "opencode")
	}

	p2 := NewOpenCodeServeProvider("/usr/local/bin/opencode", "key2", "glm-5.1", "http://localhost:8080")
	if p2.binPath != "/usr/local/bin/opencode" {
		t.Errorf("custom binPath = %q, want %q", p2.binPath, "/usr/local/bin/opencode")
	}
	if p2.baseURL != "http://localhost:8080" {
		t.Errorf("custom baseURL = %q, want %q", p2.baseURL, "http://localhost:8080")
	}
}

func TestOpenCodeServeProviderContextWindow(t *testing.T) {
	p := NewOpenCodeServeProvider("", "", "opencode-go/glm-5.1", "")
	p.WithContextWindow(128000)

	// Don't call NewSession since that tries to start the server.
	// Just verify the context window is propagated to sessions.
	sess := &OpenCodeSession{
		provider:      p,
		sessionID:     "test-session",
		model:         "glm-5.1",
		providerID:    "opencode-go",
		contextWindow: p.contextWindow,
	}
	if sess.contextWindow != 128000 {
		t.Errorf("contextWindow = %d, want 128000", sess.contextWindow)
	}
}
