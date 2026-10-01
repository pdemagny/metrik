package cmd

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"
)

//go:embed version.txt
var version string

func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show blobrctl version.",
		Long:  "Show blobrctl version.",
		Args:  cobra.ExactArgs(0),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println(version)
		},
	}
}
