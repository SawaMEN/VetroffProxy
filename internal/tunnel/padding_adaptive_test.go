package tunnel

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
)

func TestAdaptivePaddingRoundTripAndFrameBounds(t *testing.T) {
	for _, size := range []int{0, 1, 12, 255, 16384, 16385, 300000} {
		source := bytes.Repeat([]byte{0x5a}, size)
		var wire bytes.Buffer
		encoder := newAdaptivePaddingWriter(&wire)
		if got, err := encoder.Write(source); got != size || err != nil {
			t.Fatalf("size=%d write=%d err=%v", size, got, err)
		}
		encoded := bytes.Clone(wire.Bytes())
		// Unlike initial-v1, the entire stream stays framed after eight records.
		count, offset := 0, 0
		for offset < len(encoded) {
			if len(encoded)-offset < 3 {
				t.Fatalf("size=%d truncated generated header", size)
			}
			n := int(binary.BigEndian.Uint16(encoded[offset : offset+2]))
			pad := int(encoded[offset+2])
			if n < 1 || n > maxPaddingPayload || pad > adaptivePaddingLimit(n) {
				t.Fatalf("size=%d invalid frame payload=%d padding=%d", size, n, pad)
			}
			offset += 3
			if len(encoded)-offset < n+pad {
				t.Fatalf("size=%d truncated generated record", size)
			}
			if !bytes.Equal(encoded[offset+n:offset+n+pad], make([]byte, pad)) {
				t.Fatal("adaptive padding leaked stale pooled data")
			}
			offset += n + pad
			count++
		}
		if count == 0 && size != 0 || count > 0 && size == 0 {
			t.Fatalf("size=%d generated %d records", size, count)
		}
		if got, err := io.ReadAll(newAdaptivePaddingReader(bytes.NewReader(encoded))); err != nil || !bytes.Equal(got, source) {
			t.Fatalf("size=%d roundtrip %v", size, err)
		}
	}
}

func TestAdaptivePaddingSmallWritesAndHalfClose(t *testing.T) {
	var source, wire bytes.Buffer
	writer := newAdaptivePaddingWriter(&wire)
	for i := 0; i < 32; i++ {
		payload := bytes.Repeat([]byte{byte(i)}, i+1)
		source.Write(payload)
		if n, err := writer.Write(payload); n != len(payload) || err != nil {
			t.Fatalf("record %d: %d %v", i, n, err)
		}
	}
	decoder := newAdaptivePaddingReader(bytes.NewReader(wire.Bytes()))
	var decoded bytes.Buffer
	var single [1]byte
	for {
		n, err := decoder.Read(single[:])
		decoded.Write(single[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(source.Bytes(), decoded.Bytes()) {
		t.Fatal("small reads changed adaptive stream payload")
	}
}

func TestAdaptivePaddingMalformedAndTruncated(t *testing.T) {
	for _, frame := range [][]byte{
		{0, 0, 0}, // Zero payload is not a record.
		{0x40, 0x01, 0}, // Over 16 KiB.
		{0, 1, 26, 'x'}, // For one payload byte, max pad is 24.
		{0, 3, 0, 'a', 'b'}, // Incomplete payload.
		{0, 1}, // Incomplete header.
	} {
		if _, err := io.ReadAll(newAdaptivePaddingReader(bytes.NewReader(frame))); err == nil {
			t.Fatalf("accepted malformed frame %x", frame)
		}
	}
	var wire bytes.Buffer
	if _, err := newAdaptivePaddingWriter(&wire).Write([]byte("test")); err != nil {
		t.Fatal(err)
	}
	good := wire.Bytes()
	for n := 1; n < len(good); n++ {
		_, err := io.ReadAll(newAdaptivePaddingReader(bytes.NewReader(good[:n])))
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("cut=%d: %v", n, err)
		}
	}
}

func TestAdaptivePaddingModeNegotiation(t *testing.T) {
	for _, tc := range []struct {
		values []string
		ok     bool
	}{
		{nil, true},
		{[]string{paddingVersion}, true},
		{[]string{adaptivePaddingVersion}, true},
		{[]string{adaptivePaddingVersion, adaptivePaddingVersion}, false},
		{[]string{"adaptive-v2"}, false},
		{[]string{""}, false},
	} {
		h := http.Header{}
		for _, v := range tc.values {
			h.Add(paddingHeader, v)
		}
		_, err := paddingModeRequested(h)
		if (err == nil) != tc.ok {
			t.Fatalf("%v unexpected error: %v", tc.values, err)
		}
	}
	h := http.Header{"X-Vetroff-Padding": {adaptivePaddingVersion}, "x-vetroff-padding": {paddingVersion}}
	if _, err := paddingModeRequested(h); err == nil {
		t.Fatal("accepted mixed-case duplicate padding headers")
	}
}

func TestAdaptivePaddingSOCKSEndToEndStrict(t *testing.T) {
	target := testDestination(t, func(c net.Conn) { _, _ = io.Copy(c, c) })
	client, addr := testClient(t, &Server{
		Username: "alice", Password: "secret",
		AllowPrivate: true, AdaptivePadding: true,
		RequireProtocolNegotiation: true,
	})
	client.AdaptivePadding = true
	client.RequireProtocolNegotiation = true
	conn, code := connectSOCKS(t, addr, target)
	defer conn.Close()
	if code != 0 {
		t.Fatalf("SOCKS negotiation failed: %d", code)
	}
	for i := 0; i < 16; i++ {
		p := bytes.Repeat([]byte{byte(i)}, i+1)
		if _, err := conn.Write(p); err != nil {
			t.Fatal(err)
		}
		got := make([]byte, len(p))
		if _, err := io.ReadFull(conn, got); err != nil || !bytes.Equal(got, p) {
			t.Fatalf("interactive record %d: %v", i, err)
		}
	}
	large := bytes.Repeat([]byte("abcdefgh"), 32768)
	done := make(chan error, 1)
	go func() { _, err := conn.Write(large); done <- err }()
	got := make([]byte, len(large))
	if _, err := io.ReadFull(conn, got); err != nil || !bytes.Equal(got, large) {
		t.Fatal("bulk roundtrip failed:", err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestAdaptivePaddingServerMustOptIn(t *testing.T) {
	target := testDestination(t, func(c net.Conn) { _, _ = io.Copy(c, c) })
	client, addr := testClient(t, &Server{Username: "alice", Password: "secret", AllowPrivate: true})
	client.AdaptivePadding = true
	conn, code := connectSOCKS(t, addr, target)
	defer conn.Close()
	if code == 0 {
		t.Fatal("server accepted adaptive padding without opt-in")
	}
}
