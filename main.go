// loopia_register.go
// Command‑line tool to snipe an expiring domain the instant Loopia releases it
// (usually at 04:00 UTC for .se/.nu).  It fires the first order 30 ms before the
// drop, performs five ultra‑fast retries, then switches to exponential back‑off
// for up to one hour.
//
// Usage:
//   export LOOPIA_USERNAME="apiuser@loopiaapi"
//   export LOOPIA_PASSWORD="secret"
//   go run loopia_register.go -domain example.se
//
// Flags:
//   -domain string   Fully‑qualified domain to register (required)
//   -dry             Simulate calls without touching the API
//   -now             Start registration attempts immediately instead of waiting for drop time
//
// Requires Go 1.22+ and the XML‑RPC client lib:
//   go get github.com/kolo/xmlrpc
//
// Loopia API docs: https://www.loopia.com/api/
// © 2025 – MIT Licence.

package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"os"
	"sync"
	"time"

	"github.com/go-vgo/robotgo"
	"github.com/kolo/xmlrpc"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

const (
	loopiaEndpoint      = "https://api.loopia.se/RPCSERV"
	fastRetryCount      = 3 // number of immediate retries after drop
	fastRetryInterval   = 500 * time.Millisecond
	initialBackoff      = 1 * time.Second // starting back‑off interval
	maxBackoff          = 5 * time.Minute // cap for exponential back‑off
	purchasingWindow    = 1 * time.Hour   // keep trying for at most one hour
	preDroplead         = 0 * time.Millisecond
	dropHourUTC         = 4                // 04:00 UTC is when .se/.nu domains are dropped
	keepAwakeInterval   = 1 * time.Minute  // interval for mouse movement to keep computer awake
	timeRecheckInterval = 10 * time.Minute // interval to recheck time while waiting for drop time
)

// Default configuration file name
var configFileName = "config.json"

// Config represents the configuration file structure
type Config struct {
	Username string   `json:"username"`
	Password string   `json:"password"`
	Domains  []string `json:"domains"`
}

// Result represents the result of a domain registration attempt
type Result struct {
	Domain  string
	Success bool
	Error   error
}

// LoopiaClient wraps an xmlrpc.Client and automatically inserts
// username + password as the first two parameters of every call.
type LoopiaClient struct {
	username string
	password string
	rpc      *xmlrpc.Client
	dryRun   bool // if true, no RPC is executed (timing only)

	// Rate limiting
	callsMutex      sync.Mutex
	callsThisHour   int
	hourStartTime   time.Time
	stopOnErrorCode bool // if true, stop sending requests on 401 or 429 errors
}

func NewLoopiaClient(username, password string, dry bool) (*LoopiaClient, error) {
	jar, _ := cookiejar.New(nil)
	httpClient := &http.Client{Jar: jar, Timeout: 15 * time.Second}

	c, err := xmlrpc.NewClient(loopiaEndpoint, httpClient.Transport)
	if err != nil {
		return nil, err
	}
	return &LoopiaClient{
		username:        username,
		password:        password,
		rpc:             c,
		dryRun:          dry,
		callsThisHour:   0,
		hourStartTime:   time.Now(),
		stopOnErrorCode: true,
	}, nil
}

