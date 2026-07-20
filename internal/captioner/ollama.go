package captioner

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// OllamaProvider captions images via a local Ollama server's /api/generate endpoint
// using a vision-capable model (llava, qwen2.5vl, qwen3-vl, ...).
type OllamaProvider struct {
	Host   string // e.g. http://localhost:11434
	Model  string // e.g. llava, qwen2.5vl, qwen3-vl
	Client *http.Client
}

// NewOllamaProvider builds a provider with sane defaults.
func NewOllamaProvider(host, model string) *OllamaProvider {
	if host == "" {
		host = "http://localhost:11434"
	}
	if model == "" {
		model = "llava"
	}
	return &OllamaProvider{
		Host:   host,
		Model:  model,
		Client: &http.Client{Timeout: 10 * time.Minute}, // first model load + inference can be slow
	}
}

func (p *OllamaProvider) Name() string { return "ollama" }

type ollamaGenerateRequest struct {
	Model     string   `json:"model"`
	Prompt    string   `json:"prompt"`
	Images    []string `json:"images"`
	Stream    bool     `json:"stream"`
	KeepAlive string   `json:"keep_alive"` // keep model resident between batch images
}

type ollamaGenerateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// Caption sends the image (base64-encoded) and prompt to Ollama and parses the
// response into a Caption and Keywords list.
func (p *OllamaProvider) Caption(ctx context.Context, r Request) (Result, error) {
	var res Result

	data, err := os.ReadFile(r.ImagePath)
	if err != nil {
		return res, fmt.Errorf("reading image: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(data)

	prompt := r.Prompt
	if prompt == "" {
		prompt = DefaultPrompt
	}

	reqBody := ollamaGenerateRequest{
		Model:     p.Model,
		Prompt:    prompt,
		Images:    []string{encoded},
		Stream:    false,
		KeepAlive: "15m",
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return res, err
	}

	url := strings.TrimRight(p.Host, "/") + "/api/generate"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return res, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.Client.Do(httpReq)
	if err != nil {
		return res, fmt.Errorf("connecting to Ollama at %s: %w\nis Ollama running? try `ollama serve` and `ollama pull %s`", p.Host, err, p.Model)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return res, fmt.Errorf("reading Ollama response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return res, fmt.Errorf("Ollama returned %d: %s\nis the model pulled? try `ollama pull %s`", resp.StatusCode, string(body), p.Model)
	}

	var genResp ollamaGenerateResponse
	if err := json.Unmarshal(body, &genResp); err != nil {
		return res, fmt.Errorf("parsing Ollama response: %w", err)
	}

	return ParseOllamaText(genResp.Response), nil
}

// ParseOllamaText splits a model's free-text response into Caption and Keywords,
// looking for a "Keywords:" marker line (case-insensitive).
func ParseOllamaText(text string) Result {
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)
	idx := strings.Index(lower, "keywords:")
	if idx == -1 {
		return Result{Caption: text}
	}
	caption := strings.TrimSpace(text[:idx])
	kwPart := strings.TrimSpace(text[idx+len("keywords:"):])
	kwPart = strings.ReplaceAll(kwPart, "\n", ",")
	rawKw := strings.Split(kwPart, ",")
	keywords := make([]string, 0, len(rawKw))
	for _, k := range rawKw {
		k = strings.TrimSpace(k)
		k = strings.Trim(k, ".")
		if k != "" {
			keywords = append(keywords, k)
		}
	}
	return Result{Caption: caption, Keywords: keywords}
}
