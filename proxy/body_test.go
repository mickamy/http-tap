package proxy_test

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/mickamy/http-tap/proxy"
)

func TestCaptureReader_CapturesUpToMax(t *testing.T) {
	t.Parallel()

	data := strings.Repeat("a", 100)
	cr := proxy.NewCaptureReader(strings.NewReader(data), 30)

	got, err := io.ReadAll(cr)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != data {
		t.Errorf("Read returned %d bytes, want %d", len(got), len(data))
	}
	if len(cr.Bytes()) != 30 {
		t.Errorf("captured %d bytes, want 30", len(cr.Bytes()))
	}
	if string(cr.Bytes()) != strings.Repeat("a", 30) {
		t.Errorf("captured content mismatch")
	}
}

func TestCaptureReader_ShortData(t *testing.T) {
	t.Parallel()

	data := "hello"
	cr := proxy.NewCaptureReader(strings.NewReader(data), 1024)

	got, err := io.ReadAll(cr)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != data {
		t.Errorf("Read = %q, want %q", got, data)
	}
	if string(cr.Bytes()) != data {
		t.Errorf("Bytes = %q, want %q", cr.Bytes(), data)
	}
}

func TestCaptureReader_Empty(t *testing.T) {
	t.Parallel()

	cr := proxy.NewCaptureReader(strings.NewReader(""), 1024)

	got, err := io.ReadAll(cr)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("Read returned %d bytes, want 0", len(got))
	}
	if len(cr.Bytes()) != 0 {
		t.Errorf("captured %d bytes, want 0", len(cr.Bytes()))
	}
}

func TestCaptureReader_SmallReads(t *testing.T) {
	t.Parallel()

	data := "abcdefghij"
	cr := proxy.NewCaptureReader(strings.NewReader(data), 5)

	buf := make([]byte, 3)
	var total []byte
	for {
		n, err := cr.Read(buf)
		total = append(total, buf[:n]...)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if string(total) != data {
		t.Errorf("Read = %q, want %q", total, data)
	}
	if string(cr.Bytes()) != "abcde" {
		t.Errorf("Bytes = %q, want %q", cr.Bytes(), "abcde")
	}
}

func gzipEncode(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDecompressGzip_ValidGzip(t *testing.T) {
	t.Parallel()

	original := []byte("hello gzip world")
	compressed := gzipEncode(t, original)

	got := proxy.DecompressGzip(compressed)
	if !bytes.Equal(got, original) {
		t.Errorf("DecompressGzip = %q, want %q", got, original)
	}
}

func TestDecompressGzip_NotGzip(t *testing.T) {
	t.Parallel()

	data := []byte(`{"key":"value"}`)
	got := proxy.DecompressGzip(data)
	if !bytes.Equal(got, data) {
		t.Errorf("DecompressGzip modified non-gzip data")
	}
}

func TestDecompressGzip_Empty(t *testing.T) {
	t.Parallel()

	got := proxy.DecompressGzip(nil)
	if got != nil {
		t.Errorf("DecompressGzip(nil) = %v, want nil", got)
	}
}

func TestDecompressGzip_TooShort(t *testing.T) {
	t.Parallel()

	got := proxy.DecompressGzip([]byte{0x1f})
	if !bytes.Equal(got, []byte{0x1f}) {
		t.Errorf("DecompressGzip modified single-byte data")
	}
}

func TestDecompressGzip_InvalidGzipHeader(t *testing.T) {
	t.Parallel()

	// Starts with gzip magic but is truncated/corrupted.
	data := []byte{0x1f, 0x8b, 0x00, 0x00}
	got := proxy.DecompressGzip(data)
	if !bytes.Equal(got, data) {
		t.Errorf("DecompressGzip should return original data on invalid gzip")
	}
}
