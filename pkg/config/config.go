// Package config provides functionality for loading and saving configuration
package config

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
	"github.com/uberswe/LoopiaDomainBackorder/pkg/domain"
)

// DefaultConfigFileName is the default name for the configuration file
const DefaultConfigFileName = "config.json"

// Load loads the configuration from the config file.
// If the file doesn't exist, it returns a default configuration.
func Load(configFileName string) (*domain.Config, error) {
	// Default configuration
	config := &domain.Config{
		Username:      os.Getenv("LOOPIA_USERNAME"),
		Password:      os.Getenv("LOOPIA_PASSWORD"),
		Domains:       []string{},
		CacheDir:      "cache",
		CachedLists:   make(map[string]string),
		LastCacheTime: "",
	}

	// Check if config file exists
	if _, err := os.Stat(configFileName); os.IsNotExist(err) {
		log.Warn().Str("file", configFileName).Msg("Configuration file not found, using environment variables")
		return config, nil
	}

	// Read config file
	data, err := os.ReadFile(configFileName)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse config file
	var fileConfig domain.Config
	if err := json.Unmarshal(data, &fileConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Merge with defaults, ensuring backward compatibility
	if fileConfig.Username != "" {
		config.Username = fileConfig.Username
	}
	if fileConfig.Password != "" {
		config.Password = fileConfig.Password
	}
	if len(fileConfig.Domains) > 0 {
		config.Domains = fileConfig.Domains
	}
	if fileConfig.CacheDir != "" {
		config.CacheDir = fileConfig.CacheDir
	}
	if fileConfig.CachedLists != nil {
		config.CachedLists = fileConfig.CachedLists
	}
	if fileConfig.LastCacheTime != "" {
		config.LastCacheTime = fileConfig.LastCacheTime
	}

	return config, nil
}

// Save saves the configuration to the config file
func Save(config *domain.Config, configFileName string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	err = os.WriteFile(configFileName, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}
