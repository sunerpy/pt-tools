package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sunerpy/pt-tools/cli/internal/types"
)

const (
	dirName  = ".pt-tools-cli"
	fileName = "config.json"
)

// Load reads the CLI configuration from ~/.pt-tools-cli/config.json.
func Load() (*types.CLIConfig, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &types.CLIConfig{}, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg types.CLIConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

// Save writes the CLI configuration to ~/.pt-tools-cli/config.json.
func Save(cfg *types.CLIConfig) error {
	dir, err := configDir()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	path := filepath.Join(dir, fileName)
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// Clear removes the cached session cookie.
func Clear() error {
	cfg, err := Load()
	if err != nil {
		return err
	}
	cfg.Cookie = ""
	cfg.Expires = 0
	return Save(cfg)
}

// ConfigDir returns the path to ~/.pt-tools-cli/
func ConfigDir() (string, error) {
	return configDir()
}

func configDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	return filepath.Join(home, dirName), nil
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, fileName), nil
}

// IsCookieValid checks if the cached cookie is still valid.
func IsCookieValid(cfg *types.CLIConfig) bool {
	if cfg.Cookie == "" {
		return false
	}
	if cfg.Expires > 0 && time.Now().Unix() > cfg.Expires {
		return false
	}
	return true
}
