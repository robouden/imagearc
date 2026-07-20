package main

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/robouden/fotoarch/internal/catalog"
	"github.com/robouden/fotoarch/internal/metadata"
	"github.com/robouden/fotoarch/internal/pipeline"
)

func newCatalogCmd() *cobra.Command {
	var (
		noAI    bool
		output  string
		recurse bool
	)

	cmd := &cobra.Command{
		Use:   "catalog <dir>",
		Short: "Build a CSV catalog of a folder's photos and their metadata",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := args[0]
			_ = noAI // catalog only ever reads existing metadata; kept for CLI compatibility

			if err := metadata.CheckExifTool(); err != nil {
				return err
			}

			files, err := pipeline.Walk(root, recurse)
			if err != nil {
				return err
			}

			var rows []catalog.Row
			for _, f := range files {
				m, err := metadata.Read(f)
				if err != nil {
					fmt.Printf("[error] %s: %v\n", f, err)
					continue
				}
				rows = append(rows, catalog.Row{
					Path:     f,
					Filename: filepath.Base(f),
					Caption:  m.Caption,
					Keywords: strings.Join(m.Keywords, ", "),
					Byline:   m.Byline,
					Location: m.Location,
					Date:     m.Date,
				})
				fmt.Printf("[read] %s\n", f)
			}

			if err := catalog.Write(output, rows); err != nil {
				return err
			}
			fmt.Printf("wrote %d rows to %s\n", len(rows), output)
			return nil
		},
	}

	cmd.Flags().BoolVar(&noAI, "no-ai", true, "read existing IPTC only, no AI, no network (always true today)")
	cmd.Flags().StringVarP(&output, "output", "o", "catalog.csv", "output CSV path")
	cmd.Flags().BoolVar(&recurse, "recurse", true, "recurse into subdirectories")

	return cmd
}
