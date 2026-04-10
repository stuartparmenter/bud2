package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type OpenCodeServeProvider struct {
	binPath string
	apiKey  string
	model   string
	baseURL string
	cmd     *exec.Cmd
	client  *http.Client

	mu            sync.Mutex
	running       bool
	authSet       bool
	mcpRegistered bool

	contextWindow int
}

func NewOpenCodeServeProvider(binPath, apiKey, model, baseURL string) *OpenCodeServeProvider {
	if baseURL == "" {
		baseURL = "http://127.0.0.1:4096"
	}
	if binPath == "" {
		binPath = "opencode"
	}
	return &OpenCodeServeProvider{
		binPath:       binPath,
		apiKey:        apiKey,
		model:         model,
		baseURL:       baseURL,
		client:        &http.Client{Timeout: 30 * time.Minute},
		contextWindow: MaxContextTokensDefault,
	}
}

func (p *OpenCodeServeProvider) Name() string { return "opencode-serve" }

func (p *OpenCodeServeProvider) WithContextWindow(tokens int) *OpenCodeServeProvider {
	p.contextWindow = tokens
	return p
}

func (p *OpenCodeServeProvider) Start() error {
	p.mu.Lock()
	if p.running {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	// Check if an opencode server is already running at the base URL.
	// If so, reuse it instead of spawning a new process (e.g. when the
	// executive's provider is already running and subagents share the same URL).
	resp, err := p.client.Get(p.baseURL + "/global/health")
	if err == nil {
		io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			log.Printf("[opencode-serve] Reusing existing opencode server at %s", p.baseURL)
			p.mu.Lock()
			p.running = true
			p.mu.Unlock()
			if err := p.setAuthOnce(context.Background()); err != nil {
				log.Printf("[opencode-serve] Warning: failed to set auth: %v", err)
			}
			return nil
		}
	}

	bin, err := exec.LookPath(p.binPath)
	if err != nil {
		return fmt.Errorf("opencode binary not found at %q: %w", p.binPath, err)
	}

	parsed, parseErr := url.Parse(p.baseURL)
	host := "127.0.0.1"
	port := "4096"
	if parseErr == nil {
		if parsed.Hostname() != "" {
			host = parsed.Hostname()
		}
		if parsed.Port() != "" {
			port = parsed.Port()
		}
	}

	p.cmd = exec.Command(bin, "serve", "--hostname", host, "--port", port)
	p.cmd.Stdout = os.Stderr
	p.cmd.Stderr = os.Stderr
	if err := p.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start opencode serve: %w", err)
	}

	p.mu.Lock()
	p.running = true
	p.mu.Unlock()

	log.Printf("[opencode-serve] Started opencode serve (pid=%d) listening on %s:%s", p.cmd.Process.Pid, host, port)

	for i := 0; i < 60; i++ {
		resp, err := p.client.Get(p.baseURL + "/global/health")
		if err == nil {
			io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				log.Printf("[opencode-serve] Health check passed")
				if err := p.setAuthOnce(context.Background()); err != nil {
					log.Printf("[opencode-serve] Warning: failed to set auth: %v", err)
				}
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("opencode serve did not become healthy within 30 seconds")
}

func (p *OpenCodeServeProvider) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.running || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	if err := p.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("failed to kill opencode serve: %w", err)
	}
	_ = p.cmd.Wait()
	p.running = false
	p.authSet = false
	log.Printf("[opencode-serve] Stopped opencode serve")
	return nil
}

