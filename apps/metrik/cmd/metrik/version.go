package main

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
		Short: "Show metrik version.",
		Long:  "Show metrik version.",
		Args:  cobra.ExactArgs(0),
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println(version)
		},
	}
}
