package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/robouden/imagearc/internal/captioner"
	"github.com/robouden/imagearc/internal/catalog"
	"github.com/robouden/imagearc/internal/config"
	"github.com/robouden/imagearc/internal/metadata"
	"github.com/robouden/imagearc/internal/pipeline"
	"github.com/robouden/imagearc/internal/store"
	"github.com/robouden/imagearc/web"
)

// streamEvent is the JSON shape sent over /api/stream (SSE).
type streamEvent struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	Caption string `json:"caption,omitempty"`
	Error   string `json:"error,omitempty"`
	Total   int    `json:"total,omitempty"`
}

// broadcaster fans out batch progress events to any connected SSE clients.
type broadcaster struct {
	mu   sync.Mutex
	subs map[chan streamEvent]struct{}
}

func newBroadcaster() *broadcaster {
	return &broadcaster{subs: make(map[chan streamEvent]struct{})}
}

func (b *broadcaster) subscribe() chan streamEvent {
	ch := make(chan streamEvent, 64)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *broadcaster) unsubscribe(ch chan streamEvent) {
	b.mu.Lock()
	delete(b.subs, ch)
	b.mu.Unlock()
	close(ch)
}

func (b *broadcaster) publish(ev streamEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

type server struct {
	bcast *broadcaster
	st    *store.Store
}

func newServer() *server {
	s := &server{bcast: newBroadcaster()}
	if st, err := store.Open(store.DefaultPath()); err == nil {
		s.st = st
	} else {
		fmt.Fprintf(os.Stderr, "warning: library index unavailable: %v\n", err)
	}
	return s
}

func (s *server) routes(mux *http.ServeMux) {
	sub, err := fs.Sub(web.Assets, ".")
	if err != nil {
		panic(err)
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))
	mux.HandleFunc("/api/caption", s.handleCaption)
	mux.HandleFunc("/api/stream", s.handleStream)
	mux.HandleFunc("/api/metadata", s.handleMetadata)
	mux.HandleFunc("/api/catalog", s.handleCatalog)
	mux.HandleFunc("/api/browse", s.handleBrowse)
	mux.HandleFunc("/api/models", s.handleModels)
	mux.HandleFunc("/api/index", s.handleIndex)
	mux.HandleFunc("/api/refresh", s.handleRefresh)
	mux.HandleFunc("/api/search", s.handleSearch)
	mux.HandleFunc("/api/geo", s.handleGeo)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/thumb", s.handleThumb)
	mux.HandleFunc("/api/image", s.handleImage)
}

type browseResponse struct {
	Path   string   `json:"path"`   // absolute, cleaned
	Parent string   `json:"parent"` // "" when at filesystem root
	Dirs   []string `json:"dirs"`   // subdirectory names, sorted
}

// handleBrowse lists subdirectories of a server-side path for the folder picker.
func (s *server) handleBrowse(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if path == "" {
		if home, err := os.UserHomeDir(); err == nil {
			path = home
		} else {
			path = "/"
		}
	}
	path = filepath.Clean(path)
	entries, err := os.ReadDir(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	dirs := []string{}
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, e.Name())
		}
	}
	sort.Strings(dirs)
	parent := filepath.Dir(path)
	if parent == path {
		parent = ""
	}
	json.NewEncoder(w).Encode(browseResponse{Path: path, Parent: parent, Dirs: dirs})
}

// knownModels lists suggested models for providers that have no live listing API.
var knownModels = map[string][]string{
	"anthropic":         {"claude-sonnet-5", "claude-opus-4-8", "claude-haiku-4-5-20251001"},
	"openai":            {"gpt-4o", "gpt-4o-mini", "gpt-4.1"},
	"gemini":            {"gemini-2.5-flash", "gemini-2.5-pro", "gemini-2.0-flash"},
	"openai-compatible": {},
}

