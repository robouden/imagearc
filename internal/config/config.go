// Package config handles ImageArc's persistent configuration file and API keys from env.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config is the persisted user configuration (~/.config/imagearc/config.json).
type Config struct {
	DefaultProvider string `json:"defaultProvider"`
	DefaultModel    string `json:"defaultModel"`
	OllamaHost      string `json:"ollamaHost"`
	Workers         int    `json:"workers"`
}

// Path returns the config file path, honoring XDG_CONFIG_HOME.
func Path() (string, error) {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".config")
	}
	return filepath.Join(dir, "imagearc", "config.json"), nil
}

// Load reads the config file, returning sane defaults if it doesn't exist.
func Load() (*Config, error) {
	cfg := &Config{
		DefaultProvider: "ollama",
		DefaultModel:    "llava",
		OllamaHost:      "http://localhost:11434",
		Workers:         0,
	}
	p, err := Path()
	if err != nil {
		return cfg, nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Save writes the config file, creating parent directories as needed.
func Save(cfg *Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}

// APIKey returns the API key for a provider from environment variables.
// anthropic -> ANTHROPIC_API_KEY, openai -> OPENAI_API_KEY, gemini -> GEMINI_API_KEY,
// openai-compatible -> OPENAI_COMPATIBLE_API_KEY.
func APIKey(provider string) string {
	switch provider {
	case "anthropic":
		return os.Getenv("ANTHROPIC_API_KEY")
	case "openai":
		return os.Getenv("OPENAI_API_KEY")
	case "gemini":
		return os.Getenv("GEMINI_API_KEY")
	case "openai-compatible":
		return os.Getenv("OPENAI_COMPATIBLE_API_KEY")
	default:
		return ""
	}
}
