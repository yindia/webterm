package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"webterm/internal/config"
)

func newConfigCommand() *cobra.Command {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage webterm configuration",
	}

	var path string
	initCmd := &cobra.Command{
		Use:   "init",
		Short: "Create default config template",
		RunE: func(_ *cobra.Command, _ []string) error {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("file already exists: %s", path)
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}

			cfg := config.Default()
			cfg.Auth.Password = "change-me"
			cfg.Sessions.SnapshotKey = "change-me-snapshot"

			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}

			if err := config.Write(path, cfg); err != nil {
				return err
			}

			fmt.Printf("created config template at %s\n", path)
			fmt.Println("default password: change-me")
			return nil
		},
	}
	initCmd.Flags().StringVar(&path, "path", "webterm.yaml", "output config path")
	configCmd.AddCommand(initCmd)

	return configCmd
}
