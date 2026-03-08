package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

func newCompletionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion",
		Short: "Generate shell completion scripts",
	}

	cmd.AddCommand(newCompletionShellCommand("bash", func(c *cobra.Command, out io.Writer) error {
		return c.Root().GenBashCompletion(out)
	}))
	cmd.AddCommand(newCompletionShellCommand("zsh", func(c *cobra.Command, out io.Writer) error {
		return c.Root().GenZshCompletion(out)
	}))
	cmd.AddCommand(newCompletionShellCommand("fish", func(c *cobra.Command, out io.Writer) error {
		return c.Root().GenFishCompletion(out, true)
	}))

	return cmd
}

func newCompletionShellCommand(name string, fn func(*cobra.Command, io.Writer) error) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: fmt.Sprintf("Generate %s completion", name),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fn(cmd, cmd.OutOrStdout())
		},
	}
}
