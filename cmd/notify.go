package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"webterm/internal/config"
)

func newNotifyCommand() *cobra.Command {
	var configPath string
	var title string
	var message string
	var level string
	cmd := &cobra.Command{
		Use:   "notify SESSION",
		Short: "Send monitoring notification",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			cfg, err := config.Load(configPath)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					cfg = config.Default()
					fmt.Printf("config file %q not found; using defaults\n", configPath)
				} else {
					return err
				}
			}
			if strings.TrimSpace(title) == "" {
				return errors.New("--title is required")
			}
			reqBody, err := json.Marshal(map[string]any{
				"session_id": args[0],
				"title":      title,
				"message":    message,
				"level":      level,
			})
			if err != nil {
				return err
			}
			url := serverURL(cfg) + "/api/monitoring/v1/notify"
			req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(reqBody))
			if err != nil {
				return err
			}
			req.Header.Set("Content-Type", "application/json")
			if strings.TrimSpace(cfg.Monitoring.Token) != "" {
				req.Header.Set("X-Webterm-Monitor-Token", cfg.Monitoring.Token)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK {
				return fmt.Errorf("notify failed: %s", res.Status)
			}
			fmt.Printf("notified %s\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "webterm.yaml", "path to config file")
	cmd.Flags().StringVar(&title, "title", "", "notification title")
	cmd.Flags().StringVar(&message, "message", "", "notification message")
	cmd.Flags().StringVar(&level, "level", "info", "notification level (info/wait/error/done)")
	return cmd
}
