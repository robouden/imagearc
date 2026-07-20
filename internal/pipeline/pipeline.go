// Package pipeline walks a folder tree and processes images through a worker pool,
// emitting progress events.
package pipeline

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Status describes the outcome of processing a single file.
type Status string

const (
	StatusStarted Status = "started"
	StatusDone    Status = "done"
	StatusError   Status = "error"
	StatusSkipped Status = "skipped"
)

// Event reports progress for one file.
type Event struct {
	Path    string
	Status  Status
	Caption string
	Err     error
}

var imageExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".tif": true, ".tiff": true, ".png": true,
	".cr2": true, ".cr3": true, ".nef": true, ".arw": true, ".dng": true,
	".raf": true, ".orf": true, ".rw2": true, ".pef": true, ".srw": true,
	".raw": true, ".3fr": true, ".erf": true, ".kdc": true, ".mrw": true,
	".nrw": true, ".x3f": true,
}

// IsImage reports whether path has a recognized image/RAW extension.
func IsImage(path string) bool {
	return imageExtensions[strings.ToLower(filepath.Ext(path))]
}

// Walk collects image file paths under root. If recurse is false, only root's
// immediate children are scanned.
func Walk(root string, recurse bool) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		if IsImage(root) {
			return []string{root}, nil
		}
		return nil, nil
	}

	var files []string
	if recurse {
		err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && IsImage(path) {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if !e.IsDir() && IsImage(e.Name()) {
				files = append(files, filepath.Join(root, e.Name()))
			}
		}
	}
	return files, nil
}

// ProcessFunc processes a single file and returns the caption text (for reporting).
type ProcessFunc func(ctx context.Context, path string) (caption string, err error)

// Run walks root and feeds files to a worker pool of `workers` goroutines (default
// NumCPU if <= 0), calling fn for each and emitting Events on the returned channel.
// The channel is closed once all files are processed.
func Run(ctx context.Context, root string, recurse bool, workers int, fn ProcessFunc) (<-chan Event, error) {
	files, err := Walk(root, recurse)
	if err != nil {
		return nil, err
	}
	if workers <= 0 {
		workers = runtime.NumCPU()
	}

	events := make(chan Event, len(files))
	paths := make(chan string)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for p := range paths {
				select {
				case <-ctx.Done():
					events <- Event{Path: p, Status: StatusSkipped, Err: ctx.Err()}
					continue
				default:
				}
				events <- Event{Path: p, Status: StatusStarted}
				caption, err := fn(ctx, p)
				if err != nil {
					events <- Event{Path: p, Status: StatusError, Err: err}
					continue
				}
				events <- Event{Path: p, Status: StatusDone, Caption: caption}
			}
		}()
	}

	go func() {
		for _, p := range files {
			paths <- p
		}
		close(paths)
		wg.Wait()
		close(events)
	}()

	return events, nil
}
