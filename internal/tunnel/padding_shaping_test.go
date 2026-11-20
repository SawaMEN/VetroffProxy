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

func TestTrafficShapingPaddingChoices(t *testing.T) {
	if shapedPaddingLength(58, 0, 100) != 3 {
		t.Fatal("expected exactly 64-byte inner record")
	}
	if shapedPaddingLength(58, 13, 2) > 2 || shapedPaddingLength(58, 1, 0) != 0 {
		t.Fatal("shaping exceeded budget")
	}
	for n := 1; n <= maxPaddingPayload; n += 47 {
		for _, sample := range []byte{0, 13, 127, 255} {
			available := uint64(n / 8)
			pad := shapedPaddingLength(n, sample, available)
			if pad < 0 || pad > adaptivePaddingLimit(n) || uint64(pad) > available {
				t.Fatalf("invalid n=%d sample=%d pad=%d", n, sample, pad)
			}
		}
	}
}

func TestTrafficShapingCumulativeBudgetAndLegacyDecoder(t *testing.T) {
	for _, fragment := range []bool{false, true} {
		var encoded, source bytes.Buffer
		writer := newShapedAdaptivePaddingWriter(&encoded, context.Background(), 0, fragment, true)
		if n, err := writer.Write(nil); n != 0 || err != nil || encoded.Len() != 0 {
			t.Fatal("empty write emitted data")
		}
		for i := 0; i < 140; i++ {
			size := 1 + (i*923)%18000
			chunk := bytes.Repeat([]byte{byte(i)}, size)
			source.Write(chunk)
			if n, err := writer.Write(chunk); err != nil || n != size {
				t.Fatalf("write %d n=%d err=%v", size, n, err)
			}
		}
		wire := encoded.Bytes()
		totalPadding, offset, frames := 0, 0, 0
		for offset < len(wire) {
			if len(wire)-offset < 3 {
				t.Fatal("truncated frame header")
			}
			n := int(binary.BigEndian.Uint16(wire[offset : offset+2]))
			pad := int(wire[offset+2])
			offset += 3
			if n <= 0 || n > maxPaddingPayload || pad > adaptivePaddingLimit(n) || offset+n+pad > len(wire) {
				t.Fatalf("invalid frame n=%d pad=%d", n, pad)
			}
			if fragment && n > stealth.MaxFragmentSize {
				t.Fatalf("oversized inner fragment: %d", n)
			}
			if !bytes.Equal(wire[offset+n:offset+n+pad], make([]byte, pad)) {
				t.Fatal("nonzero padding leaked")
			}
			totalPadding += pad
			offset += n + pad
			frames++
		}
		if frames < 140 || totalPadding > source.Len()/int(stealth.PaddingBudgetDivisor) {
			t.Fatalf("fragment=%t frames=%d padding=%d useful=%d", fragment, frames, totalPadding, source.Len())
		}
		got, err := io.ReadAll(newAdaptivePaddingReader(bytes.NewReader(wire)))
		if err != nil || !bytes.Equal(got, source.Bytes()) {
			t.Fatalf("existing adaptive-v1 decoder failed: %v", err)
		}
	}
}

func TestTrafficShapingSOCKSInteroperability(t *testing.T) {
	for _, tc := range []struct {
		name string
		client, server, fragment bool
	}{
		{"shaped client", true, false, false},
		{"shaped server", false, true, false},
		{"shaped both and fragmented", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := testDestination(t, func(c net.Conn) { _, _ = io.Copy(c, c) })
			client, addr := testClient(t, &Server{
				Username: "alice", Password: "secret", AllowPrivate: true,
				AdaptivePadding: true, AdaptiveFragmentation: tc.fragment,
				TrafficShaping: tc.server, RequireProtocolNegotiation: true,
			})
			client.AdaptivePadding = true
			client.AdaptiveFragmentation = tc.fragment
			client.TrafficShaping = tc.client
			client.RequireProtocolNegotiation = true
			conn, code := connectSOCKS(t, addr, target)
			defer conn.Close()
			if code != 0 {
				t.Fatalf("SOCKS rejected %d", code)
			}
			payload := bytes.Repeat([]byte("shaping-test"), 12000)
			done := make(chan error, 1)
			go func() { _, err := conn.Write(payload); done <- err }()
			got := make([]byte, len(payload))
			if _, err := io.ReadFull(conn, got); err != nil || !bytes.Equal(got, payload) {
				t.Fatalf("shaped echo corrupted: %v", err)
			}
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestTrafficShapingHalfClose(t *testing.T) {
	target := testDestination(t, func(c net.Conn) {
		all, _ := io.ReadAll(c)
		_, _ = c.Write(all)
	})
	client, addr := testClient(t, &Server{
		Username: "alice", Password: "secret", AllowPrivate: true,
		AdaptivePadding: true, TrafficShaping: true,
	})
	client.AdaptivePadding = true
	client.TrafficShaping = true
	conn, code := connectSOCKS(t, addr, target)
	defer conn.Close()
	if code != 0 {
		t.Fatalf("SOCKS rejected %d", code)
	}
	payload := bytes.Repeat([]byte("shaped-half-close"), 2000)
	if _, err := conn.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := conn.(*net.TCPConn).CloseWrite(); err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(conn)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("half-close corrupted data: %v", err)
	}
}
