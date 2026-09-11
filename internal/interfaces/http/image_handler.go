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

// isHEAD 请求方法是否为 HEAD。前端预载用 HEAD 登记转码需求（等待就绪），
// GET 取回内容；两者共用同一签名 URL
func isHEAD(r *http.Request) bool { return r.Method == http.MethodHead }

func handleImage(
	logger *zap.Logger,
	signer *urlconv.Signer,
	coordinator *appimage.TranscodeCoordinator,
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
		formatStr := query.Get("fmt")
		raw := query.Has("raw")

		err := signer.ValidateRequestFromValues(query)
		if err != nil {
			http.Error(w, "invalid signature: "+err.Error(), http.StatusForbidden)
			return
		}

		// 解析格式参数，空值默认 WebP
		format := appimage.ImageFormatWebP
		switch formatStr {
		case "avif":
			format = appimage.ImageFormatAVIF
		case "webp", "":
			// 默认 WebP
		default:
			http.Error(w, "unsupported format: "+formatStr, http.StatusBadRequest)
			return
		}

		absPath := filepath.Join(absRootDir, relativePath)

		if raw {
			reader, err := os.Open(absPath)
			if err != nil {
				handleAcquireError(w, logger, err)
				return
			}
			defer reader.Close()
			serveVariant(w, r, appimage.ImageFormatWebP, relativePath, reader)
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

		spec, err := appimage.NewSpec(width, quality, format)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// HEAD = 登记需求并等待就绪（低优先级）；GET = 用户当前查看（高优先级）。
		// 两者经由同一缓存与队列，GET 可搭乘进行中的同一转码
		priority := appimage.PrioHigh
		if isHEAD(r) {
			priority = appimage.PrioLow
		}

		file, err := coordinator.Acquire(r.Context(), absPath, spec, priority)
		if err != nil {
			handleAcquireError(w, logger, err)
			return
		}
		reader, err := file.Open()
		if err != nil {
			handleAcquireError(w, logger, err)
			return
		}
		defer reader.Close()
		serveVariant(w, r, format, relativePath, reader)
	}
}

// serveVariant 写出变体产物响应头与内容
func serveVariant(w http.ResponseWriter, r *http.Request, format appimage.ImageFormat, relativePath string, reader io.ReadSeekCloser) {
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", `"immutable"`)

	// 根据请求格式显式设置 Content-Type，不依赖内容嗅探（嗅探无法识别 AVIF）
	switch format {
	case appimage.ImageFormatAVIF:
		w.Header().Set("Content-Type", "image/avif")
	default:
		w.Header().Set("Content-Type", "image/webp")
	}

	// 使用 mime.FormatMediaType 安全格式化 Content-Disposition 响应头，防止头部注入并正确转义文件名
	filename := filepath.Base(relativePath)
	cd := mime.FormatMediaType("inline", map[string]string{
		"filename": filename,
	})
	w.Header().Set("Content-Disposition", cd)

	http.ServeContent(w, r, "", time.Now(), reader)
}

// handleAcquireError 统一映射获取变体产物的已知错误到 HTTP 状态码
func handleAcquireError(w http.ResponseWriter, logger *zap.Logger, err error) {
	if errors.Is(err, context.Canceled) {
		http.Error(w, "request canceled", http.StatusRequestTimeout)
		return
	}
	if errors.Is(err, os.ErrNotExist) {
		http.Error(w, "image not found", http.StatusNotFound)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
	logger.Error("process image", zap.Error(err))
}
