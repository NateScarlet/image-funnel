package urlconv

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"main/internal/application/image"
	"main/internal/scalar"
	"main/internal/util"
)

// ImageURLPrefix 图片访问 URL 的路径前缀，形如 /image/<sig>/<relPath>?<params>。
// 签名作为路径段承载（而非查询参数），使「被签名的字符串」与「签名之后的部分」重合
const ImageURLPrefix = "/image/"

type Signer struct {
	secretKey []byte
	rootDir   string
}

func NewSigner(secretKey, rootDir string) *Signer {
	return &Signer{
		secretKey: []byte(secretKey),
		rootDir:   rootDir,
	}
}

// GenerateSignedURL 生成图片访问 URL：/image/<sig>/<relPath>?<params>
//
// 签名对象是 URL 中 ImageURLPrefix 之后的整个字符串（转义后的相对路径与查询串的原始字形），
// 因此任何未参与生成的参数改动都会使签名失效——包括未知参数，
// 无需维护参数白名单
func (s *Signer) GenerateSignedURL(absPath string, opts ...image.SignOption) (scalar.URI, error) {
	relPath, err := s.toRelativePath(absPath)
	if err != nil {
		return scalar.URI{}, err
	}

	// 只允许根目录内的文件：签名不应为越界路径提供凭据（纵深防御，与 handler 侧一致）
	if err := util.EnsurePathInRoot(s.rootDir, relPath); err != nil {
		return scalar.URI{}, fmt.Errorf("refuse to sign path outside root: %w", err)
	}

	fileInfo, err := os.Stat(filepath.Join(s.rootDir, relPath))
	if err != nil {
		return scalar.URI{}, fmt.Errorf("failed to get file info: %w", err)
	}

	params := url.Values{}
	for _, opt := range opts {
		opt(params)
	}
	// 修改时间与大小是图片身份的一部分（见 domain/image 的 ID 编码）：
	// 同一路径的文件被替换后即另一张图片，其 URL 必须随之失效
	params.Set("t", fmt.Sprintf("%d", fileInfo.ModTime().Unix()))
	params.Set("s", fmt.Sprintf("%d", fileInfo.Size()))

	payload := urlPayload(relPath, params)

	return scalar.ParseURI(ImageURLPrefix + s.signatureFor(payload) + "/" + payload)
}

// urlPayload 构建被签名的载荷：逐段转义的相对路径，以及（非空时）查询串。
// 逐段转义而非整体转义，否则路径分隔符会被编码成 %2F，多级路径会塌成一段
func urlPayload(relPath string, params url.Values) string {
	segments := strings.Split(filepath.ToSlash(relPath), "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	escaped := strings.Join(segments, "/")

	rawQuery := params.Encode()
	if rawQuery == "" {
		return escaped
	}
	return escaped + "?" + rawQuery
}

// signatureFor 计算载荷的 HMAC-SHA256 签名并编码为 URL 安全形式。
// 使用无填充 base64：签名位于路径段中，避免 "=" 在部分中间件/代理中被特殊处理
func (s *Signer) signatureFor(payload string) string {
	h := hmac.New(sha256.New, s.secretKey)
	fmt.Fprintf(h, "%s", payload)
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

// ValidateSignedURL 校验请求目标串的签名，通过后返回解码后的相对路径。
//
// target 必须是线路上的原始目标串（handler 传入 r.RequestURI）。签名覆盖的是原始字形，
// 因此此处不做 url.Values 往返（Encode 会把 %20 改写为 +、丢失 ?a 与 ?a= 的区别及参数顺序），
// 也不用 url.URL.Path（已解码，无法区分 %2F 与 /）；仅对 absolute-form 做最小归一
func (s *Signer) ValidateSignedURL(target string) (string, error) {
	raw, err := originForm(target)
	if err != nil {
		return "", err
	}

	rest, ok := strings.CutPrefix(raw, ImageURLPrefix)
	if !ok {
		return "", fmt.Errorf("not an image url")
	}

	// 签名是第一段，其后全部内容（含查询串）都是被签名的字符串
	idx := strings.Index(rest, "/")
	if idx < 0 {
		return "", fmt.Errorf("missing relative path")
	}
	sig, payload := rest[:idx], rest[idx+1:]
	if sig == "" || payload == "" {
		return "", fmt.Errorf("missing signature or path")
	}

	// 直接比较编码后的签名：hmac.Equal 为常量时间比较，
	// 且避免「先解码再比较」引入多余的失败分支
	if !hmac.Equal([]byte(s.signatureFor(payload)), []byte(sig)) {
		return "", fmt.Errorf("invalid signature")
	}

	// 验签通过后才解码路径。载荷中查询串分隔符是首个字面 ?，
	// 其之前即路径部分——这一约定成立的前提是签发侧对路径逐段 PathEscape，
	// 因而文件名中的 ? 已被编码为 %3F，不可能与分隔符混淆
	pathPart := payload
	if i := strings.Index(payload, "?"); i >= 0 {
		pathPart = payload[:i]
	}
	relPath, err := url.PathUnescape(pathPart)
	if err != nil {
		return "", fmt.Errorf("invalid path escaping: %w", err)
	}
	return relPath, nil
}

// originForm 取得请求目标串中 origin-form 部分的起始下标。
// 正常的 origin-form（以 / 开头）直接可用；absolute-form 由代理转发产生，
// 需剥离 scheme 与 authority——此处按分隔符定位后**原样切片**，
// 不借助 url.URL 重建（EscapedPath 会在 RawPath 失效时重新编码，破坏被签名的字形）
func originForm(target string) (string, error) {
	if strings.HasPrefix(target, "/") {
		return target, nil
	}

	schemeEnd := strings.Index(target, "://")
	if schemeEnd < 0 {
		return "", fmt.Errorf("unsupported request target")
	}
	pathStart := strings.Index(target[schemeEnd+3:], "/")
	if pathStart < 0 {
		return "", fmt.Errorf("unsupported request target")
	}
	return target[schemeEnd+3+pathStart:], nil
}

func (s *Signer) toRelativePath(absPath string) (string, error) {
	absPath = filepath.Clean(absPath)
	rootDir := filepath.Clean(s.rootDir)

	if !filepath.IsAbs(absPath) {
		return filepath.ToSlash(absPath), nil
	}

	relPath, err := filepath.Rel(rootDir, absPath)
	if err != nil {
		// 无法求出相对路径（如跨驱动器）属异常状态，不可静默退化为绝对路径
		return "", fmt.Errorf("resolve path relative to root: %w", err)
	}

	relPath = filepath.Clean(relPath)
	if relPath == "." {
		relPath = filepath.Base(absPath)
	}

	return filepath.ToSlash(relPath), nil
}
