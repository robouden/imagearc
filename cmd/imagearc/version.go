package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the ImageArc version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("imagearc %s\n", version)
			return nil
		},
	}
}
