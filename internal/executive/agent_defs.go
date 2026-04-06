package executive

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	claudecode "github.com/severity1/claude-agent-sdk-go"
	"gopkg.in/yaml.v3"
)

// SkillGrants holds the centralized skill grant configuration.
type SkillGrants struct {
	Grants map[string][]string `yaml:"grants"` // pattern -> skill names
}

// LoadSkillGrants reads state/system/skill-grants.yaml.
// Returns empty grants (not an error) if the file is missing.
func LoadSkillGrants(statePath string) (*SkillGrants, error) {
	grantsPath := filepath.Join(statePath, "system", "skill-grants.yaml")
	data, err := os.ReadFile(grantsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &SkillGrants{Grants: make(map[string][]string)}, nil
		}
		return nil, fmt.Errorf("read skill-grants.yaml: %w", err)
	}
	var sg SkillGrants
	if err := yaml.Unmarshal(data, &sg); err != nil {
		return nil, fmt.Errorf("parse skill-grants.yaml: %w", err)
	}
	if sg.Grants == nil {
		sg.Grants = make(map[string][]string)
	}
	return &sg, nil
}

// resolveGrantedSkills returns the skills granted to the given "namespace:agent" key.
// Match priority: exact key > "namespace:*" > wildcard patterns (filepath.Match) > "*".
// Returns nil if no grants file entry applies.
func resolveGrantedSkills(grants *SkillGrants, key string) ([]string, bool) {
	if grants == nil || len(grants.Grants) == 0 {
		return nil, false
	}

	// 1. Exact match
	if skills, ok := grants.Grants[key]; ok {
		return skills, true
	}

	// 2. Namespace wildcard: "namespace:*"
	if idx := strings.Index(key, ":"); idx != -1 {
		nsWild := key[:idx] + ":*"
		if skills, ok := grants.Grants[nsWild]; ok {
			return skills, true
		}
	}

	// 3. Pattern wildcards (e.g. "autopilot-*:planner")
	for pattern, skills := range grants.Grants {
		if pattern == "*" || strings.HasSuffix(pattern, ":*") {
			continue // already handled above or handled as global below
		}
		matched, err := filepath.Match(pattern, key)
		if err == nil && matched {
			return skills, true
		}
	}

	// 4. Global wildcard "*"
	if skills, ok := grants.Grants["*"]; ok {
		return skills, true
	}

	return nil, false
}

// LoadAllAgents scans state/system/plugins/*/agents/ and builds an AgentDefinition map
// for use with the SDK's WithAgents option. Keys are "namespace:agent" (e.g.
// "autopilot-vision:explorer", "bud:coder").
//
// For each agent, the prompt is assembled as: agent body + concatenated skill content
// (same logic as ResolveSubagentConfig). The Tools list contains the agent's declared
// tools with Agent(...) syntax normalized to plain "Agent".
func LoadAllAgents(statePath string) (map[string]claudecode.AgentDefinition, error) {
	pluginsDir := filepath.Join(statePath, "system", "plugins")
	namespaces, err := os.ReadDir(pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]claudecode.AgentDefinition{}, nil
		}
		return nil, fmt.Errorf("read plugins dir: %w", err)
	}

	// Load aliases for skill resolution
	aliases, aliasErr := LoadAgentAliases(statePath)
	if aliasErr != nil {
		aliases = &AgentAliases{Agents: make(map[string]string), Skills: make(map[string]string)}
	}

	// Load centralized skill grants (optional — falls back to agent.Skills if missing)
	grants, _ := LoadSkillGrants(statePath)

	defs := make(map[string]claudecode.AgentDefinition)

	for _, nsEntry := range namespaces {
		if !nsEntry.IsDir() {
			continue
		}
		namespace := nsEntry.Name()
		agentsDir := filepath.Join(pluginsDir, namespace, "agents")

		agentFiles, err := os.ReadDir(agentsDir)
		if err != nil {
			continue // no agents dir for this plugin
		}

		for _, f := range agentFiles {
			if f.IsDir() {
				continue
			}
			fname := f.Name()
			ext := filepath.Ext(fname)
			if ext != ".yaml" && ext != ".md" {
				continue
			}
			agentName := strings.TrimSuffix(fname, ext)
			key := namespace + ":" + agentName

			data, err := os.ReadFile(filepath.Join(agentsDir, fname))
			if err != nil {
				continue
			}

			agent, err := parseAgentData(data, agentName)
			if err != nil {
				continue
			}

			// Determine skill list: grants file wins; fall back to agent.Skills
			var skillNames []string
			if grantedSkills, ok := resolveGrantedSkills(grants, key); ok {
				skillNames = grantedSkills
			} else {
				skillNames = agent.Skills
			}

			// Assemble prompt: agent body + skill content
			var skillParts []string
			for _, skillName := range skillNames {
				if target, ok := aliases.Skills[skillName]; ok {
					skillName = target
				}
				content, skillErr := LoadSkillContent(allPluginDirs(statePath), skillName)
				if skillErr != nil || content == "" {
					continue
				}
				skillParts = append(skillParts, content)
			}
			skillContent := strings.Join(skillParts, "\n\n---\n\n")

			var prompt string
			if agent.Body != "" {
				if skillContent != "" {
					prompt = "## Agent Behavioral Guide\n\n" + agent.Body + "\n\n---\n\n" + skillContent
				} else {
					prompt = "## Agent Behavioral Guide\n\n" + agent.Body
				}
			} else {
				prompt = skillContent
			}

			// Build tools list, normalizing Agent(...) declarations to plain "Agent"
			var tools []string
			seen := make(map[string]bool)
			for _, t := range agent.Tools {
				t = strings.TrimSpace(t)
				if strings.HasPrefix(t, "Agent(") {
					t = "Agent"
				}
				if t != "" && !seen[t] {
					seen[t] = true
					tools = append(tools, t)
				}
			}

			defs[key] = claudecode.AgentDefinition{
				Description: agent.Description,
				Prompt:      prompt,
				Tools:       tools,
				Model:       parseAgentModel(agent.Model),
			}
		}
	}

	return defs, nil
}

// parseAgentModel converts a model string from agent YAML to an AgentModel enum value.
func parseAgentModel(model string) claudecode.AgentModel {
	switch strings.ToLower(strings.TrimSpace(model)) {
	case "sonnet":
		return claudecode.AgentModelSonnet
	case "opus":
		return claudecode.AgentModelOpus
	case "haiku":
		return claudecode.AgentModelHaiku
	default:
		return claudecode.AgentModelInherit
	}
}
