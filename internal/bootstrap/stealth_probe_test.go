package bootstrap

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/SawaMEN/VetroffProxy/internal/contract"
	"github.com/SawaMEN/VetroffProxy/internal/profile"
	"github.com/SawaMEN/VetroffProxy/internal/stealth"
)

func TestProbeGuardStrictSetupGeneratesSeparateSecret(t *testing.T) {
	options := fixture(t, "main.example")
	options.ProtocolPolicy = contract.ProtocolStrict
	options.ProbeGuard = true
	if err := Generate(options); err != nil {
		t.Fatal(err)
	}
	server, err := ReadServer(filepath.Join(options.Output, "server.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := stealth.ValidateProbeKey(server.ProbeKey, true); err != nil {
		t.Fatal(err)
	}
	if server.ProbeKey == server.Password {
		t.Fatal("probe key must be distinct from basic password")
	}
	fd, err := os.Open(filepath.Join(options.Output, "connection.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer fd.Close()
	link, err := profile.Read(fd)
	if err != nil || link.ProbeKey != server.ProbeKey || link.ProtocolPolicy != contract.ProtocolStrict {
		t.Fatalf("generated secure link lost probe key: %+v %v", link, err)
	}
}

func TestProbeGuardSetupRejectsNonStrictWithoutWriting(t *testing.T) {
	o := fixture(t, "main.example")
	o.ProbeGuard = true
	if err := Generate(o); err == nil {
		t.Fatal("probe guard was allowed without strict policy")
	}
	if _, err := os.Stat(o.Output); !os.IsNotExist(err) {
		t.Fatal("invalid probe guard created private output")
	}
}
