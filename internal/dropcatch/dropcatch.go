// Package dropcatch implements the dropcatch command functionality
package dropcatch

import (
	"context"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/uberswe/LoopiaDomainBackorder/pkg/api"
	"github.com/uberswe/LoopiaDomainBackorder/pkg/domain"
	"github.com/uberswe/LoopiaDomainBackorder/pkg/util"
)

const (
	fastRetryCount    = 3 // number of immediate retries after drop
	fastRetryInterval = 100 * time.Millisecond
	initialBackoff    = 1 * time.Second // starting back‑off interval
	maxBackoff        = 5 * time.Minute // cap for exponential back‑off
	purchasingWindow  = 1 * time.Hour   // keep trying for at most one hour
	preDroplead       = 100 * time.Millisecond
)

// AttemptDomainRegistration attempts to register a domain with retries
func AttemptDomainRegistration(ctx context.Context, client *api.Client, domainName string, firstShot time.Time, resultCh chan<- domain.Result) {
	attemptNo := 0
	backoff := time.Duration(0) // zero => fast retry window

	for {
		select {
		case <-ctx.Done():
			log.Warn().
				Str("domain", domainName).
				Dur("window", purchasingWindow).
				Msg("No success within purchasing window")
			resultCh <- domain.Result{Domain: domainName, Success: false, Error: ctx.Err()}
			return
		default:
		}

		start := time.Now()
		attemptNo++

		log.Info().
			Int("attempt", attemptNo).
			Str("domain", domainName).
			Time("start_time", start).
			Msg("Starting domain registration attempt")

		err := client.Attempt(domainName)
		attemptDuration := time.Since(start)

		if err == nil {
			log.Info().
				Int("attempt", attemptNo).
				Str("domain", domainName).
				Dur("total_time", time.Since(firstShot)).
				Dur("attempt_duration", attemptDuration).
				Msg("SUCCESS – domain registered")
			resultCh <- domain.Result{Domain: domainName, Success: true, Error: nil}
			return
		}

		log.Warn().
			Int("attempt", attemptNo).
			Str("domain", domainName).
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

// Run handles the dropcatch command functionality
func Run(config *domain.Config, domainName string, dry bool, startNow bool, keepAwakeFlag bool) {
	// Check if we have any domains to register
	if domainName != "" {
		config.Domains = append(config.Domains, domainName)
	}

	if len(config.Domains) == 0 {
		log.Fatal().Msg("No domains specified. Use -domain flag or add domains to config file")
	}

	// Create Loopia client
	client, err := api.NewClient(config.Username, config.Password, dry)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create Loopia client")
	}

	// Calculate start time
	now := time.Now()
	drop := util.NextDrop(now)
	firstShot := drop.Add(-preDroplead)

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), firstShot.Sub(now)+purchasingWindow)
	defer cancel()

	if startNow {
		// If -now flag is set, start immediately
		firstShot = time.Now()
		log.Info().Msg("Starting immediately due to -now flag")
	} else if wait := time.Until(firstShot); wait > 0 {
		log.Info().
			Dur("wait_time", wait).
			Str("first_attempt_time", firstShot.UTC().Format(time.RFC3339Nano)).
			Msg("Waiting until first attempt")

		// Start keep-awake routine if requested
		if keepAwakeFlag {
			go util.KeepAwake(ctx)
		}

		// Wait with periodic time rechecking
		for {
			// Recalculate the current time and drop time
			now = time.Now()
			drop = util.NextDrop(now)
			firstShot = drop.Add(-preDroplead)

			// Calculate the new wait time
			wait = time.Until(firstShot)

			// If it's time to start or less than a minute left, break the loop
			if wait <= 0 || wait < util.TimeRecheckInterval {
				break
			}

			// Sleep for the shorter of the wait time or the recheck interval
			sleepTime := wait
			if sleepTime > util.TimeRecheckInterval {
				sleepTime = util.TimeRecheckInterval
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

	// Create slice to store results
	var results []domain.Result
	var resultsMutex sync.Mutex
	var wg sync.WaitGroup

	// Process domains in parallel
	log.Info().Int("domains", len(config.Domains)).Msg("Processing domains in parallel")

	for _, domainToRegister := range config.Domains {
		// Add to wait group before starting goroutine
		wg.Add(1)

		// Create a copy of domain for the goroutine
		domainCopy := domainToRegister

		// Start a goroutine for each domain
		go func() {
			defer wg.Done()

			// Create a separate context for each domain to prevent cancellation affecting other domains
			domainCtx, domainCancel := context.WithTimeout(context.Background(), purchasingWindow)
			defer domainCancel()

			// Create a channel for this domain's result
			resultCh := make(chan domain.Result, 1)

			log.Info().Str("domain", domainCopy).Msg("Starting registration attempt for domain")

			// Process this domain
			AttemptDomainRegistration(domainCtx, client, domainCopy, firstShot, resultCh)

			// Get the result
			result := <-resultCh

			// Safely append to results slice
			resultsMutex.Lock()
			results = append(results, result)
			resultsMutex.Unlock()

			log.Info().
				Str("domain", domainCopy).
				Bool("success", result.Success).
				Msg("Completed registration attempt for domain")
		}()
	}

	// Wait for all goroutines to complete
	wg.Wait()
	log.Info().Msg("All domain registration attempts completed")

	// Process results
	successCount := 0
	failCount := 0

	for _, result := range results {
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
