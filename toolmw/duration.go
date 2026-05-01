// Package toolmw provides concrete middleware implementations for the
// agentsdk tool system: timeout, risk gating, secret protection, etc.
package toolmw

import (
	"time"

	"github.com/codewandler/agentsdk/actionmw"
)

// ParseDuration parses a human-friendly duration string. Deprecated: use
// actionmw.ParseDuration for surface-neutral action middleware.
func ParseDuration(s string) (time.Duration, error) { return actionmw.ParseDuration(s) }

// parseDuration is kept for package-internal compatibility.
func parseDuration(s string) (time.Duration, error) { return ParseDuration(s) }
