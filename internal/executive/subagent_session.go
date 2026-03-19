// Package executive provides the executive session manager for Bud.
// This file implements the subagent session infrastructure for Project 2:
// long-lived Claude subprocesses that work autonomously under executive supervision.
package executive

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// SubagentStatus describes the current lifecycle state of a subagent session.
type SubagentStatus int

const (
	SubagentRunning         SubagentStatus = iota // Claude subprocess is running
	SubagentWaitingForInput                       // Blocked on request_input tool
	SubagentCompleted                             // Finished successfully
	SubagentFailed                                // Exited with error
)

func (s SubagentStatus) String() string {
	switch s {
	case SubagentRunning:
		return "running"
	case SubagentWaitingForInput:
		return "waiting_for_input"
	case SubagentCompleted:
		return "completed"
	case SubagentFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// SubagentSession manages a long-lived Claude subprocess for a specific task.
// The subagent runs autonomously with a restricted tool set and can surface
// questions to the executive via the request_input MCP tool.
type SubagentSession struct {
	// Identifiers
	ID              string    // Bud-internal UUID
	ClaudeSessionID string    // Claude's session ID (for --resume across turns)
	Task            string    // Short task description
	SpawnedAt       time.Time // When the session was created

	// State (protected by mu)
	mu              sync.Mutex
	status          SubagentStatus
	pendingQuestion string // Set when SubagentWaitingForInput
	result          string // Final output when Completed
	lastErr         error  // Error when Failed

	// Signal channels
	questionReady chan struct{} // Closed when a question is waiting
	answerReady   chan string   // Executive sends answer here

	// File paths for question/answer IPC (fallback for non-blocking scenarios)
	stateDir     string // Base state directory
	questionFile string // {stateDir}/subagent-questions/{ID}.txt
	answerFile   string // {stateDir}/subagent-answers/{ID}.txt

	// MCP config for this session's request_input server
	mcpConfigPath string

	// ClaudeSessionID from result event (updated each turn)
	claudeSessionID string
}

// Status returns the current lifecycle status.
func (s *SubagentSession) Status() SubagentStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status
}

// PendingQuestion returns the question waiting for an answer, or "" if none.
func (s *SubagentSession) PendingQuestion() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pendingQuestion
}

// Result returns the final output (only meaningful when Completed).
func (s *SubagentSession) Result() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.result
}

// Err returns the error (only meaningful when Failed).
func (s *SubagentSession) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastErr
}

// SubagentManager maintains a registry of active subagent sessions and
// provides spawn/answer/status operations for the executive.
type SubagentManager struct {
	mu       sync.RWMutex
	sessions map[string]*SubagentSession

	stateDir    string // For question/answer file IPC
	mcpBinaryPath string // Path to subagent-mcp binary (request_input server)

	// Notify executive when a subagent has a pending question
	QuestionNotify chan *SubagentSession
}

// NewSubagentManager creates a new manager. stateDir is the bud state directory.
func NewSubagentManager(stateDir string) *SubagentManager {
	m := &SubagentManager{
		sessions:       make(map[string]*SubagentSession),
		stateDir:       stateDir,
		QuestionNotify: make(chan *SubagentSession, 16),
	}
	// Ensure question/answer directories exist
	os.MkdirAll(filepath.Join(stateDir, "subagent-questions"), 0755)
	os.MkdirAll(filepath.Join(stateDir, "subagent-answers"), 0755)
	return m
}

// SubagentConfig controls how a subagent session is spawned.
type SubagentConfig struct {
	// Task is what the subagent should do (injected as system prompt).
	Task string

	// SystemPromptAppend is extra content appended to the subagent's system prompt.
	// Use this to restrict tools, set constraints, or give role context.
	SystemPromptAppend string

	// Model overrides the default model (empty = use default).
	Model string

	// WorkDir overrides the working directory for the Claude subprocess.
	WorkDir string

	// AllowedTools restricts which built-in tools the subagent can use.
	// Empty = all tools. Example: "Bash,Read,Glob,Grep,Write"
	AllowedTools string
}

