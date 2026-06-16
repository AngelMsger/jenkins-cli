// Package timeutil renders the epoch-millisecond timestamps Jenkins returns
// into the forms a coding agent can use directly: an absolute RFC3339 instant
// and a compact relative phrase ("3h ago"). Hand-converting Jenkins' millis is
// exactly the kind of footgun the CLI should absorb, not the agent.
package timeutil

import (
	"fmt"
	"time"
)

// FromMillis converts a Jenkins epoch-millisecond timestamp to a UTC time.
func FromMillis(ms int64) time.Time { return time.UnixMilli(ms).UTC() }

// HumanSince renders how long ago t was, relative to now, as a compact phrase:
// "just now", "5m ago", "3h ago", "2d ago" (or "… from now" for future times).
func HumanSince(t time.Time) string {
	return humanSinceAt(t, time.Now())
}

// humanSinceAt is HumanSince with an injectable "now" for deterministic tests.
func humanSinceAt(t, now time.Time) string {
	d := now.Sub(t)
	future := false
	if d < 0 {
		future = true
		d = -d
	}
	if d < time.Minute {
		if future {
			return "in a moment"
		}
		return "just now"
	}
	phrase := compactDuration(d)
	if future {
		return "in " + phrase
	}
	return phrase + " ago"
}

// compactDuration renders a duration to a single dominant unit (m, h or d).
func compactDuration(d time.Duration) string {
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
