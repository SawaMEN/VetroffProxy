package tunnel

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"
)

func TestTimingJitterDelayBudget(t *testing.T) {
	maximum := 5 * time.Millisecond
	if timingJitterDelay(0, maximum) != 0 || timingJitterDelay(3, maximum) != maximum {
		t.Fatal("jitter endpoints do not match the configured budget")
	}
	for i := 0; i < 256; i++ {
		delay := timingJitterDelay(byte(i), maximum)
		if delay < 0 || delay > maximum {
			t.Fatalf("jitter sample %d exceeded budget: %v", i, delay)
		}
	}
}

func TestTimingJitterCancelsPromptly(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if err := waitTimingJitter(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled writer did not stop:", err)
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatal("cancelled jitter waited too long")
	}
	if err := waitTimingJitter(ctx, 0); err != nil {
		t.Fatalf("disabled jitter should not wait: %v", err)
	}
}

func TestTimedAdaptivePaddingWriterPreservesPayload(t *testing.T) {
	var encoded, source bytes.Buffer
	writer := newTimedAdaptivePaddingWriter(&encoded, context.Background(), 2*time.Millisecond)
	for i := 0; i < 20; i++ {
		data := bytes.Repeat([]byte{byte(i)}, i+1)
		source.Write(data)
		if n, err := writer.Write(data); err != nil || n != len(data) {
			t.Fatalf("small frame %d: n=%d err=%v", i, n, err)
		}
	}
	large := bytes.Repeat([]byte{0xab}, 20<<10)
	source.Write(large)
	if n, err := writer.Write(large); err != nil || n != len(large) {
		t.Fatalf("large frame: n=%d err=%v", n, err)
	}
	got, err := io.ReadAll(newAdaptivePaddingReader(&encoded))
	if err != nil || !bytes.Equal(got, source.Bytes()) {
		t.Fatalf("timed adaptive padding roundtrip failed: %v", err)
	}
}
