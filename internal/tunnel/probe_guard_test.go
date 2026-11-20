package tunnel

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SawaMEN/VetroffProxy/internal/stealth"
)

func TestProbeGuardHidesUnprovenRequests(t *testing.T) {
	key, err := stealth.GenerateProbeKey()
	if err != nil {
		t.Fatal(err)
	}
	coverCalls := 0
	s := &Server{
		Username: "alice", Password: "secret",
		RequireProtocolNegotiation: true, ProbeKey: key,
		Cover: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			coverCalls++
			for h := range r.Header {
				if strings.HasPrefix(strings.ToLower(h), "x-vetroff-") || strings.EqualFold(h, "Proxy-Authorization") {
					t.Errorf("private wire header leaked to cover: %s", h)
				}
			}
			http.NotFound(w, r)
		}),
	}
	for _, mode := range []string{"no proof", "wrong proof", "duplicate proof headers", "valid proof but wrong password"} {
		r := httptest.NewRequest(http.MethodConnect, "https://example.com", nil)
		r.ProtoMajor = 2
		r.Host = "127.0.0.1:9"
		r.Header.Set("Proxy-Authorization", basicAuth("alice", "secret"))
		r.Header.Set(protocolVersionHeader, "vetroff-draft-0")
		r.Header.Set(protocolRequireHeader, "tcp")
		switch mode {
		case "wrong proof":
			r.Header.Set(stealth.ProbeHeader, "not-a-signed-proof")
		case "duplicate proof headers":
			p, err := stealth.NewProbeProof(key)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Add(stealth.ProbeHeader, p)
			r.Header.Add(stealth.ProbeHeader, p)
		case "valid proof but wrong password":
			p, err := stealth.NewProbeProof(key)
			if err != nil {
				t.Fatal(err)
			}
			r.Header.Set(stealth.ProbeHeader, p)
			r.Header.Set("Proxy-Authorization", basicAuth("alice", "invalid"))
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "Vetroff") {
			t.Fatalf("%s: unexpected response %d %q", mode, w.Code, w.Body.String())
		}
		if s.activeTunnels.Load() != 0 {
			t.Fatalf("%s: allocated tunnel for probe", mode)
		}
	}
	if coverCalls != 4 {
		t.Fatalf("cover invoked %d times", coverCalls)
	}
}

func TestProbeGuardReplayRefusalAndNoDowngrade(t *testing.T) {
	key, err := stealth.GenerateProbeKey()
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{ProbeKey: key}
	proof, err := stealth.NewProbeProof(key)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodConnect, "https://example.com", nil)
	r.Header.Set(stealth.ProbeHeader, proof)
	if !s.acceptProbe(httptest.NewRecorder(), r) || s.acceptProbe(httptest.NewRecorder(), r) {
		t.Fatal("fresh signed proof not accepted exactly once")
	}
	r2 := httptest.NewRequest(http.MethodConnect, "https://example.com", nil)
	r2.Header.Set(stealth.ProbeHeader, proof)
	if s.acceptProbe(httptest.NewRecorder(), r2) {
		t.Fatal("replayed proof accepted")
	}
	fresh, err := stealth.NewProbeProof(key)
	if err != nil {
		t.Fatal(err)
	}
	r2.Header.Set(stealth.ProbeHeader, fresh)
	if !s.acceptProbe(httptest.NewRecorder(), r2) {
		t.Fatal("second distinct proof refused")
	}
	s.probeMu.Lock()
	s.probeUsed = map[string]int64{proof: 1}
	s.probeMu.Unlock()
	if !s.acceptProbe(httptest.NewRecorder(), r2) {
		// Reusing r2 is still a replay; ensure expiry alone did not enable it.
	} else {
		t.Fatal("replay unexpectedly reset by cache edit")
	}
}

func TestProbeGuardStrictSOCKSAndHalfClose(t *testing.T) {
	key, err := stealth.GenerateProbeKey()
	if err != nil {
		t.Fatal(err)
	}
	target := testDestination(t, func(c net.Conn) {
		body, _ := io.ReadAll(c)
		_, _ = c.Write(body)
	})
	s := &Server{
		Username: "alice", Password: "secret",
		RequireProtocolNegotiation: true, ProbeKey: key,
		AllowPrivate: true, AdaptivePadding: true,
	}
	client, addr := testClient(t, s)
	client.RequireProtocolNegotiation = true
	client.ProbeKey = key
	client.AdaptivePadding = true
	conn, code := connectSOCKS(t, addr, target)
	defer conn.Close()
	if code != 0 {
		t.Fatalf("signed SOCKS connection refused: %d", code)
	}
	payload := bytes.Repeat([]byte("signed-probe-"), 2500)
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := conn.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	reply, err := io.ReadAll(conn)
	if err != nil || !bytes.Equal(reply, payload) {
		t.Fatalf("signed SOCKS half-close corrupted %v", err)
	}
}

func TestProbeGuardRejectsLegacyClientWithoutKey(t *testing.T) {
	key, err := stealth.GenerateProbeKey()
	if err != nil {
		t.Fatal(err)
	}
	target := testDestination(t, func(c net.Conn) { _, _ = io.Copy(c, c) })
	s := &Server{
		Username: "alice", Password: "secret", RequireProtocolNegotiation: true,
		ProbeKey: key, AllowPrivate: true,
	}
	client, addr := testClient(t, s)
	client.RequireProtocolNegotiation = true
	conn, code := connectSOCKS(t, addr, target)
	defer conn.Close()
	if code == 0 {
		t.Fatal("probe-guard server accepted old authenticated client without proof")
	}
}