func (p *OpenCodeServeProvider) setAuthOnce(ctx context.Context) error {
	p.mu.Lock()
	if p.authSet || p.apiKey == "" {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	body, _ := json.Marshal(map[string]any{
		"type": "api",
		"key":  p.apiKey,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, p.baseURL+"/auth/opencode-go", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to set auth: %w", err)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("auth request failed (status %d)", resp.StatusCode)
	}

	p.mu.Lock()
	p.authSet = true
	p.mu.Unlock()
	log.Printf("[opencode-serve] Auth set for opencode-go")
	return nil
}

func (p *OpenCodeServeProvider) registerMCP(ctx context.Context, mcpURL string) error {
	p.mu.Lock()
	if p.mcpRegistered {
		p.mu.Unlock()
		return nil
	}
	p.mu.Unlock()

	body, _ := json.Marshal(map[string]any{
		"name": "bud2",
		"config": map[string]any{
			"type": "remote",
			"url":  mcpURL,
		},
	})
	resp, err := p.doProviderRequest(ctx, http.MethodPost, "/mcp", body)
	if err != nil {
		return fmt.Errorf("failed to register MCP server: %w", err)
	}

	p.mu.Lock()
	p.mcpRegistered = true
	p.mu.Unlock()
	log.Printf("[opencode-serve] Registered MCP server: %s", mcpURL)
	_ = resp
	return nil
}

func (p *OpenCodeServeProvider) NewSession(opts SessionOpts) (Session, error) {
	if !p.running {
		if err := p.Start(); err != nil {
			return nil, err
		}
	}
	model := p.model
	if opts.Model != "" {
		model = opts.Model
	}
	providerID, modelID := splitOpenCodeModel(model)

	return &OpenCodeSession{
		provider:      p,
		sessionID:     generateSessionID(),
		model:         modelID,
		providerID:    providerID,
		mcpURL:        opts.MCPServerURL,
		contextWindow: p.contextWindow,
	}, nil
}

func (p *OpenCodeServeProvider) doProviderRequest(ctx context.Context, method, endpoint string, body []byte) ([]byte, error) {
	reqURL := p.baseURL + endpoint
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("opencode API error (status %d): %s", resp.StatusCode, string(respBody))
	}
	return respBody, nil
}

type OpenCodeSession struct {
	provider      *OpenCodeServeProvider
	sessionID     string
	ocSessionID   string
	model         string
	providerID    string
	mcpURL        string
	contextWindow int

	mu                    sync.Mutex
	lastUsage             *SessionUsage
	isResuming            bool
	lastProcessedMsgCount int
	repliedPerms          map[string]bool // permission IDs we've already processed
}

func (s *OpenCodeSession) SessionID() string { return s.sessionID }

func (s *OpenCodeSession) ShouldReset() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastUsage == nil {
		return false
	}
	threshold := s.contextWindow
	if threshold <= 0 {
		threshold = MaxContextTokensDefault
	}
	totalContext := s.lastUsage.CacheReadInputTokens + s.lastUsage.InputTokens
	if totalContext > threshold {
		log.Printf("[opencode-session] Context tokens %d exceeds threshold %d, should reset",
			totalContext, threshold)
		return true
	}
	return false
}

func (s *OpenCodeSession) PrepareForResume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.isResuming = true
}

func (s *OpenCodeSession) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessionID = generateSessionID()
	s.ocSessionID = ""
	s.lastUsage = nil
	s.isResuming = false
	s.lastProcessedMsgCount = 0
	s.repliedPerms = make(map[string]bool)
	log.Printf("[opencode-session] Session reset, new ID: %s", s.sessionID)
}

func (s *OpenCodeSession) LastUsage() *SessionUsage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastUsage
}

func (s *OpenCodeSession) Close() error { return nil }

func (s *OpenCodeSession) SendPrompt(ctx context.Context, prompt string, cb StreamCallbacks) (*SessionResult, error) {
	s.mu.Lock()
	resuming := s.isResuming && s.ocSessionID != ""
	s.mu.Unlock()

	if !resuming || s.ocSessionID == "" {
		if err := s.createSession(ctx); err != nil {
			return nil, err
		}
		if s.mcpURL != "" {
			if err := s.provider.registerMCP(ctx, s.mcpURL); err != nil {
				log.Printf("[opencode-session] Warning: MCP registration failed: %v", err)
			}
		}
	}

	// Use a detached context for the prompt so that signal_done cancelling
	// the parent context doesn't abort the opencode request.
	promptCtx, promptCancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer promptCancel()

	// Start a background goroutine that polls for pending permissions and
	// resolves them while the synchronous /message call is running. When the
	// model needs tool permission (bash, edit, etc.), opencode blocks the
	// /message response until the permission is resolved. This poller finds
	// pending permissions and routes them through OnPermission (or auto-approves
	// if no handler is set).
	doneCh := make(chan struct{})
	go s.permissionPoller(promptCtx, cb.OnPermission, doneCh)

	result, err := s.sendPromptSync(promptCtx, prompt, cb)
	close(doneCh)

	if err != nil {
		return nil, err
	}

	s.queryToolCalls(promptCtx, cb)
	return result, nil
}

// permissionPoller periodically checks for pending opencode permissions
// and resolves them. It runs in a background goroutine while /message blocks.
func (s *OpenCodeSession) permissionPoller(ctx context.Context, handler PermissionHandler, doneCh chan struct{}) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-doneCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.handlePendingPermissions(ctx, handler)
		}
	}
}

