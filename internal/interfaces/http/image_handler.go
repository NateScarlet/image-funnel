package http

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	appimage "main/internal/application/image"
	"main/internal/domain/relpath"
	"main/internal/infrastructure/urlconv"
	"main/internal/shared/transcode"
	"main/internal/util"

	"go.uber.org/zap"
)

// isHEAD 请求方法是否为 HEAD。前端预载用 HEAD 登记转码需求（等待就绪），
// GET 取回内容；两者共用同一签名 URL
func isHEAD(r *http.Request) bool { return r.Method == http.MethodHead }

// formatDecision 一次请求的响应形态：重定向到 raw URL、重定向到 w=源宽 的规范 URL，
// 或转码为 format 指定的格式
type formatDecision struct {
	redirectToRaw  bool
	canonicalWidth int // >0 表示应 302 到 w=该宽度 的规范 URL
	format         transcode.ImageFormat
}

func handleImage(
	logger *zap.Logger,
	signer *urlconv.Signer,
	coordinator *appimage.TranscodeCoordinator,
	probe appimage.SourceProbe,
	absRootDir string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// 验签先于一切响应分支：签名是 /image 的唯一信任边界（该端点无登录态），
		// 包括 304 短路在内都不得绕过它。
		//
		// 必须使用 RequestURI（线路上的原始目标串）：URL.Path 已解码，无法区分 %2F 与 /，
		// 而签名覆盖的是原始字形；RequestURI 不经过 url.Values 往返，故参数顺序与
		// ?a ／ ?a= 等区别都被完整保留。
		//
		// 返回的根相对路径自带「位于根目录内」的保证（围栏在验签时由返回类型构造完成），
		// 故此处无需再自行复核，越界载荷在验签阶段即已失败。
		//
		// 验签同时核对 URL 声明的图片身份（t/s）与磁盘现状：授权绑定的是签发时的
		// 那个文件版本，同路径文件被替换后旧 URL 即失效。该比对先于 304 短路、
		// 源图读取、变体缓存查询与转码登记，过期身份不会产生任何转码工作。
		relPath, err := signer.ValidateSignedURL(r.RequestURI)
		if err != nil {
			switch {
			// 过期身份与文件不存在对客户端同为 404：不透露「该路径曾经有效」
			case errors.Is(err, urlconv.ErrStaleIdentity), errors.Is(err, os.ErrNotExist):
				http.Error(w, "image not found", http.StatusNotFound)
			case errors.Is(err, urlconv.ErrInvalidSignature):
				// 不向客户端回显失败原因：细分原因会给探测者反馈，且具体错误对排障无价值
				http.Error(w, "invalid signature", http.StatusForbidden)
			default:
				// stat 的其他内部错误走既有映射，不得伪装成 404
				handleAcquireError(w, logger, err)
			}
			return
		}

		// 分段耗时随响应头返回；命中状态与编码耗时由协调器给出，解码耗时在各分支就地测量
		timing := ServerTiming{Encoder: coordinator.EncoderID()}
		setTimingHeader := func() {
			w.Header().Set("Server-Timing", FormatServerTiming(timing))
		}

		// 参数解析必须先于 304 短路：Vary 是否设置取决于 raw 是否出现
		query := r.URL.Query()
		raw := query.Has("raw")
		hasWidth := query.Has("w")

		// raw 直接返回源文件，内容与 Accept 无关，因此不参与内容协商缓存的分叉
		if !raw {
			w.Header().Set("Vary", "Accept")
		}

		const etag = `"immutable"`
		if r.Header.Get("If-None-Match") == etag {
			setTimingHeader()
			w.WriteHeader(http.StatusNotModified)
			return
		}

		// raw 与 width 是两个已签名参数的语义矛盾（宽高变体 vs 原图），
		// 属生成侧错误，静默取舍会掩盖 bug
		if raw && hasWidth {
			http.Error(w, "raw parameter cannot be combined with width", http.StatusBadRequest)
			return
		}

		absPath := relPath.Abs(absRootDir)

		// raw 是唯一直接返回全尺寸字节的入口：一次打开并嗅探 MIME 即可服务，
		// 不读取宽度（源图尺寸能否解码与 raw 无关）
		if raw {
			decodeStart := time.Now()
			source, err := probe.Open(absPath)
			timing.DecodeMs = durationMs(time.Since(decodeStart))
			if err != nil {
				handleAcquireError(w, logger, err)
				return
			}
			defer source.Reader.Close()
			setTimingHeader()
			serveOriginal(w, r, source.ContentType, relPath.Base(), source.Reader)
			return
		}

		// 未指定 w 表示全尺寸诉求（width=0）；显式的 0、负数与非数字都是错误，
		// 不得像旧实现那样静默退化为「返回原图」
		width := 0
		if hasWidth {
			width, err = strconv.Atoi(query.Get("w"))
			if err != nil || width <= 0 {
				http.Error(w, "width parameter must be a positive integer", http.StatusBadRequest)
				return
			}
		}

		accept := r.Header.Get("Accept")
		format := negotiateFormat(accept)

		// w>0 时先只查变体缓存：命中即服务，整个过程不打开源图
		if width > 0 {
			spec, err := transcode.NewSpec(width, format)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			file, variantTiming, err := coordinator.CachedVariant(r.Context(), absPath, spec)
			if err != nil {
				handleAcquireError(w, logger, err)
				return
			}
			if file != nil {
				timing.CacheHit = variantTiming.CacheHit
				setTimingHeader()
				if err := serveVariantFile(w, r, format, relPath.Base(), file); err != nil {
					handleAcquireError(w, logger, err)
				}
				return
			}
		}

		// 缓存未命中（或未指定宽度）：一次打开源图，同时取得宽度与 MIME
		decodeStart := time.Now()
		source, err := probe.Probe(r.Context(), absPath)
		timing.DecodeMs = durationMs(time.Since(decodeStart))
		if err != nil {
			handleAcquireError(w, logger, err)
			return
		}
		defer source.Reader.Close()

		decision := decideFormat(accept, width, source.Width, util.Accepts(accept, source.ContentType))
		if decision.redirectToRaw {
			setTimingHeader()
			if err := redirectToRaw(w, signer, relPath, query); err != nil {
				handleAcquireError(w, logger, err)
			}
			return
		}
		// 源图 MIME 不被接受时，w 超过源宽的全尺寸诉求收敛到 w=源宽 的规范 URL。
		// 该重定向只在探源未命中变体缓存后发生，命中路径不会走到这里
		if decision.canonicalWidth > 0 {
			setTimingHeader()
			if err := redirectToCanonicalWidth(w, signer, relPath, query, decision.canonicalWidth); err != nil {
				handleAcquireError(w, logger, err)
			}
			return
		}

		// 有效宽度取 min(w, 源宽)：源图更窄时不放大，也让所有 w >= 源宽的请求
		// 收敛到同一份缓存产物。宽度未知（0）时保持 w，由编码器自身的上限保证不放大
		effectiveWidth := width
		if width > 0 && source.Width > 0 && width > source.Width {
			effectiveWidth = source.Width
		}

		spec, err := transcode.NewSpec(effectiveWidth, decision.format)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// HEAD = 登记需求并等待就绪（低优先级）；GET = 用户当前查看（高优先级）。
		// 两者经由同一缓存与队列，GET 可搭乘进行中的同一转码
		priority := transcode.PrioHigh
		if isHEAD(r) {
			priority = transcode.PrioLow
		}

		file, variantTiming, err := coordinator.Acquire(r.Context(), absPath, spec, priority)
		if err != nil {
			handleAcquireError(w, logger, err)
			return
		}
		timing.CacheHit = variantTiming.CacheHit
		timing.QueueMs = durationMs(variantTiming.QueueDuration)
		timing.EncodeMs = durationMs(variantTiming.EncodeDuration)
		setTimingHeader()
		if err := serveVariantFile(w, r, decision.format, relPath.Base(), file); err != nil {
			handleAcquireError(w, logger, err)
		}
	}
}