// call invokes an XML‑RPC method with authentication prepended.
func (c *LoopiaClient) call(method string, params ...interface{}) (interface{}, error) {
	all := append([]interface{}{c.username, c.password}, params...)

	// Create a logger event for this specific call
	reqLogger := log.With().
		Str("method", method).
		Str("operation", "api_call").
		Time("request_time", time.Now()).
		Logger()

	if c.dryRun {
		reqLogger.Info().
			Interface("params", params).
			Msg("[DRY-RUN] API call simulated")
		return "OK", nil
	}

	// Rate limiting check
	c.callsMutex.Lock()

	// Check if we need to reset the hour counter
	now := time.Now()
	if now.Sub(c.hourStartTime) >= time.Hour {
		reqLogger.Info().
			Int("previous_hour_calls", c.callsThisHour).
			Time("new_hour_start", now).
			Msg("Resetting API call counter for new hour")
		c.callsThisHour = 0
		c.hourStartTime = now
	}

	// Check if we've reached the limit
	if c.callsThisHour >= 60 {
		c.callsMutex.Unlock()
		errMsg := "API call limit of 60 calls per hour reached"
		reqLogger.Error().
			Int("calls_this_hour", c.callsThisHour).
			Time("hour_start", c.hourStartTime).
			Time("hour_end", c.hourStartTime.Add(time.Hour)).
			Msg(errMsg)
		return nil, errors.New(errMsg)
	}

	// Check if we should stop due to previous error
	if c.stopOnErrorCode {
		// We'll check this flag but still allow the call to proceed
		// This way the application can decide what to do with the error
		reqLogger.Warn().
			Msg("Making API call despite previous 401/429 error")
	}

	// Increment the counter
	c.callsThisHour++
	callNumber := c.callsThisHour
	c.callsMutex.Unlock()

	// Log the request details
	reqLogger.Info().
		Interface("params", params).
		Int("calls_this_hour", callNumber).
		Msg("Sending API request")

	// Record the start time for precise timing
	start := time.Now()

	// Make the actual API call
	var reply interface{}
	err := c.rpc.Call(method, all, &reply)

	// Calculate the duration
	duration := time.Since(start)

	// Log the response with timing information
	respLogger := reqLogger.With().
		Dur("duration_ms", duration).
		Time("response_time", time.Now()).
		Logger()

	if err != nil {
		respLogger.Error().
			Err(err).
			Msg("API call failed")

		// Check for specific error codes
		if errStr := err.Error(); errStr == "401 Unauthorized" || errStr == "429 Too Many Requests" {
			c.callsMutex.Lock()
			c.stopOnErrorCode = true
			c.callsMutex.Unlock()

			respLogger.Error().
				Str("error_code", errStr).
				Msg("Received critical error code, stopping further API calls")
		}

		return nil, err
	}

	respLogger.Info().
		Interface("response", reply).
		Msg("API call successful")

	return reply, nil
}

func (c *LoopiaClient) orderDomain(domain string) error {
	// Log the domain order attempt
	log.Info().
		Str("domain", domain).
		Str("operation", "order_domain").
		Time("attempt_time", time.Now()).
		Msg("Attempting to order domain")

	// orderDomain(..., domain, true) – true == pay with credits automatically
	_, err := c.call("orderDomain", domain, true)

	if err != nil {
		log.Error().
			Err(err).
			Str("domain", domain).
			Str("operation", "order_domain").
			Time("failure_time", time.Now()).
			Msg("Domain order failed")
	} else {
		log.Info().
			Str("domain", domain).
			Str("operation", "order_domain").
			Time("success_time", time.Now()).
			Msg("Domain order successful")
	}

	return err
}

func (c *LoopiaClient) payInvoiceIfAny(domain string) error {
	log.Info().
		Str("domain", domain).
		Str("operation", "check_invoice").
		Time("check_time", time.Now()).
		Msg("Checking for invoice to pay")

	resp, err := c.call("getDomain", domain)
	if err != nil {
		log.Error().
			Err(err).
			Str("domain", domain).
			Str("operation", "check_invoice").
			Msg("Failed to get domain information")
		return err
	}

	m, ok := resp.(map[string]interface{})
	if !ok {
		log.Error().
			Str("domain", domain).
			Str("operation", "check_invoice").
			Interface("response", resp).
			Msg("Unexpected response format from getDomain")
		return errors.New("unexpected response format from getDomain")
	}

	ref, _ := m["reference_no"].(string)
	if ref == "" {
		log.Info().
			Str("domain", domain).
			Str("operation", "check_invoice").
			Msg("No invoice to pay")
		return nil // nothing to pay
	}

	log.Info().
		Str("domain", domain).
		Str("reference", ref).
		Str("operation", "pay_invoice").
		Time("payment_attempt_time", time.Now()).
		Msg("Attempting to pay invoice")

	_, err = c.call("payInvoiceUsingCredits", ref)

	if err != nil {
		log.Error().
			Err(err).
			Str("domain", domain).
			Str("reference", ref).
			Str("operation", "pay_invoice").
			Time("failure_time", time.Now()).
			Msg("Invoice payment failed")
	} else {
		log.Info().
			Str("domain", domain).
			Str("reference", ref).
			Str("operation", "pay_invoice").
			Time("success_time", time.Now()).
			Msg("Invoice payment successful")
	}

	return err
}