// Spawn creates a new SubagentSession and starts it in a background goroutine.
// Returns the session ID. The session runs until the task is done, it calls
// signal_done, or it encounters an unrecoverable error.
func (m *SubagentManager) Spawn(ctx context.Context, cfg SubagentConfig) (*SubagentSession, error) {
	id := generateSessionUUID()

	session := &SubagentSession{
		ID:            id,
		Task:          cfg.Task,
		SpawnedAt:     time.Now(),
		status:        SubagentRunning,
		questionReady: make(chan struct{}),
		answerReady:   make(chan string, 1),
		stateDir:      m.stateDir,
		questionFile:  filepath.Join(m.stateDir, "subagent-questions", id+".txt"),
		answerFile:    filepath.Join(m.stateDir, "subagent-answers", id+".txt"),
	}

	// Build the MCP config for request_input
	mcpConfigPath, err := m.writeMCPConfig(session)
	if err != nil {
		return nil, fmt.Errorf("failed to write MCP config: %w", err)
	}
	session.mcpConfigPath = mcpConfigPath

	// Register session
	m.mu.Lock()
	m.sessions[id] = session
	m.mu.Unlock()

	// Start the session goroutine
	go m.runSession(ctx, session, cfg)

	log.Printf("[subagent-manager] Spawned session %s: %s", id, truncate(cfg.Task, 60))
	return session, nil
}

// Answer provides a reply to a subagent's pending question.
// Returns error if the session is not waiting for input.
func (m *SubagentManager) Answer(sessionID, answer string) error {
	m.mu.RLock()
	session, ok := m.sessions[sessionID]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("subagent session not found: %s", sessionID)
	}

	session.mu.Lock()
	if session.status != SubagentWaitingForInput {
		session.mu.Unlock()
		return fmt.Errorf("subagent %s is not waiting for input (status: %s)", sessionID, session.status)
	}
	session.status = SubagentRunning
	session.pendingQuestion = ""
	session.mu.Unlock()

	// Write answer to file (the MCP server polls this)
	if err := os.WriteFile(session.answerFile, []byte(answer), 0644); err != nil {
		return fmt.Errorf("failed to write answer: %w", err)
	}

	log.Printf("[subagent-manager] Answer provided to session %s", sessionID)
	return nil
}

