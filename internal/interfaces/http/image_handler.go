package http

import (
	"context"
	"errors"
	"io"
	"mime"
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

		var reader io.ReadSeekCloser

		if raw {
			reader, err = os.Open(absPath)
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

			var file appimage.File
			file, err = imageProcessor.Process(r.Context(), absPath, width, quality)
			if err == nil {
				reader, err = file.Open()
			}
		}

		if errors.Is(err, context.Canceled) {
			http.Error(w, "request canceled", http.StatusRequestTimeout)
			return
		}
		if errors.Is(err, os.ErrNotExist) {
			http.Error(w, "image not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			logger.Error("process image", zap.Error(err))
			return
		}
		defer reader.Close()

		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		w.Header().Set("ETag", etag)

		// 使用 mime.FormatMediaType 安全格式化 Content-Disposition 响应头，防止头部注入并正确转义文件名
		filename := filepath.Base(relativePath)
		cd := mime.FormatMediaType("inline", map[string]string{
			"filename": filename,
		})
		w.Header().Set("Content-Disposition", cd)

		http.ServeContent(w, r, "", time.Now(), reader)
	}
}
