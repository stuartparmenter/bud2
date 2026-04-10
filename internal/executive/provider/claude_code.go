package provider

import "fmt"

// ClaudeCodeProvider creates sessions that use the Claude Code SDK (subprocess).
// The actual session management is done by SimpleSession in the executive package,
// which implements the Session interface directly.
type ClaudeCodeProvider struct {
	model string
}

func NewClaudeCodeProvider(model string) *ClaudeCodeProvider {
	if model == "" {
		model = DefaultModelClaude
	}
	return &ClaudeCodeProvider{model: model}
}

func (p *ClaudeCodeProvider) Name() string { return "claude-code" }

func (p *ClaudeCodeProvider) Model() string { return p.model }

// NewSession returns an error for claude-code because sessions are created
// directly via SimpleSession in the executive package, which implements
// the Session interface. The provider is used for config resolution only.
func (p *ClaudeCodeProvider) NewSession(opts SessionOpts) (Session, error) {
	return nil, fmt.Errorf("claude-code sessions are created via SimpleSession, not through provider.NewSession")
}