// List returns a snapshot of all active sessions.
func (m *SubagentManager) List() []*SubagentSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sessions := make([]*SubagentSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

// Get returns a session by ID.
func (m *SubagentManager) Get(id string) *SubagentSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

// Cleanup removes completed/failed sessions older than the given duration.
func (m *SubagentManager) Cleanup(olderThan time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	cutoff := time.Now().Add(-olderThan)
	var removed int
	for id, s := range m.sessions {
		s.mu.Lock()
		done := s.status == SubagentCompleted || s.status == SubagentFailed
		old := s.SpawnedAt.Before(cutoff)
		s.mu.Unlock()
		if done && old {
			delete(m.sessions, id)
			os.Remove(s.questionFile)
			os.Remove(s.answerFile)
			if s.mcpConfigPath != "" {
				os.Remove(s.mcpConfigPath)
			}
			removed++
		}
	}
	return removed
}

// runSession runs the subagent Claude subprocess to completion.
// It loops, allowing multi-turn sessions via --resume when question routing occurs.
func (m *SubagentManager) runSession(ctx context.Context, session *SubagentSession, cfg SubagentConfig) {
	defer func() {
		// Clean up question file on exit
		os.Remove(session.questionFile)
	}()

	// Build system prompt for the subagent
	systemPrompt := buildSubagentSystemPrompt(cfg)

	// Build the prompt (task description as first user turn)
	prompt := fmt.Sprintf("## Task\n%s\n\nBegin work on this task. When you are done, call signal_done.", cfg.Task)

	const maxTurns = 20 // Safety valve against infinite loops
	for turn := 0; turn < maxTurns; turn++ {
		// Clear previous question file before each turn
		os.Remove(session.questionFile)

		// Build args
		args := []string{
			"--print",
			"--dangerously-skip-permissions",
			"--output-format", "stream-json",
			"--verbose",
			"--append-system-prompt", systemPrompt,
		}

		if session.claudeSessionID != "" {
			args = append(args, "--resume", session.claudeSessionID)
		}
		if cfg.Model != "" {
			args = append(args, "--model", cfg.Model)
		}
		if session.mcpConfigPath != "" {
			args = append(args, "--mcp-config", session.mcpConfigPath)
		}
		if cfg.AllowedTools != "" {
			args = append(args, "--allowedTools", cfg.AllowedTools)
		}
		args = append(args, prompt)

		log.Printf("[subagent] Session %s turn %d: running claude (resume=%s)", session.ID, turn, session.claudeSessionID)

		result, newSessionID, err := runSubagentClaude(ctx, args, cfg.WorkDir)

		// Store new Claude session ID for --resume on next turn
		if newSessionID != "" {
			session.mu.Lock()
			session.claudeSessionID = newSessionID
			session.mu.Unlock()
		}

		if err != nil {
			log.Printf("[subagent] Session %s turn %d error: %v", session.ID, turn, err)
			session.mu.Lock()
			session.status = SubagentFailed
			session.lastErr = err
			session.mu.Unlock()
			return
		}

		// Check if a question was written to the question file
		questionData, qerr := os.ReadFile(session.questionFile)
		if qerr == nil && len(questionData) > 0 {
			question := strings.TrimSpace(string(questionData))
			log.Printf("[subagent] Session %s has question: %s", session.ID, truncate(question, 80))

			// Notify executive
			session.mu.Lock()
			session.status = SubagentWaitingForInput
			session.pendingQuestion = question
			session.mu.Unlock()

			// Send to notification channel (non-blocking — executive may not be listening)
			select {
			case m.QuestionNotify <- session:
			default:
			}

			// Wait for answer file to appear (poll)
			answer, waitErr := waitForAnswer(ctx, session.answerFile, 30*time.Minute)
			if waitErr != nil {
				log.Printf("[subagent] Session %s: answer wait failed: %v", session.ID, waitErr)
				session.mu.Lock()
				session.status = SubagentFailed
				session.lastErr = waitErr
				session.mu.Unlock()
				return
			}

			// Resume with the answer as the new prompt
			prompt = fmt.Sprintf("The answer to your question is: %s\n\nPlease continue your task.", answer)
			os.Remove(session.answerFile)
			continue
		}

		// No question — session completed
		log.Printf("[subagent] Session %s completed", session.ID)
		session.mu.Lock()
		session.status = SubagentCompleted
		session.result = result
		session.mu.Unlock()
		return
	}

	// Hit max turns
	session.mu.Lock()
	session.status = SubagentFailed
	session.lastErr = fmt.Errorf("exceeded max turns (%d)", maxTurns)
	session.mu.Unlock()
}

// runSubagentClaude runs claude --print with the given args and returns
// (resultText, claudeSessionID, error).
func runSubagentClaude(ctx context.Context, args []string, workDir string) (string, string, error) {
	log.Printf("[subagent] Running: claude %s", strings.Join(args, " "))

	cmd := exec.CommandContext(ctx, "claude", args...)
	if workDir != "" {
		cmd.Dir = workDir
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", "", fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", "", fmt.Errorf("start: %w", err)
	}

	// Drain stderr
	go func() {
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			log.Printf("[subagent stderr] %s", sc.Text())
		}
	}()

	resultText, sessionID := parseSubagentOutput(stdout)

	if err := cmd.Wait(); err != nil {
		if ctx.Err() != nil {
			return resultText, sessionID, ctx.Err()
		}
		return resultText, sessionID, fmt.Errorf("claude exit: %w", err)
	}

	return resultText, sessionID, nil
}

