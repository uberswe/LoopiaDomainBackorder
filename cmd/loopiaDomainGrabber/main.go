// Package main provides the entry point for the loopiaDomainGrabber application.
//
// loopiaDomainGrabber is a command-line tool for working with domains from Loopia.
// It provides two main commands:
//
//  1. dropcatch - Snipes expiring domains the instant Loopia releases them
//     (usually at 04:00 UTC for .se/.nu domains).
//
//  2. available - Downloads domain lists, checks for domains expiring today based on
//     the local date, and evaluates them based on criteria like length and pronounceability.
//
// The application is structured following standard Go project layout conventions,
// with packages organized by functionality:
//
// - cmd/loopiaDomainGrabber: Main application entry point
// - internal/dropcatch: Implementation of the dropcatch command
// - internal/available: Implementation of the available command
// - pkg/api: Loopia API client
// - pkg/config: Configuration handling
// - pkg/domain: Domain-related models
// - pkg/util: Utility functions
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/uberswe/LoopiaDomainBackorder/internal/available"
	"github.com/uberswe/LoopiaDomainBackorder/internal/dropcatch"
	"github.com/uberswe/LoopiaDomainBackorder/pkg/config"
)

// Version information
var (
	// Version is the current version of the application
	Version = "0.1.0"
	// Commit is the git commit hash of the build
	Commit = "unknown"
	// BuildDate is the date when the binary was built
	BuildDate = "unknown"
)

// setupLogging configures zerolog to write logs to both console and file
// It creates a new log file for each day and cleans up log files older than 30 days
func setupLogging() error {
	// Set global time format to include microseconds
	zerolog.TimeFieldFormat = "2006-01-02 15:04:05.000000"

	// Set log level to Debug to see more detailed logs
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	// Create log directory if it doesn't exist
	logDir := "logs"
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	// Clean up old log files
	cleanupOldLogs(logDir, 30)

	// Configure console output
	consoleWriter := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.StampMicro, // Use Go's built-in microsecond format
	}

	// Configure file output with daily rotation
	logFileName := filepath.Join(logDir, time.Now().Format("2006-01-02")+".log")
	logFile, err := os.OpenFile(logFileName, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}

	// Use MultiLevelWriter to write to both console and file
	multi := zerolog.MultiLevelWriter(consoleWriter, logFile)
	log.Logger = zerolog.New(multi).With().Timestamp().Caller().Logger()

	log.Info().Str("file", logFileName).Msg("Logging to file initialized")
	return nil
}

// cleanupOldLogs removes log files older than the specified number of days
func cleanupOldLogs(logDir string, maxAgeDays int) {
	// Get the cutoff time
	cutoffTime := time.Now().AddDate(0, 0, -maxAgeDays)

	// List all files in the log directory
	files, err := filepath.Glob(filepath.Join(logDir, "*.log"))
	if err != nil {
		log.Error().Err(err).Msg("Failed to list log files during cleanup")
		return
	}

	// Check each file
	for _, file := range files {
		// Extract date from filename (format: YYYY-MM-DD.log)
		baseName := filepath.Base(file)
		if len(baseName) < 10 { // Minimum length for YYYY-MM-DD.log
			continue
		}

		dateStr := strings.TrimSuffix(baseName, ".log")
		fileDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			log.Debug().Err(err).Str("file", file).Msg("Skipping file with invalid date format during cleanup")
			continue
		}

		// Delete file if it's older than the cutoff time
		if fileDate.Before(cutoffTime) {
			if err := os.Remove(file); err != nil {
				log.Error().Err(err).Str("file", file).Msg("Failed to delete old log file")
			} else {
				log.Info().Str("file", file).Msg("Deleted old log file")
			}
		}
	}
}

func main() {
	// Setup logging with file output and rotation
	if err := setupLogging(); err != nil {
		fmt.Printf("Error setting up logging: %v\n", err)
		os.Exit(1)
	}

	// Check if we have any arguments
	if len(os.Args) < 2 {
		fmt.Println("Usage: loopiaDomainGrabber <command> [options]")
		fmt.Println("Commands:")
		fmt.Println("  dropcatch   Attempt to register domains as they expire")
		fmt.Println("  available   Find valuable domains expiring today or on a specified date")
		fmt.Println("  version     Display version information")
		fmt.Println("Run 'loopiaDomainGrabber <command> -h' for command-specific help")
		os.Exit(1)
	}

	// Get the command
	command := os.Args[1]

	// Remove the command from os.Args to make flag parsing work
	os.Args = append(os.Args[:1], os.Args[2:]...)

	// Define common flags
	configFile := flag.String("config", config.DefaultConfigFileName, "Path to configuration file")

	// Command-specific handling
	switch command {
	case "dropcatch":
		// Define dropcatch-specific flags
		domain := flag.String("domain", "", "Domain to register (can be specified multiple times)")
		dry := flag.Bool("dry", false, "Dry‑run – don't hit Loopia API")
		startNow := flag.Bool("now", false, "Start registration attempts immediately instead of waiting for drop time")
		keepAwakeFlag := flag.Bool("keep-awake", false, "Keep computer awake by moving mouse")

		// Parse flags
		flag.Parse()

		// Load configuration
		cfg, err := config.Load(*configFile)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to load configuration")
		}

		// Check if we have credentials
		if cfg.Username == "" || cfg.Password == "" {
			// Try environment variables as fallback
			cfg.Username = os.Getenv("LOOPIA_USERNAME")
			cfg.Password = os.Getenv("LOOPIA_PASSWORD")

			if cfg.Username == "" || cfg.Password == "" {
				log.Fatal().Msg("No credentials found. Set them in config file or LOOPIA_USERNAME and LOOPIA_PASSWORD environment variables")
			}
		}

		// Run dropcatch command
		dropcatch.Run(cfg, *domain, *dry, *startNow, *keepAwakeFlag)

	case "available":
		// Define available-specific flags
		dateStr := flag.String("date", "", "Date to check for expiring domains (format: YYYY-MM-DD)")

		// Parse flags
		flag.Parse()

		// Load configuration
		cfg, err := config.Load(*configFile)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to load configuration")
		}

		// Run available command
		available.Run(cfg, *dateStr)

	case "version":
		// Display version information
		fmt.Printf("loopiaDomainGrabber version %s\n", Version)
		fmt.Printf("Commit: %s\n", Commit)
		fmt.Printf("Build Date: %s\n", BuildDate)

	default:
		fmt.Printf("Unknown command: %s\n", command)
		fmt.Println("Available commands: dropcatch, available, version")
		os.Exit(1)
	}
}
