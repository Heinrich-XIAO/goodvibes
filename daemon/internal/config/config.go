// SPDX-License-Identifier: AGPL-3.0-or-later
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Config holds local daemon settings stored in ~/.goodvibes/config.json.
type Config struct {
	ServerURL          string `json:"server_url"`
	APIKey             string `json:"api_key"`
	Username           string `json:"username"`
	ListenAddr         string `json:"listen_addr"`
	IdleTimeoutSeconds int    `json:"idle_timeout_seconds"`
	HeartbeatInterval  int    `json:"heartbeat_interval_seconds"`
	PrivacyMode        string `json:"privacy_mode"` // "strict" (default: hashes + metrics only) | "standard"
	DataDir            string `json:"data_dir"`
}

// DefaultDir returns ~/.goodvibes (or GOODVIBES_HOME if set).
func DefaultDir() string {
	if env := os.Getenv("GOODVIBES_HOME"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".goodvibes"
	}
	return filepath.Join(home, ".goodvibes")
}

// Default returns safe, privacy-first defaults suitable for low-spec machines.
func Default() Config {
	dir := DefaultDir()
	return Config{
		ServerURL:          "http://127.0.0.1:3000",
		ListenAddr:         "127.0.0.1:42420",
		IdleTimeoutSeconds: 120,
		HeartbeatInterval:  30,
		PrivacyMode:        "strict",
		DataDir:            dir,
	}
}

// ConfigPath returns the path to config.json.
func ConfigPath() string {
	return filepath.Join(DefaultDir(), "config.json")
}

// Load reads config from disk, falling back to defaults for unset fields.
func Load() (Config, error) {
	cfg := Default()
	path := ConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = "127.0.0.1:42420"
	}
	if cfg.IdleTimeoutSeconds <= 0 {
		cfg.IdleTimeoutSeconds = 120
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = 30
	}
	if cfg.DataDir == "" {
		cfg.DataDir = DefaultDir()
	}
	if cfg.PrivacyMode == "" {
		cfg.PrivacyMode = "strict"
	}
	return cfg, nil
}

// Save writes config to disk with 0600 permissions (protects API key).
func Save(cfg Config) error {
	dir := cfg.DataDir
	if dir == "" {
		dir = DefaultDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), data, 0o600)
}
