package cmd

import (
	"errors"
	"fmt"
	"net"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"webterm/internal/config"
)

func newDoctorCommand() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run environment diagnostics",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			if errors.Is(err, os.ErrNotExist) {
				cfg = config.Default()
			}

			failures := make([]string, 0)

			if runtime.GOOS != "windows" {
				if _, statErr := os.Stat(cfg.Terminal.Shell); statErr != nil {
					failures = append(failures, fmt.Sprintf("shell path not found: %s", cfg.Terminal.Shell))
				}
			}

			bindAddr := net.JoinHostPort(cfg.Server.Bind, fmt.Sprintf("%d", cfg.Server.Port))
			ln, listenErr := net.Listen("tcp", bindAddr)
			if listenErr != nil {
				failures = append(failures, fmt.Sprintf("port check failed on %s: %v", bindAddr, listenErr))
			} else {
				_ = ln.Close()
			}

			if len(failures) > 0 {
				for _, failure := range failures {
					fmt.Printf("[FAIL] %s\n", failure)
				}
				return fmt.Errorf("%d doctor checks failed", len(failures))
			}

			fmt.Println("[OK] shell path check")
			fmt.Println("[OK] network port check")
			fmt.Println("doctor checks passed")
			return nil
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "webterm.yaml", "path to config file")
	return cmd
}
