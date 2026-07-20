// Package captioner defines the AI captioning provider interface and implementations.
package captioner

import "context"

// Request is a captioning request for a single image.
type Request struct {
	ImagePath string
	Prompt    string
}

// Result is the caption and extracted keywords for an image.
type Result struct {
	Caption  string
	Keywords []string
}

// Captioner generates captions/keywords for images.
type Captioner interface {
	Caption(ctx context.Context, r Request) (Result, error)
	Name() string
}

// DefaultPrompt asks the model for a natural-language caption plus keywords.
const DefaultPrompt = "Describe this photograph in one or two natural, vivid sentences suitable " +
	"for a photo caption. Then on a new line write 'Keywords:' followed by a comma-separated list " +
	"of 8-15 relevant single or short-phrase keywords."
