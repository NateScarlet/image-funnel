package http

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	appimage "main/internal/application/image"
	"main/internal/infrastructure/urlconv"

	"go.uber.org/zap"
)

func handleImage(
	logger *zap.Logger,
	signer *urlconv.Signer,
	imageProcessor appimage.Processor,
	absRootDir string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const etag = `"immutable"`
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		query := r.URL.Query()
		relativePath := query.Get("path")
		widthStr := query.Get("w")
		qualityStr := query.Get("q")
		raw := query.Has("raw")

		err := signer.ValidateRequestFromValues(query)
		if err != nil {
			http.Error(w, "invalid signature: "+err.Error(), http.StatusForbidden)
			return
		}

		absPath := filepath.Join(absRootDir, relativePath)

		var file io.ReadSeekCloser

		if raw {
			file, err = os.Open(absPath)
		} else {
			width := 0
			if widthStr != "" {
				if w, err := strconv.Atoi(widthStr); err == nil {
					width = w
				}
			}

			quality := 0
			if qualityStr != "" {
				if q, err := strconv.Atoi(qualityStr); err == nil {
					quality = q
				}
			}

			file, err = imageProcessor.Process(r.Context(), absPath, width, quality)
		}

		if errors.Is(err, context.Canceled) {
			http.Error(w, "request canceled", http.StatusRequestTimeout)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			logger.Error("process image", zap.Error(err))
			return
		}
		defer file.Close()

		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("ETag", etag)

		http.ServeContent(w, r, "", time.Now(), file)
	}
}
