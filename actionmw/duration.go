// Package actionmw provides reusable middleware for action.Action execution.
package actionmw

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ParseDuration parses a human-friendly duration string.
// It extends time.ParseDuration with support for:
//   - "min" / "mins" as minutes
//   - "sec" / "secs" as seconds
//   - "hr" / "hrs" as hours
//   - bare integers/floats treated as seconds
//   - compound forms like "2m30s", "1h30m"
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty duration string")
	}
	if d, err := time.ParseDuration(s); err == nil {
		return d, nil
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		if n < 0 {
			return 0, fmt.Errorf("negative duration %q", s)
		}
		return time.Duration(n * float64(time.Second)), nil
	}
	lower := strings.ToLower(s)
	if d, ok := parseHumanSuffix(lower); ok {
		return d, nil
	}
	return 0, fmt.Errorf("invalid duration %q", s)
}

var humanDurationPattern = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(sec|secs|second|seconds|min|mins|minute|minutes|hr|hrs|hour|hours)$`)

func parseHumanSuffix(s string) (time.Duration, bool) {
	m := humanDurationPattern.FindStringSubmatch(s)
	if m == nil {
		return 0, false
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, false
	}
	var unit time.Duration
	switch m[2] {
	case "sec", "secs", "second", "seconds":
		unit = time.Second
	case "min", "mins", "minute", "minutes":
		unit = time.Minute
	case "hr", "hrs", "hour", "hours":
		unit = time.Hour
	default:
		return 0, false
	}
	return time.Duration(n * float64(unit)), true
}
