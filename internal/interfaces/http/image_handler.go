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
	"main/internal/util"

	"go.uber.org/zap"
)

// isHEAD 请求方法是否为 HEAD。前端预载用 HEAD 登记转码需求（等待就绪），
// GET 取回内容；两者共用同一签名 URL
func isHEAD(r *http.Request) bool { return r.Method == http.MethodHead }

// formatDecision 格式决策结果
type formatDecision struct {
	format       appimage.ImageFormat // 0 表示返回原图
	serveOriginal bool
	contentType  string // 原图的 MIME 类型（仅当 serveOriginal=true 时有效）
	sourceWidth  int    // 源图宽度（用于缓存键计算）
}

func handleImage(
	logger *zap.Logger,
	signer *urlconv.Signer,
	coordinator *appimage.TranscodeCoordinator,
	absRootDir string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 所有响应都加上 Vary: Accept，保证缓存正确性（含错误响应）
		w.Header().Set("Vary", "Accept")

		const etag = `"immutable"`
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		query := r.URL.Query()
		relativePath := query.Get("path")
		widthStr := query.Get("w")
		raw := query.Has("raw")

		// 不再支持 fmt 参数；改由 Accept 头协商
		if query.Has("fmt") {
			http.Error(w, "format parameter (fmt) is no longer supported; use Accept header", http.StatusBadRequest)
			return
		}

		// 不再支持 q 参数：画质由服务端编码器配置决定（AVIF 见 svtav1CRF、WebP 见 webpQuality），
		// 客户端指定画质既无意义（编码器已定档）又会让同一张图因参数不同重复编码
		if query.Has("q") {
			http.Error(w, "quality parameter (q) is no longer supported; quality is configured server-side", http.StatusBadRequest)
			return
		}

		// raw=true 不能与 w 同时使用
		if raw && widthStr != "" {
			http.Error(w, "raw parameter cannot be combined with width", http.StatusBadRequest)
			return
		}

		err := signer.ValidateRequestFromValues(query)
		if err != nil {
			http.Error(w, "invalid signature: "+err.Error(), http.StatusForbidden)
			return
		}

		absPath := filepath.Join(absRootDir, relativePath)

		// 读取源图元数据（宽度、高度），用于决定是否需要缩放
		meta, err := coordinator.Meta(r.Context(), absPath)
		if err != nil {
			handleAcquireError(w, logger, err)
			return
		}
		sourceWidth := meta.Width

		width := 0
		if widthStr != "" {
			if w, err := strconv.Atoi(widthStr); err == nil {
				width = w
			}
		}

		// 根据 Accept 头决定格式
		decision := decideFormat(r, absPath, relativePath, width, sourceWidth)

		if decision.serveOriginal || raw {
			// 直接返回原图（raw=true 或 无需转码时）
			file, err := os.Open(absPath)
			if err != nil {
				handleAcquireError(w, logger, err)
				return
			}
			defer file.Close()

			// 使用共享读取器检测 MIME 类型（读取前 512 字节）
			contentType, _, err := util.DetectContentType(file, relativePath)
			if err != nil {
				handleAcquireError(w, logger, err)
				return
			}
			// 回退到文件开头用于服务
			if _, err := file.Seek(0, io.SeekStart); err != nil {
				handleAcquireError(w, logger, err)
				return
			}
			serveOriginal(w, r, contentType, relativePath, file)
			return
		}

		// 需要转码：创建 Spec 并获取变体
		spec, err := appimage.NewSpec(width, decision.format)
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
		serveVariant(w, r, decision.format, relativePath, reader)
	}
}

// decideFormat 根据 Accept 头、请求参数和源图信息决定返回格式
// 返回的 formatDecision 中如果 serveOriginal=true，则 contentType 为源图 MIME
func decideFormat(r *http.Request, absPath, relativePath string, width, sourceWidth int) formatDecision {
	accept := r.Header.Get("Accept")
	preferredFormats := util.PreferredImageFormats(accept)

	// 是否需要缩放：仅当显式指定宽度且小于源图宽度时才需要。
	// 画质不参与该判断——转码与否本质是分辨率问题，画质由编码器统一配置
	needsResize := width > 0 && width < sourceWidth

	// 无需缩放时尝试返回原图，前提是源图 MIME 在客户端接受范围内
	if !needsResize {
		// 检测源图 MIME 类型
		file, err := os.Open(absPath)
		if err == nil {
			contentType, _, err := util.DetectContentType(file, relativePath)
			file.Close()
			if err == nil {
				// 直接解析 Accept 头检查源图 MIME 是否被接受（不限于支持的输出格式）
				acceptedTypes := util.ParseAcceptHeader(accept)
				for _, at := range acceptedTypes {
					if at.Type == contentType || at.Type == "image/*" || at.Type == "*/*" {
						return formatDecision{
							serveOriginal: true,
							contentType:   contentType,
							sourceWidth:   sourceWidth,
						}
					}
				}
			}
		}
	}

	// 需要转码：根据 Accept 优先级选择格式
	for _, pf := range preferredFormats {
		switch pf {
		case "image/avif":
			return formatDecision{format: appimage.ImageFormatAVIF, sourceWidth: sourceWidth}
		case "image/webp":
			return formatDecision{format: appimage.ImageFormatWebP, sourceWidth: sourceWidth}
		}
	}

	// 默认 WebP
	return formatDecision{format: appimage.ImageFormatWebP, sourceWidth: sourceWidth}
}

// serveOriginal 返回原始图片文件
func serveOriginal(w http.ResponseWriter, r *http.Request, contentType, relativePath string, reader io.ReadSeeker) {
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", `"immutable"`)
	w.Header().Set("Content-Type", contentType)

	filename := filepath.Base(relativePath)
	cd := mime.FormatMediaType("inline", map[string]string{
		"filename": filename,
	})
	w.Header().Set("Content-Disposition", cd)

	http.ServeContent(w, r, "", time.Now(), reader)
}

// serveVariant 写出变体产物响应头与内容
func serveVariant(w http.ResponseWriter, r *http.Request, format appimage.ImageFormat, relativePath string, reader io.ReadSeekCloser) {
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", `"immutable"`)

	// 根据转码格式显式设置 Content-Type
	switch format {
	case appimage.ImageFormatAVIF:
		w.Header().Set("Content-Type", "image/avif")
	default:
		w.Header().Set("Content-Type", "image/webp")
	}

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