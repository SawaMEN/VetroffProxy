package stealth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
)

const (
	ProbeHeader = "X-Vetroff-Proof"
	ProbeAcceptedHeader = "X-Vetroff-Proof-Accepted"
	ProbeWindow = 30 * time.Second
	MaxProbeCache = 4096
	probeDomain = "VetroffProxy/probe-v1:"
)

var ErrProbeKey = errors.New("probe key requires strict protocol negotiation and 32 random bytes")

// The separate probe key is not a password or a replacement for HTTPS and
// authentication. It is generated once and must be kept out of public covers.
func GenerateProbeKey() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b[:]), nil
}

func keyBytes(value string) ([]byte, bool) {
	if len(value) != 43 {
		return nil, false
	}
	b, err := base64.RawURLEncoding.DecodeString(value)
	return b, err == nil && len(b) == 32 && base64.RawURLEncoding.EncodeToString(b) == value
}

func ValidateProbeKey(value string, strict bool) error {
	if value == "" {
		return nil
	}
	if _, ok := keyBytes(value); !ok || !strict {
		return ErrProbeKey
	}
	return nil
}

func proofMAC(key []byte, stamp, nonce string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(probeDomain))
	mac.Write([]byte(stamp))
	mac.Write([]byte{'.'})
	mac.Write([]byte(nonce))
	return mac.Sum(nil)
}

// A fresh nonce and wall-clock timestamp produce one bounded authenticated
// proof for an already-TLS-protected proxy request. No alternative transport
// or response downgrade is attempted on failure.
func NewProbeProof(key string) (string, error) {
	b, ok := keyBytes(key)
	if !ok {
		return "", ErrProbeKey
	}
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	token := base64.RawURLEncoding.EncodeToString(nonce[:])
	signature := base64.RawURLEncoding.EncodeToString(proofMAC(b, stamp, token))
	return stamp + "." + token + "." + signature, nil
}

// ProbeConfirmation authenticates server acceptance of one precise request.
func ProbeConfirmation(key, proof string) (string, error) {
	b, ok := keyBytes(key)
	if !ok { return "", ErrProbeKey }
	mac := hmac.New(sha256.New, b)
	mac.Write([]byte("VetroffProxy/probe-accepted-v1:"))
	mac.Write([]byte(proof))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func VerifyProbeConfirmation(key, proof, received string) bool {
	if len(received) != 43 { return false }
	expected, err := ProbeConfirmation(key, proof)
	return err == nil && hmac.Equal([]byte(expected), []byte(received))
}

func VerifyProbeProof(key, proof string, now time.Time) (int64, bool) {
	b, ok := keyBytes(key)
	if !ok || len(proof) > 90 || len(proof) < 65 {
		return 0, false
	}
	pieces := strings.Split(proof, ".")
	if len(pieces) != 3 || len(pieces[1]) != 16 || len(pieces[2]) != 43 {
		return 0, false
	}
	stamp, err := strconv.ParseInt(pieces[0], 10, 64)
	if err != nil || strconv.FormatInt(stamp, 10) != pieces[0] ||
		stamp < now.Unix()-int64(ProbeWindow/time.Second) ||
		stamp > now.Unix()+int64(ProbeWindow/time.Second) {
		return 0, false
	}
	nonce, err := base64.RawURLEncoding.DecodeString(pieces[1])
	if err != nil || len(nonce) != 12 || base64.RawURLEncoding.EncodeToString(nonce) != pieces[1] {
		return 0, false
	}
	sig, err := base64.RawURLEncoding.DecodeString(pieces[2])
	if err != nil || len(sig) != sha256.Size ||
		!hmac.Equal(sig, proofMAC(b, pieces[0], pieces[1])) {
		return 0, false
	}
	return stamp, true
}
