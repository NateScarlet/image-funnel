package util

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestDetectContentType_ImageFormats(t *testing.T) {
	tests := []struct {
		name         string
		data         []byte
		filename     string
		expectedType string
	}{
		{
			name:         "JPEG",
			data:         []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00},
			filename:     "test.jpg",
			expectedType: "image/jpeg",
		},
		{
			name:         "PNG",
			data:         []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			filename:     "test.png",
			expectedType: "image/png",
		},
		{
			name:         "GIF",
			data:         []byte{0x47, 0x49, 0x46, 0x38, 0x39, 0x61},
			filename:     "test.gif",
			expectedType: "image/gif",
		},
		{
			name:         "WebP",
			data:         []byte{0x52, 0x49, 0x46, 0x46, 0x00, 0x00, 0x00, 0x00, 0x57, 0x45, 0x42, 0x50},
			filename:     "test.webp",
			expectedType: "image/webp",
		},
		{
			name:         "AVIF",
			data:         []byte{0x00, 0x00, 0x00, 0x20, 0x66, 0x74, 0x79, 0x70, 0x61, 0x76, 0x69, 0x66},
			filename:     "test.avif",
			expectedType: "image/avif",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := bytes.NewReader(tt.data)
			contentType, reader, err := DetectContentType(r, tt.filename)
			if err != nil {
				t.Fatalf("DetectContentType error: %v", err)
			}
			if contentType != tt.expectedType {
				t.Errorf("expected content type %q, got %q", tt.expectedType, contentType)
			}

			// Verify reader replays the data correctly
			allData, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("reading replayed reader: %v", err)
			}
			if !bytes.Equal(allData, tt.data) {
				t.Errorf("replayed data mismatch: got %d bytes, want %d", len(allData), len(tt.data))
			}
		})
	}
}

func TestDetectContentType_ExtensionFallback(t *testing.T) {
	// Generic binary data that sniffing can't identify
	data := []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05}

	tests := []struct {
		name         string
		filename     string
		expectedType string
	}{
		{"unknown extension", "test.bin", "application/octet-stream"},
		{"jpg extension", "test.jpg", "image/jpeg"},
		{"png extension", "test.png", "image/png"},
		{"webp extension", "test.webp", "image/webp"},
		{"avif extension", "test.avif", "image/avif"},
		{"json extension (text/plain fix)", "test.json", "application/json"},
		{"yaml extension (unknown binary -> octet-stream)", "test.yaml", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := bytes.NewReader(data)
			contentType, reader, err := DetectContentType(r, tt.filename)
			if err != nil {
				t.Fatalf("DetectContentType error: %v", err)
			}
			if contentType != tt.expectedType {
				t.Errorf("expected content type %q, got %q", tt.expectedType, contentType)
			}

			// Verify reader replays correctly
			allData, err := io.ReadAll(reader)
			if err != nil {
				t.Fatalf("reading replayed reader: %v", err)
			}
			if !bytes.Equal(allData, data) {
				t.Errorf("replayed data mismatch")
			}
		})
	}
}

func TestDetectContentType_EmptyReader(t *testing.T) {
	r := strings.NewReader("")
	contentType, reader, err := DetectContentType(r, "test.txt")
	if err != nil {
		t.Fatalf("DetectContentType error: %v", err)
	}
	// Empty reader returns text/plain from http.DetectContentType
	// then falls back to extension (none for .txt), so stays text/plain
	if contentType != "text/plain; charset=utf-8" {
		t.Errorf("expected text/plain for empty, got %q", contentType)
	}

	allData, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading replayed reader: %v", err)
	}
	if len(allData) != 0 {
		t.Errorf("expected empty replayed data, got %d bytes", len(allData))
	}
}

func TestDetectContentType_ShortReader(t *testing.T) {
	// Less than 512 bytes - http.DetectContentType returns text/plain
	// but we should fall back to filename extension
	data := []byte{0x89, 0x50, 0x4E, 0x47} // PNG header only
	r := bytes.NewReader(data)
	contentType, reader, err := DetectContentType(r, "test.png")
	if err != nil {
		t.Fatalf("DetectContentType error: %v", err)
	}
	// Should fall back to .png extension -> image/png
	if contentType != "image/png" {
		t.Errorf("expected image/png for short PNG with .png filename, got %q", contentType)
	}

	allData, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("reading replayed reader: %v", err)
	}
	if !bytes.Equal(allData, data) {
		t.Errorf("replayed data mismatch")
	}
}