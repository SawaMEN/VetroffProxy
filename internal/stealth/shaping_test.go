package stealth

import (
	"errors"
	"testing"
)

func TestValidateTrafficShaping(t *testing.T) {
	for _, tc := range []struct {
		enabled, adaptive, ok bool
	}{
		{false, false, true},
		{false, true, true},
		{true, true, true},
		{true, false, false},
	} {
		err := ValidateTrafficShaping(tc.enabled, tc.adaptive)
		if (err == nil) != tc.ok {
			t.Fatalf("enabled=%t adaptive=%t: %v", tc.enabled, tc.adaptive, err)
		}
		if err != nil && !errors.Is(err, ErrTrafficShaping) {
			t.Fatal(err)
		}
	}
	if PaddingBudgetDivisor != 8 {
		t.Fatal("traffic budget contract changed")
	}
}
