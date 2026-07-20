package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the FotoArch version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("fotoarch %s\n", version)
			return nil
		},
	}
}