// handleModels returns available models for a provider. For ollama it queries the
// local daemon's /api/tags; other providers return a curated suggestion list.
func (s *server) handleModels(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")
	if provider != "ollama" {
		json.NewEncoder(w).Encode(map[string]any{"models": knownModels[provider]})
		return
	}
	cfg, _ := config.Load()
	host := strings.TrimRight(cfg.OllamaHost, "/")
	if host == "" {
		host = "http://localhost:11434"
	}
	resp, err := http.Get(host + "/api/tags")
	if err != nil {
		http.Error(w, "cannot reach Ollama at "+host+" (is it running?)", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	var tags struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	models := []string{}
	for _, m := range tags.Models {
		models = append(models, m.Name)
	}
	json.NewEncoder(w).Encode(map[string]any{"models": models})
}

type captionRequest struct {
	Folder   string `json:"folder"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Recurse  bool   `json:"recurse"`
	DryRun   bool   `json:"dryRun"`
}

func (s *server) handleCaption(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req captionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg, _ := config.Load()
	if req.Provider == "" {
		req.Provider = cfg.DefaultProvider
	}
	if req.Model == "" {
		req.Model = cfg.DefaultModel
	}

	cap, err := captioner.New(req.Provider, req.Model, cfg.OllamaHost, config.APIKey(req.Provider))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	go func() {
		ctx := context.Background()
		if s.st != nil {
			s.st.AddSource(req.Folder, req.Recurse) // remember for auto-refresh/watch
		}
		if files, err := pipeline.Walk(req.Folder, req.Recurse); err == nil {
			s.bcast.publish(streamEvent{Status: "start", Total: len(files)})
		}
		process := func(ctx context.Context, path string) (string, error) {
			res, err := cap.Caption(ctx, captioner.Request{ImagePath: path})
			if err != nil {
				return "", err
			}
			if !req.DryRun {
				if err := metadata.Write(path, metadata.Meta{Caption: res.Caption, Keywords: res.Keywords}); err != nil {
					return "", err
				}
				// Re-read the file so the index gets the full record (caption +
				// existing EXIF date/GPS), not just the caption we just wrote.
				s.indexOne(path)
			}
			return res.Caption, nil
		}
		workers := cfg.Workers
		if req.Provider == "ollama" && workers == 0 {
			workers = 1 // one local GPU serves the vision model serially; avoid timeouts
		}
		events, err := pipeline.Run(ctx, req.Folder, req.Recurse, workers, process)
		if err != nil {
			s.bcast.publish(streamEvent{Status: "error", Error: err.Error()})
			s.bcast.publish(streamEvent{Status: "complete"})
			return
		}
		for ev := range events {
			se := streamEvent{Path: ev.Path, Status: string(ev.Status), Caption: ev.Caption}
			if ev.Err != nil {
				se.Error = ev.Err.Error()
			}
			s.bcast.publish(se)
		}
		s.bcast.publish(streamEvent{Status: "complete"})
	}()

	w.WriteHeader(http.StatusAccepted)
}

func (s *server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.bcast.subscribe()
	defer s.bcast.unsubscribe(ch)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(ev)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

type metadataPayload struct {
	Path     string   `json:"path"`
	Caption  string   `json:"caption"`
	Keywords []string `json:"keywords"`
	Byline   string   `json:"byline"`
	Location string   `json:"location"`
}

func (s *server) handleMetadata(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		path := r.URL.Query().Get("path")
		if path == "" {
			http.Error(w, "missing path", http.StatusBadRequest)
			return
		}
		m, err := metadata.Read(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(metadataPayload{
			Path: path, Caption: m.Caption, Keywords: m.Keywords, Byline: m.Byline, Location: m.Location,
		})
	case http.MethodPost:
		var p metadataPayload
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if p.Path == "" {
			http.Error(w, "missing path", http.StatusBadRequest)
			return
		}
		m := metadata.Meta{Caption: p.Caption, Keywords: p.Keywords, Byline: p.Byline, Location: p.Location}
		if err := metadata.Write(p.Path, m); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "GET or POST only", http.StatusMethodNotAllowed)
	}
}

type catalogRequest struct {
	Folder  string `json:"folder"`
	Output  string `json:"output"`
	Recurse bool   `json:"recurse"`
}

func (s *server) handleCatalog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	var req catalogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.Output == "" {
		req.Output = "catalog.csv"
	}
	if err := metadata.CheckExifTool(); err != nil {
		http.Error(w, err.Error(), http.StatusFailedDependency)
		return
	}
	files, err := pipeline.Walk(req.Folder, req.Recurse)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var rows []catalog.Row
	for _, f := range files {
		m, err := metadata.Read(f)
		if err != nil {
			continue
		}
		rows = append(rows, catalog.Row{
			Path: f, Filename: filepath.Base(f), Caption: m.Caption,
			Keywords: strings.Join(m.Keywords, ", "), Byline: m.Byline, Location: m.Location, Date: m.Date,
		})
	}
	if err := catalog.Write(req.Output, rows); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"rows": len(rows), "output": req.Output})
}
