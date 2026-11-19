package transportengine

import (
	"runtime"
	"testing"

	"github.com/SawaMEN/VetroffProxy/internal/buildinfo"
	"github.com/SawaMEN/VetroffProxy/internal/neqo"
)

func TestAutoEngineSelection(t *testing.T) {
	if runtime.GOOS == "linux" && (runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64") {
		p, err := Resolve(Config{Engine: "auto", NeckoBundle: "/nonexistent/verified-at-apply", Transport: "h3-connect"})
		if err != nil || p.Engine != "necko" || p.Transport != "h3-connect" {
			t.Fatalf("compatible packaged Necko: %+v, %v", p, err)
		}
		p, err = Resolve(Config{Engine: "auto", NeckoBundle: "/nonexistent", Transport: "h2-connect", AutoNeckoExcluded: true})
		if err != nil || p.Engine == "necko" {
			t.Fatalf("unsupported policy selected Necko: %+v %v", p, err)
		}
	}
	p, err := Resolve(Config{Engine: "auto", Transport: "h3-connect", AutoNeckoExcluded: true})
	if err != nil {
		t.Fatal(err)
	}
	expected := "builtin"
	if buildinfo.NSSAvailable && neqo.Available {
		expected = "mozilla"
	}
	if p.Engine != expected || p.Transport != "h3-connect" {
		t.Fatalf("H3 fallback must preserve transport: %+v, expected %s", p, expected)
	}
	p, err = Resolve(Config{Engine: "auto", Transport: "h2-post", TLSBackend: "go", AutoNeckoExcluded: true})
	if err != nil || p.Engine != "builtin" {
		t.Fatalf("explicit Go TLS must not select NSS: %+v %v", p, err)
	}
}

func TestExplicitNeckoNeverDowngrades(t *testing.T) {
	_, err := Resolve(Config{Engine: "necko", Transport: "wss"})
	if err == nil {
		t.Fatal("explicit necko must reject unsupported transport")
	}
	_, err = Resolve(Config{Engine: "builtin", NeckoBundle: "/tmp/necko"})
	if err == nil {
		t.Fatal("builtin may not accept a Necko bundle")
	}
}

func TestNativeProfileEngineSelection(t *testing.T) {
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		t.Skip("native platform")
	}
	for _, engine := range []string{"auto", "necko"} {
		plan, err := Resolve(Config{Engine: engine, NeckoBundle: "/verified-at-apply", Transport: "h2-post", TLSClientProfile: "firefox-158", HTTP2ReceiveProfile: "firefox-158"})
		if err != nil || plan.Engine != "necko" {
			t.Fatal("native profile selection", plan, err)
		}
	}
	for _, profile := range []Config{{Engine: "necko", Transport: "h3-connect", TLSClientProfile: "firefox-158"}, {Engine: "necko", Transport: "h3-post", HTTP2ReceiveProfile: "firefox-158"}, {Engine: "necko", Transport: "h2-post", TLSClientProfile: "unknown"}} {
		if _, err := Resolve(profile); err == nil {
			t.Fatal("accepted incompatible profile")
		}
	}
}

func TestFirefoxEngineRequiresNativeNecko(t *testing.T) {
	bundle := "/nonexistent/verified-at-apply"
	if runtime.GOOS != "linux" || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") {
		if _, err := Resolve(Config{Engine: "firefox", NeckoBundle: bundle}); err == nil {
			t.Fatal("firefox mode must reject unsupported platforms")
		}
		return
	}

	for _, transport := range []string{"h2-connect", "h3-connect", "h2-post", "h3-post"} {
		p, err := Resolve(Config{Engine: "firefox", NeckoBundle: bundle, Transport: transport})
		if err != nil || p.Engine != "necko" || p.NeckoBundle != bundle || p.Transport != transport {
			t.Fatalf("firefox must resolve to native Gecko without changing carrier: %+v %v", p, err)
		}
	}
	for _, cfg := range []Config{
		{Engine: "firefox", NeckoBundle: bundle, Transport: "wss"},
		{Engine: "firefox", NeckoBundle: bundle, Transport: "h2-connect", TLSBackend: "go"},
		{Engine: "firefox", NeckoBundle: bundle, Transport: "h3-connect", QUICBackend: "quic-go"},
		{Engine: "firefox", NeckoBundle: bundle, Transport: "h3-connect", H3Fallback: "h2"},
	} {
		if p, err := Resolve(cfg); err == nil {
			t.Fatalf("firefox accepted non-native transport or downgrade: %+v", p)
		}
	}
}
