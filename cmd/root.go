package cmd

import "github.com/spf13/cobra"

func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "webterm",
		Short: "Self-hosted browser terminal",
	}

	root.AddCommand(newServeCommand())
	root.AddCommand(newDoctorCommand())
	root.AddCommand(newConfigCommand())
	root.AddCommand(newVersionCommand())
	root.AddCommand(newCompletionCommand())

	return root
}
