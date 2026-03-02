package proxy

import (
	"bytes"
	"compress/gzip"
	"io"
)

// CaptureReader wraps an io.Reader and stores the first maxSize bytes
// that pass through it. The underlying reader is read normally; the
// captured bytes are available via Bytes after reading is done.
type CaptureReader struct {
	r       io.Reader
	buf     []byte
	maxSize int
}

// NewCaptureReader returns a CaptureReader that stores up to maxSize bytes.
func NewCaptureReader(r io.Reader, maxSize int) *CaptureReader {
	return &CaptureReader{r: r, maxSize: maxSize}
}

func (cr *CaptureReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	if remaining := cr.maxSize - len(cr.buf); remaining > 0 && n > 0 {
		take := min(n, remaining)
		cr.buf = append(cr.buf, p[:take]...)
	}
	return n, err
}

// Bytes returns the captured bytes so far.
func (cr *CaptureReader) Bytes() []byte {
	return cr.buf
}

// DecompressGzip decompresses gzip data. If data is not gzip-encoded
// (detected by the two-byte magic header 0x1f 0x8b), it is returned unchanged.
func DecompressGzip(data []byte) []byte {
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return data
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return data
	}
	decoded, err := io.ReadAll(r)
	_ = r.Close()
	if err != nil {
		return data
	}
	return decoded
}