// parseSubagentOutput reads stream-json from the Claude subprocess and
// extracts the result text and session ID.
func parseSubagentOutput(r io.Reader) (string, string) {
	var resultText, sessionID string
	scanner := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		var eventType string
		if v, ok := raw["type"]; ok {
			json.Unmarshal(v, &eventType)
		}

		switch eventType {
		case "result":
			if v, ok := raw["session_id"]; ok {
				json.Unmarshal(v, &sessionID)
			}
			if v, ok := raw["result"]; ok {
				json.Unmarshal(v, &resultText)
			}
		case "assistant":
			// Also extract text from assistant content blocks as fallback
			if v, ok := raw["message"]; ok {
				var msg struct {
					Content []struct {
						Type string `json:"type"`
						Text string `json:"text"`
					} `json:"content"`
				}
				if err := json.Unmarshal(v, &msg); err == nil && resultText == "" {
					var sb strings.Builder
					for _, block := range msg.Content {
						if block.Type == "text" {
							sb.WriteString(block.Text)
						}
					}
					if sb.Len() > 0 {
						resultText = sb.String()
					}
				}
			}
		}
	}

	return resultText, sessionID
}

// waitForAnswer polls for the answer file until it appears or ctx is cancelled.
func waitForAnswer(ctx context.Context, answerFile string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		data, err := os.ReadFile(answerFile)
		if err == nil && len(data) > 0 {
			return strings.TrimSpace(string(data)), nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return "", fmt.Errorf("timeout waiting for answer after %v", timeout)
}

// writeMCPConfig writes the MCP config JSON for the subagent's request_input server.
// Returns the path to the written config file.
func (m *SubagentManager) writeMCPConfig(session *SubagentSession) (string, error) {
	// Find the subagent-mcp binary
	mcpBinary := m.mcpBinaryPath
	if mcpBinary == "" {
		// Look for it in the same directory as the current binary
		execPath, err := os.Executable()
		if err == nil {
			candidate := filepath.Join(filepath.Dir(execPath), "subagent-mcp")
			if _, err := os.Stat(candidate); err == nil {
				mcpBinary = candidate
			}
		}
	}
	if mcpBinary == "" {
		// Fallback: look in bud2/bin/
		candidate := filepath.Join(m.stateDir, "..", "bin", "subagent-mcp")
		candidate = filepath.Clean(candidate)
		if _, err := os.Stat(candidate); err == nil {
			mcpBinary = candidate
		}
	}
	if mcpBinary == "" {
		return "", fmt.Errorf("subagent-mcp binary not found (looked in exec dir and ../bin/)")
	}

	config := map[string]any{
		"mcpServers": map[string]any{
			"subagent": map[string]any{
				"command": mcpBinary,
				"args": []string{
					"--session-id", session.ID,
					"--state-dir", m.stateDir,
				},
			},
		},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return "", err
	}

	configPath := filepath.Join(m.stateDir, "subagent-questions", session.ID+"-mcp.json")
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		return "", err
	}

	return configPath, nil
}

// buildSubagentSystemPrompt constructs the system prompt for a subagent session.
func buildSubagentSystemPrompt(cfg SubagentConfig) string {
	var sb strings.Builder
	sb.WriteString(`You are a focused task assistant working autonomously on behalf of Bud.

CONSTRAINTS:
- Do NOT use talk_to_user or discord_react — you cannot communicate directly with the user.
- Do NOT use AskUserQuestion.
- If you need information from the user, call the mcp__subagent__request_input tool with your question.
- When your task is complete, call signal_done with a summary.
- Keep reasoning internal. Output decisions and outcomes, not your full thought process.
`)

	if cfg.SystemPromptAppend != "" {
		sb.WriteString("\n")
		sb.WriteString(cfg.SystemPromptAppend)
	}

	return sb.String()
}
