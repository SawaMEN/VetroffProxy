package vetroff_test

import (
	"testing"
	"time"

	vetroff "github.com/SawaMEN/VetroffProxy"
	"github.com/SawaMEN/VetroffProxy/internal/profile"
)

func TestAdaptivePaddingSharedProfilePreservedBySDK(t *testing.T) {
	link, err := profile.Encode(profile.Config{
		Version:  1,
		Servers:  []string{"https://edge.example.com"},
		Username: "alice", Password: "secret",
		AdaptivePadding: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := vetroff.ClientConfigFromLink(link)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.AdaptivePadding || parsed.Padding {
		t.Fatalf("adaptive padding was lost on SDK import: %#v", parsed)
	}
}

func TestAdaptivePaddingRejectsConflictingClientModes(t *testing.T) {
	_, err := vetroff.NewClient(vetroff.ClientConfig{Padding: true, AdaptivePadding: true})
	if err == nil {
		t.Fatal("client accepted two incompatible padding modes")
	}
}

func TestAdaptivePaddingRejectsBondProfile(t *testing.T) {
	_, err := profile.Encode(profile.Config{
		Version:  1,
		Servers:  []string{"https://a.example.com", "https://b.example.com"},
		Username: "alice", Password: "secret",
		AdaptivePadding: true,
		Bond:            true,
	})
	if err == nil {
		t.Fatal("bond profile accepted unsupported adaptive padding")
	}
}

func TestTimingJitterLinkToSDK(t *testing.T) {
	link, err := profile.Encode(profile.Config{
		Version:         1,
		Servers:         []string{"https://example.com"},
		Username:        "alice",
		Password:        "secret",
		AdaptivePadding: true,
		TimingJitter:    "3ms",
	})
	if err != nil {
		t.Fatal(err)
	}
	config, err := vetroff.ClientConfigFromLink(link)
	if err != nil || !config.AdaptivePadding || config.TimingJitter != 3*time.Millisecond {
		t.Fatalf("lost timing settings: %+v %v", config, err)
	}
}

func TestTimingJitterSDKRejectsInvalidSettings(t *testing.T) {
	for _, cfg := range []vetroff.ClientConfig{
		{TimingJitter: time.Millisecond},
		{AdaptivePadding: true, TimingJitter: -time.Millisecond},
		{AdaptivePadding: true, TimingJitter: 6 * time.Millisecond},
	} {
		if _, err := vetroff.NewClient(cfg); err == nil {
			t.Fatal("client accepted invalid timing options", cfg)
		}
	}
	for _, cfg := range []vetroff.ServerConfig{
		{TimingJitter: time.Millisecond},
		{AdaptivePadding: true, TimingJitter: 6 * time.Millisecond},
	} {
		if _, err := vetroff.NewServer(cfg); err == nil {
			t.Fatal("server accepted invalid timing options", cfg)
		}
	}
}

func TestTimingJitterProfileRejectsInvalidConfigurations(t *testing.T) {
	for _, config := range []profile.Config{
		{TimingJitter: "1ms"},
		{AdaptivePadding: true, TimingJitter: "6ms"},
		{AdaptivePadding: true, TimingJitter: "-1ms"},
		{AdaptivePadding: true, TimingJitter: "0s"},
	} {
		config.Version = 1
		config.Servers = []string{"https://example.com"}
		config.Username, config.Password = "alice", "secret"
		if _, err := profile.Encode(config); err == nil {
			t.Fatal("invalid stealth profile accepted", config)
		}
	}
}
