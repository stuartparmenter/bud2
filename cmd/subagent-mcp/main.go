// cmd/subagent-mcp/main.go
//
// Stdio MCP server for subagent sessions.
// Serves one tool: request_input — used by subagents to ask questions without
// calling talk_to_user or AskUserQuestion (which are off-limits for subagents).
//
// When a subagent calls request_input(question):
//   1. This server writes the question to {state_dir}/subagent-questions/{session_id}.txt
//   2. Polls {state_dir}/subagent-answers/{session_id}.txt for an answer (up to 30 min)
//   3. Returns the answer as the tool result
//
// The SubagentManager in bud2 monitors the question file, routes to the executive,
// and writes the answer file when the user responds.
//
// Usage: subagent-mcp --session-id <uuid> --state-dir <path>

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	sessionID = flag.String("session-id", "", "Subagent session ID")
	stateDir  = flag.String("state-dir", "", "Bud state directory path")
)

func main() {
	flag.Parse()
	if *sessionID == "" || *stateDir == "" {
		fmt.Fprintf(os.Stderr, "Usage: subagent-mcp --session-id <uuid> --state-dir <path>\n")
		os.Exit(1)
	}

	questionFile := filepath.Join(*stateDir, "subagent-questions", *sessionID+".txt")
	answerFile := filepath.Join(*stateDir, "subagent-answers", *sessionID+".txt")

	log.SetOutput(os.Stderr)
	log.Printf("[subagent-mcp] Started for session %s", *sessionID)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var msg map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &msg); err != nil {
			continue
		}

		var method string
		var id json.RawMessage
		if v, ok := msg["method"]; ok {
			json.Unmarshal(v, &method)
		}
		if v, ok := msg["id"]; ok {
			id = v
		}

		switch method {
		case "initialize":
			respond(id, map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "subagent-mcp", "version": "1.0"},
			})

		case "notifications/initialized":
			// No response needed

		case "tools/list":
			respond(id, map[string]any{
				"tools": []map[string]any{
					{
						"name":        "request_input",
						"description": "Request information or clarification from the executive. Use this when you need input that you cannot determine on your own. The executive will route your question to the user and provide the answer.",
						"inputSchema": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"question": map[string]any{
									"type":        "string",
									"description": "The question to ask. Be specific about what information you need and why.",
								},
							},
							"required": []string{"question"},
						},
					},
				},
			})

		case "tools/call":
			var params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			if v, ok := msg["params"]; ok {
				json.Unmarshal(v, &params)
			}

			if params.Name == "request_input" {
				question, _ := params.Arguments["question"].(string)
				if question == "" {
					respondError(id, -32602, "question is required")
					continue
				}

				log.Printf("[subagent-mcp] Question from subagent: %s", truncate(question, 100))

				// Write question to file
				if err := os.WriteFile(questionFile, []byte(question), 0644); err != nil {
					respondError(id, -32603, fmt.Sprintf("failed to write question: %v", err))
					continue
				}

				// Poll for answer (up to 30 minutes)
				answer, err := waitForAnswer(answerFile, 30*time.Minute)
				if err != nil {
					log.Printf("[subagent-mcp] Answer timeout or error: %v", err)
					respondError(id, -32603, fmt.Sprintf("answer not received: %v", err))
					continue
				}

				log.Printf("[subagent-mcp] Answer received: %s", truncate(answer, 80))

				// Clean up question file
				os.Remove(questionFile)

				respond(id, map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": answer},
					},
				})
			} else {
				respondError(id, -32601, fmt.Sprintf("unknown tool: %s", params.Name))
			}

		default:
			if id != nil {
				respondError(id, -32601, fmt.Sprintf("method not found: %s", method))
			}
		}
	}

	if err := scanner.Err(); err != nil {
		log.Printf("[subagent-mcp] Scanner error: %v", err)
	}
}

func respond(id json.RawMessage, result any) {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}
	data, _ := json.Marshal(msg)
	fmt.Printf("%s\n", data)
}

func respondError(id json.RawMessage, code int, message string) {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	}
	data, _ := json.Marshal(msg)
	fmt.Printf("%s\n", data)
}

func waitForAnswer(answerFile string, timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(answerFile)
		if err == nil && len(data) > 0 {
			answer := strings.TrimSpace(string(data))
			os.Remove(answerFile)
			return answer, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return "", fmt.Errorf("timeout after %v", timeout)
}

func truncate(s string, n int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
