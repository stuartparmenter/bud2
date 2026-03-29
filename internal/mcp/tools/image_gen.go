package tools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/vthunder/bud2/internal/mcp"
)

// imageInCapable lists model slugs that support image input (editing/style transfer).
// Imagen models (text-only) should NOT be added here.
var imageInCapable = map[string]bool{
	"nano-banana":     true,
	"nano-banana-pro": true,
}

func registerImageGenTools(server *mcp.Server, deps *Dependencies) {
	server.RegisterTool("generate_image", mcp.ToolDef{
		Description: "Generate an image using Replicate's Gemini image generation models (nano-banana). Returns the path to the saved image file.",
		Properties: map[string]mcp.PropDef{
			"prompt":       {Type: "string", Description: "Text prompt describing the image to generate"},
			"model":        {Type: "string", Description: `Model to use: "flash" (default, google/nano-banana) or "pro" (google/nano-banana-pro)`},
			"aspect_ratio": {Type: "string", Description: `Aspect ratio of the output image, e.g. "1:1" (default), "16:9", "4:3"`},
			"image_url":    {Type: "string", Description: "URL of an input image for editing or style transfer. Only supported by flash and pro models."},
		},
		Required: []string{"prompt"},
	}, func(ctx any, args map[string]any) (string, error) {
		token := os.Getenv("REPLICATE_API_TOKEN")
		if token == "" {
			return "", fmt.Errorf("REPLICATE_API_TOKEN not set")
		}

		prompt, ok := args["prompt"].(string)
		if !ok || prompt == "" {
			return "", fmt.Errorf("prompt is required")
		}

		modelSlug := "nano-banana"
		if m, _ := args["model"].(string); m == "pro" {
			modelSlug = "nano-banana-pro"
		}

		imageURL, _ := args["image_url"].(string)
		if imageURL != "" && !imageInCapable[modelSlug] {
			return "", fmt.Errorf("model %q does not support image input; use \"flash\" or \"pro\" instead", modelSlug)
		}

		aspectRatio := "1:1"
		if ar, _ := args["aspect_ratio"].(string); ar != "" {
			aspectRatio = ar
		}

		// Create prediction
		predURL := fmt.Sprintf("https://api.replicate.com/v1/models/google/%s/predictions", modelSlug)
		input := map[string]any{
			"prompt":       prompt,
			"aspect_ratio": aspectRatio,
		}
		if imageURL != "" {
			input["image"] = imageURL
		}
		body := map[string]any{
			"input": input,
		}
		bodyBytes, err := json.Marshal(body)
		if err != nil {
			return "", fmt.Errorf("marshal request: %w", err)
		}

		req, err := http.NewRequest("POST", predURL, bytes.NewReader(bodyBytes))
		if err != nil {
			return "", fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Prefer", "wait")

		client := &http.Client{Timeout: 70 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("replicate API call: %w", err)
		}
		defer resp.Body.Close()

		respData, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", fmt.Errorf("read response: %w", err)
		}
		if resp.StatusCode >= 400 {
			return "", fmt.Errorf("replicate API error %d: %s", resp.StatusCode, string(respData))
		}

		var prediction struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			URLs   struct {
				Get string `json:"get"`
			} `json:"urls"`
			Output json.RawMessage `json:"output"`
			Error  any             `json:"error"`
		}
		if err := json.Unmarshal(respData, &prediction); err != nil {
			return "", fmt.Errorf("parse prediction response: %w", err)
		}

		// extractOutputURL handles string or []string output shapes
		extractOutputURL := func(raw json.RawMessage) (string, error) {
			if len(raw) == 0 {
				return "", fmt.Errorf("no output URLs in succeeded prediction")
			}
			// Try []string first
			var arr []string
			if err := json.Unmarshal(raw, &arr); err == nil && len(arr) > 0 {
				return arr[0], nil
			}
			// Try plain string
			var s string
			if err := json.Unmarshal(raw, &s); err == nil && s != "" {
				return s, nil
			}
			return "", fmt.Errorf("unrecognised output format: %s", string(raw))
		}

		// Poll until done (if not already succeeded via Prefer: wait)
		if prediction.Status != "succeeded" && prediction.Status != "failed" {
			if prediction.URLs.Get == "" {
				return "", fmt.Errorf("no polling URL in response")
			}
			deadline := time.Now().Add(120 * time.Second)
			pollClient := &http.Client{Timeout: 10 * time.Second}
			for time.Now().Before(deadline) {
				time.Sleep(2 * time.Second)

				pollReq, err := http.NewRequest("GET", prediction.URLs.Get, nil)
				if err != nil {
					return "", fmt.Errorf("create poll request: %w", err)
				}
				pollReq.Header.Set("Authorization", "Bearer "+token)

				pollResp, err := pollClient.Do(pollReq)
				if err != nil {
					continue
				}
				pollData, _ := io.ReadAll(pollResp.Body)
				pollResp.Body.Close()

				if err := json.Unmarshal(pollData, &prediction); err != nil {
					continue
				}
				if prediction.Status == "succeeded" || prediction.Status == "failed" {
					break
				}
			}
		}

		if prediction.Status == "failed" {
			return "", fmt.Errorf("image generation failed: %v", prediction.Error)
		}
		if prediction.Status != "succeeded" {
			return "", fmt.Errorf("image generation timed out (last status: %s)", prediction.Status)
		}

		// Download the image
		imgURL, err := extractOutputURL(prediction.Output)
		if err != nil {
			return "", err
		}
		imgResp, err := http.Get(imgURL) //nolint:noctx
		if err != nil {
			return "", fmt.Errorf("download image: %w", err)
		}
		defer imgResp.Body.Close()

		imgData, err := io.ReadAll(imgResp.Body)
		if err != nil {
			return "", fmt.Errorf("read image: %w", err)
		}

		outPath := fmt.Sprintf("/tmp/bud-image-%d.png", time.Now().Unix())
		if err := os.WriteFile(outPath, imgData, 0644); err != nil {
			return "", fmt.Errorf("save image: %w", err)
		}

		return fmt.Sprintf("Image saved to %s", outPath), nil
	})
}