// decideFormat 根据请求宽度、源图宽度与源图 MIME 是否被接受，决定响应形态。
// width=0 表示未指定宽度（全尺寸诉求）；sourceWidth=0 表示源图宽度不可知，
// 此时无法证明请求已覆盖全尺寸，只能转码
func decideFormat(accept string, width, sourceWidth int, sourceAcceptable bool) formatDecision {
	fullSize := width == 0 || (sourceWidth > 0 && width >= sourceWidth)
	if fullSize && sourceAcceptable {
		return formatDecision{redirectToRaw: true}
	}
	// 源图 MIME 不被接受时，全尺寸产物与 w=源宽 的转码产物字节相同；
	// 让 w 严格大于源宽的请求 302 到该规范 URL，客户端便只占一个缓存条目。
	// 严格大于（而非 >=）避免规范 URL 自重定向；源宽不可知（0）不重定向，保持按 w 转码
	if !sourceAcceptable && sourceWidth > 0 && width > sourceWidth {
		return formatDecision{canonicalWidth: sourceWidth}
	}
	return formatDecision{format: negotiateFormat(accept)}
}

// negotiateFormat 根据 Accept 优先级选择转码目标格式
func negotiateFormat(accept string) transcode.ImageFormat {
	for _, pf := range util.PreferredImageFormats(accept) {
		switch pf {
		case "image/avif":
			return transcode.ImageFormatAVIF
		case "image/webp":
			return transcode.ImageFormatWebP
		}
	}

	// 默认 WebP
	return transcode.ImageFormatWebP
}

