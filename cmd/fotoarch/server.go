package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/robouden/fotoarch/internal/captioner"
	"github.com/robouden/fotoarch/internal/catalog"
	"github.com/robouden/fotoarch/internal/config"
	"github.com/robouden/fotoarch/internal/metadata"
	"github.com/robouden/fotoarch/internal/pipeline"
	"github.com/robouden/fotoarch/web"
)

// streamEvent is the JSON shape sent over /api/stream (SSE).
type streamEvent struct {
	Path    string `json:"path"`
	Status  string `json:"status"`
	Caption string `json:"caption,omitempty"`
	Error   string `json:"error,omitempty"`
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
}

func newServer() *server {
	return &server{bcast: newBroadcaster()}
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
		process := func(ctx context.Context, path string) (string, error) {
			res, err := cap.Caption(ctx, captioner.Request{ImagePath: path})
			if err != nil {
				return "", err
			}
			if !req.DryRun {
				if err := metadata.Write(path, metadata.Meta{Caption: res.Caption, Keywords: res.Keywords}); err != nil {
					return "", err
				}
			}
			return res.Caption, nil
		}
		events, err := pipeline.Run(ctx, req.Folder, req.Recurse, cfg.Workers, process)
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
