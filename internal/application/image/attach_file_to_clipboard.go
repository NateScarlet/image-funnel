package image

import (
	"context"
	"main/internal/apperror"
	"main/internal/util"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"go.uber.org/zap"
)

// AttachFileToClipboardResult 附加文件到剪贴板操作结果
type AttachFileToClipboardResult struct {
	Supported bool
}

// AttachFileToClipboard 将文件附加到系统剪贴板，通过随机数验证后附加文件对象
func (h *Handler) AttachFileToClipboard(
	ctx context.Context,
	paths []string,
	nonce string,
) (*AttachFileToClipboardResult, error) {
	startTime := time.Now()

	defer func() {
		h.logger.Debug("AttachFileToClipboard completed",
			zap.Strings("paths", paths),
			zap.Duration("duration", time.Since(startTime)),
		)
	}()

	// 1. 平台检测 (需求 4)
	// 如果非 Windows 平台，返回 supported: false 且不抛出错误
	if runtime.GOOS != "windows" {
		h.logger.Debug("Clipboard file attachment not supported on this platform", zap.String("goos", runtime.GOOS))
		return &AttachFileToClipboardResult{
			Supported: false,
		}, nil
	}

	if h.clipboard == nil {
		h.logger.Debug("Clipboard provider not configured")
		return &AttachFileToClipboardResult{
			Supported: false,
		}, nil
	}

	// 2. 验证剪贴板中的随机数：从 HTML 格式的 meta 标签中读取
	htmlContent, err := h.clipboard.ReadHTMLFormat()
	if err != nil {
		h.logger.Debug("Failed to read clipboard HTML", zap.Error(err))
		// 如果读不到剪贴板 HTML，也作为不支持处理而不抛出错误（可能由于跨设备或者权限不足）
		return &AttachFileToClipboardResult{
			Supported: false,
		}, nil
	}

	clipboardNonce := extractClipboardNonce(htmlContent)
	if clipboardNonce != nonce {
		// 调试：列出当前剪贴板中所有格式
		formats := h.clipboard.ListFormats()
		h.logger.Debug("Clipboard nonce mismatch (this might indicate client & server are running on different machines)",
			zap.String("expected", nonce),
			zap.String("got", clipboardNonce),
			zap.Strings("allFormats", formats))
		// 随机数验证失败时，由于这表明可能运行在不同的物理机器上，故不抛出错误，而是返回 supported: false
		return &AttachFileToClipboardResult{
			Supported: false,
		}, nil
	}

	// 将路径转换为绝对路径，并校验安全性
	var absPaths []string
	for _, p := range paths {
		var absPath string
		var relForValidation string

		if filepath.IsAbs(p) {
			absPath = p
			var err error
			relForValidation, err = filepath.Rel(h.rootDir, p)
			if err != nil {
				h.logger.Warn("Path validation failed", zap.String("path", p), zap.Error(err))
				return nil, apperror.New("PATH_INVALID", "Path validation failed", "路径校验失败")
			}
		} else {
			absPath = filepath.Join(h.rootDir, p)
			relForValidation = p
		}

		// 校验路径不逃逸根目录
		if err := util.EnsurePathInRoot(h.rootDir, relForValidation); err != nil {
			h.logger.Warn("Path validation failed", zap.String("path", p), zap.Error(err))
			return nil, apperror.New("PATH_INVALID", "Path validation failed", "路径校验失败")
		}
		absPaths = append(absPaths, absPath)
	}

	// 添加文件到剪贴板，保留已有数据
	err = h.clipboard.AddFiles(absPaths)
	if err != nil {
		h.logger.Error("Failed to add files to clipboard", zap.Error(err))
		return nil, apperror.New("CLIPBOARD_WRITE_FAILED", "Failed to add files to clipboard", "无法添加文件到剪贴板")
	}

	return &AttachFileToClipboardResult{
		Supported: true,
	}, nil
}

// clipboardTokenMetaName 剪贴板令牌在 HTML meta 标签中的 name 属性值
const clipboardTokenMetaName = "io.github.natescarlet.image-funnel.nonce"

// extractClipboardNonce 从剪贴板 HTML 格式中提取随机数
func extractClipboardNonce(htmlContent string) string {
	// 匹配 <meta name="io.github.natescarlet.image-funnel.nonce" content="..."/>
	pattern := regexp.MustCompile(
		`<meta\s+name="` + regexp.QuoteMeta(clipboardTokenMetaName) + `"\s+content="([^"]*)"`,
	)
	matches := pattern.FindStringSubmatch(htmlContent)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}
