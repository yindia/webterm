package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"

	"webterm/internal/version"
)

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print build version",
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Printf("webterm %s\n", version.Version)
			fmt.Printf("commit: %s\n", version.Commit)
			fmt.Printf("built: %s\n", version.Date)
			fmt.Printf("go: %s\n", runtime.Version())
		},
	}
}
