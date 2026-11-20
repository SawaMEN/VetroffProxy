package tunnel

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"net"
	"testing"

	"github.com/SawaMEN/VetroffProxy/internal/stealth"
)

func TestAdaptiveFragmentationWireAndOverhead(t *testing.T) {
	source := bytes.Repeat([]byte("fragmentation-byte-check"), 22000)
	var wire bytes.Buffer
	writer := newShapedAdaptivePaddingWriter(&wire, context.Background(), 0, true, false)
	n, err := writer.Write(source)
	if err != nil || n != len(source) {
		t.Fatalf("write %d/%d: %v", n, len(source), err)
	}
	encoded := wire.Bytes()
	if len(encoded) > len(source)*13/10 {
		t.Fatalf("fragmented encoding overhead exceeded 30%%: %d vs %d", len(encoded), len(source))
	}
	frames, offset := 0, 0
	for offset < len(encoded) {
		if len(encoded)-offset < 3 {
			t.Fatal("truncated frame header")
		}
		size := int(binary.BigEndian.Uint16(encoded[offset : offset+2]))
		pad := int(encoded[offset+2])
		offset += 3
		if size <= 0 || size > stealth.MaxFragmentSize || pad > adaptivePaddingLimit(size) {
			t.Fatalf("invalid frame: %d payload, %d padding", size, pad)
		}
		if offset+size+pad > len(encoded) {
			t.Fatal("short frame")
		}
		if size < stealth.MinFragmentSize && offset+size+pad != len(encoded) {
			t.Fatalf("unexpected sub-minimum intermediate fragment: %d", size)
		}
		offset += size + pad
		frames++
	}
	if frames < 50 || offset != len(encoded) {
		t.Fatalf("expected multiple bounded frames, got %d", frames)
	}
	// An existing adaptive-v1 peer with no fragmentation flag can decode
	// this unchanged wire format and must recover every application byte.
	got, err := io.ReadAll(newAdaptivePaddingReader(bytes.NewReader(encoded)))
	if err != nil || !bytes.Equal(source, got) {
		t.Fatalf("existing adaptive-v1 decoder incompatible: %v", err)
	}

	var baseline bytes.Buffer
	if _, err := newAdaptivePaddingWriter(&baseline).Write(source); err != nil {
		t.Fatal(err)
	}
	if size := binary.BigEndian.Uint16(baseline.Bytes()[:2]); size != maxPaddingPayload {
		t.Fatalf("default adaptive framing unexpectedly changed: %d", size)
	}
}

func TestAdaptiveFragmentationSmallWritesRemainSingleFrame(t *testing.T) {
	for _, size := range []int{1, 25, 256, 1024, 4096} {
		var wire bytes.Buffer
		writer := newShapedAdaptivePaddingWriter(&wire, context.Background(), 0, true, false)
		payload := bytes.Repeat([]byte{0x63}, size)
		if n, err := writer.Write(payload); n != size || err != nil {
			t.Fatalf("size %d: wrote %d: %v", size, n, err)
		}
		b := wire.Bytes()
		if len(b) < 3 || int(binary.BigEndian.Uint16(b[:2])) != size {
			t.Fatalf("small frame %d was unnecessarily split", size)
		}
		if got, err := io.ReadAll(newAdaptivePaddingReader(bytes.NewReader(b))); err != nil || !bytes.Equal(got, payload) {
			t.Fatalf("small frame %d roundtrip: %v", size, err)
		}
	}
}

func TestAdaptiveFragmentationSOCKSPeerCompatibility(t *testing.T) {
	for _, tc := range []struct {
		name           string
		client, server bool
	}{
		{"shaped upload to legacy adaptive peer", true, false},
		{"shaped downlink to legacy adaptive peer", false, true},
		{"shaped both directions", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := testDestination(t, func(c net.Conn) { _, _ = io.Copy(c, c) })
			client, addr := testClient(t, &Server{
				Username: "alice", Password: "secret", AllowPrivate: true,
				AdaptivePadding: true, AdaptiveFragmentation: tc.server,
				RequireProtocolNegotiation: true,
			})
			client.AdaptivePadding = true
			client.AdaptiveFragmentation = tc.client
			client.RequireProtocolNegotiation = true
			conn, code := connectSOCKS(t, addr, target)
			defer conn.Close()
			if code != 0 {
				t.Fatalf("SOCKS handshake rejected: %d", code)
			}
			message := bytes.Repeat([]byte("fragmentation-test-"), 4000)
			done := make(chan error, 1)
			go func() { _, err := conn.Write(message); done <- err }()
			got := make([]byte, len(message))
			if _, err := io.ReadFull(conn, got); err != nil || !bytes.Equal(message, got) {
				t.Fatalf("fragmented echo corrupted: %v", err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAdaptiveFragmentationHalfClose(t *testing.T) {
	target := testDestination(t, func(c net.Conn) {
		data, _ := io.ReadAll(c)
		_, _ = c.Write(data)
	})
	client, addr := testClient(t, &Server{
		Username: "alice", Password: "secret", AllowPrivate: true,
		AdaptivePadding: true, AdaptiveFragmentation: true,
	})
	client.AdaptivePadding = true
	client.AdaptiveFragmentation = true
	conn, code := connectSOCKS(t, addr, target)
	defer conn.Close()
	if code != 0 {
		t.Fatalf("SOCKS connection failed: %d", code)
	}
	message := bytes.Repeat([]byte("half-close-"), 3000)
	if _, err := conn.Write(message); err != nil {
		t.Fatal(err)
	}
	if err := conn.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(conn)
	if err != nil || !bytes.Equal(got, message) {
		t.Fatalf("half-close data corruption: %v", err)
	}
}
