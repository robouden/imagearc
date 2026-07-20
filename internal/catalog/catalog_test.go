package catalog

import (
	"path/filepath"
	"testing"
)

func TestWriteReadRoundTrip(t *testing.T) {
	rows := []Row{
		{Path: "/a/b.jpg", Filename: "b.jpg", Caption: "a red barn at sunset", Keywords: "barn, sunset, farm", Byline: "Jane Doe", Location: "Iowa", Date: "2024-05-01"},
		{Path: "/a/c.jpg", Filename: "c.jpg", Caption: "a blue lake", Keywords: "lake, mountains", Byline: "Jane Doe", Location: "Colorado", Date: "2024-05-02"},
	}

	dir := t.TempDir()
	csvPath := filepath.Join(dir, "catalog.csv")

	if err := Write(csvPath, rows); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := Read(csvPath)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != len(rows) {
		t.Fatalf("expected %d rows, got %d", len(rows), len(got))
	}
	for i, r := range rows {
		if got[i] != r {
			t.Errorf("row %d: expected %+v, got %+v", i, r, got[i])
		}
	}
}

func TestSearch(t *testing.T) {
	rows := []Row{
		{Path: "/a/b.jpg", Caption: "a red barn at sunset", Keywords: "barn, sunset, farm"},
		{Path: "/a/c.jpg", Caption: "a blue lake", Keywords: "lake, mountains"},
	}

	results := Search(rows, "barn")
	if len(results) != 1 || results[0].Path != "/a/b.jpg" {
		t.Errorf("expected 1 match for 'barn', got %+v", results)
	}

	results = Search(rows, "LAKE")
	if len(results) != 1 || results[0].Path != "/a/c.jpg" {
		t.Errorf("expected 1 match for 'LAKE' (case-insensitive), got %+v", results)
	}

	results = Search(rows, "")
	if len(results) != 2 {
		t.Errorf("expected empty query to return all rows, got %d", len(results))
	}

	results = Search(rows, "nonexistent")
	if len(results) != 0 {
		t.Errorf("expected no matches, got %+v", results)
	}
}
