package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

func newAgentSetupCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent-setup",
		Short: "Install agent notification helpers",
		RunE: func(_ *cobra.Command, _ []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			root := filepath.Join(home, ".webterm", "agents")
			if err := os.MkdirAll(root, 0o755); err != nil {
				return err
			}
			scriptPath := filepath.Join(root, "notify.sh")
			content := []byte("#!/bin/sh\n\n" +
				"# Usage: webterm_agent_notify SESSION TITLE [MESSAGE]\n" +
				"webterm_agent_notify() {\n" +
				"  session=\"$1\"\n" +
				"  title=\"$2\"\n" +
				"  message=\"$3\"\n" +
				"  if [ -z \"$session\" ] || [ -z \"$title\" ]; then\n" +
				"    echo \"Usage: webterm_agent_notify SESSION TITLE [MESSAGE]\"\n" +
				"    return 1\n" +
				"  fi\n" +
				"  webterm notify \"$session\" --title \"$title\" --message \"$message\"\n" +
				"}\n")
			if err := os.WriteFile(scriptPath, content, 0o755); err != nil {
				return err
			}
			readmePath := filepath.Join(root, "README.txt")
			readme := []byte("WebTerm Agent Hooks\n\n" +
				"1) Source notify.sh from your shell: \n" +
				"   . ~/.webterm/agents/notify.sh\n\n" +
				"2) Call helper from agents or scripts: \n" +
				"   webterm_agent_notify session-id \"Needs approval\" \"Waiting for input\"\n")
			if err := os.WriteFile(readmePath, readme, 0o644); err != nil {
				return err
			}
			fmt.Printf("agent helpers installed in %s\n", root)
			return nil
		},
	}
	return cmd
}
