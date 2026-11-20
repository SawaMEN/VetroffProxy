package vetroff_test

import (
	"testing"

	vetroff "github.com/SawaMEN/VetroffProxy"
	"github.com/SawaMEN/VetroffProxy/internal/profile"
)

func TestAdaptiveFragmentationSDKLinkRoundTrip(t *testing.T) {
	link, err := profile.Encode(profile.Config{
		Version: 1, Servers: []string{"https://vpn.example.com"},
		Username: "alice", Password: "secret",
		AdaptivePadding: true, AdaptiveFragmentation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := vetroff.ClientConfigFromLink(link)
	if err != nil || !cfg.AdaptivePadding || !cfg.AdaptiveFragmentation {
		t.Fatalf("fragmentation setting lost across SDK import: %+v %v", cfg, err)
	}
}

func TestAdaptiveFragmentationRejectsInvalidSettings(t *testing.T) {
	if _, err := vetroff.NewClient(vetroff.ClientConfig{AdaptiveFragmentation: true}); err == nil {
		t.Fatal("SDK allowed fragmentation without adaptive padding")
	}
	if _, err := vetroff.NewServer(vetroff.ServerConfig{AdaptiveFragmentation: true}); err == nil {
		t.Fatal("server allowed fragmentation without adaptive padding")
	}
	if _, err := profile.Encode(profile.Config{
		Version: 1, Servers: []string{"https://vpn.example.com"},
		Username: "alice", Password: "secret",
		AdaptiveFragmentation: true,
	}); err == nil {
		t.Fatal("link allowed fragmentation without adaptive padding")
	}
}
