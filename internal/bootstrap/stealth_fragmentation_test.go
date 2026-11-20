package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SawaMEN/VetroffProxy/internal/contract"
	"github.com/SawaMEN/VetroffProxy/internal/profile"
)

func TestStealthFragmentationSetupAndRoundTrip(t *testing.T) {
	o := fixture(t, "main.example")
	o.ProtocolPolicy = contract.ProtocolStrict
	o.AdaptivePadding = true
	o.AdaptiveFragmentation = true
	o.TimingJitter = 2 * time.Millisecond
	if err := Generate(o); err != nil {
		t.Fatal(err)
	}
	cfg, err := ReadServer(filepath.Join(o.Output, "server.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AdaptivePadding || !cfg.AdaptiveFragmentation || cfg.TimingJitter != "2ms" || cfg.BondHub || cfg.BondUpstream != "" {
		t.Fatalf("generated server lost strict stealth settings: %+v", cfg)
	}
	f, err := os.Open(filepath.Join(o.Output, "connection.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	link, err := profile.Read(f)
	if err != nil || !link.AdaptivePadding || !link.AdaptiveFragmentation || link.TimingJitter != "2ms" ||
		link.Bond || link.Discover || link.ProtocolPolicy != contract.ProtocolStrict {
		t.Fatalf("generated link is not a single-node strict stealth profile: %+v %v", link, err)
	}
}

func TestStealthFragmentationSetupRejectsInvalidModes(t *testing.T) {
	for _, strict := range []bool{false, true} {
		o := fixture(t, "main.example")
		o.AdaptiveFragmentation = true
		if strict {
			o.ProtocolPolicy = contract.ProtocolStrict
		}
		if err := Generate(o); err == nil {
			t.Fatalf("invalid fragmentation setup accepted: strict=%t", strict)
		}
		if _, err := os.Stat(o.Output); !os.IsNotExist(err) {
			t.Fatal("invalid setup wrote secrets to disk")
		}
	}
}
