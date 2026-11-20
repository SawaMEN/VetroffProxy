package stealth

import (
	"errors"
	"testing"
)

func TestAdaptiveFragmentationValidation(t *testing.T) {
	for _, tc := range []struct {
		enabled bool
		adaptive bool
		ok bool
	}{
		{false, false, true},
		{false, true, true},
		{true, true, true},
		{true, false, false},
	} {
		err := ValidateFragmentation(tc.enabled, tc.adaptive)
		if (err == nil) != tc.ok {
			t.Fatalf("fragment=%t adaptive=%t: %v", tc.enabled, tc.adaptive, err)
		}
		if err != nil && !errors.Is(err, ErrAdaptiveFragmentation) {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if MinFragmentSize <= 0 || MaxFragmentSize <= MinFragmentSize || MaxFragmentSize > 16384 {
		t.Fatalf("invalid frame bounds: %d..%d", MinFragmentSize, MaxFragmentSize)
	}
}
