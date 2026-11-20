package vetroff_test

import (
	"testing"

	vetroff "github.com/SawaMEN/VetroffProxy"
	"github.com/SawaMEN/VetroffProxy/internal/contract"
	"github.com/SawaMEN/VetroffProxy/internal/profile"
	"github.com/SawaMEN/VetroffProxy/internal/stealth"
)

func TestProbeKeySDKProfileRoundTripAndValidation(t *testing.T) {
	key, err := stealth.GenerateProbeKey()
	if err != nil {
		t.Fatal(err)
	}
	p := profile.Config{
		Version: 1, Servers: []string{"https://example.com"},
		Username: "alice", Password: "secret",
		ProtocolPolicy: contract.ProtocolStrict, ProbeKey: key,
	}
	link, err := profile.Encode(p)
	if err != nil {
		t.Fatal(err)
	}
	c, err := vetroff.ClientConfigFromLink(link)
	if err != nil || c.ProbeKey != key || !c.RequireProtocolNegotiation {
		t.Fatalf("SDK link lost probe key: %+v %v", c, err)
	}
	if _, err := vetroff.NewClient(vetroff.ClientConfig{ProbeKey: key}); err == nil {
		t.Fatal("SDK client allowed key without strict mode")
	}
	if _, err := vetroff.NewServer(vetroff.ServerConfig{ProbeKey: key}); err == nil {
		t.Fatal("SDK server allowed key without strict mode")
	}
	p.ProtocolPolicy = ""
	if _, err := profile.Encode(p); err == nil {
		t.Fatal("non-strict link accepted signed probe key")
	}
	p.ProtocolPolicy = contract.ProtocolStrict
	p.ProbeKey = "not-a-key"
	if _, err := profile.Encode(p); err == nil {
		t.Fatal("invalid encoded secret was accepted")
	}
}
