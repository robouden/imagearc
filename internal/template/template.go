// Package template applies per-client JSON templates on top of AI caption results.
package template

import (
	"encoding/json"
	"os"
)

// Template holds fixed metadata fields applied on top of AI-generated results.
type Template struct {
	Name           string   `json:"name"`
	Byline         string   `json:"byline"`
	Location       string   `json:"location"`
	KeywordsPrefix []string `json:"keywordsPrefix"`
	CaptionPrefix  string   `json:"captionPrefix"`
}

// Load reads a template JSON file from disk.
func Load(path string) (*Template, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Template
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// Apply merges the template's fixed fields with AI-produced caption/keywords.
// Template fields take precedence for byline/location; keywords/caption are prefixed.
func (t *Template) Apply(caption string, keywords []string) (outCaption string, outKeywords []string, byline, location string) {
	outCaption = caption
	if t.CaptionPrefix != "" {
		outCaption = t.CaptionPrefix + " " + caption
	}
	outKeywords = append(append([]string{}, t.KeywordsPrefix...), keywords...)
	byline = t.Byline
	location = t.Location
	return
}
