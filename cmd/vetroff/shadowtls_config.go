package main

// ShadowTLS v3 is a separate, authenticated TCP carrier around the regular
// Vetroff HTTPS endpoint. This command prepares configs for the upstream
// shadow-tls binary; it does not pretend to implement ShadowTLS cryptography.
import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/SawaMEN/VetroffProxy/internal/bootstrap"
	"github.com/SawaMEN/VetroffProxy/internal/profile"
	"github.com/SawaMEN/VetroffProxy/internal/tlsidentity"
)

var errShadowTLSConfig = errors.New("invalid ShadowTLS v3 configuration")

type shadowTLSConfig struct {
	V3     bool                  `json:"v3"`
	Strict bool                  `json:"strict"`
	Client *shadowTLSClientEntry `json:"client,omitempty"`
	Server *shadowTLSServerEntry `json:"server,omitempty"`
}

type shadowTLSClientEntry struct {
	Listen     string `json:"listen"`
	ServerAddr string `json:"server_addr"`
	TLSNames   string `json:"tls_names"`
	Password   string `json:"password"`
}

type shadowTLSTLSAddr struct {
	WildcardSNI string            `json:"wildcard_sni"`
	Dispatch    map[string]string `json:"dispatch"`
	Fallback    string            `json:"fallback"`
}

type shadowTLSServerEntry struct {
	Listen      string           `json:"listen"`
	ServerAddr  string           `json:"server_addr"`
	TLSAddr     shadowTLSTLSAddr `json:"tls_addr"`
	Password    string           `json:"password"`
	WildcardSNI string           `json:"wildcard_sni"`
}

// validateShadowTLSAddress refuses invalid or non-numeric ports; local bridges
// must be loopback-only to avoid exposing an unauthenticated plaintext socket.
func validateShadowTLSAddress(address string, loopbackOnly bool) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", errShadowTLSConfig
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return "", errShadowTLSConfig
	}
	if loopbackOnly {
		ip := net.ParseIP(host)
		if ip == nil || !ip.IsLoopback() {
			return "", errShadowTLSConfig
		}
	} else if host != "" && net.ParseIP(host) == nil {
		if _, err := tlsidentity.Normalize(host, ""); err != nil {
			return "", errShadowTLSConfig
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(n)), nil
}

func shadowTLSCover(cover string) (string, string, error) {
	address, err := validateShadowTLSAddress(cover, false)
	if err != nil {
		return "", "", errShadowTLSConfig
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil || net.ParseIP(host) != nil || host == "" {
		return "", "", errShadowTLSConfig
	}
	if _, err := tlsidentity.Normalize(host, ""); err != nil {
		return "", "", errShadowTLSConfig
	}
	return strings.ToLower(host), address, nil
}

func shadowTLSPassword(file string) (string, error) {
	info, err := os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || info.Size() > 256 {
		return "", errShadowTLSConfig
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", errShadowTLSConfig
	}
	// One trailing text-file newline is permitted, but embedded whitespace,
	// control bytes and shell/CLI exposure are not.
	password := strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if len(password) < 16 || len(password) > 128 {
		return "", errShadowTLSConfig
	}
	for i := 0; i < len(password); i++ {
		if password[i] <= ' ' || password[i] >= 127 {
			return "", errShadowTLSConfig
		}
	}
	return password, nil
}

func writeShadowTLSPrivate(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return errShadowTLSConfig
	}
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if errClose := f.Close(); err == nil {
		err = errClose
	}
	if err != nil {
		_ = os.Remove(path)
		return errShadowTLSConfig
	}
	return nil
}

