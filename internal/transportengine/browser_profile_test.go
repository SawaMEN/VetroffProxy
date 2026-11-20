package transportengine

import (
	"strings"
	"testing"

	"github.com/SawaMEN/VetroffProxy/internal/buildinfo"
	"github.com/SawaMEN/VetroffProxy/internal/h2profile"
	"github.com/SawaMEN/VetroffProxy/internal/nsstls"
)

func TestBrowserProfileFirefox158RequiresNativeNSS(t *testing.T) {
	for _, mode := range []string{"", "h2-connect", "h2-post"} {
		plan, err := Resolve(Config{BrowserProfile: BrowserFirefox158, Transport: mode})
		if !buildinfo.NSSAvailable {
			if err == nil {
				t.Fatalf("browser profile silently selected a non-native backend: %+v", plan)
			}
			continue
		}
		if err != nil {
			t.Fatalf("valid browser profile mode=%q: %v", mode, err)
		}
		wantTransport := mode
		if wantTransport == "" {
			wantTransport = "h2-connect"
		}
		if plan.Engine != "mozilla" || plan.Transport != wantTransport ||
			plan.TLSBackend != "nss" || plan.TLSClientProfile != nsstls.Firefox158 ||
			plan.HTTP2ReceiveProfile != h2profile.Firefox158 {
			t.Fatalf("browser preset lost native preferences: %+v", plan)
		}
	}
}

func TestBrowserProfileFirefox158RejectsIncompatibleOptions(t *testing.T) {
	for _, cfg := range []Config{
		{BrowserProfile: "unrecognized"},
		{BrowserProfile: BrowserFirefox158, Transport: "h3-connect"},
		{BrowserProfile: BrowserFirefox158, Transport: "h3-post"},
		{BrowserProfile: BrowserFirefox158, Transport: "xhttp-stream-one"},
		{BrowserProfile: BrowserFirefox158, Transport: "wss"},
		{BrowserProfile: BrowserFirefox158, TLSBackend: "go"},
		{BrowserProfile: BrowserFirefox158, Engine: "necko"},
		{BrowserProfile: BrowserFirefox158, NeckoBundle: "/fake"},
		{BrowserProfile: BrowserFirefox158, TLSClientProfile: "unknown"},
		{BrowserProfile: BrowserFirefox158, HTTP2ReceiveProfile: "unknown"},
	} {
		if plan, err := Resolve(cfg); err == nil {
			t.Fatalf("accepted incompatible profile %+v: %+v", cfg, plan)
		}
	}
}

func TestBrowserProfileFirefox158DoesNotChangeBuiltinDefault(t *testing.T) {
	for _, name := range []string{"", "none"} {
		p, err := Resolve(Config{BrowserProfile: name, Engine: "builtin", Transport: "h2-connect"})
		if err != nil || p.Engine != "builtin" || p.TLSClientProfile != "" || p.HTTP2ReceiveProfile != "" {
			t.Fatalf("disabled browser profile changed builtin: %+v %v", p, err)
		}
	}
}

func TestBrowserProfileExplicitBackendFailureIsClear(t *testing.T) {
	_, err := Resolve(Config{BrowserProfile: BrowserFirefox158, Transport: "h2-connect", TLSBackend: "go"})
	if err == nil || !strings.Contains(err.Error(), "NSS") {
		// No Go TLS fallback is allowed for a pinned native fingerprint.
		t.Fatal("expected explicit NSS requirement", err)
	}
}
