package tunnel

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/SawaMEN/VetroffProxy/internal/stealth"
)

func TestProbeGuardCacheFullFailsClosed(t *testing.T) {
	key, err := stealth.GenerateProbeKey()
	if err != nil {
		t.Fatal(err)
	}
	proof, err := stealth.NewProbeProof(key)
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{ProbeKey: key, probeUsed: make(map[string]int64)}
	expiry := time.Now().Add(time.Minute).Unix()
	for i := 0; i < stealth.MaxProbeCache; i++ {
		s.probeUsed[strconv.Itoa(i)] = expiry
	}
	r := httptest.NewRequest(http.MethodConnect, "https://example.com", nil)
	r.Header.Set(stealth.ProbeHeader, proof)
	if s.acceptProbe(httptest.NewRecorder(), r) {
		t.Fatal("full cache evicted live replay entries instead of failing closed")
	}
	for token := range s.probeUsed {
		s.probeUsed[token] = 1
	}
	if !s.acceptProbe(httptest.NewRecorder(), r) {
		t.Fatal("expired replay entries prevented legitimate fresh proof")
	}
	if len(s.probeUsed) != 1 {
		t.Fatalf("expired cache entries retained: %d", len(s.probeUsed))
	}
	if s.acceptProbe(httptest.NewRecorder(), r) {
		t.Fatal("successful proof accepted twice after cleanup")
	}
}

func TestProbeGuardManagedAccessPreflightUsesOrdinaryCover(t *testing.T) {
	key, err := stealth.GenerateProbeKey()
	if err != nil {
		t.Fatal(err)
	}
	controller := accessController(t)
	s := &Server{Access: controller, ProbeKey: key, RequireProtocolNegotiation: true}
	for _, mode := range []string{"no proof", "invalid proof"} {
		r := httptest.NewRequest(http.MethodPost, "https://cover.example"+udpPath, nil)
		r.ProtoMajor = 2
		r.Header.Set("Proxy-Authorization", basicAuth("alice", "secret"))
		if mode == "invalid proof" {
			r.Header.Set(stealth.ProbeHeader, "invalid")
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound {
			t.Fatalf("%s: expected ordinary cover 404, got %d", mode, w.Code)
		}
		if s.activeTunnels.Load() != 0 || s.activeCoverRequests.Load() != 0 {
			t.Fatal("unproven managed request retained admission slots")
		}
	}
}
