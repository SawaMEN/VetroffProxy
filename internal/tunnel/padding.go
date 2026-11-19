package tunnel

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"github.com/SawaMEN/VetroffProxy/internal/i18n"
	"io"
	"net/http"
	"sync"
)

const (
	paddingHeader          = "X-Vetroff-Padding"
	paddingVersion         = "initial-v1"
	adaptivePaddingVersion = "adaptive-v1"
	initialPaddingRecords  = 8
	maxPaddingPayload      = 16 << 10
	maxPaddingBytes        = 255
	paddingBufferSize      = 3 + maxPaddingPayload + maxPaddingBytes
)

// Negotiation is inside authenticated HTTPS. This is Vetroff's own versioned
// experiment, not wire-level compatibility with NaiveProxy or a Firefox profile.
func paddingRequested(header http.Header) (bool, error) {
	values := header.Values(paddingHeader)
	if len(values) == 0 {
		return false, nil
	}
	if len(values) != 1 || values[0] != paddingVersion {
		return false, errors.New(i18n.Text("unsupported padding mode"))
	}
	return true, nil
}

// paddingModeRequested refuses duplicates, unknown modes and mixed-case header aliases.
// Mode selection is authenticated by the HTTPS transport and must be echoed.
func paddingModeRequested(h http.Header) (string, error) {
	mode, present, valid := protocolValue(h, paddingHeader)
	if !valid {
		return "", errors.New(i18n.Text("unsupported padding mode"))
	}
	if !present {
		return "", nil
	}
	if mode != paddingVersion && mode != adaptivePaddingVersion {
		return "", errors.New(i18n.Text("unsupported padding mode"))
	}
	return mode, nil
}

func (c *Client) paddingMode() string {
	if c.AdaptivePadding {
		return adaptivePaddingVersion
	}
	if c.Padding {
		return paddingVersion
	}
	return ""
}

func adaptivePaddingLimit(payloadLength int) int {
	return min(maxPaddingBytes, 24+payloadLength/8)
}

func newAdaptivePaddingWriter(w io.Writer) *paddingWriter {
	return &paddingWriter{writer: w, adaptive: true}
}

func newAdaptivePaddingReader(r io.Reader) *paddingReader {
	return &paddingReader{reader: r, adaptive: true}
}

var paddingBuffers = sync.Pool{New: func() any { return new([paddingBufferSize]byte) }}

// paddingWriter frames the first eight records in initial-v1 and every record
// in adaptive-v1. It has one writer, just like the H2 stream.
// A failed write is terminal: retrying a partially emitted frame is unsafe.
type paddingWriter struct {
	writer   io.Writer
	records  int
	adaptive bool
	err      error
}

func newPaddingWriter(w io.Writer) *paddingWriter {
	return &paddingWriter{writer: w, records: initialPaddingRecords}
}

func (w *paddingWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	if len(p) == 0 {
		return 0, nil
	}
	if w.records == 0 && !w.adaptive {
		return w.writeRaw(p)
	}
	buffer := paddingBuffers.Get().(*[paddingBufferSize]byte)
	defer paddingBuffers.Put(buffer)
	total := 0
	for len(p) > 0 && (w.records > 0 || w.adaptive) {
		// One random byte selects a padding length, without a persistent PRNG
		// seed. Randomness failure aborts rather than weakening the mode.
		if _, err := io.ReadFull(rand.Reader, buffer[2:3]); err != nil {
			w.err = err
			return total, err
		}
		n := min(len(p), maxPaddingPayload)
		pad := int(buffer[2])
		if w.adaptive {
			// Limit added traffic for large records; no unnecessary delays.
			pad %= adaptivePaddingLimit(n) + 1
			buffer[2] = byte(pad)
		}
		binary.BigEndian.PutUint16(buffer[:2], uint16(n))
		copy(buffer[3:3+n], p[:n])
		// A pooled buffer can contain another stream's plaintext. Never transmit it
		// as padding. Padding is zero-filled inside the encrypted TLS connection.
		clear(buffer[3+n : 3+n+pad])
		if err := writePaddingFrame(w.writer, buffer[:3+n+pad]); err != nil {
			w.err = err
			return total, err
		}
		total += n
		p = p[n:]
		if !w.adaptive {
			w.records--
		}
	}
	if len(p) > 0 {
		n, err := w.writeRaw(p)
		return total + n, err
	}
	return total, nil
}
func (w *paddingWriter) writeRaw(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if n < 0 || n > len(p) {
		n = 0
		err = errors.New(i18n.Text("invalid underlying write count"))
	}
	if n < len(p) && err == nil {
		err = io.ErrShortWrite
	}
	w.err = err
	return n, err
}
func writePaddingFrame(w io.Writer, p []byte) error {
	for len(p) > 0 {
		n, err := w.Write(p)
		if n < 0 || n > len(p) {
			return errors.New(i18n.Text("invalid underlying write count"))
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		p = p[n:]
	}
	return nil
}

// paddingReader verifies a complete bounded record before exposing its payload.
// EOF at a record boundary is a normal half-close; EOF inside any record is an
// error, including missing padding bytes. No padding-only records are accepted.
type paddingReader struct {
	reader   io.Reader
	records  int
	adaptive bool
	buffer   []byte
	pending  []byte
	err      error
}

func newPaddingReader(r io.Reader) *paddingReader {
	return &paddingReader{reader: r, records: initialPaddingRecords}
}
func (r *paddingReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.pending) > 0 {
		n := copy(p, r.pending)
		r.pending = r.pending[n:]
		r.releaseConsumedBuffer()
		return n, nil
	}
	if r.err != nil {
		return 0, r.err
	}
	if r.records == 0 && !r.adaptive {
		r.buffer = nil
		return r.reader.Read(p)
	}
	var header [3]byte
	if _, err := io.ReadFull(r.reader, header[:]); err != nil {
		r.err = err
		r.buffer = nil
		return 0, err
	}
	n := int(binary.BigEndian.Uint16(header[:2]))
	pad := int(header[2])
	if n == 0 || n > maxPaddingPayload || (r.adaptive && pad > adaptivePaddingLimit(n)) {
		r.err = errors.New(i18n.Text("invalid padding record length"))
		r.buffer = nil
		return 0, r.err
	}
	if r.buffer == nil {
		r.buffer = make([]byte, maxPaddingPayload+maxPaddingBytes)
	}
	if _, err := io.ReadFull(r.reader, r.buffer[:n+pad]); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		r.err = err
		r.buffer = nil
		return 0, err
	}
	r.pending = r.buffer[:n]
	if !r.adaptive {
		r.records--
	}
	count := copy(p, r.pending)
	r.pending = r.pending[count:]
	r.releaseConsumedBuffer()
	return count, nil
}

func (r *paddingReader) releaseConsumedBuffer() {
	if len(r.pending) == 0 {
		r.pending = nil
		if r.records == 0 && !r.adaptive {
			r.buffer = nil
		}
	}
}