// attempt tries to register and immediately pay for the domain.
func (c *LoopiaClient) attempt(domain string) error {
	attemptStart := time.Now()

	// Check if we should stop due to previous 401/429 error
	c.callsMutex.Lock()
	if c.stopOnErrorCode {
		c.callsMutex.Unlock()
		errMsg := "Aborting attempt due to previous 401/429 error"
		log.Error().
			Str("domain", domain).
			Str("operation", "registration_attempt").
			Msg(errMsg)
		return errors.New(errMsg)
	}
	c.callsMutex.Unlock()

	log.Info().
		Str("domain", domain).
		Str("operation", "registration_attempt").
		Time("start_time", attemptStart).
		Msg("Starting complete domain registration attempt")

	// Try to order the domain
	if err := c.orderDomain(domain); err != nil {
		log.Error().
			Err(err).
			Str("domain", domain).
			Str("operation", "registration_attempt").
			Dur("duration", time.Since(attemptStart)).
			Time("end_time", time.Now()).
			Msg("Domain registration attempt failed at order step")
		return err
	}

	// Try to pay for the domain if needed
	err := c.payInvoiceIfAny(domain)
	attemptEnd := time.Now()
	attemptDuration := attemptEnd.Sub(attemptStart)

	if err != nil {
		log.Error().
			Err(err).
			Str("domain", domain).
			Str("operation", "registration_attempt").
			Dur("duration", attemptDuration).
			Time("end_time", attemptEnd).
			Msg("Domain registration attempt failed at payment step")
		return err
	}

	log.Info().
		Str("domain", domain).
		Str("operation", "registration_attempt").
		Dur("duration", attemptDuration).
		Time("end_time", attemptEnd).
		Msg("Complete domain registration attempt successful")

	return nil
}

// nextDrop returns the next date at 04:00 UTC strictly after now.
func nextDrop(now time.Time) time.Time {
	utc := now.UTC()
	drop := time.Date(utc.Year(), utc.Month(), utc.Day(), dropHourUTC, 0, 0, 0, time.UTC)
	if !utc.Before(drop) {
		drop = drop.Add(24 * time.Hour)
	}
	return drop
}

// keepAwake keeps the computer awake by simulating mouse movement every minute.
// In a real implementation, this would use a library like robotgo to actually move the mouse.
// For this implementation, we'll just log a message.
func keepAwake(ctx context.Context) {
	ticker := time.NewTicker(keepAwakeInterval)
	defer ticker.Stop()

	log.Info().Msg("Starting keep-awake routine")

	for {
		select {
		case <-ctx.Done():
			log.Info().Msg("Stopping keep-awake routine")
			return
		case <-ticker.C:
			x, y := robotgo.GetMousePos()
			dx := rand.Intn(20) - 10
			dy := rand.Intn(20) - 10 // Random value between -2 and 2
			robotgo.MoveSmooth(x+dx, y+dy)

		}
	}
}

// loadConfig loads the configuration from the config file.
// If the file doesn't exist, it returns a default configuration.
func loadConfig() (*Config, error) {
	// Check if config file exists
	if _, err := os.Stat(configFileName); os.IsNotExist(err) {
		log.Warn().Str("file", configFileName).Msg("Configuration file not found, using environment variables")

		// Return default config using environment variables
		return &Config{
			Username: os.Getenv("LOOPIA_USERNAME"),
			Password: os.Getenv("LOOPIA_PASSWORD"),
			Domains:  []string{},
		}, nil
	}

	// Read config file
	data, err := os.ReadFile(configFileName)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Parse config file
	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &config, nil
}