// handlePendingPermissions fetches pending permissions for THIS session only
// and routes them through the handler. If no handler is set, permissions are
// denied (safer default — prevents unapproved tool usage).
func (s *OpenCodeSession) handlePendingPermissions(ctx context.Context, handler PermissionHandler) {
	s.mu.Lock()
	ocID := s.ocSessionID
	s.mu.Unlock()

	if ocID == "" {
		return
	}

	perms, err := s.fetchPendingPermissions(ctx, ocID)
	if err != nil || len(perms) == 0 {
		return
	}

	for _, perm := range perms {
		// Skip if already processed
		s.mu.Lock()
		if s.repliedPerms[perm.ID] {
			s.mu.Unlock()
			continue
		}
		s.repliedPerms[perm.ID] = true
		s.mu.Unlock()

		var decision PermissionDecision
		if handler != nil {
			decision = handler(PermissionRequest{
				ID:        perm.ID,
				SessionID: perm.SessionID,
				Type:      perm.Type,
				Title:     perm.Title,
				Metadata:  perm.Metadata,
			})
		} else {
			log.Printf("[opencode-session] Denying permission %s (type=%s) — no handler set", perm.ID, perm.Type)
			decision = PermissionDeny
		}
		s.replyPermission(ctx, perm.ID, decision)
	}
}

type pendingPermission struct {
	ID        string         `json:"id"`
	SessionID string         `json:"sessionID"`
	Type      string         `json:"permission"`
	Title     string         `json:"title"`
	Metadata  map[string]any `json:"metadata"`
}

func (s *OpenCodeSession) fetchPendingPermissions(ctx context.Context, sessionID string) ([]pendingPermission, error) {
	// Try V2 endpoint: GET /permission
	resp, err := s.doRequest(ctx, http.MethodGet, "/permission", nil)
	if err != nil {
		return nil, err
	}

	var perms []pendingPermission
	if err := json.Unmarshal(resp, &perms); err != nil {
		// Try V1: GET /session/{id}/permissions
		v1Resp, v1Err := s.doRequest(ctx, http.MethodGet, fmt.Sprintf("/session/%s/permissions", sessionID), nil)
		if v1Err != nil {
			return nil, fmt.Errorf("failed to list permissions: v2=%v v1=%v", err, v1Err)
		}
		if err2 := json.Unmarshal(v1Resp, &perms); err2 != nil {
			return nil, fmt.Errorf("failed to parse permissions: %w", err2)
		}
	}

	// Filter to our session only — don't process permissions for other sessions
	var filtered []pendingPermission
	for _, p := range perms {
		if p.SessionID == sessionID {
			filtered = append(filtered, p)
		}
	}
	return filtered, nil
}

func (s *OpenCodeSession) replyPermission(ctx context.Context, permID string, decision PermissionDecision) {
	if permID == "" {
		return
	}
	reply := "allow"
	if decision == PermissionDeny {
		reply = "deny"
	}

	// Try V1 endpoint first
	v1Body, _ := json.Marshal(map[string]any{"response": reply})
	endpoint := fmt.Sprintf("/session/%s/permissions/%s", s.ocSessionID, permID)
	if _, err := s.doRequest(ctx, http.MethodPost, endpoint, v1Body); err == nil {
		log.Printf("[opencode-session] Permission %s replied: %s", permID, reply)
		return
	}

	// Try V2 endpoint
	v2Body, _ := json.Marshal(map[string]any{"reply": "once"})
	if decision == PermissionDeny {
		v2Body, _ = json.Marshal(map[string]any{"reply": "reject"})
	}
	v2Endpoint := fmt.Sprintf("/permission/%s/reply", permID)
	if _, err := s.doRequest(ctx, http.MethodPost, v2Endpoint, v2Body); err != nil {
		log.Printf("[opencode-session] Warning: failed to reply to permission %s: %v", permID, err)
	} else {
		log.Printf("[opencode-session] Permission %s replied (v2): %s", permID, reply)
	}
}

func (s *OpenCodeSession) createSession(ctx context.Context) error {
	createBody, _ := json.Marshal(map[string]any{
		"title": "bud-executive",
	})
	resp, err := s.doRequest(ctx, http.MethodPost, "/session", createBody)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	var sessionResp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(resp, &sessionResp); err != nil {
		return fmt.Errorf("failed to parse session create response: %w", err)
	}

	s.mu.Lock()
	s.ocSessionID = sessionResp.ID
	s.isResuming = false
	s.lastProcessedMsgCount = 0
	s.repliedPerms = make(map[string]bool)
	s.mu.Unlock()

	log.Printf("[opencode-session] Created opencode session %s", s.ocSessionID)
	return nil
}

