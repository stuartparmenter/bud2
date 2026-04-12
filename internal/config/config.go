package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ExtensionsConfig controls how remote extensions (plugins and skills) are
// fetched and updated.
type ExtensionsConfig struct {
	// UpdateInterval is a Go duration string (e.g. "1h", "30m", "0") that
	// controls how often floating (unpinned) remote skills are re-fetched.
	// "0" disables automatic updates entirely. Default when empty: 1 hour.
	UpdateInterval string `yaml:"update_interval,omitempty"`
}

// ParsedUpdateInterval returns the parsed duration, defaulting to 1 hour when
// UpdateInterval is empty. Returns 0 if the value is "0" (updates disabled).
func (e ExtensionsConfig) ParsedUpdateInterval() time.Duration {
	if e.UpdateInterval == "" {
		return time.Hour
	}
	d, err := time.ParseDuration(e.UpdateInterval)
	if err != nil {
		return time.Hour // fall back to default on bad config
	}
	return d
}

type BudConfig struct {
	Providers       map[string]ProviderConfig `yaml:"providers"`
	Models          map[string]string         `yaml:"models"`
	TerminalManager string                    `yaml:"terminal_manager,omitempty"`
	Extensions      ExtensionsConfig          `yaml:"extensions,omitempty"`
}

type ProviderConfig struct {
	Type       string                 `yaml:"type"`
	APIKeyEnv  string                 `yaml:"api_key_env"`
	BaseURL    string                 `yaml:"base_url,omitempty"`
	Models     map[string]ModelConfig `yaml:"models,omitempty"`
	Properties map[string]any         `yaml:"properties,omitempty"`
}

type ModelConfig struct {
	ContextWindow   int `yaml:"context_window,omitempty"`
	MaxOutputTokens int `yaml:"max_output_tokens,omitempty"`
}

func Load(path string) (*BudConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}
	var cfg BudConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file %s: %w", path, err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func DefaultConfig() *BudConfig {
	return &BudConfig{
		Providers: map[string]ProviderConfig{
			"claude-code": {
				Type: "claude-code",
			},
		},
		Models: map[string]string{
			"executive": "claude-code/claude-sonnet-4-20250514",
		},
	}
}

func (c *BudConfig) Validate() error {
	for name, pc := range c.Providers {
		switch pc.Type {
		case "claude-code", "opencode-serve", "openai-compatible":
		default:
			return fmt.Errorf("provider %q: unknown type %q (must be claude-code, opencode-serve, or openai-compatible)", name, pc.Type)
		}
	}
	for role, ref := range c.Models {
		providerName, _, err := SplitModelRef(ref)
		if err != nil {
			return fmt.Errorf("models.%s: %w", role, err)
		}
		if _, ok := c.Providers[providerName]; !ok {
			return fmt.Errorf("models.%s: references unknown provider %q", role, providerName)
		}
	}
	if c.TerminalManager != "" && c.TerminalManager != "zellij" && c.TerminalManager != "tmux" {
		return fmt.Errorf("terminal_manager: must be \"zellij\" or \"tmux\", got %q", c.TerminalManager)
	}
	return nil
}

func (c *BudConfig) GetTerminalManager() string {
	if c.TerminalManager == "" {
		return "zellij"
	}
	return c.TerminalManager
}

func (c *BudConfig) ResolveModel(role string) (providerName, modelID string, err error) {
	ref, ok := c.Models[role]
	if !ok {
		// Fall back to "executive" role if the requested role isn't configured
		if role != "executive" {
			return c.ResolveModel("executive")
		}
		return "", "", fmt.Errorf("no model configured for role %q", role)
	}
	providerName, modelID, err = SplitModelRef(ref)
	if err != nil {
		return "", "", err
	}
	if _, ok := c.Providers[providerName]; !ok {
		return "", "", fmt.Errorf("provider %q not found in config for role %q", providerName, role)
	}
	return providerName, modelID, nil
}

func (c *BudConfig) APIKey(providerName string) (string, error) {
	pc, ok := c.Providers[providerName]
	if !ok {
		return "", fmt.Errorf("provider %q not found", providerName)
	}
	if pc.APIKeyEnv == "" {
		return "", nil
	}
	key := os.Getenv(pc.APIKeyEnv)
	if key == "" {
		return "", fmt.Errorf("environment variable %s (api_key_env for provider %q) is not set", pc.APIKeyEnv, providerName)
	}
	return key, nil
}

func (c *BudConfig) ContextWindow(providerName, modelID string) int {
	pc, ok := c.Providers[providerName]
	if !ok {
		return 0
	}
	if mc, ok := pc.Models[modelID]; ok && mc.ContextWindow > 0 {
		return mc.ContextWindow
	}
	return 0
}

func SplitModelRef(ref string) (providerName, modelID string, err error) {
	idx := strings.Index(ref, "/")
	if idx < 0 {
		return "", "", fmt.Errorf("invalid model reference %q: must be provider/model", ref)
	}
	return ref[:idx], ref[idx+1:], nil
}
