package main

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
)

type staticResponseWriter struct {
	w http.ResponseWriter
}

func (srw staticResponseWriter) Header() http.Header {
	return srw.w.Header()
}

func (srw staticResponseWriter) Write(b []byte) (int, error) {
	return srw.w.Write(b)
}

func (srw staticResponseWriter) WriteHeader(statusCode int) {
	srw.w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
	if statusCode == http.StatusOK || statusCode == http.StatusPartialContent {
		if strings.Contains(srw.w.Header().Get("Content-Type"), "text/html") {
			srw.w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		} else {
			srw.w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
	} else {
		srw.w.Header().Set("Cache-Control", "no-store")
	}
	srw.w.WriteHeader(statusCode)
}

func weakETag(stat fs.FileInfo) string {
	return fmt.Sprintf(`W/"%x-%x"`, stat.ModTime().Unix(), stat.Size())
}

func serveIndex(w http.ResponseWriter, r *http.Request, frontendDir string) {
	w.Header().Set("Cross-Origin-Resource-Policy", "cross-origin")
	w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")

	indexPath := filepath.Join(frontendDir, "index.html")
	f, err := os.Open(indexPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to open index.html: %v", err), http.StatusInternalServerError)
		return
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to stat index.html: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("ETag", weakETag(stat))
	http.ServeContent(w, r, "index.html", stat.ModTime(), f)
}

func addStaticRoutes(r *mux.Router, frontendDir string) {
	// Create static file handler with custom response writer
	staticHandler := http.FileServer(http.Dir(frontendDir))
	customStaticHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		srw := staticResponseWriter{w: w}
		staticHandler.ServeHTTP(srw, r)
	})

	// Serve static files from /static prefix
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", customStaticHandler))

	// Serve specific static files
	r.Handle("/favicon.ico", customStaticHandler)
	r.Handle("/sw.js", customStaticHandler)
	r.Handle("/sw.js.map", customStaticHandler)
	r.Handle("/manifest.webmanifest", customStaticHandler)

	// Catch-all route for single-page application
	r.PathPrefix("/").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if the requested path exists as a file
		reqPath := filepath.Join(frontendDir, strings.TrimPrefix(r.URL.Path, "/"))
		if _, err := os.Stat(reqPath); err == nil {
			// File exists, serve it
			srw := staticResponseWriter{w: w}
			staticHandler.ServeHTTP(srw, r)
			return
		}

		// File doesn't exist, serve index.html for SPA routing
		serveIndex(w, r, frontendDir)
	})
}