func (s *OpenCodeSession) sendPromptSync(ctx context.Context, prompt string, cb StreamCallbacks) (*SessionResult, error) {
	promptBody := map[string]any{
		"parts": []map[string]any{
			{"type": "text", "text": prompt},
		},
	}
	if s.providerID != "" && s.model != "" {
		promptBody["model"] = map[string]any{
			"providerID": s.providerID,
			"modelID":    s.model,
		}
	}

	promptJSON, _ := json.Marshal(promptBody)
	endpoint := fmt.Sprintf("/session/%s/message", s.ocSessionID)
	log.Printf("[opencode-session] Sending prompt to session %s (prompt_len=%d)", s.ocSessionID, len(prompt))
	resp, err := s.doRequest(ctx, http.MethodPost, endpoint, promptJSON)
	if err != nil {
		log.Printf("[opencode-session] Prompt failed: %v", err)
		return nil, fmt.Errorf("failed to send prompt: %w", err)
	}
	log.Printf("[opencode-session] Got response (len=%d)", len(resp))

	return s.parseMessageResponse(resp, cb)
}

func (s *OpenCodeSession) parseMessageResponse(resp []byte, cb StreamCallbacks) (*SessionResult, error) {
	var msg struct {
		Info struct {
			ID        string `json:"id"`
			Role      string `json:"role"`
			CreatedAt int64  `json:"createdAt"`
			Metadata  struct {
				InputTokens  int `json:"inputTokens"`
				OutputTokens int `json:"outputTokens"`
			} `json:"metadata"`
		} `json:"info"`
		Parts []json.RawMessage `json:"parts"`
	}
	if err := json.Unmarshal(resp, &msg); err != nil {
		return nil, fmt.Errorf("failed to parse message response: %w\nresponse: %s", err, truncateResp(resp))
	}

	log.Printf("[opencode-session] Parsed response: %d parts, info.id=%s", len(msg.Parts), msg.Info.ID)

	var textLen int
	for i, rawPart := range msg.Parts {
		var part struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			Name  string          `json:"name"`  // tool name (legacy format)
			Tool  string          `json:"tool"`  // tool name (opencode SDK format)
			Args  json.RawMessage `json:"args"`  // tool args (legacy)
			State json.RawMessage `json:"state"` // tool state (opencode SDK format)
		}
		if err := json.Unmarshal(rawPart, &part); err != nil {
			log.Printf("[opencode-session] Warning: failed to parse part %d: %v", i, err)
			continue
		}

		log.Printf("[opencode-session] Response part %d: type=%s name=%s tool=%s text_len=%d", i, part.Type, part.Name, part.Tool, len(part.Text))

		switch part.Type {
		case "text":
			textLen += len(part.Text)
			if cb.OnText != nil {
				cb.OnText(part.Text)
			}
		case "tool":
			// opencode SDK format: tool name in "tool" field, args in "state.input"
			toolName := part.Tool
			if toolName == "" {
				toolName = part.Name
			}
			var args map[string]any
			if len(part.State) > 0 {
				var state struct {
					Input map[string]any `json:"input"`
				}
				if err := json.Unmarshal(part.State, &state); err == nil && len(state.Input) > 0 {
					args = state.Input
				}
			}
			if args == nil && len(part.Args) > 0 {
				_ = json.Unmarshal(part.Args, &args)
			}
			if toolName != "" && cb.OnTool != nil {
				log.Printf("[opencode-session] Tool call: %s args=%v", toolName, args)
				cb.OnTool(toolName, args)
			}
		case "tool-invocation", "tool-call", "tool-use":
			// Legacy/alternative format: tool name in "name" field
			toolName := part.Name
			var args map[string]any
			if len(part.Args) > 0 {
				_ = json.Unmarshal(part.Args, &args)
			}
			if toolName != "" && cb.OnTool != nil {
				log.Printf("[opencode-session] Tool call (legacy): %s args=%v", toolName, args)
				cb.OnTool(toolName, args)
			}
		case "reasoning", "thinking":
			if cb.OnThinking != nil {
				cb.OnThinking(part.Text)
			}
		case "step-start", "step-finish":
			// Agentic loop lifecycle markers, no content needed
		default:
			log.Printf("[opencode-session] Skipping part type: %s", part.Type)
		}
	}

	usage := &SessionUsage{
		InputTokens:  msg.Info.Metadata.InputTokens,
		OutputTokens: msg.Info.Metadata.OutputTokens,
	}
	if usage.InputTokens == 0 && usage.OutputTokens == 0 {
		usage.OutputTokens = estimateTokens(textLen)
	}

	s.mu.Lock()
	s.lastUsage = usage
	s.mu.Unlock()

	if cb.OnResult != nil {
		cb.OnResult(usage)
	}

	return &SessionResult{
		SessionID: s.ocSessionID,
		Usage:     usage,
	}, nil
}

