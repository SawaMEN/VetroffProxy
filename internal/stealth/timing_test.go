package stealth

import (
	"errors"
	"testing"
	"time"
)

func TestTimingJitterValidation(t *testing.T) {
	for _, tc := range []struct {
		d        time.Duration
		adaptive bool
		ok       bool
	}{
		{0, false, true},
		{0, true, true},
		{time.Millisecond, true, true},
		{5 * time.Millisecond, true, true},
		{time.Millisecond, false, false},
		{-time.Nanosecond, true, false},
		{5*time.Millisecond + time.Nanosecond, true, false},
	} {
		err := ValidateTimingJitter(tc.d, tc.adaptive)
		if (err == nil) != tc.ok {
			t.Fatalf("jitter=%s adaptive=%t: %v", tc.d, tc.adaptive, err)
		}
		if err != nil && !errors.Is(err, ErrTimingJitter) {
			t.Fatal("unexpected validation error", err)
		}
	}
}

func TestTimingJitterParsing(t *testing.T) {
	for _, tc := range []struct {
		value    string
		adaptive bool
		ok       bool
	}{
		{"", false, true},
		{"", true, true},
		{"1ms", true, true},
		{"5ms", true, true},
		{"0s", true, false},
		{"6ms", true, false},
		{"1ms", false, false},
		{"not-a-duration", true, false},
		{"-1ms", true, false},
	} {
		d, err := ParseTimingJitter(tc.value, tc.adaptive)
		if (err == nil) != tc.ok {
			t.Fatalf("value=%q adaptive=%t: %v", tc.value, tc.adaptive, err)
		}
		if err == nil && FormatTimingJitter(d) != tc.value {
			t.Fatalf("roundtrip %q -> %q", tc.value, FormatTimingJitter(d))
		}
	}
}
