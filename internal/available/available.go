// Package available implements the available command functionality
package available

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/uberswe/LoopiaDomainBackorder/pkg/config"
	"github.com/uberswe/LoopiaDomainBackorder/pkg/domain"
	"github.com/uberswe/LoopiaDomainBackorder/pkg/util"
)

// URLs to download
var domainListURLs = []string{
	"https://data.internetstiftelsen.se/bardate_domains.txt",
	"https://data.internetstiftelsen.se/bardate_domains_nu.txt",
}

// Run handles the available command functionality
func Run(cfg *domain.Config) {
	log.Info().Msg("Running available command to find valuable domains expiring today")

	// Check if we need to download new files (cache expired)
	needsDownload := true
	if cfg.LastCacheTime != "" {
		lastCache, err := time.Parse(time.RFC3339, cfg.LastCacheTime)
		if err == nil {
			// Check if cache is less than 24 hours old
			if time.Since(lastCache) < 24*time.Hour {
				needsDownload = false
				log.Info().Time("last_cache", lastCache).Msg("Using cached domain lists (less than 24 hours old)")
			}
		}
	}

	// Initialize cache map if needed
	if cfg.CachedLists == nil {
		cfg.CachedLists = make(map[string]string)
	}

	// Create cache directory if it doesn't exist
	if cfg.CacheDir == "" {
		cfg.CacheDir = "cache"
	}

	err := os.MkdirAll(cfg.CacheDir, 0755)
	if err != nil {
		log.Error().Err(err).Str("dir", cfg.CacheDir).Msg("Failed to create cache directory")
		return
	}

	// Download files if needed
	if needsDownload {
		downloadDomainLists(cfg)
	}

	// Process domain lists
	domains := processDomainLists(cfg)

	// Display top domains
	displayTopDomains(domains)
}

// downloadDomainLists downloads the domain lists and caches them
func downloadDomainLists(cfg *domain.Config) {
	log.Info().Msg("Downloading domain lists")

	for _, url := range domainListURLs {
		log.Info().Str("url", url).Msg("Downloading domain list")

		// Download file
		resp, err := http.Get(url)
		if err != nil {
			log.Error().Err(err).Str("url", url).Msg("Failed to download domain list")
			continue
		}
		defer resp.Body.Close()

		// Read response body
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Error().Err(err).Str("url", url).Msg("Failed to read domain list")
			continue
		}

		// Save to cache
		filename := fmt.Sprintf("%s/%s", cfg.CacheDir, filepath.Base(url))
		err = os.WriteFile(filename, body, 0644)
		if err != nil {
			log.Error().Err(err).Str("file", filename).Msg("Failed to save domain list to cache")
			continue
		}

		// Update cache map
		cfg.CachedLists[url] = filename
		log.Info().Str("url", url).Str("file", filename).Msg("Domain list cached")
	}

	// Update last cache time
	cfg.LastCacheTime = time.Now().Format(time.RFC3339)

	// Save updated config
	err := config.Save(cfg, config.DefaultConfigFileName)
	if err != nil {
		log.Error().Err(err).Msg("Failed to save updated configuration")
	}
}

