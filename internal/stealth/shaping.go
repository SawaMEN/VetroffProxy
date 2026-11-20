package stealth

import "errors"

// PaddingBudgetDivisor caps additional zero-filled padding to 12.5% of
// cumulative useful application bytes in one sender stream. Mandatory
// three-byte frame headers are not included.
const PaddingBudgetDivisor uint64 = 8

var ErrTrafficShaping = errors.New("traffic-shaping requires adaptive padding")

func ValidateTrafficShaping(enabled, adaptive bool) error {
	if enabled && !adaptive {
		return ErrTrafficShaping
	}
	return nil
}
