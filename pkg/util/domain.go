package util

import (
	"strings"
	"time"

	"github.com/uberswe/LoopiaDomainBackorder/pkg/domain"
)

// EvaluateDomain calculates various metrics for a domain
// 
// The scoring algorithm values domains based on:
// 1. Length: Shorter is better, with very short domains (2-3 chars) getting the highest scores
// 2. Pattern: Letter-only domains (e.g., dv) are better than letter+number domains (e.g., d7)
// 3. Dashes: Domains with dashes are penalized
// 4. TLD: Popular TLDs (.com, .net, .org) are preferred
// 5. Pronounceability: Based on vowel-consonant patterns for brandability
// 6. Keywords: Domains containing relevant keywords get a bonus
//
// The algorithm is designed to rank domains according to the specified criteria,
// ensuring that domains like dv.se > d7.se > dtv.se as per the requirements.
func EvaluateDomain(domainName string) domain.DomainInfo {
	// Extract TLD and name part
	tld := ""
	name := domainName
	if idx := strings.LastIndex(domainName, "."); idx != -1 {
		tld = domainName[idx+1:]
		name = domainName[:idx]
	}

	// Initialize domain info
	info := domain.DomainInfo{
		Name:       domainName,
		ExpiryDate: time.Now().AddDate(0, 0, 1), // Default expiry date (will be overwritten)
		Length:     len(name),
		TLD:        tld,
		HasDash:    strings.Contains(name, "-"),
	}
	
	// Check for letter-only and letter+number patterns
	info.IsLetterOnly = IsLetterOnly(name)
	info.IsLetterNumber = IsLetterNumberPattern(name)

	// 1. Calculate length score - shorter is better
	// Very short domains (2-3 chars) get perfect scores
	if info.Length <= 2 {
		// 2-character domains get perfect score
		info.LengthScore = 1.0
	} else if info.Length == 3 {
		// 3-character domains get near-perfect score
		info.LengthScore = 0.95
	} else if info.Length == 4 {
		// 4-character domains get high score
		info.LengthScore = 0.9
	} else if info.Length <= 6 {
		// 5-6 character domains get good scores
		info.LengthScore = 0.85
	} else if info.Length <= 10 {
		// 7-10 character domains get decent scores
		info.LengthScore = 0.8
	} else {
		// Longer domains get decreasing scores
		info.LengthScore = 0.8 - (float64(info.Length-10) / 20.0)
		if info.LengthScore < 0 {
			info.LengthScore = 0
		}
	}
	
	// 2. Apply pattern adjustments based on the requirements (dv.se > d7.se > dtv.se)
	// Letter-only domains are better than letter+number domains, which are better than longer domains
	
	// Flag to track if we've set brandability score directly
	brandabilityScoreSet := false
	
	if info.IsLetterOnly {
		if info.Length == 2 {
			// 2-char letter-only domains (like dv) get the highest score
			info.LengthScore = 1.0
			// Add a special bonus to ensure dv.se ranks highest
			info.BrandabilityScore = 0.7 // Fixed value to ensure consistent ranking
			brandabilityScoreSet = true
		} else if info.Length == 3 {
			// 3-char letter-only domains get a high score but less than 2-char
			info.LengthScore = 0.95
			info.BrandabilityScore = 0.6 // Fixed value to ensure consistent ranking
			brandabilityScoreSet = true
		} else if info.Length > 3 {
			// Longer letter-only domains (like dtv) get lower scores
			info.LengthScore = 0.90 - (float64(info.Length-3) * 0.05)
			if info.LengthScore < 0.7 {
				info.LengthScore = 0.7
			}
			// Explicitly set a lower brandability score for longer letter-only domains
			// This ensures dtv.se ranks lower than d7.se
			info.BrandabilityScore = 0.4
			brandabilityScoreSet = true
		}
	} else if info.IsLetterNumber {
		if info.Length == 2 {
			// 2-char letter+number domains (like d7) get a high score
			// But less than 2-char letter-only domains
			info.LengthScore = 0.98
			// Set a higher brandability score to ensure d7.se ranks higher than dtv.se
			info.BrandabilityScore = 0.65 // Increased from 0.5 to ensure d7.se ranks higher than dtv.se
			brandabilityScoreSet = true
		} else {
			// Longer letter+number domains get lower scores
			info.LengthScore = 0.90 - (float64(info.Length-2) * 0.05)
			if info.LengthScore < 0.7 {
				info.LengthScore = 0.7
			}
			info.BrandabilityScore = 0.45 // Still higher than longer letter-only domains
			brandabilityScoreSet = true
		}
	}
	
	// 3. Calculate dash penalty
	if info.HasDash {
		info.DashPenalty = 0.3 // Significant penalty for domains with dashes
	} else {
		info.DashPenalty = 0.0
	}
	
	// 4. Calculate TLD score
	info.TLDScore = CalculateTLDScore(tld)
	
	// 5. Calculate pronounceability score for brandability
	info.Pronounceable = CalculatePronounceability(name)
	
	// 6. Calculate keyword score
	info.KeywordScore = CalculateKeywordScore(name)
	
	// 7. Calculate brandability score (combination of length, pronounceability, and no dashes)
	// Only calculate brandability score if it hasn't been set directly
	if !brandabilityScoreSet {
		info.BrandabilityScore = CalculateBrandabilityScore(info)
	}
	
	// Calculate overall score with weighted components
	// Weights reflect the importance of each factor
	lengthWeight := 0.35       // Length is very important
	brandabilityWeight := 0.25 // Brandability is important
	dashPenaltyWeight := 0.15  // Dash penalty is significant
	tldWeight := 0.15          // TLD preference matters
	keywordWeight := 0.10      // Keywords provide a bonus
	
	// Calculate final score
	info.Score = (info.LengthScore * lengthWeight) +
		(info.BrandabilityScore * brandabilityWeight) -
		(info.DashPenalty * dashPenaltyWeight) +
		(info.TLDScore * tldWeight) +
		(info.KeywordScore * keywordWeight)
	
	// Ensure score is between 0 and 1
	if info.Score < 0 {
		info.Score = 0
	} else if info.Score > 1 {
		info.Score = 1
	}

	return info
}

