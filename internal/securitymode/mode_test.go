package securitymode

import (
	"errors"
	"testing"
)

func TestModeDoesNotDowngrade(t *testing.T) {
	for _, mode := range []string{"", "tls"} {
		if err := Validate(mode); err != nil { t.Errorf("TLS %q rejected: %v", mode, err) }
	}
	for _, tc := range []struct { mode string; want error }{
		{"reality", ErrRealityUnavailable}, {"faketls", ErrFakeTLSUnavailable},
		{"TLS", ErrUnknown}, {"none", ErrUnknown}, {"tls12", ErrUnknown},
	} {
		if err := Validate(tc.mode); !errors.Is(err, tc.want) {
			t.Errorf("%q: got %v, want %v", tc.mode, err, tc.want)
		}
	}
}
