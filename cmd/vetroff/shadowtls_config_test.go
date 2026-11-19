package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/VetroffProxy/internal/profile"
)

func shadowTLSFixture(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	passfile := filepath.Join(dir, "password")
	if err := os.WriteFile(passfile, []byte("test-shadowtls-password-32-bytes\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return dir, passfile
}

func TestShadowTLSV3ServerBridge(t *testing.T) {
	dir, passwordFile := shadowTLSFixture(t)
	server, serverPath := checkServerFixture(t)
	server.Listen = "127.0.0.1:8443"
	saveCheckServer(t, serverPath, server)
	output := filepath.Join(dir, "shadowtls-server.json")
	var result bytes.Buffer
	err := shadowTLSConfigCommand([]string{
		"--role", "server", "--config", serverPath,
		"--cover", "cover.example.com:443", "--password-file", passwordFile,
		"--out", output,
	}, &result)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(output)
	if err != nil { t.Fatal(err) }
	var cfg shadowTLSConfig
	if err := json.Unmarshal(raw, &cfg); err != nil { t.Fatal(err) }
	if !cfg.V3 || !cfg.Strict || cfg.Server == nil || cfg.Client != nil ||
		cfg.Server.Listen != "0.0.0.0:443" || cfg.Server.ServerAddr != "127.0.0.1:8443" ||
		cfg.Server.TLSAddr.Fallback != "cover.example.com:443" ||
		cfg.Server.TLSAddr.WildcardSNI != "off" || cfg.Server.WildcardSNI != "off" {
		t.Fatalf("bad official ShadowTLS server config: %+v", cfg)
	}
	if cfg.Server.Password != "test-shadowtls-password-32-bytes" { t.Fatal("password changed") }
	if strings.Contains(result.String(), cfg.Server.Password) || strings.Contains(result.String(), serverPath) {
		t.Fatal("secret or private path printed")
	}
	info, _ := os.Stat(output)
	if info.Mode().Perm() != 0600 { t.Fatal("config is not private") }
	if err := shadowTLSConfigCommand([]string{
		"--role", "server", "--config", serverPath,
		"--cover", "cover.example.com:443", "--password-file", passwordFile,
		"--out", output,
	}, &bytes.Buffer{}); err == nil {
		t.Fatal("existing private config overwritten")
	}
}

func TestShadowTLSV3ClientBridgePreservesInnerTLS(t *testing.T) {
	dir, passwordFile := shadowTLSFixture(t)
	original, err := profile.Encode(profile.Config{
		Version: 1, Servers: []string{"https://edge.example.com"}, Username: "alice",
		Password: "inner-proxy-secret", SNI: "edge.example.com", Transport: "h2-connect",
	})
	if err != nil { t.Fatal(err) }
	source := filepath.Join(dir, "connection.txt")
	if err := profile.WriteSecretFile(source, original); err != nil { t.Fatal(err) }
	output, rewritten := filepath.Join(dir, "shadowtls-client.json"), filepath.Join(dir, "connection-local.txt")
	var result bytes.Buffer
	if err := shadowTLSConfigCommand([]string{
		"--role", "client", "--link-file", source,
		"--cover", "cover.example.com:443", "--listen", "127.0.0.1:9443",
		"--password-file", passwordFile, "--out", output, "--local-link-out", rewritten,
	}, &result); err != nil { t.Fatal(err) }
	var cfg shadowTLSConfig
	raw, _ := os.ReadFile(output)
	if err := json.Unmarshal(raw, &cfg); err != nil { t.Fatal(err) }
	if !cfg.V3 || !cfg.Strict || cfg.Server != nil || cfg.Client == nil ||
		cfg.Client.Listen != "127.0.0.1:9443" ||
		cfg.Client.ServerAddr != "edge.example.com:443" ||
		cfg.Client.TLSNames != "cover.example.com" {
		t.Fatalf("bad official ShadowTLS client config: %+v", cfg)
	}
	local, err := profile.ReadSecretFile(rewritten)
	if err != nil { t.Fatal(err) }
	if len(local.Servers) != 1 || local.Servers[0] != "https://127.0.0.1:9443" ||
		local.SNI != "edge.example.com" || local.Password != "inner-proxy-secret" ||
		local.Encryption != "tls13" || local.Transport != "h2-connect" {
		t.Fatalf("inner HTTPS trust or transport was lost: %+v", local)
	}
	for _, path := range []string{output, rewritten} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0600 { t.Fatal("unsafe output permissions", path) }
	}
	if strings.Contains(result.String(), "inner-proxy-secret") || strings.Contains(result.String(), cfg.Client.Password) {
		t.Fatal("secret leaked to stdout")
	}
}

func TestShadowTLSV3RejectsUnsafeConfigurations(t *testing.T) {
	dir, pass := shadowTLSFixture(t)
	invalidPass := filepath.Join(dir, "public-password")
	if err := os.WriteFile(invalidPass, []byte("public-shared-secret-32-bytes"), 0644); err != nil { t.Fatal(err) }
	server, path := checkServerFixture(t)
	server.Listen = ":443"
	saveCheckServer(t, path, server)
	for _, test := range []struct{ name string; args []string }{
		{"public-password", []string{"--role", "server", "--config", path, "--cover", "cover.example.com:443", "--out", filepath.Join(dir, "public.json"), "--password-file", invalidPass}},
		{"public-inner-server", []string{"--role", "server", "--config", path, "--cover", "cover.example.com:443", "--out", filepath.Join(dir, "unsafe.json"), "--password-file", pass}},
		{"cover-ip", []string{"--role", "server", "--config", path, "--cover", "1.1.1.1:443", "--out", filepath.Join(dir, "ip.json"), "--password-file", pass}},
		{"missing-local-link", []string{"--role", "client", "--link-file", path, "--cover", "cover.example.com:443", "--out", filepath.Join(dir, "missing.json"), "--password-file", pass}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := shadowTLSConfigCommand(test.args, &bytes.Buffer{}); err == nil { t.Fatal("unsafe config accepted") }
		})
	}
}

func TestShadowTLSV3RejectsUDPOnlyProfile(t *testing.T) {
	dir, pass := shadowTLSFixture(t)
	link, err := profile.Encode(profile.Config{Version: 1, Servers: []string{"https://edge.example.com"}, Username: "alice", Password: "test", Transport: "h3-connect"})
	if err != nil { t.Fatal(err) }
	file := filepath.Join(dir, "h3.txt")
	if err := profile.WriteSecretFile(file, link); err != nil { t.Fatal(err) }
	if err := shadowTLSConfigCommand([]string{
		"--role", "client", "--link-file", file, "--cover", "cover.example.com:443",
		"--password-file", pass, "--out", filepath.Join(dir, "shadowtls.json"),
		"--local-link-out", filepath.Join(dir, "local.txt"),
	}, &bytes.Buffer{}); err == nil {
		t.Fatal("HTTP/3 over TCP-only ShadowTLS accepted")
	}
}
