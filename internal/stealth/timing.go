// Package stealth validates opt-in local traffic-shaping policy. It contains no
// wire protocol code and can be reused by CLI, SDK, profile and tunnel layers.
package stealth

import (
	"errors"
	"time"
)

const MaxTimingJitter = 5 * time.Millisecond
const SmallRecordLimit = 256

var ErrTimingJitter = errors.New("timing-jitter must be 0..5ms and requires adaptive padding")

// ValidateTimingJitter rejects unbounded latency and accidental use with the
// older raw-tail framing mode. Zero disables the local feature.
func ValidateTimingJitter(d time.Duration, adaptive bool) error {
	if d < 0 || d > MaxTimingJitter || d > 0 && !adaptive {
		return ErrTimingJitter
	}
	return nil
}

// ParseTimingJitter validates persisted configuration; the empty string is off.
// Serialized profiles omit zero rather than supplying a potentially ambiguous 0s.
func ParseTimingJitter(value string, adaptive bool) (time.Duration, error) {
	if value == "" {
		return 0, nil
	}
	d, err := time.ParseDuration(value)
	if err != nil || d == 0 {
		return 0, ErrTimingJitter
	}
	if err := ValidateTimingJitter(d, adaptive); err != nil {
		return 0, err
	}
	return d, nil
}

func FormatTimingJitter(d time.Duration) string {
	if d == 0 {
		return ""
	}
	return d.String()
}