// redirectSigned 用请求自带的 t/s 与指定参数现场签名并发出 302（无 body）。
// t/s 取自已验签的请求而非磁盘，故不会把磁盘现状（文件可能已变更）混入目标；
// 缓存头与 200 一致，Vary 由 handler 在参数解析阶段统一设置
func redirectSigned(w http.ResponseWriter, signer *urlconv.Signer, relPath relpath.Path, query, params url.Values) error {
	for _, key := range []string{"t", "s"} {
		if value := query.Get(key); value != "" {
			params.Set(key, value)
		}
	}

	target, err := signer.SignURL(relPath, params)
	if err != nil {
		return err
	}

	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", `"immutable"`)
	w.Header().Set("Location", target.String())
	// 302 不写 body；HEAD 走同一分支同样只有响应头
	w.WriteHeader(http.StatusFound)
	return nil
}

// redirectToRaw 让全尺寸且源图 MIME 可接受的诉求指向 raw URL
func redirectToRaw(w http.ResponseWriter, signer *urlconv.Signer, relPath relpath.Path, query url.Values) error {
	params := url.Values{}
	params.Set("raw", "")
	return redirectSigned(w, signer, relPath, query, params)
}

// redirectToCanonicalWidth 让源图 MIME 不被接受的全尺寸诉求收敛到 w=源宽 的规范 URL
func redirectToCanonicalWidth(w http.ResponseWriter, signer *urlconv.Signer, relPath relpath.Path, query url.Values, width int) error {
	params := url.Values{}
	params.Set("w", strconv.Itoa(width))
	return redirectSigned(w, signer, relPath, query, params)
}

// serveOriginal 返回原始图片文件
func serveOriginal(w http.ResponseWriter, r *http.Request, contentType, filename string, reader io.ReadSeeker) {
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", `"immutable"`)
	w.Header().Set("Content-Type", contentType)

	cd := mime.FormatMediaType("inline", map[string]string{
		"filename": filename,
	})
	w.Header().Set("Content-Disposition", cd)

	http.ServeContent(w, r, "", time.Now(), reader)
}

// serveVariantFile 打开缓存句柄并写出变体响应
func serveVariantFile(w http.ResponseWriter, r *http.Request, format transcode.ImageFormat, filename string, file transcode.File) error {
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	serveVariant(w, r, format, filename, reader)
	return nil
}

// serveVariant 写出变体产物响应头与内容
func serveVariant(w http.ResponseWriter, r *http.Request, format transcode.ImageFormat, filename string, reader io.ReadSeekCloser) {
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", `"immutable"`)

	// 根据转码格式显式设置 Content-Type
	switch format {
	case transcode.ImageFormatAVIF:
		w.Header().Set("Content-Type", "image/avif")
	default:
		w.Header().Set("Content-Type", "image/webp")
	}

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