// shadowTLSConfigCommand prepares upstream v0.2.25-compatible strict v3 JSON,
// avoiding password arguments and keeping Vetroff's own TLS trust intact.
func shadowTLSConfigCommand(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("vetroff shadowtls-config", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	role := fs.String("role", "", "server or client")
	configPath := fs.String("config", "", "private Vetroff server config (server role)")
	linkFile := fs.String("link-file", "", "private Vetroff connection profile (client role)")
	passwordFile := fs.String("password-file", "", "shared ShadowTLS password in a mode-0600 file")
	cover := fs.String("cover", "", "TLS 1.3 handshake server, domain:port")
	listen := fs.String("listen", "", "ShadowTLS listener: public on server, loopback on client")
	output := fs.String("out", "", "new private upstream ShadowTLS JSON file")
	localLink := fs.String("local-link-out", "", "new local Vetroff connection profile (client role)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = fmt.Fprintln(out, "usage: vetroff shadowtls-config --role client|server --cover hostname:443 --password-file FILE --out FILE [options]")
			fs.SetOutput(out)
			fs.PrintDefaults()
			return nil
		}
		return errShadowTLSConfig
	}
	if fs.NArg() != 0 || *output == "" || *passwordFile == "" || *cover == "" {
		return errShadowTLSConfig
	}
	coverName, coverAddr, err := shadowTLSCover(*cover)
	if err != nil {
		return err
	}
	password, err := shadowTLSPassword(*passwordFile)
	if err != nil {
		return err
	}
	c := shadowTLSConfig{V3: true, Strict: true}
	var privateLink []byte
	switch *role {
	case "server":
		if *configPath == "" || *linkFile != "" || *localLink != "" {
			return errShadowTLSConfig
		}
		if *listen == "" {
			*listen = "0.0.0.0:443"
		}
		public, err := validateShadowTLSAddress(*listen, false)
		if err != nil {
			return err
		}
		server, err := bootstrap.ReadServer(*configPath)
		if err != nil || server.H3Listen != "" {
			return errShadowTLSConfig
		}
		backend, err := validateShadowTLSAddress(server.Listen, true)
		if err != nil {
			return err
		}
		if public == backend {
			return errShadowTLSConfig
		}
		c.Server = &shadowTLSServerEntry{
			Listen: public, ServerAddr: backend, Password: password, WildcardSNI: "off",
			TLSAddr: shadowTLSTLSAddr{WildcardSNI: "off", Dispatch: map[string]string{}, Fallback: coverAddr},
		}
	case "client":
		if *linkFile == "" || *configPath != "" || *localLink == "" || *localLink == *output {
			return errShadowTLSConfig
		}
		if *listen == "" {
			*listen = "127.0.0.1:9443"
		}
		local, err := validateShadowTLSAddress(*listen, true)
		if err != nil {
			return err
		}
		p, err := profile.ReadSecretFile(*linkFile)
		if err != nil || len(p.Servers) != 1 || p.Bond || p.Discover || p.Transport == "h3-post" || p.Transport == "h3-connect" {
			return errShadowTLSConfig
		}
		origin, err := url.Parse(p.Servers[0])
		if err != nil {
			return errShadowTLSConfig
		}
		proxyName := p.SNI
		if proxyName == "" {
			proxyName = origin.Hostname()
		}
		if _, err := tlsidentity.Normalize(proxyName, ""); err != nil || net.ParseIP(proxyName) != nil {
			return errShadowTLSConfig
		}
		port := origin.Port()
		if port == "" {
			port = "443"
		}
		remote, err := validateShadowTLSAddress(net.JoinHostPort(origin.Hostname(), port), false)
		if err != nil {
			return err
		}
		// Do not weaken the Vetroff HTTPS certificate or SPKI pin because
		// its TCP connection now terminates on a local ShadowTLS listener.
		p.Servers = []string{"https://" + local}
		p.SNI = proxyName
		link, err := profile.Encode(p)
		if err != nil {
			return errShadowTLSConfig
		}
		privateLink = []byte(link + "\n")
		c.Client = &shadowTLSClientEntry{Listen: local, ServerAddr: remote, TLSNames: coverName, Password: password}
	default:
		return errShadowTLSConfig
	}
	blob, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return errShadowTLSConfig
	}
	if err := writeShadowTLSPrivate(*output, append(blob, '\n')); err != nil {
		return err
	}
	if privateLink != nil {
		if err := writeShadowTLSPrivate(*localLink, privateLink); err != nil {
			_ = os.Remove(*output)
			return err
		}
	}
	_, _ = fmt.Fprintln(out, "strict ShadowTLS v3 configuration created (private files; credentials not printed)")
	return nil
}
