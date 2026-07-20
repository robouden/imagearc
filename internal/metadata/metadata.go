// Package metadata wraps the exiftool CLI to read/write IPTC/XMP metadata,
// using Lightroom-style .xmp sidecars for RAW files.
package metadata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Meta holds the metadata fields ImageArc manages.
type Meta struct {
	Caption  string
	Keywords []string
	Byline   string
	Location string
	Headline string
	Date     string
	Lat      *float64 // GPS latitude (decimal), nil if absent
	Lon      *float64 // GPS longitude (decimal), nil if absent
	Raw      map[string]any // extra/raw exiftool fields, populated on Read
}

// rawExtensions lists file extensions that require an .xmp sidecar instead of
// in-file embedding, following Lightroom convention.
var rawExtensions = map[string]bool{
	".cr2": true, ".cr3": true, ".nef": true, ".arw": true, ".dng": true,
	".raf": true, ".orf": true, ".rw2": true, ".pef": true, ".srw": true,
	".raw": true, ".3fr": true, ".erf": true, ".kdc": true, ".mrw": true,
	".nrw": true, ".x3f": true,
}

// IsRAW reports whether path's extension requires a sidecar rather than in-file writes.
func IsRAW(path string) bool {
	return rawExtensions[strings.ToLower(filepath.Ext(path))]
}

// CheckExifTool verifies exiftool is on PATH, returning an install hint error if not.
func CheckExifTool() error {
	if _, err := exec.LookPath("exiftool"); err != nil {
		return fmt.Errorf(
			"exiftool not found on PATH.\nInstall it: Linux (apt install libimage-exiftool-perl), " +
				"macOS (brew install exiftool), Windows (https://exiftool.org, add to PATH)")
	}
	return nil
}

// Read runs exiftool -j on path and parses IPTC/XMP fields into a Meta.
func Read(path string) (Meta, error) {
	var m Meta
	cmd := exec.Command("exiftool", "-j", "-n",
		"-DateTimeOriginal", "-Composite:GPSLatitude", "-Composite:GPSLongitude",
		"-IPTC:All", "-XMP:All", path)
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return m, fmt.Errorf("exiftool read failed: %w: %s", err, stderr.String())
	}
	var results []map[string]any
	if err := json.Unmarshal(out.Bytes(), &results); err != nil {
		return m, fmt.Errorf("parsing exiftool output: %w", err)
	}
	if len(results) == 0 {
		return m, fmt.Errorf("no exiftool result for %s", path)
	}
	raw := results[0]
	m.Raw = raw
	m.Caption = firstString(raw, "Caption-Abstract", "Description")
	m.Keywords = toStringSlice(firstVal(raw, "Keywords", "Subject"))
	m.Byline = firstString(raw, "By-line", "Creator")
	m.Location = firstString(raw, "Sub-location", "Location")
	m.Headline = firstString(raw, "Headline")
	m.Date = normalizeDate(firstString(raw, "DateTimeOriginal", "DateCreated", "CreateDate"))
	if lat, ok := firstFloat(raw, "GPSLatitude"); ok {
		m.Lat = &lat
	}
	if lon, ok := firstFloat(raw, "GPSLongitude"); ok {
		m.Lon = &lon
	}
	return m, nil
}

// firstFloat returns the first key parseable as a float64.
func firstFloat(m map[string]any, keys ...string) (float64, bool) {
	switch v := firstVal(m, keys...).(type) {
	case float64:
		return v, true
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// normalizeDate converts exiftool's "YYYY:MM:DD HH:MM:SS" to ISO "YYYY-MM-DD HH:MM:SS"
// (date portion), which sorts lexically and is friendlier to display.
func normalizeDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 10 && s[4] == ':' && s[7] == ':' {
		return s[0:4] + "-" + s[5:7] + "-" + s[8:]
	}
	return s
}

func firstVal(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

func firstString(m map[string]any, keys ...string) string {
	v := firstVal(m, keys...)
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func toStringSlice(v any) []string {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		return []string{t}
	case []any:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// buildWriteArgs constructs the exiftool argument list for writing m to targetPath.
// Factored out from Write so it can be unit-tested without exiftool installed.
// targetPath is the file exiftool should be invoked against (sidecar .xmp for RAW,
// the original file otherwise).
func buildWriteArgs(targetPath string, m Meta, overwriteOriginal bool) []string {
	args := []string{}
	if m.Caption != "" {
		args = append(args,
			"-IPTC:Caption-Abstract="+m.Caption,
			"-XMP-dc:Description="+m.Caption,
		)
	}
	if len(m.Keywords) > 0 {
		for _, kw := range m.Keywords {
			args = append(args, "-IPTC:Keywords="+kw, "-XMP-dc:Subject="+kw)
		}
	}
	if m.Byline != "" {
		args = append(args,
			"-IPTC:By-line="+m.Byline,
			"-XMP-dc:Creator="+m.Byline,
		)
	}
	if m.Location != "" {
		args = append(args,
			"-IPTC:Sub-location="+m.Location,
			"-XMP-iptcCore:Location="+m.Location,
		)
	}
	if m.Headline != "" {
		args = append(args, "-IPTC:Headline="+m.Headline)
	}
	if m.Date != "" {
		args = append(args, "-IPTC:DateCreated="+m.Date)
	}
	if overwriteOriginal {
		args = append(args, "-overwrite_original")
	}
	args = append(args, targetPath)
	return args
}

// Write persists m for path via exiftool. RAW files get an .xmp sidecar written
// alongside them (Lightroom convention: <path>.xmp); JPEG/TIFF are edited in place.
func Write(path string, m Meta) error {
	if err := CheckExifTool(); err != nil {
		return err
	}
	if IsRAW(path) {
		sidecar := path + ".xmp"
		// Create the sidecar from scratch if it doesn't exist yet.
		if _, err := os.Stat(sidecar); os.IsNotExist(err) {
			create := exec.Command("exiftool", "-o", sidecar, path)
			var stderr bytes.Buffer
			create.Stderr = &stderr
			if err := create.Run(); err != nil {
				return fmt.Errorf("creating xmp sidecar: %w: %s", err, stderr.String())
			}
		}
		args := buildWriteArgs(sidecar, m, true)
		return runExifTool(args)
	}
	args := buildWriteArgs(path, m, true)
	return runExifTool(args)
}

func runExifTool(args []string) error {
	cmd := exec.Command("exiftool", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("exiftool write failed: %w: %s", err, stderr.String())
	}
	return nil
}