// ocMessage represents a message in the opencode session history,
// used for extracting tool calls from intermediate messages.
type ocMessage struct {
	Info struct {
		ID        string `json:"id"`
		Role      string `json:"role"`
		CreatedAt int64  `json:"createdAt"`
	} `json:"info"`
	Parts []json.RawMessage `json:"parts"`
}

// parseOcPart extracts tool call info from an opencode message part.
// Returns (toolName, args, true) for tool calls, or ("", nil, false) otherwise.
func parseOcPart(raw json.RawMessage) (string, map[string]any, bool) {
	var p struct {
		Type  string          `json:"type"`
		Text  string          `json:"text"`
		Name  string          `json:"name"`
		Tool  string          `json:"tool"`
		Args  json.RawMessage `json:"args"`
		State json.RawMessage `json:"state"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", nil, false
	}

	switch p.Type {
	case "tool":
		toolName := p.Tool
		if toolName == "" {
			toolName = p.Name
		}
		var args map[string]any
		if len(p.State) > 0 {
			var state struct {
				Input map[string]any `json:"input"`
			}
			if err := json.Unmarshal(p.State, &state); err == nil && len(state.Input) > 0 {
				args = state.Input
			}
		}
		if args == nil && len(p.Args) > 0 {
			_ = json.Unmarshal(p.Args, &args)
		}
		return toolName, args, true
	case "tool-invocation", "tool-call", "tool-use":
		var args map[string]any
		if len(p.Args) > 0 {
			_ = json.Unmarshal(p.Args, &args)
		}
		return p.Name, args, true
	default:
		return "", nil, false
	}
}

// queryToolCalls fetches session messages after the prompt response and
// logs any tool calls from intermediate messages (which the synchronous
// /message endpoint doesn't return). This fills the gap in executive logs
// where tool calls made during opencode's agentic loop are invisible.
func (s *OpenCodeSession) queryToolCalls(ctx context.Context, cb StreamCallbacks) {
	s.mu.Lock()
	ocID := s.ocSessionID
	fromCount := s.lastProcessedMsgCount
	s.mu.Unlock()

	if ocID == "" || cb.OnTool == nil {
		return
	}

	resp, err := s.doRequest(ctx, http.MethodGet, fmt.Sprintf("/session/%s/message", ocID), nil)
	if err != nil {
		log.Printf("[opencode-session] queryToolCalls: failed to fetch messages: %v", err)
		return
	}

	var messages []ocMessage
	if err := json.Unmarshal(resp, &messages); err != nil {
		log.Printf("[opencode-session] queryToolCalls: failed to parse %d bytes: %v", len(resp), err)
		return
	}

	log.Printf("[opencode-session] queryToolCalls: session %s has %d messages (fromCount=%d)", ocID, len(messages), fromCount)

	// Clamp fromCount to message range in case of session resets
	startIdx := fromCount
	if startIdx > len(messages) {
		startIdx = 0
	}

	toolCallCount := 0
	for i := startIdx; i < len(messages); i++ {
		msg := messages[i]
		for _, rawPart := range msg.Parts {
			toolName, args, isTool := parseOcPart(rawPart)
			if isTool && toolName != "" {
				cb.OnTool(toolName, args)
				toolCallCount++
			}
		}
	}

	s.mu.Lock()
	s.lastProcessedMsgCount = len(messages)
	s.mu.Unlock()

	log.Printf("[opencode-session] queryToolCalls: found %d tool calls in messages %d-%d of %d", toolCallCount, startIdx, len(messages)-1, len(messages))
}

func (s *OpenCodeSession) doRequest(ctx context.Context, method, endpoint string, body []byte) ([]byte, error) {
	reqURL := s.provider.baseURL + endpoint
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.provider.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.provider.apiKey)
	}

	resp, err := s.provider.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("opencode API error (status %d): %s", resp.StatusCode, truncateResp(respBody))
	}
	return respBody, nil
}

func splitOpenCodeModel(model string) (providerID, modelID string) {
	idx := strings.Index(model, "/")
	if idx < 0 {
		return "", model
	}
	return model[:idx], model[idx+1:]
}

func generateSessionID() string {
	return fmt.Sprintf("bud-oc-%d", time.Now().UnixNano())
}

func estimateTokens(charCount int) int {
	if charCount < 4 {
		return 1
	}
	return charCount / 4
}

func truncateResp(data []byte) string {
	const maxLen = 500
	if len(data) <= maxLen {
		return string(data)
	}
	return string(data[:maxLen]) + "..."
}
