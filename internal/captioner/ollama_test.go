package captioner

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParseOllamaText(t *testing.T) {
	text := "A red barn stands in a golden field at sunset.\nKeywords: barn, sunset, field, farm, golden light"
	res := ParseOllamaText(text)

	wantCaption := "A red barn stands in a golden field at sunset."
	if res.Caption != wantCaption {
		t.Errorf("Caption = %q, want %q", res.Caption, wantCaption)
	}
	wantKeywords := []string{"barn", "sunset", "field", "farm", "golden light"}
	if !reflect.DeepEqual(res.Keywords, wantKeywords) {
		t.Errorf("Keywords = %v, want %v", res.Keywords, wantKeywords)
	}
}

func TestParseOllamaTextNoKeywords(t *testing.T) {
	res := ParseOllamaText("Just a caption with no keyword marker.")
	if res.Caption != "Just a caption with no keyword marker." {
		t.Errorf("unexpected caption: %q", res.Caption)
	}
	if len(res.Keywords) != 0 {
		t.Errorf("expected no keywords, got %v", res.Keywords)
	}
}

func TestOllamaGenerateResponseJSON(t *testing.T) {
	sample := `{"model":"llava","response":"A misty mountain lake at dawn.\nKeywords: lake, mountains, mist, dawn","done":true}`
	var resp ollamaGenerateResponse
	if err := json.Unmarshal([]byte(sample), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	res := ParseOllamaText(resp.Response)
	if res.Caption != "A misty mountain lake at dawn." {
		t.Errorf("Caption = %q", res.Caption)
	}
	if len(res.Keywords) != 4 {
		t.Errorf("expected 4 keywords, got %v", res.Keywords)
	}
}
