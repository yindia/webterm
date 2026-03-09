package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"webterm/internal/config"
	"webterm/internal/monitoring"
	"webterm/internal/server"
)

var runServer = server.Run

func newServeCommand() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:     "serve",
		Aliases: []string{"up"},
		Short:   "Start webterm server",
		RunE: func(_ *cobra.Command, _ []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					cfg = config.Default()
					fmt.Printf("config file %q not found; using defaults\n", configPath)
				} else {
					return err
				}
			}

			addr := net.JoinHostPort(cfg.Server.Bind, fmt.Sprintf("%d", cfg.Server.Port))
			url := fmt.Sprintf("http://%s", addr)
			if cfg.Server.Bind == "0.0.0.0" {
				url = fmt.Sprintf("http://localhost:%d", cfg.Server.Port)
			}

			fmt.Printf("webterm listening on %s\n", addr)
			fmt.Printf("open %s in your browser\n", url)

			ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer cancel()

			if cfg.Monitoring.Enabled {
				go runMonitoringDaemon(ctx, cfg)
			}

			return runServer(ctx, cfg)
		},
	}

	cmd.Flags().StringVar(&configPath, "config", "webterm.yaml", "path to config file")
	return cmd
}

func runMonitoringDaemon(ctx context.Context, cfg config.Config) {
	url := serverURL(cfg)
	daemon := monitoring.NewDaemon(url, cfg.Monitoring.Token, cfg.Monitoring.SampleInterval)
	daemon.EnableTmux()
	for {
		if err := daemon.Run(ctx); err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			time.Sleep(2 * time.Second)
			continue
		}
		return
	}
}
