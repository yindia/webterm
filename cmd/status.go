package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"webterm/internal/config"
)

type statusSession struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Command   string             `json:"command"`
	Attention string             `json:"attention"`
	CPU       float64            `json:"cpu_percent"`
	Activity  []statusActivityPt `json:"activity"`
}

type statusActivityPt struct {
	Score float64 `json:"score"`
}

func newStatusCommand() *cobra.Command {
	var configPath string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show monitoring status",
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
			url := serverURL(cfg) + "/api/monitoring/v1/summary"
			req, err := http.NewRequest(http.MethodGet, url, nil)
			if err != nil {
				return err
			}
			if strings.TrimSpace(cfg.Monitoring.Token) != "" {
				req.Header.Set("X-Webterm-Monitor-Token", cfg.Monitoring.Token)
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if res.StatusCode != http.StatusOK {
				return fmt.Errorf("status request failed: %s", res.Status)
			}
			var payload struct {
				Sessions []statusSession `json:"sessions"`
			}
			if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
				return err
			}
			fmt.Printf("%-14s %-5s %-9s %s\n", "SESSION", "CPU", "ATTENTION", "ACTIVITY")
			for _, session := range payload.Sessions {
				fmt.Printf("%-14s %-5s %-9s %s\n",
					trimLabel(session.Name, 14),
					fmt.Sprintf("%.0f%%", session.CPU),
					trimLabel(session.Attention, 9),
					sparkline(session.Activity),
				)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&configPath, "config", "webterm.yaml", "path to config file")
	return cmd
}

func sparkline(points []statusActivityPt) string {
	if len(points) == 0 {
		return "-"
	}
	chars := []rune("▁▂▃▄▅▆▇█")
	max := 0.0
	for _, p := range points {
		if p.Score > max {
			max = p.Score
		}
	}
	if max == 0 {
		return strings.Repeat(string(chars[0]), len(points))
	}
	var out strings.Builder
	for _, p := range points {
		idx := int((p.Score / max) * float64(len(chars)-1))
		out.WriteRune(chars[idx])
	}
	return out.String()
}

func trimLabel(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}
