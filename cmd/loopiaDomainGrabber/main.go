// Package main provides the entry point for the loopiaDomainGrabber application.
//
// loopiaDomainGrabber is a command-line tool for working with domains from Loopia.
// It provides two main commands:
//
// 1. dropcatch - Snipes expiring domains the instant Loopia releases them
//    (usually at 04:00 UTC for .se/.nu domains).
//
// 2. available - Downloads domain lists, checks for domains expiring today based on
//    the local date, and evaluates them based on criteria like length and pronounceability.
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
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/uberswe/LoopiaDomainBackorder/internal/available"
	"github.com/uberswe/LoopiaDomainBackorder/internal/dropcatch"
	"github.com/uberswe/LoopiaDomainBackorder/pkg/config"
)

func main() {
	// Configure zerolog with console output and microsecond precision
	// Set global time format to include microseconds
	zerolog.TimeFieldFormat = "2006-01-02 15:04:05.000000"

	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: time.StampMicro, // Use Go's built-in microsecond format
	}
	log.Logger = zerolog.New(output).With().Timestamp().Caller().Logger()
	
	// Set log level to Debug to see more detailed logs
	zerolog.SetGlobalLevel(zerolog.DebugLevel)

	// Check if we have any arguments
	if len(os.Args) < 2 {
		fmt.Println("Usage: loopiaDomainGrabber <command> [options]")
		fmt.Println("Commands:")
		fmt.Println("  dropcatch   Attempt to register domains as they expire")
		fmt.Println("  available   Find valuable domains expiring today")
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
		// Parse flags
		flag.Parse()
		
		// Load configuration
		cfg, err := config.Load(*configFile)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to load configuration")
		}
		
		// Run available command
		available.Run(cfg)
		
	default:
		fmt.Printf("Unknown command: %s\n", command)
		fmt.Println("Available commands: dropcatch, available")
		os.Exit(1)
	}
}