package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"golang.org/x/image/draw"

	"github.com/robouden/imagearc/internal/metadata"
	"github.com/robouden/imagearc/internal/pipeline"
	"github.com/robouden/imagearc/internal/store"
)

const thumbMax = 320 // px, longest edge

// handleIndex walks a folder, reads IPTC/XMP metadata, and upserts into the
// library index, streaming progress over /api/stream.
func (s *server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.st == nil {
		http.Error(w, "library index unavailable", http.StatusServiceUnavailable)
		return
	}
	var req struct {
		Folder  string `json:"folder"`
		Recurse bool   `json:"recurse"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := metadata.CheckExifTool(); err != nil {
		http.Error(w, err.Error(), http.StatusFailedDependency)
		return
	}
	if _, err := pipeline.Walk(req.Folder, req.Recurse); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	go s.reindex(req.Folder, req.Recurse, func(ev streamEvent) { s.bcast.publish(ev) })
	w.WriteHeader(http.StatusAccepted)
}

// handleRefresh re-indexes every remembered source folder, streaming progress.
func (s *server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	if s.st == nil {
		http.Error(w, "library index unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := metadata.CheckExifTool(); err != nil {
		http.Error(w, err.Error(), http.StatusFailedDependency)
		return
	}
	srcs, err := s.st.Sources()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	go func() {
		for _, src := range srcs {
			s.reindex(src.Path, src.Recurse, func(ev streamEvent) { s.bcast.publish(ev) })
		}
		if len(srcs) == 0 {
			s.bcast.publish(streamEvent{Status: "complete"})
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}

// reindex incrementally indexes a folder: unchanged files (same mtime) are
// skipped, changed/new files are re-read, entries for deleted files are pruned,
// and the folder is recorded as a source. emit may be nil for silent runs.
func (s *server) reindex(folder string, recurse bool, emit func(streamEvent)) {
	if s.st == nil {
		return
	}
	send := func(ev streamEvent) {
		if emit != nil {
			emit(ev)
		}
	}
	files, err := pipeline.Walk(folder, recurse)
	if err != nil {
		send(streamEvent{Status: "error", Error: err.Error()})
		send(streamEvent{Status: "complete"})
		return
	}
	send(streamEvent{Status: "start", Total: len(files)})
	present := make(map[string]bool, len(files))
	for _, f := range files {
		present[f] = true
		if fi, err := os.Stat(f); err == nil {
			if mt, ok := s.st.Mtime(f); ok && mt == fi.ModTime().Unix() {
				send(streamEvent{Path: f, Status: "skipped"})
				continue
			}
		}
		m, err := metadata.Read(f)
		if err != nil {
			send(streamEvent{Path: f, Status: "error", Error: err.Error()})
			continue
		}
		s.st.Upsert(store.Photo{
			Path: f, Caption: m.Caption, Keywords: strings.Join(m.Keywords, ", "),
			Byline: m.Byline, Location: m.Location, Date: m.Date,
		})
		send(streamEvent{Path: f, Status: "done", Caption: m.Caption})
	}
	if known, err := s.st.PathsUnder(folder); err == nil {
		for _, p := range known {
			if !present[p] {
				s.st.Delete(p)
			}
		}
	}
	s.st.AddSource(folder, recurse)
	send(streamEvent{Status: "complete"})
}

// indexOne reads and upserts a single image (used by the live watcher).
func (s *server) indexOne(path string) {
	if s.st == nil || !pipeline.IsImage(path) {
		return
	}
	m, err := metadata.Read(path)
	if err != nil {
		return
	}
	s.st.Upsert(store.Photo{
		Path: path, Caption: m.Caption, Keywords: strings.Join(m.Keywords, ", "),
		Byline: m.Byline, Location: m.Location, Date: m.Date,
	})
}

// rescanSources incrementally re-indexes every remembered source (silent).
// Skipped when exiftool is unavailable.
func (s *server) rescanSources() {
	if s.st == nil || metadata.CheckExifTool() != nil {
		return
	}
	srcs, err := s.st.Sources()
	if err != nil {
		return
	}
	for _, src := range srcs {
		s.reindex(src.Path, src.Recurse, nil)
	}
}

// handleSearch runs a full-text + filter query against the index.
func (s *server) handleSearch(w http.ResponseWriter, r *http.Request) {
	if s.st == nil {
		http.Error(w, "library index unavailable", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	photos, total, err := s.st.Search(store.Query{
		Text: q.Get("q"), Keyword: q.Get("keyword"),
		Location: q.Get("location"), Byline: q.Get("byline"),
		Limit: limit, Offset: offset,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if photos == nil {
		photos = []store.Photo{}
	}
	json.NewEncoder(w).Encode(map[string]any{"photos": photos, "total": total})
}

// handleStats returns dashboard aggregates.
func (s *server) handleStats(w http.ResponseWriter, r *http.Request) {
	if s.st == nil {
		http.Error(w, "library index unavailable", http.StatusServiceUnavailable)
		return
	}
	st, err := s.st.Stats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(st)
}

// handleThumb serves a disk-cached, resized JPEG thumbnail for an image path.
func (s *server) handleThumb(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	cache := thumbCachePath(path)
	if !thumbFresh(cache, path) {
		if err := makeThumb(path, cache); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, cache)
}

// handleImage serves the full image; for RAW it serves the embedded preview.
func (s *server) handleImage(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		http.Error(w, "missing path", http.StatusBadRequest)
		return
	}
	if metadata.IsRAW(path) {
		if b, err := rawPreview(path); err == nil {
			w.Header().Set("Content-Type", "image/jpeg")
			w.Write(b)
			return
		}
	}
	http.ServeFile(w, r, path)
}

// --- thumbnail helpers ---

func thumbCachePath(src string) string {
	dir, err := os.UserCacheDir()
	if err != nil {
		dir = os.TempDir()
	}
	sum := sha1.Sum([]byte(src))
	return filepath.Join(dir, "imagearc", "thumbs", hex.EncodeToString(sum[:])+".jpg")
}

// thumbFresh reports whether the cached thumb exists and is newer than the source.
func thumbFresh(cache, src string) bool {
	ci, err := os.Stat(cache)
	if err != nil {
		return false
	}
	si, err := os.Stat(src)
	if err != nil {
		return true // source gone; keep cached
	}
	return !ci.ModTime().Before(si.ModTime())
}

func makeThumb(src, cache string) error {
	img, err := decodeImage(src)
	if err != nil {
		return err
	}
	b := img.Bounds()
	scale := float64(thumbMax) / float64(max(b.Dx(), b.Dy()))
	if scale > 1 {
		scale = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, int(float64(b.Dx())*scale), int(float64(b.Dy())*scale)))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
	if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
		return err
	}
	f, err := os.Create(cache)
	if err != nil {
		return err
	}
	defer f.Close()
	return jpeg.Encode(f, dst, &jpeg.Options{Quality: 82})
}

// decodeImage decodes an image, falling back to the embedded preview for RAW
// or otherwise-undecodable files via exiftool.
func decodeImage(path string) (image.Image, error) {
	if f, err := os.Open(path); err == nil {
		img, _, derr := image.Decode(f)
		f.Close()
		if derr == nil {
			return img, nil
		}
	}
	b, err := rawPreview(path)
	if err != nil {
		return nil, fmt.Errorf("cannot decode %s", filepath.Base(path))
	}
	return jpeg.Decode(bytes.NewReader(b))
}

// rawPreview extracts an embedded JPEG preview via exiftool.
func rawPreview(path string) ([]byte, error) {
	for _, tag := range []string{"-PreviewImage", "-JpgFromRaw", "-ThumbnailImage"} {
		if b, err := exec.Command("exiftool", "-b", tag, path).Output(); err == nil && len(b) > 0 {
			return b, nil
		}
	}
	return nil, fmt.Errorf("no embedded preview in %s", filepath.Base(path))
}