// attemptDomainRegistration attempts to register a domain with retries
func attemptDomainRegistration(ctx context.Context, client *LoopiaClient, domain string, firstShot time.Time, resultCh chan<- Result) {
	attemptNo := 0
	backoff := time.Duration(0) // zero => fast retry window

	for {
		select {
		case <-ctx.Done():
			log.Warn().
				Str("domain", domain).
				Dur("window", purchasingWindow).
				Msg("No success within purchasing window")
			resultCh <- Result{Domain: domain, Success: false, Error: ctx.Err()}
			return
		default:
		}

		start := time.Now()
		attemptNo++

		log.Info().
			Int("attempt", attemptNo).
			Str("domain", domain).
			Time("start_time", start).
			Msg("Starting domain registration attempt")

		err := client.attempt(domain)
		attemptDuration := time.Since(start)

		if err == nil {
			log.Info().
				Int("attempt", attemptNo).
				Str("domain", domain).
				Dur("total_time", time.Since(firstShot)).
				Dur("attempt_duration", attemptDuration).
				Msg("SUCCESS – domain registered")
			resultCh <- Result{Domain: domain, Success: true, Error: nil}
			return
		}

		log.Warn().
			Int("attempt", attemptNo).
			Str("domain", domain).
			Err(err).
			Dur("attempt_duration", attemptDuration).
			Msg("Attempt failed")

		// Choose delay for next attempt
		var delay time.Duration
		if attemptNo <= fastRetryCount {
			delay = fastRetryInterval
		} else {
			if backoff == 0 {
				backoff = initialBackoff
			} else {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
			delay = backoff
		}

		// keep consistent cadence – deduct time spent inside the attempt
		if sleep := delay - time.Since(start); sleep > 0 {
			time.Sleep(sleep)
		}
	}
}

func main() {
	// Configure zerolog with console output and millisecond precision
	output := zerolog.ConsoleWriter{
		Out:        os.Stdout,
		TimeFormat: "2006-01-02T15:04:05.000Z07:00",
	}
	log.Logger = zerolog.New(output).With().Timestamp().Caller().Logger()
	zerolog.SetGlobalLevel(zerolog.InfoLevel)

	// Parse command-line flags
	domain := flag.String("domain", "", "Domain to register (can be specified multiple times)")
	dry := flag.Bool("dry", false, "Dry‑run – don’t hit Loopia API")
	startNow := flag.Bool("now", false, "Start registration attempts immediately instead of waiting for drop time")
	keepAwakeFlag := flag.Bool("keep-awake", false, "Keep computer awake by moving mouse")
	configFile := flag.String("config", configFileName, "Path to configuration file")
	flag.Parse()

	// Set the config file name if specified
	if *configFile != configFileName {
		configFileName = *configFile
	}

	// Load configuration
	config, err := loadConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	// If domain flag is provided, add it to the domains from config
	if *domain != "" {
		config.Domains = append(config.Domains, *domain)
	}

	// Check if we have any domains to register
	if len(config.Domains) == 0 {
		log.Fatal().Msg("No domains specified. Use -domain flag or add domains to config file")
	}

	// Check if we have credentials
	if config.Username == "" || config.Password == "" {
		// Try environment variables as fallback
		config.Username = os.Getenv("LOOPIA_USERNAME")
		config.Password = os.Getenv("LOOPIA_PASSWORD")

		if config.Username == "" || config.Password == "" {
			log.Fatal().Msg("No credentials found. Set them in config file or LOOPIA_USERNAME and LOOPIA_PASSWORD environment variables")
		}
	}

	// Create Loopia client
	client, err := NewLoopiaClient(config.Username, config.Password, *dry)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create Loopia client")
	}

	// Calculate start time
	now := time.Now()
	drop := nextDrop(now)
	firstShot := drop.Add(-preDroplead)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), firstShot.Sub(now)+purchasingWindow)
	defer cancel()

	if *startNow {
		// If -now flag is set, start immediately
		firstShot = time.Now()
		log.Info().Msg("Starting immediately due to -now flag")
	} else if wait := time.Until(firstShot); wait > 0 {
		log.Info().
			Dur("wait_time", wait).
			Str("first_attempt_time", firstShot.UTC().Format(time.RFC3339Nano)).
			Msg("Waiting until first attempt")

		// Start keep-awake routine if requested
		if *keepAwakeFlag {
			go keepAwake(ctx)
		}

		// Wait with periodic time rechecking
		for {
			// Recalculate the current time and drop time
			now = time.Now()
			drop = nextDrop(now)
			firstShot = drop.Add(-preDroplead)

			// Calculate the new wait time
			wait = time.Until(firstShot)

			// If it's time to start or less than a minute left, break the loop
			if wait <= 0 || wait < timeRecheckInterval {
				break
			}

			// Sleep for the shorter of the wait time or the recheck interval
			sleepTime := wait
			if sleepTime > timeRecheckInterval {
				sleepTime = timeRecheckInterval
			}

			log.Info().
				Dur("sleep_time", sleepTime).
				Dur("remaining_wait", wait).
				Str("updated_first_attempt_time", firstShot.UTC().Format(time.RFC3339Nano)).
				Msg("Sleeping and will recheck time")

			time.Sleep(sleepTime)
		}

		// Final sleep for any remaining time (less than a minute)
		if wait := time.Until(firstShot); wait > 0 {
			time.Sleep(wait)
		}
	}

	// Create channel for results
	resultCh := make(chan Result, len(config.Domains))

	// Start a goroutine for each domain
	var wg sync.WaitGroup
	for _, d := range config.Domains {
		wg.Add(1)
		go func(domain string) {
			defer wg.Done()
			attemptDomainRegistration(ctx, client, domain, firstShot, resultCh)
		}(d)
	}

	// Wait for all goroutines to finish in a separate goroutine
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Process results
	successCount := 0
	failCount := 0

	for result := range resultCh {
		if result.Success {
			successCount++
			log.Info().Str("domain", result.Domain).Msg("Domain registration successful")
		} else {
			failCount++
			log.Error().Str("domain", result.Domain).Err(result.Error).Msg("Domain registration failed")
		}
	}

	// Log summary
	log.Info().
		Int("total", len(config.Domains)).
		Int("success", successCount).
		Int("failed", failCount).
		Msg("Domain registration summary")
}
