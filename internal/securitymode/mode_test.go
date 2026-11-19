package securitymode

import (
	"errors"
	"testing"
)

func TestSecurityModeDoesNotDowngrade(t *testing.T) {
	for _, mode := range []string{"", "tls"} {
		if err := Validate(mode); err != nil {
			t.Errorf("TLS %q rejected: %v", mode, err)
		}
	}
	for _, tc := range []struct {
		mode string
		want error
	}{
		{"shadowtls-v3", ErrShadowTLSExternal},
		{"TLS", ErrUnknown},
		{"none", ErrUnknown},
		{"tls12", ErrUnknown},
	} {
		if err := Validate(tc.mode); !errors.Is(err, tc.want) {
			t.Errorf("%q: got %v, want %v", tc.mode, err, tc.want)
		}
	}
}
