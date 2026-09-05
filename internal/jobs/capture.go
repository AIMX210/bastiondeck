package jobs

import (
	"os"
	"sync"
)

// cappedWriter writes output to disk while retaining an in-memory ring of the
// last previewBytes bytes and enforcing a total byte cap.
type cappedWriter struct {
	path    string
	f       *os.File
	max     int64
	written int64
	preview []byte
	keep    int
	mu      sync.Mutex
	closed  bool
}

func newCappedWriter(path string, max int64) *cappedWriter {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		f = nil
	}
	return &cappedWriter{path: path, f: f, max: max, keep: 2048}
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	w.written += int64(n)
	if w.f != nil && (w.max <= 0 || w.written <= w.max) {
		_, _ = w.f.Write(p)
	}
	w.preview = appendRing(w.preview, p, w.keep)
	return n, nil
}

// appendRing keeps only the last keep bytes across chunks.
func appendRing(ring, p []byte, keep int) []byte {
	ring = append(ring, p...)
	if len(ring) > keep {
		ring = ring[len(ring)-keep:]
	}
	return ring
}

// Preview returns the last bytes captured.
func (w *cappedWriter) Preview() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return string(w.preview)
}

// Bytes returns total bytes observed.
func (w *cappedWriter) Bytes() int64 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.written
}

// Close flushes the file.
func (w *cappedWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed || w.f == nil {
		return nil
	}
	w.closed = true
	return w.f.Close()
}
