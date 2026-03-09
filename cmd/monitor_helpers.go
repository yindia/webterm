package cmd

import (
	"fmt"
	"net"
	"strings"

	"webterm/internal/config"
)

func serverURL(cfg config.Config) string {
	addr := net.JoinHostPort(cfg.Server.Bind, fmt.Sprintf("%d", cfg.Server.Port))
	url := fmt.Sprintf("http://%s", addr)
	if cfg.Server.Bind == "0.0.0.0" || strings.TrimSpace(cfg.Server.Bind) == "" {
		url = fmt.Sprintf("http://localhost:%d", cfg.Server.Port)
	}
	return url
}
