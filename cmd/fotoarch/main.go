// Command fotoarch is an AI photo captioning + IPTC/XMP metadata tool for photographers.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "0.1.0"

func main() {
	root := &cobra.Command{
		Use:   "fotoarch",
		Short: "FotoArch: AI photo captioning and IPTC/XMP metadata tool",
		Long: `FotoArch is a cross-platform, open-source CLI and web app that captions your
photos with a local or cloud AI model and writes the results as standard
IPTC/XMP metadata (in-file for JPEG/TIFF, .xmp sidecars for RAW). Non-destructive:
pixels are never touched. No lock-in: everything lands in CSV/JSON/XMP.`,
	}

	root.AddCommand(newCaptionCmd())
	root.AddCommand(newCatalogCmd())
	root.AddCommand(newServeCmd())
	root.AddCommand(newVersionCmd())

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
