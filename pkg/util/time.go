// Package util provides utility functions for the application
package util

import (
	"context"
	"github.com/go-vgo/robotgo"
	"github.com/rs/zerolog/log"
	"math/rand"
	"time"
)

const (
	// DropHourUTC is when .se/.nu domains are dropped (04:00 UTC)
	DropHourUTC = 4
	// TomorrowCutoffHourUTC is when the day changes for the available command (06:00 UTC)
	TomorrowCutoffHourUTC = 6
	// KeepAwakeInterval is the interval for mouse movement to keep computer awake
	KeepAwakeInterval = 1 * time.Minute
	// TimeRecheckInterval is the interval to recheck time while waiting for drop time
	TimeRecheckInterval = 10 * time.Minute
)

// NextDrop returns the next date at 04:00 UTC strictly after now.
func NextDrop(now time.Time) time.Time {
	utc := now.UTC()
	drop := time.Date(utc.Year(), utc.Month(), utc.Day(), DropHourUTC, 0, 0, 0, time.UTC)
	if !utc.Before(drop) {
		drop = drop.Add(24 * time.Hour)
	}
	return drop
}

// GetReferenceDate returns the reference date for determining which domains are expiring today.
// If the current UTC time is after the drop time (4 AM UTC), it returns the next day's date
// to show domains for the next release date.
//
// IMPORTANT: Always pass local time (time.Now()) to this function, not UTC time (time.Now().UTC()).
// Using UTC time would cause the function to use the wrong date if the local time is in a different day
// than the UTC time (e.g., at 00:38 CEST, which is 22:38 UTC of the previous day).
func GetReferenceDate(now time.Time) time.Time {
	// Convert to UTC to check against drop time
	utcNow := now.UTC()

	// Get the current local date at midnight UTC
	localDate := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// Check if current UTC time is after today's drop time (4 AM UTC)
	todayDropTime := time.Date(utcNow.Year(), utcNow.Month(), utcNow.Day(), DropHourUTC, 0, 0, 0, time.UTC)

	// If current time is after today's drop time, return tomorrow's date
	if utcNow.After(todayDropTime) {
		return localDate.Add(24 * time.Hour)
	}

	return localDate
}

// KeepAwake keeps the computer awake by simulating mouse movement every minute.
func KeepAwake(ctx context.Context) {
	ticker := time.NewTicker(KeepAwakeInterval)
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
			dy := rand.Intn(20) - 10 // Random value between -10 and 10
			robotgo.MoveSmooth(x+dx, y+dy)
		}
	}
}