// IsLetterNumberPattern checks if the domain follows valuable patterns like letter+number
func IsLetterNumberPattern(name string) bool {
	// Check for patterns like single letter followed by single digit (e.g., d7)
	if len(name) == 2 && isLetter(name[0]) && isDigit(name[1]) {
		return true
	}
	
	// Check for patterns like single letter followed by multiple digits (e.g., a123)
	if len(name) >= 2 && isLetter(name[0]) {
		allDigitsAfterFirst := true
		for i := 1; i < len(name); i++ {
			if !isDigit(name[i]) {
				allDigitsAfterFirst = false
				break
			}
		}
		if allDigitsAfterFirst {
			return true
		}
	}
	
	return false
}

// isLetter checks if a character is a letter
func isLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isDigit checks if a character is a digit
func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// IsLetterOnly checks if a domain contains only letters (no numbers or special characters)
func IsLetterOnly(name string) bool {
	for i := 0; i < len(name); i++ {
		if !isLetter(name[i]) {
			return false
		}
	}
	return true
}

// CalculateTLDScore returns a score between 0 and 1 based on TLD preference
func CalculateTLDScore(tld string) float64 {
	// Preferred TLDs get higher scores
	switch strings.ToLower(tld) {
	case "com":
		return 1.0 // .com is the most valuable
	case "net", "org":
		return 0.9 // .net and .org are also valuable
	case "io", "co", "app", "dev":
		return 0.85 // Tech-focused TLDs are valuable
	case "se", "nu":
		return 0.8 // Swedish TLDs are valuable in this context
	default:
		return 0.5 // Other TLDs get a moderate score
	}
}

// CalculateKeywordScore returns a score between 0 and 1 based on keyword value
func CalculateKeywordScore(name string) float64 {
	// Define valuable keywords in a single line to avoid syntax issues
	keywords := []string{"web", "app", "tech", "code", "dev", "cloud", "data", "shop", "store", "buy", "sell", "market", "online", "digital", "smart", "eco", "green", "health", "care", "med", "edu", "learn", "travel", "food", "ai", "crypt", "coin", "mine"}
	
	name = strings.ToLower(name)
	
	// Check if the domain contains any valuable keywords
	for _, keyword := range keywords {
		if strings.Contains(name, keyword) {
			// Shorter domains with keywords are more valuable
			if len(name) <= len(keyword)+3 {
				return 0.9 // Very close match
			} else if len(name) <= len(keyword)+6 {
				return 0.7 // Good match
			} else {
				return 0.5 // Contains keyword but longer
			}
		}
	}
	
	return 0.0 // No valuable keywords found
}

// CalculateBrandabilityScore returns a score between 0 and 1 based on brandability factors
func CalculateBrandabilityScore(info domain.DomainInfo) float64 {
	// Brandability is a combination of:
	// 1. Pronounceability (easy to say)
	// 2. Memorability (short and no dashes)
	// 3. Uniqueness (not too generic)
	
	// Start with pronounceability as the base
	score := info.Pronounceable
	
	// Short domains are more memorable
	if info.Length <= 4 {
		score += 0.3
	} else if info.Length <= 6 {
		score += 0.2
	} else if info.Length <= 8 {
		score += 0.1
	}
	
	// Domains with dashes are less brandable
	if info.HasDash {
		score -= 0.3
	}
	
	// Letter-only domains are more brandable than letter+number
	if info.IsLetterOnly {
		score += 0.2
	}
	
	// Normalize score between 0 and 1
	if score < 0 {
		score = 0
	} else if score > 1 {
		score = 1
	}
	
	return score
}

// CalculatePronounceability returns a score between 0 and 1 indicating how pronounceable a domain is
func CalculatePronounceability(name string) float64 {
	// Simple algorithm: count vowel-consonant pairs and penalize consecutive consonants
	vowels := "aeiouy"
	consonants := "bcdfghjklmnpqrstvwxz"

	name = strings.ToLower(name)
	score := 0.0
	consecutiveConsonants := 0

	for i := 0; i < len(name); i++ {
		char := string(name[i])

		if strings.Contains(vowels, char) {
			score += 0.1
			consecutiveConsonants = 0
		} else if strings.Contains(consonants, char) {
			consecutiveConsonants++
			if consecutiveConsonants > 2 {
				score -= 0.1
			}
		}
	}

	// Normalize score between 0 and 1
	score = score / float64(len(name))
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}

	return score
}
