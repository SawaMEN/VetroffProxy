package stealth

import "errors"

const (
	// These bounds are for inner authenticated-HTTPS frames only.
	MinFragmentSize = 1024
	MaxFragmentSize = 4096
)

var ErrAdaptiveFragmentation = errors.New("adaptive-fragmentation requires adaptive padding")

func ValidateFragmentation(enabled, adaptive bool) error {
	if enabled && !adaptive {
		return ErrAdaptiveFragmentation
	}
	return nil
}
