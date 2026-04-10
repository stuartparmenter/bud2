package embedding

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client handles text generation via Ollama
type Client struct {
	baseURL         string
	model           string
	generationModel string
	client          *http.Client
}

// NewClient creates a new Ollama embedding client
func NewClient(baseURL, model string) *Client {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "nomic-embed-text"
	}
	return &Client{
		baseURL:         baseURL,
		model:           model,
		generationModel: "llama3.2", // fast, available by default
		client: &http.Client{
			Timeout: 300 * time.Second, // 5 minutes for long-running compressions
		},
	}
}

// generateRequest is the Ollama API request format for generation
type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

// generateResponse is the Ollama API response format for generation
type generateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// Generate creates text completion using Ollama
func (c *Client) Generate(prompt string) (string, error) {
	if prompt == "" {
		return "", fmt.Errorf("empty prompt")
	}

	reqBody := generateRequest{
		Model:  c.generationModel,
		Prompt: prompt,
		Stream: false,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	start := time.Now()
	resp, err := c.client.Post(
		c.baseURL+"/api/generate",
		"application/json",
		bytes.NewReader(jsonBody),
	)
	if err != nil {
		return "", fmt.Errorf("ollama request (took %s): %w", time.Since(start), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama error (status %d, took %s): %s", resp.StatusCode, time.Since(start), string(body))
	}

	var result generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response (took %s): %w", time.Since(start), err)
	}

	return result.Response, nil
}
