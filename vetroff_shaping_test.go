package vetroff_test

import (
	"testing"

	vetroff "github.com/SawaMEN/VetroffProxy"
	"github.com/SawaMEN/VetroffProxy/internal/profile"
)

func TestTrafficShapingSDKProfileRoundTrip(t *testing.T) {
	link, err := profile.Encode(profile.Config{
		Version: 1, Servers: []string{"https://example.com"},
		Username: "alice", Password: "secret",
		AdaptivePadding: true, TrafficShaping: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := vetroff.ClientConfigFromLink(link)
	if err != nil || !cfg.AdaptivePadding || !cfg.TrafficShaping {
		t.Fatalf("SDK link lost shaping: %+v %v", cfg, err)
	}
}

func TestTrafficShapingRequiresAdaptivePaddingAcrossSDK(t *testing.T) {
	if _, err := vetroff.NewClient(vetroff.ClientConfig{TrafficShaping: true}); err == nil {
		t.Fatal("client accepted incompatible shaping mode")
	}
	if _, err := vetroff.NewServer(vetroff.ServerConfig{TrafficShaping: true}); err == nil {
		t.Fatal("server accepted incompatible shaping mode")
	}
	if _, err := profile.Encode(profile.Config{
		Version: 1, Servers: []string{"https://example.com"},
		Username: "alice", Password: "secret",
		TrafficShaping: true,
	}); err == nil {
		t.Fatal("share link accepted incompatible shaping mode")
	}
}
