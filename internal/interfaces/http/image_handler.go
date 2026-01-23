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
		// 为不支持 Cache-Control: immutable 的浏览器(Chrome) 提供 304 响应
		// 缓存条目时会规范要求按 URL 隔离，所以 ETag 不需全局唯一，不考虑错误客户端
		const etag = `"immutable"`
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

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

		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("ETag", etag)
		http.ServeFile(w, r, processedPath)
	}
}
