// Package domain contains domain-related models and logic
package domain

import "time"

// Config represents the configuration file structure
type Config struct {
	Username      string            `json:"username"`
	Password      string            `json:"password"`
	Domains       []string          `json:"domains"`
	CacheDir      string            `json:"cache_dir"`
	CachedLists   map[string]string `json:"cached_lists"`
	LastCacheTime string            `json:"last_cache_time"`
}

// Result represents the result of a domain registration attempt
type Result struct {
	Domain  string
	Success bool
	Error   error
}

// DomainInfo represents information about a domain
type DomainInfo struct {
	Name             string
	ExpiryDate       time.Time
	Length           int
	TLD              string     // Top-level domain (.com, .se, etc.)
	HasDash          bool       // Whether the domain contains dashes
	IsLetterOnly     bool       // Whether the domain contains only letters
	IsLetterNumber   bool       // Whether the domain follows letter+number pattern
	
	// Scoring components
	LengthScore      float64    // Score based on domain length (0-1)
	DashPenalty      float64    // Penalty for domains with dashes (0-1)
	TLDScore         float64    // Score based on TLD preference (0-1)
	KeywordScore     float64    // Score based on keyword value (0-1)
	Pronounceable    float64    // Score based on pronounceability (0-1)
	BrandabilityScore float64   // Score based on brandability factors (0-1)
	
	Score            float64    // Overall score (weighted combination)
}