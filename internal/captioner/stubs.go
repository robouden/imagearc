package captioner

import (
	"context"
	"fmt"
)

// AnthropicProvider will call the Claude Messages API (vision-capable models,
// e.g. current model id "claude-sonnet-5") to caption images. Not yet implemented.
type AnthropicProvider struct {
	APIKey string
	Model  string // e.g. "claude-sonnet-5"
	// Endpoint is the Messages API, https://api.anthropic.com/v1/messages
}

func (p *AnthropicProvider) Name() string { return "anthropic" }

func (p *AnthropicProvider) Caption(ctx context.Context, r Request) (Result, error) {
	return Result{}, fmt.Errorf("anthropic provider: not yet implemented")
}

// OpenAIProvider will call the OpenAI Chat Completions / Responses API with vision.
type OpenAIProvider struct {
	APIKey string
	Model  string // e.g. "gpt-4o"
}

func (p *OpenAIProvider) Name() string { return "openai" }

func (p *OpenAIProvider) Caption(ctx context.Context, r Request) (Result, error) {
	return Result{}, fmt.Errorf("openai provider: not yet implemented")
}

// GeminiProvider will call the Google Gemini generateContent API with vision.
type GeminiProvider struct {
	APIKey string
	Model  string // e.g. "gemini-1.5-pro"
}

func (p *GeminiProvider) Name() string { return "gemini" }

func (p *GeminiProvider) Caption(ctx context.Context, r Request) (Result, error) {
	return Result{}, fmt.Errorf("gemini provider: not yet implemented")
}

// OpenAICompatibleProvider targets any OpenAI-compatible /v1/chat/completions
// endpoint (e.g. vLLM, LM Studio, LocalAI).
type OpenAICompatibleProvider struct {
	BaseURL string
	APIKey  string
	Model   string
}

func (p *OpenAICompatibleProvider) Name() string { return "openai-compatible" }

func (p *OpenAICompatibleProvider) Caption(ctx context.Context, r Request) (Result, error) {
	return Result{}, fmt.Errorf("openai-compatible provider: not yet implemented")
}
