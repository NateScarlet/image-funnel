package urlconv

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"main/internal/application/image"
	"main/internal/domain/relpath"
	"main/internal/scalar"
)

// ImageURLPrefix 图片访问 URL 的路径前缀，形如 /image/<sig>/<relPath>?<params>。
// 签名作为路径段承载（而非查询参数），使「被签名的字符串」与「签名之后的部分」重合
const ImageURLPrefix = "/image/"

var (
	// ErrInvalidSignature 签名无效：URL 不是本服务签发的
	ErrInvalidSignature = errors.New("invalid image url signature")

	// ErrStaleIdentity URL 声明的图片身份（秒级修改时间 + 字节大小）与文件当前状态不一致，
	// 或身份参数缺失/非法。对外与「文件不存在」同为 404
	ErrStaleIdentity = errors.New("stale image identity")
)

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
	relStr, err := s.toRelativePath(absPath)
	if err != nil {
		return scalar.URI{}, err
	}

	// 只允许根目录内的文件：签名不应为越界路径提供凭据（纵深防御，与验签侧一致）
	rel, err := relpath.New(s.rootDir, relStr)
	if err != nil {
		return scalar.URI{}, fmt.Errorf("refuse to sign path outside root: %w", err)
	}

	fileInfo, err := os.Stat(rel.Abs(s.rootDir))
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

	return s.SignURL(rel, params)
}

