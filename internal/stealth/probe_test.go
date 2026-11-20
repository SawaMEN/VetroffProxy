package stealth

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestProbeKeyAndTimeBoundProof(t *testing.T) {
	key, err := GenerateProbeKey()
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateProbeKey(key, true); err != nil {
		t.Fatal(err)
	}
	if err := ValidateProbeKey("", false); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(ValidateProbeKey(key, false), ErrProbeKey) {
		t.Fatal("non-strict probe guard accepted")
	}
	if !errors.Is(ValidateProbeKey(base64.RawURLEncoding.EncodeToString(make([]byte, 16)), true), ErrProbeKey) {
		t.Fatal("short probe key accepted")
	}
	now := time.Now()
	proof, err := NewProbeProof(key)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(proof, key) {
		t.Fatal("probe proof disclosed raw secret")
	}
	stamp, ok := VerifyProbeProof(key, proof, now)
	if !ok || stamp == 0 {
		t.Fatal("generated proof failed to verify")
	}
	if _, ok := VerifyProbeProof(key, proof, now.Add(2*ProbeWindow)); ok {
		t.Fatal("expired probe proof was accepted")
	}
	if _, ok := VerifyProbeProof(key, proof, now.Add(-2*ProbeWindow)); ok {
		t.Fatal("future-dated probe proof was accepted")
	}
	otherKey, _ := GenerateProbeKey()
	if _, ok := VerifyProbeProof(otherKey, proof, now); ok {
		t.Fatal("proof accepted with unrelated key")
	}
	for _, malformed := range []string{"", "0", proof+".x", proof[:len(proof)-1]+"x", strings.Replace(proof, ".", "..", 1)} {
		if _, ok := VerifyProbeProof(key, malformed, now); ok {
			t.Fatalf("malformed proof accepted: %q", malformed)
		}
	}
	seen := map[string]bool{proof: true}
	for i := 0; i < 128; i++ {
		another, err := NewProbeProof(key)
		if err != nil {
			t.Fatal(err)
		}
		if seen[another] {
			t.Fatal("probe nonce reused")
		}
		seen[another] = true
		if _, ok := VerifyProbeProof(key, another, time.Now()); !ok {
			t.Fatal("fresh nonce failed to verify")
		}
	}
}
