package provider

import (
	"context"
	"fmt"
)

type Provider interface {
	Name() string
	NewSession(opts SessionOpts) (Session, error)
}

type Session interface {
	SendPrompt(ctx context.Context, prompt string, cb StreamCallbacks) (*SessionResult, error)
	SessionID() string
	ShouldReset() bool
	PrepareForResume()
	Reset()
	LastUsage() *SessionUsage
	Close() error
}

type StreamCallbacks struct {
	OnText       func(text string)
	OnThinking   func(text string)
	OnTool       func(name string, input map[string]any)
	OnResult     func(usage *SessionUsage)
	OnPermission PermissionHandler // nil = auto-approve
}

// PermissionHandler is called when the opencode provider encounters a
// permission prompt. It should return the decision. When OnPermission is
// nil, all permissions are auto-approved.
type PermissionHandler func(perm PermissionRequest) PermissionDecision

type PermissionRequest struct {
	ID        string
	SessionID string
	Type      string // "edit", "bash", etc.
	Title     string
	Metadata  map[string]any
}

type PermissionDecision string

const (
	PermissionAllow PermissionDecision = "allow"
	PermissionDeny  PermissionDecision = "deny"
)

type SessionOpts struct {
	Model        string
	WorkDir      string
	MCPServerURL string
}

type SessionResult struct {
	SessionID string
	Usage     *SessionUsage
}

type SessionUsage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
	NumTurns                 int `json:"num_turns"`
	DurationMs               int `json:"duration_ms"`
	DurationApiMs            int `json:"duration_api_ms"`
	ContextWindow            int `json:"context_window,omitempty"`
	MaxOutputTokens          int `json:"max_output_tokens,omitempty"`
}

func (u *SessionUsage) TotalInputTokens() int {
	if u == nil {
		return 0
	}
	return u.InputTokens + u.CacheCreationInputTokens + u.CacheReadInputTokens
}

func (u *SessionUsage) CacheHitRate() float64 {
	if u == nil {
		return 0
	}
	total := u.TotalInputTokens()
	if total == 0 {
		return 0
	}
	return float64(u.CacheReadInputTokens) / float64(total)
}

var ErrInterrupted = fmt.Errorf("session interrupted")

const MaxContextTokensDefault = 150000

const DefaultModelClaude = "claude-sonnet-4-20250514"