// SignURL 为一组已经确定的参数签名，不读取磁盘。
//
// 与 GenerateSignedURL 的差别仅在 t/s 的来源：这里由调用方提供（例如从已验签的
// 请求中原样取回），从而允许 handler 现场签发指向同一图片身份的重定向 URL，
// 而不把磁盘现状混入 URL
func (s *Signer) SignURL(rel relpath.Path, params url.Values) (scalar.URI, error) {
	payload := urlPayload(rel.String(), params)
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

// ValidateSignedURL 校验请求目标串的签名，并核对 URL 声明的图片身份与磁盘现状，
// 通过后返回解码后的根相对路径。
//
// 返回值自带「该路径位于根目录内」的保证：围栏在构造时完成（relpath.New），
// 因此调用方无需（也不应）再自行复核根目录边界。
//
// 签名只证明 URL 由本服务签发；URL 中的 t/s 声明的图片身份则把授权绑定到签发时的
// 那一个文件版本。同路径文件被替换后旧 URL 必须失效，故身份比对与验签在同一接缝
// 完成——调用方（handler）无需另立参数契约，也无法遗忘这一步。
//
// target 必须是线路上的原始目标串（handler 传入 r.RequestURI）。签名覆盖的是原始字形，
// 因此此处不做 url.Values 往返（Encode 会把 %20 改写为 +、丢失 ?a 与 ?a= 的区别及参数顺序），
// 也不用 url.URL.Path（已解码，无法区分 %2F 与 /）；仅对 absolute-form 做最小归一
func (s *Signer) ValidateSignedURL(target string) (relpath.Path, error) {
	rel, payload, err := s.verifySignature(target)
	if err != nil {
		return relpath.Path{}, fmt.Errorf("%w: %w", ErrInvalidSignature, err)
	}

	if err := s.verifyIdentity(rel, payload); err != nil {
		return relpath.Path{}, err
	}
	return rel, nil
}

// verifySignature 校验签名并解码路径；payload 供 verifyIdentity 取回 URL 声明的身份
func (s *Signer) verifySignature(target string) (relpath.Path, string, error) {
	raw, err := originForm(target)
	if err != nil {
		return relpath.Path{}, "", err
	}

	rest, ok := strings.CutPrefix(raw, ImageURLPrefix)
	if !ok {
		return relpath.Path{}, "", fmt.Errorf("not an image url")
	}

	// 签名是第一段，其后全部内容（含查询串）都是被签名的字符串
	idx := strings.Index(rest, "/")
	if idx < 0 {
		return relpath.Path{}, "", fmt.Errorf("missing relative path")
	}
	sig, payload := rest[:idx], rest[idx+1:]
	if sig == "" || payload == "" {
		return relpath.Path{}, "", fmt.Errorf("missing signature or path")
	}

	// 直接比较编码后的签名：hmac.Equal 为常量时间比较，
	// 且避免「先解码再比较」引入多余的失败分支
	if !hmac.Equal([]byte(s.signatureFor(payload)), []byte(sig)) {
		return relpath.Path{}, "", fmt.Errorf("invalid signature")
	}

	// 验签通过后才解码路径。载荷中查询串分隔符是首个字面 ?，
	// 其之前即路径部分——这一约定成立的前提是签发侧对路径逐段 PathEscape，
	// 因而文件名中的 ? 已被编码为 %3F，不可能与分隔符混淆
	pathPart := payload
	if i := strings.Index(payload, "?"); i >= 0 {
		pathPart = payload[:i]
	}
	relStr, err := url.PathUnescape(pathPart)
	if err != nil {
		return relpath.Path{}, "", fmt.Errorf("invalid path escaping: %w", err)
	}

	// 密钥泄漏场景下伪造的签名同样受围栏约束：越界载荷无法被构造成根相对路径
	rel, err := relpath.New(s.rootDir, relStr)
	if err != nil {
		return relpath.Path{}, "", fmt.Errorf("path outside root: %w", err)
	}
	return rel, payload, nil
}

// verifyIdentity 核对 URL 声明的图片身份（t/s）与文件当前状态。
//
// 精度与签发侧一致：秒级修改时间 + 字节大小（见 GenerateSignedURL）。
// t/s 缺失、空串或非整数时按不一致处理（fail closed），不得退化为跳过校验。
//
// 文件不存在返回 os.ErrNotExist，其他 stat 错误原样返回：这些都不是「身份过期」，
// 由调用方沿用既有错误映射，避免把内部错误伪装成 404。
func (s *Signer) verifyIdentity(rel relpath.Path, payload string) error {
	declared, err := parseIdentity(payload)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrStaleIdentity, err)
	}

	info, err := os.Stat(rel.Abs(s.rootDir))
	if err != nil {
		return fmt.Errorf("stat image: %w", err)
	}

	if declared.modUnix != info.ModTime().Unix() || declared.size != info.Size() {
		return fmt.Errorf("%w: declared t=%d s=%d, actual t=%d s=%d",
			ErrStaleIdentity, declared.modUnix, declared.size, info.ModTime().Unix(), info.Size())
	}
	return nil
}

// imageIdentity URL 声明的图片身份：秒级修改时间与字节大小
type imageIdentity struct {
	modUnix int64
	size    int64
}

// parseIdentity 从已验签的载荷中取回 t/s。
// 参数缺失、空串或无法解析为整数时返回错误，由调用方按身份不一致处理。
func parseIdentity(payload string) (imageIdentity, error) {
	i := strings.Index(payload, "?")
	if i < 0 {
		return imageIdentity{}, fmt.Errorf("missing t and s")
	}

	query, err := url.ParseQuery(payload[i+1:])
	if err != nil {
		return imageIdentity{}, fmt.Errorf("parse query: %w", err)
	}

	tStr, sStr := query.Get("t"), query.Get("s")
	if tStr == "" || sStr == "" {
		return imageIdentity{}, fmt.Errorf("missing t or s")
	}

	modUnix, err := strconv.ParseInt(tStr, 10, 64)
	if err != nil {
		return imageIdentity{}, fmt.Errorf("invalid t %q: %w", tStr, err)
	}
	size, err := strconv.ParseInt(sStr, 10, 64)
	if err != nil {
		return imageIdentity{}, fmt.Errorf("invalid s %q: %w", sStr, err)
	}
	return imageIdentity{modUnix: modUnix, size: size}, nil
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
