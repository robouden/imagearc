package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/robouden/imagearc/internal/captioner"
	"github.com/robouden/imagearc/internal/config"
	"github.com/robouden/imagearc/internal/metadata"
	"github.com/robouden/imagearc/internal/pipeline"
	"github.com/robouden/imagearc/internal/template"
)

func newCaptionCmd() *cobra.Command {
	var (
		provider     string
		model        string
		recurse      bool
		templatePath string
		workers      int
		dryRun       bool
		ollamaHost   string
	)

	cmd := &cobra.Command{
		Use:   "caption <dir|file>",
		Short: "Caption photos with AI and write IPTC/XMP metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := args[0]
			cfg, _ := config.Load()
			if provider == "" {
				provider = cfg.DefaultProvider
			}
			if model == "" {
				model = cfg.DefaultModel
			}
			if ollamaHost == "" {
				ollamaHost = cfg.OllamaHost
			}
			if workers == 0 {
				workers = cfg.Workers
			}
			if provider == "ollama" && workers == 0 {
				workers = 1 // one local GPU serves the vision model serially; avoid timeouts
			}

			if !dryRun {
				if err := metadata.CheckExifTool(); err != nil {
					return err
				}
			}

			var tmpl *template.Template
			if templatePath != "" {
				t, err := template.Load(templatePath)
				if err != nil {
					return fmt.Errorf("loading template: %w", err)
				}
				tmpl = t
			}

			cap, err := captioner.New(provider, model, ollamaHost, config.APIKey(provider))
			if err != nil {
				return err
			}
			fmt.Printf("provider=%s model=%s dry-run=%v\n", cap.Name(), model, dryRun)

			ctx := context.Background()
			process := func(ctx context.Context, path string) (string, error) {
				res, err := cap.Caption(ctx, captioner.Request{ImagePath: path})
				if err != nil {
					return "", err
				}
				m := metadata.Meta{Caption: res.Caption, Keywords: res.Keywords}
				if tmpl != nil {
					c, kw, byline, loc := tmpl.Apply(res.Caption, res.Keywords)
					m.Caption, m.Keywords, m.Byline, m.Location = c, kw, byline, loc
				}
				if !dryRun {
					if err := metadata.Write(path, m); err != nil {
						return "", err
					}
				}
				return m.Caption, nil
			}

			files, err := pipeline.Walk(root, recurse)
			if err != nil {
				return err
			}
			total := len(files)
			width := len(strconv.Itoa(total))
			fmt.Printf("found %d image(s)\n", total)

			events, err := pipeline.Run(ctx, root, recurse, workers, process)
			if err != nil {
				return err
			}
			var errs, done int
			// prog returns a "[  cur/total  42%]" progress prefix.
			prog := func(cur int) string {
				pct := 0
				if total > 0 {
					pct = cur * 100 / total
				}
				return fmt.Sprintf("[%*d/%d %3d%%]", width, cur, total, pct)
			}
			for ev := range events {
				switch ev.Status {
				case pipeline.StatusStarted:
					fmt.Printf("%s started %s\n", prog(done+1), ev.Path)
				case pipeline.StatusDone:
					done++
					fmt.Printf("%s done    %s -> %s\n", prog(done), ev.Path, ev.Caption)
				case pipeline.StatusError:
					errs++
					done++
					fmt.Printf("%s error   %s: %v\n", prog(done), ev.Path, ev.Err)
				case pipeline.StatusSkipped:
					done++
					fmt.Printf("%s skipped %s\n", prog(done), ev.Path)
				}
			}
			if errs > 0 {
				return fmt.Errorf("%d file(s) failed", errs)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "ollama", "captioning provider: ollama, anthropic, openai, gemini, openai-compatible")
	cmd.Flags().StringVar(&model, "model", "llava", "model name (e.g. llava, qwen2.5vl, qwen3-vl)")
	cmd.Flags().BoolVar(&recurse, "recurse", false, "recurse into subdirectories")
	cmd.Flags().StringVar(&templatePath, "template", "", "path to a client template JSON file")
	cmd.Flags().IntVar(&workers, "workers", 0, "number of parallel workers (default NumCPU)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "caption without writing metadata")
	cmd.Flags().StringVar(&ollamaHost, "ollama-host", "", "Ollama server URL (default http://localhost:11434)")

	return cmd
}
