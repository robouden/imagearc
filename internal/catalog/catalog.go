// Package catalog reads/writes a CSV catalog of captioned/tagged photos and
// supports simple substring search.
package catalog

import (
	"encoding/csv"
	"os"
	"strings"
)

// Row is one catalog entry.
type Row struct {
	Path     string
	Filename string
	Caption  string
	Keywords string // comma-separated
	Byline   string
	Location string
	Date     string
}

var header = []string{"path", "filename", "caption", "keywords", "byline", "location", "date"}

// Write writes rows to a CSV file at path, overwriting any existing file.
func Write(path string, rows []Row) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	if err := w.Write(header); err != nil {
		return err
	}
	for _, r := range rows {
		rec := []string{r.Path, r.Filename, r.Caption, r.Keywords, r.Byline, r.Location, r.Date}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	return w.Error()
}

// Read parses a CSV catalog file back into Rows.
func Read(path string) ([]Row, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	rows := make([]Row, 0, len(records)-1)
	for _, rec := range records[1:] { // skip header
		if len(rec) < 7 {
			continue
		}
		rows = append(rows, Row{
			Path:     rec[0],
			Filename: rec[1],
			Caption:  rec[2],
			Keywords: rec[3],
			Byline:   rec[4],
			Location: rec[5],
			Date:     rec[6],
		})
	}
	return rows, nil
}

// Search returns rows whose caption or keywords contain query (case-insensitive substring match).
func Search(rows []Row, query string) []Row {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return rows
	}
	var out []Row
	for _, r := range rows {
		if strings.Contains(strings.ToLower(r.Caption), q) || strings.Contains(strings.ToLower(r.Keywords), q) {
			out = append(out, r)
		}
	}
	return out
}