// processDomainLists processes the cached domain lists and returns domains expiring on the reference date.
// The reference date is determined by the local date (not UTC date) to ensure that domains expiring
// "today" are correctly identified regardless of the user's time zone.
func processDomainLists(cfg *domain.Config) []domain.DomainInfo {
	var domains []domain.DomainInfo
	// Use local time (not UTC) to ensure we get the correct reference date based on the user's local date.
	// This is crucial for correct operation when the local date differs from the UTC date
	// (e.g., at 00:38 CEST, which is 22:38 UTC of the previous day).
	now := time.Now()
	referenceDate := util.GetReferenceDate(now)

	log.Info().
		Time("local_time", now).
		Time("reference_date", referenceDate).
		Msg("Using reference date for domain filtering")

	for _, filename := range cfg.CachedLists {
		log.Info().Str("file", filename).Msg("Processing domain list")

		// Read file
		data, err := os.ReadFile(filename)
		if err != nil {
			log.Error().Err(err).Str("file", filename).Msg("Failed to read cached domain list")
			continue
		}

		// Process each line
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			// Parse domain info
			parts := strings.Fields(line)
			if len(parts) < 2 {
				continue
			}

			domainName := parts[0]
			expiryDateStr := parts[1]

			// Parse expiry date (format may vary, adjust as needed)
			expiryDate, err := time.Parse("2006-01-02", expiryDateStr)
			if err != nil {
				log.Debug().Err(err).Str("domain", domainName).Str("date", expiryDateStr).Msg("Failed to parse expiry date")
				continue
			}

			// Add debug logging for specific domains of interest
			if domainName == "d7.se" {
				log.Info().
					Str("domain", domainName).
					Time("expiry_date", expiryDate).
					Time("reference_date", referenceDate).
					Bool("year_match", expiryDate.Year() == referenceDate.Year()).
					Bool("month_match", expiryDate.Month() == referenceDate.Month()).
					Bool("day_match", expiryDate.Day() == referenceDate.Day()).
					Msg("Checking domain of interest")

				// Calculate metrics for d7.se to debug its score
				domainInfo := util.EvaluateDomain(domainName)

				// Extract name part for pattern checking
				domainNameOnly := domainName
				if idx := strings.LastIndex(domainName, "."); idx != -1 {
					domainNameOnly = domainName[:idx]
				}

				log.Info().
					Str("domain", domainName).
					Str("name_part", domainNameOnly).
					Float64("length_score", domainInfo.LengthScore).
					Float64("pronounceability", domainInfo.Pronounceable).
					Float64("total_score", domainInfo.Score).
					Int("length", domainInfo.Length).
					Bool("is_letter_number", util.IsLetterNumberPattern(domainNameOnly)).
					Msg("Score details for domain of interest")
			}

			// Check if domain expires on the reference date
			if expiryDate.Year() == referenceDate.Year() && expiryDate.Month() == referenceDate.Month() && expiryDate.Day() == referenceDate.Day() {
				log.Debug().
					Str("domain", domainName).
					Time("expiry_date", expiryDate).
					Msg("Found domain expiring on reference date")

				// Calculate domain metrics
				domainInfo := util.EvaluateDomain(domainName)
				domainInfo.ExpiryDate = expiryDate
				domains = append(domains, domainInfo)
			}
		}
	}

	return domains
}

// displayTopDomains displays the top domains sorted by score
func displayTopDomains(domains []domain.DomainInfo) {
	// Sort domains by score
	sort.Slice(domains, func(i, j int) bool {
		return domains[i].Score > domains[j].Score
	})

	// Display top domains
	log.Info().Int("total", len(domains)).Msg("Found domains expiring today")
	fmt.Println("\nTop valuable domains expiring today:")
	fmt.Println("======================================")

	// Print header with explanation
	//fmt.Println("Scoring factors:")
	//fmt.Println("- Length: Shorter domains are better (2-3 chars are ideal)")
	//fmt.Println("- Pattern: Letter-only domains (dv) > Letter+Number domains (d7) > Longer domains (dtv)")
	//fmt.Println("- Dashes: Domains with dashes are penalized")
	//fmt.Println("- TLD: Popular TLDs (.com, .net, .org) are preferred")
	//fmt.Println("- Brand: Combination of pronounceability and memorability")
	//fmt.Println("- Keyword: Domains containing valuable keywords get a bonus")
	//fmt.Println()

	// Print column headers
	fmt.Printf("%-4s %-20s %-7s %-7s %-7s %-7s %-7s %-7s %s\n",
		"Rank", "Domain", "Score", "Length", "TLD", "Brand", "Keyword", "Dash", "Type")
	fmt.Println(strings.Repeat("-", 80))

	maxToShow := 100
	if len(domains) < maxToShow {
		maxToShow = len(domains)
	}

	for i := 0; i < maxToShow; i++ {
		d := domains[i]

		// Determine d type
		domainType := "Standard"
		if d.IsLetterOnly && d.Length <= 3 {
			domainType = "Premium (Letter-only)"
		} else if d.IsLetterNumber && d.Length == 2 {
			domainType = "Premium (Letter+Number)"
		} else if d.Length <= 3 {
			domainType = "Premium (Ultra-short)"
		} else if d.Length <= 5 {
			domainType = "Premium (Short)"
		} else if d.HasDash {
			domainType = "Standard (Has Dash)"
		}

		// Format dash penalty for display (show 0 if no penalty)
		dashPenalty := "-"
		if d.HasDash {
			dashPenalty = fmt.Sprintf("%.2f", d.DashPenalty)
		}

		// Format keyword score for display (show - if no keywords)
		keywordScore := "-"
		if d.KeywordScore > 0 {
			keywordScore = fmt.Sprintf("%.2f", d.KeywordScore)
		}

		// Print d with all scoring components
		fmt.Printf("%-4d %-20s %-7.2f %-7.2f %-7.2f %-7.2f %-7s %-7s %s\n",
			i+1,
			d.Name,
			d.Score,
			d.LengthScore,
			d.TLDScore,
			d.BrandabilityScore,
			keywordScore,
			dashPenalty,
			domainType)
	}
}
