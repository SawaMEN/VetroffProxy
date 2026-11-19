// Package securitymode validates security carriers before any network I/O.
// Unsupported wire protocols may never downgrade silently to TLS.
package securitymode

import "errors"

const TLS = "tls"

var (
	ErrUnknown = errors.New("unknown security mode (choose tls)")
	ErrRealityUnavailable = errors.New("REALITY is not implemented; automatic TLS fallback is prohibited")
	ErrFakeTLSUnavailable = errors.New("FakeTLS is not implemented; choose an explicit interoperable variant first")
)

// Validate preserves existing TLS profiles. A synthetic TLS fingerprint
// does not implement REALITY, ShadowTLS or MTProto FakeTLS.
func Validate(mode string) error {
	switch mode {
	case "", TLS:
		return nil
	case "reality":
		return ErrRealityUnavailable
	case "faketls":
		return ErrFakeTLSUnavailable
	default:
		return ErrUnknown
	}
}
