// Package securitymode keeps the inner Vetroff HTTPS TLS contract strict.
// ShadowTLS v3 is a separate outer TCP carrier, configured via shadowtls-config.
package securitymode

import "errors"

const TLS = "tls"

var (
	ErrUnknown = errors.New("unknown Vetroff inner security mode (choose tls)")
	ErrShadowTLSExternal = errors.New("ShadowTLS v3 is an outer TCP carrier; use shadowtls-config to configure the official implementation")
)

// Validate never silently replaces an unsupported inner security protocol with TLS.
func Validate(mode string) error {
	switch mode {
	case "", TLS:
		return nil
	case "shadowtls-v3":
		return ErrShadowTLSExternal
	default:
		return ErrUnknown
	}
}
