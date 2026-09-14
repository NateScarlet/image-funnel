package util

import (
	"bytes"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

// DetectContentType reads up to 512 bytes from r for MIME sniffing,
// then returns the detected content type and a reader that replays
// the sniffed bytes followed by the rest of r.
// This avoids re-opening the source when the caller already has an open handle.
func DetectContentType(r io.Reader, filename string) (string, io.Reader, error) {
	buf := make([]byte, 512)
	n, err := io.ReadFull(r, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return "", nil, err
	}

	// If we read less than 512 bytes, http.DetectContentType may not work reliably.
	// For image files this shouldn't happen in practice, but we handle it.
	contentType := http.DetectContentType(buf[:n])

	// If sniffing returns text/plain (likely due to insufficient data),
	// try to infer from filename extension
	if contentType == "text/plain; charset=utf-8" {
		if ext := filepath.Ext(filename); ext != "" {
			if extType := mime.TypeByExtension(strings.ToLower(ext)); extType != "" {
				contentType = extType
			}
		}
	}

	contentReader := io.MultiReader(bytes.NewReader(buf[:n]), r)

	// Fallback to filename extension when sniffing is generic
	if ext := extForContentType(contentType, filename); ext != "" {
		if extType := mime.TypeByExtension(ext); extType != "" {
			return extType, contentReader, nil
		}
	}

	return contentType, contentReader, nil
}

// extForContentType returns the file extension if it can improve
// the detected content type (e.g., when sniffing returns generic types).
func extForContentType(detectedType, filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	if ext == "" {
		return ""
	}

	switch {
	case detectedType == "application/octet-stream":
		// When sniffing is generic but filename has specific extension
		return ext
	case strings.HasPrefix(detectedType, "text/plain"):
		// Text files need extension correction
		// Though for media uploads this is unlikely
		switch ext {
		case ".js", ".md", ".yaml", ".yml", ".css", ".csv", ".tsv", ".xml", ".html", ".json":
			return ext
		}
		return ""
	default:
		// Non-generic types trust sniffing result
		return ""
	}
}