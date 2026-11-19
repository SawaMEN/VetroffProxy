package vetroff_test

import (
	"testing"

	vetroff "github.com/SawaMEN/VetroffProxy"
	"github.com/SawaMEN/VetroffProxy/internal/profile"
)

func TestAdaptivePaddingSharedProfilePreservedBySDK(t *testing.T) {
	link, err := profile.Encode(profile.Config{
		Version: 1,
		Servers: []string{"https://edge.example.com"},
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
		Version: 1,
		Servers: []string{"https://a.example.com", "https://b.example.com"},
		Username: "alice", Password: "secret",
		AdaptivePadding: true,
		Bond: true,
	})
	if err == nil {
		t.Fatal("bond profile accepted unsupported adaptive padding")
	}
}
