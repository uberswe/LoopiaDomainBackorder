// Package api provides a client for interacting with the Loopia API
package api

import (
	"errors"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"

	"github.com/kolo/xmlrpc"
	"github.com/rs/zerolog/log"
)

const (
	loopiaEndpoint = "https://api.loopia.se/RPCSERV"
)

// Client wraps an xmlrpc.Client and automatically inserts
// username + password as the first two parameters of every call.
type Client struct {
	username string
	password string
	rpc      *xmlrpc.Client
	dryRun   bool // if true, no RPC is executed (timing only)

	// Rate limiting
	callsMutex    sync.Mutex
	callsThisHour int
	hourStartTime time.Time
	stopOn401     bool // if true, stop sending requests on 401 Unauthorized errors
	stopOn429     bool // if true, stop sending requests on 429 Too Many Requests errors
}

// NewClient creates a new Loopia API client
func NewClient(username, password string, dry bool) (*Client, error) {
	jar, _ := cookiejar.New(nil)
	httpClient := &http.Client{Jar: jar, Timeout: 15 * time.Second}

	c, err := xmlrpc.NewClient(loopiaEndpoint, httpClient.Transport)
	if err != nil {
		return nil, err
	}
	return &Client{
		username:      username,
		password:      password,
		rpc:           c,
		dryRun:        dry,
		callsThisHour: 0,
		hourStartTime: time.Now(),
		stopOn401:     false,
		stopOn429:     false,
	}, nil
}

// Call invokes an XML‑RPC method with authentication prepended.
func (c *Client) Call(method string, params ...interface{}) (interface{}, error) {
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
	if c.stopOn401 || c.stopOn429 {
		// We'll check these flags but still allow the call to proceed
		// This way the application can decide what to do with the error
		errorType := ""
		if c.stopOn401 {
			errorType = "401 Unauthorized"
		}
		if c.stopOn429 {
			if errorType != "" {
				errorType += " or "
			}
			errorType += "429 Too Many Requests"
		}
		reqLogger.Warn().
			Str("error_type", errorType).
			Msg("Making API call despite previous error")
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
		errStr := err.Error()
		if errStr == "401 Unauthorized" || errStr == "429 Too Many Requests" {
			c.callsMutex.Lock()
			if errStr == "401 Unauthorized" {
				c.stopOn401 = true
				respLogger.Error().
					Str("error_code", errStr).
					Msg("Received 401 Unauthorized error, stopping further API calls")
			} else if errStr == "429 Too Many Requests" {
				c.stopOn429 = true
				respLogger.Error().
					Str("error_code", errStr).
					Msg("Received 429 Too Many Requests error, stopping further API calls")
			}
			c.callsMutex.Unlock()
		}

		return nil, err
	}

	respLogger.Info().
		Interface("response", reply).
		Msg("API call successful")

	return reply, nil
}

// OrderDomain attempts to order a domain
func (c *Client) OrderDomain(domain string) error {
	// Log the domain order attempt
	log.Info().
		Str("domain", domain).
		Str("operation", "order_domain").
		Time("attempt_time", time.Now()).
		Msg("Attempting to order domain")

	// orderDomain(..., domain, true) – true == pay with credits automatically
	_, err := c.Call("orderDomain", domain, true)

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

// PayInvoiceIfAny checks if there's an invoice for the domain and pays it
func (c *Client) PayInvoiceIfAny(domain string) error {
	log.Info().
		Str("domain", domain).
		Str("operation", "check_invoice").
		Time("check_time", time.Now()).
		Msg("Checking for invoice to pay")

	resp, err := c.Call("getDomain", domain)
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

	_, err = c.Call("payInvoiceUsingCredits", ref)

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

// Attempt tries to register and immediately pay for the domain.
func (c *Client) Attempt(domain string) error {
	attemptStart := time.Now()

	// Check if we should stop due to previous 401 or 429 error
	c.callsMutex.Lock()
	var errMsg string
	if c.stopOn401 && c.stopOn429 {
		errMsg = "Aborting attempt due to previous 401 Unauthorized and 429 Too Many Requests errors"
	} else if c.stopOn401 {
		errMsg = "Aborting attempt due to previous 401 Unauthorized error"
	} else if c.stopOn429 {
		errMsg = "Aborting attempt due to previous 429 Too Many Requests error"
	}

	if errMsg != "" {
		c.callsMutex.Unlock()
		log.Error().
			Str("domain", domain).
			Str("operation", "registration_attempt").
			Bool("stopOn401", c.stopOn401).
			Bool("stopOn429", c.stopOn429).
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
	if err := c.OrderDomain(domain); err != nil {
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
	err := c.PayInvoiceIfAny(domain)
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
