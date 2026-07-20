package captioner

import "fmt"

// New builds a Captioner by provider name.
func New(provider, model, ollamaHost, apiKey string) (Captioner, error) {
	switch provider {
	case "ollama", "":
		return NewOllamaProvider(ollamaHost, model), nil
	case "anthropic":
		return &AnthropicProvider{APIKey: apiKey, Model: model}, nil
	case "openai":
		return &OpenAIProvider{APIKey: apiKey, Model: model}, nil
	case "gemini":
		return &GeminiProvider{APIKey: apiKey, Model: model}, nil
	case "openai-compatible":
		return &OpenAICompatibleProvider{APIKey: apiKey, Model: model}, nil
	default:
		return nil, fmt.Errorf("unknown provider %q", provider)
	}
}
