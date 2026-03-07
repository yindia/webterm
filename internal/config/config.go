package config

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Auth     AuthConfig     `yaml:"auth"`
	Terminal TerminalConfig `yaml:"terminal"`
	Policy   PolicyConfig   `yaml:"policy"`
	Sessions SessionConfig  `yaml:"sessions"`
	Logging  LoggingConfig  `yaml:"logging"`
}

type ServerConfig struct {
	Bind string `yaml:"bind"`
	Port int    `yaml:"port"`
}

type AuthConfig struct {
	Mode            string        `yaml:"mode"`
	PasswordHash    string        `yaml:"password_hash"`
	Token           string        `yaml:"token"`
	SessionTTL      time.Duration `yaml:"session_ttl"`
	CSRFSecret      string        `yaml:"csrf_secret"`
	RateLimitMax    int           `yaml:"rate_limit_max"`
	RateLimitWindow time.Duration `yaml:"rate_limit_window"`
}

type TerminalConfig struct {
	Shell      string   `yaml:"shell"`
	WorkingDir string   `yaml:"working_dir"`
	Env        []string `yaml:"env"`
	User       string   `yaml:"user"`
	Group      string   `yaml:"group"`
}

type PolicyConfig struct {
	WhitelistEnabled bool         `yaml:"whitelist_enabled"`
	Rules            []PolicyRule `yaml:"rules"`
}

type PolicyRule struct {
	Type    string `yaml:"type"`
	Pattern string `yaml:"pattern"`
}

type SessionConfig struct {
	MaxSessions      int           `yaml:"max_sessions"`
	IdleTimeout      time.Duration `yaml:"idle_timeout"`
	SnapshotDir      string        `yaml:"snapshot_dir"`
	SnapshotInterval time.Duration `yaml:"snapshot_interval"`
	SnapshotKey      string        `yaml:"snapshot_key"`
}

type LoggingConfig struct {
	Audit bool   `yaml:"audit"`
	Level string `yaml:"level"`
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			Bind: "0.0.0.0",
			Port: 8080,
		},
		Auth: AuthConfig{
			Mode:            "password",
			SessionTTL:      24 * time.Hour,
			CSRFSecret:      "change-this-secret",
			RateLimitMax:    10,
			RateLimitWindow: 10 * time.Minute,
		},
		Terminal: TerminalConfig{
			Shell:      "/bin/bash",
			WorkingDir: "~",
			Env:        []string{},
			User:       "",
			Group:      "",
		},
		Policy: PolicyConfig{
			WhitelistEnabled: false,
			Rules:            []PolicyRule{},
		},
		Sessions: SessionConfig{
			MaxSessions:      10,
			IdleTimeout:      24 * time.Hour,
			SnapshotDir:      "~/.webterm/snapshots",
			SnapshotInterval: 5 * time.Second,
			SnapshotKey:      "",
		},
		Logging: LoggingConfig{
			Audit: true,
			Level: "info",
		},
	}
}

func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	cfg := Default()
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func Write(path string, cfg Config) error {
	raw, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, raw, 0o600)
}
