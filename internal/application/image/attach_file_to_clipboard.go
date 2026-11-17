package image

import (
	"context"
	"main/internal/apperror"
	"main/internal/domain/relpath"
	"path/filepath"
	"regexp"
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

	// 平台能力由适配器回答：非 Windows 实现直接返回「不支持」错误，
	// 经下面的错误路径自然映射为 supported: false，无需在应用层重复判断 GOOS。
	htmlContent, err := h.clipboard.ReadHTMLFormat()
	if err != nil {
		// 读不到剪贴板 HTML（平台不支持 / 跨设备 / 权限不足）统一降级为 supported: false
		return h.clipboardUnsupported("read_html_format_failed", zap.Error(err))
	}

	clipboardNonce := extractClipboardNonce(htmlContent)
	if clipboardNonce != nonce {
		// nonce 不匹配通常意味着客户端与服务端不在同一台机器上，不抛错，降级为 supported: false
		return h.clipboardUnsupported("nonce_mismatch",
			zap.String("expected", nonce),
			zap.String("got", clipboardNonce),
			zap.String("hint", "client and server may be running on different machines"),
		)
	}

	// 前端可能给出绝对路径或根相对路径两种形式，两种都要规整为已围栏的根相对路径
	var absPaths []string
	for _, p := range paths {
		var relStr string

		if filepath.IsAbs(p) {
			var err error
			relStr, err = filepath.Rel(h.rootDir, p)
			if err != nil {
				h.logger.Warn("Path validation failed", zap.String("path", p), zap.Error(err))
				return nil, apperror.New("PATH_INVALID", "Path validation failed", "路径校验失败")
			}
		} else {
			relStr = p
		}

		// 构造即围栏：逃逸根目录的路径无法被构造出来
		relPath, err := relpath.New(h.rootDir, relStr)
		if err != nil {
			h.logger.Warn("Path validation failed", zap.String("path", p), zap.Error(err))
			return nil, apperror.New("PATH_INVALID", "Path validation failed", "路径校验失败")
		}
		absPaths = append(absPaths, relPath.Abs(h.rootDir))
	}

	// 添加文件到剪贴板，并写回已校验的 HTML 内容（保留 nonce 数据）
	err = h.clipboard.AddFiles(absPaths, htmlContent)
	if err != nil {
		h.logger.Error("Failed to add files to clipboard", zap.Error(err))
		return nil, apperror.New("CLIPBOARD_WRITE_FAILED", "Failed to add files to clipboard", "无法添加文件到剪贴板")
	}

	return &AttachFileToClipboardResult{
		Supported: true,
	}, nil
}

// clipboardUnsupported 统一记录未能附加文件的具体原因，对外一律降级为 supported=false。
// 所有失败情形都不抛错，由客户端统一降级为复制文本/路径；reason 仅供服务端诊断。
func (h *Handler) clipboardUnsupported(reason string, fields ...zap.Field) (*AttachFileToClipboardResult, error) {
	h.logger.Debug("clipboard file attachment unsupported",
		append([]zap.Field{zap.String("reason", reason)}, fields...)...)
	return &AttachFileToClipboardResult{Supported: false}, nil
}

// clipboardTokenMetaName 剪贴板令牌在 HTML meta 标签中的 name 属性值
const clipboardTokenMetaName = "io.github.natescarlet.image-funnel.nonce"

// clipboardNoncePattern 匹配剪贴板 HTML 中携带 nonce 的 meta 标签，包级只编译一次
var clipboardNoncePattern = regexp.MustCompile(
	`<meta\s+name="` + regexp.QuoteMeta(clipboardTokenMetaName) + `"\s+content="([^"]*)"`,
)

// extractClipboardNonce 从剪贴板 HTML 格式中提取随机数
func extractClipboardNonce(htmlContent string) string {
	matches := clipboardNoncePattern.FindStringSubmatch(htmlContent)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}
