package http

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"strconv"

	"main/internal/infrastructure/urlconv"

	"go.uber.org/zap"
)

type ImageProcessor interface {
	Process(ctx context.Context, path string, width int, quality int) (string, error)
}

func handleImage(
	logger *zap.Logger,
	signer *urlconv.Signer,
	imageProcessor ImageProcessor,
	absRootDir string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		relativePath := query.Get("path")
		timestamp := query.Get("t")
		size := query.Get("s")
		signature := query.Get("sig")
		widthStr := query.Get("w")
		qualityStr := query.Get("q")
		raw := query.Has("raw")

		if relativePath == "" || timestamp == "" || size == "" || signature == "" {
			http.Error(w, "missing required parameters", http.StatusBadRequest)
			return
		}

		err := signer.ValidateRequestFromValues(query)
		if err != nil {
			http.Error(w, "invalid signature: "+err.Error(), http.StatusForbidden)
			return
		}

		absPath := filepath.Join(absRootDir, relativePath)
		if raw {
			http.ServeFile(w, r, absPath)
			return
		}

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

		processedPath, err := imageProcessor.Process(r.Context(), absPath, width, quality)
		if errors.Is(err, context.Canceled) {
			http.Error(w, "request canceled", http.StatusRequestTimeout)
			return
		}
		if err != nil {
			logger.Error("process image", zap.Error(err))
			http.ServeFile(w, r, absPath)
			return
		}

		http.ServeFile(w, r, processedPath)
	}
}
