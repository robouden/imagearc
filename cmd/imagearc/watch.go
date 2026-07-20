package main

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/robouden/imagearc/internal/metadata"
)

// watcher live-indexes changes under the remembered source folders.
type watcher struct {
	s       *server
	fw      *fsnotify.Watcher
	mu      sync.Mutex
	pending map[string]*time.Timer
}

// startWatching sets up fsnotify watches for every source folder. It is a no-op
// without a store or exiftool.
func (s *server) startWatching() {
	if s.st == nil || metadata.CheckExifTool() != nil {
		return
	}
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return
	}
	wc := &watcher{s: s, fw: fw, pending: map[string]*time.Timer{}}
	srcs, _ := s.st.Sources()
	for _, src := range srcs {
		wc.addTree(src.Path, src.Recurse)
	}
	go wc.loop()
}

// addTree adds watches for root (and all subdirectories when recurse is set).
func (wc *watcher) addTree(root string, recurse bool) {
	if !recurse {
		wc.fw.Add(root)
		return
	}
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			wc.fw.Add(p)
		}
		return nil
	})
}

func (wc *watcher) loop() {
	for {
		select {
		case ev, ok := <-wc.fw.Events:
			if !ok {
				return
			}
			wc.handle(ev)
		case _, ok := <-wc.fw.Errors:
			if !ok {
				return
			}
		}
	}
}

func (wc *watcher) handle(ev fsnotify.Event) {
	if ev.Op&fsnotify.Create != 0 {
		if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
			wc.fw.Add(ev.Name) // newly created subdirectory
			return
		}
	}
	if ev.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		wc.s.st.Delete(ev.Name)
		return
	}
	if ev.Op&(fsnotify.Create|fsnotify.Write) != 0 {
		wc.debounce(ev.Name)
	}
}

// debounce coalesces rapid writes to a file into a single re-index after quiet.
func (wc *watcher) debounce(path string) {
	wc.mu.Lock()
	defer wc.mu.Unlock()
	if t, ok := wc.pending[path]; ok {
		t.Stop()
	}
	wc.pending[path] = time.AfterFunc(1500*time.Millisecond, func() {
		wc.mu.Lock()
		delete(wc.pending, path)
		wc.mu.Unlock()
		wc.s.indexOne(path)
	})
}
