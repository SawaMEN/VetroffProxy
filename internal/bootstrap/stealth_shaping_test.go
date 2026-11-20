package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SawaMEN/VetroffProxy/internal/contract"
	"github.com/SawaMEN/VetroffProxy/internal/profile"
)

func TestTrafficShapingStrictSetupAndLink(t *testing.T) {
	o := fixture(t, "main.example")
	o.ProtocolPolicy = contract.ProtocolStrict
	o.AdaptivePadding = true
	o.TrafficShaping = true
	if err := Generate(o); err != nil {
		t.Fatal(err)
	}
	server, err := ReadServer(filepath.Join(o.Output, "server.json"))
	if err != nil || !server.AdaptivePadding || !server.TrafficShaping {
		t.Fatalf("server setup lost shaping: %+v %v", server, err)
	}
	f, err := os.Open(filepath.Join(o.Output, "connection.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	link, err := profile.Read(f)
	if err != nil || !link.AdaptivePadding || !link.TrafficShaping ||
		link.ProtocolPolicy != contract.ProtocolStrict || link.Bond || link.Discover {
		t.Fatalf("connection link lost shaping: %+v %v", link, err)
	}
}

func TestTrafficShapingSetupRejectsBadMode(t *testing.T) {
	o := fixture(t, "main.example")
	o.ProtocolPolicy = contract.ProtocolStrict
	o.TrafficShaping = true
	if err := Generate(o); err == nil {
		t.Fatal("setup accepted shaping without adaptive padding")
	}
	if _, err := os.Stat(o.Output); !os.IsNotExist(err) {
		t.Fatal("invalid setup wrote secret files")
	}
}
