package urlconv

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"main/internal/application/image"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// splitSignedURL 把生成的 URL 拆成签名、相对路径与原始查询串，便于断言与篡改
func splitSignedURL(t *testing.T, raw string) (sig, relPath, rawQuery string) {
	t.Helper()
	u, err := url.Parse(raw)
	require.NoError(t, err)

	// 形如 /image/<sig>/<relPath>
	trimmed := strings.TrimPrefix(u.Path, "/image/")
	idx := strings.Index(trimmed, "/")
	require.GreaterOrEqual(t, idx, 0, "URL 应包含签名后的路径段: %q", raw)
	return trimmed[:idx], trimmed[idx+1:], u.RawQuery
}

func newTestSigner(t *testing.T) (*Signer, string) {
	t.Helper()
	rootDir := t.TempDir()
	return NewSigner("test-secret-key", rootDir), rootDir
}

func writeSource(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte("image-bytes"), 0o644))
}

func TestGenerateSignedURL_Format(t *testing.T) {
	signer, rootDir := newTestSigner(t)
	relPath := "photo.jpg"
	writeSource(t, filepath.Join(rootDir, relPath))

	signedURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, relPath))
	require.NoError(t, err)

	sig, gotPath, rawQuery := splitSignedURL(t, signedURL.String())
	assert.NotEmpty(t, sig, "签名应位于路径段中")
	assert.Equal(t, relPath, gotPath)
	// 时间戳与大小仍在查询串中，用于文件变更后的缓存失效
	assert.Contains(t, rawQuery, "t=")
	assert.Contains(t, rawQuery, "s=")
	assert.True(t, strings.HasPrefix(signedURL.String(), "/image/"), "URL 应为 /image/ 前缀: %q", signedURL.String())
	// 签名不得包含 /，否则会被误当作路径分隔符
	assert.NotContains(t, sig, "/")
}

func TestValidate_RoundTrip(t *testing.T) {
	signer, rootDir := newTestSigner(t)
	relPath := "photo.jpg"
	writeSource(t, filepath.Join(rootDir, relPath))

	signedURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, relPath))
	require.NoError(t, err)

	gotPath, err := signer.ValidateSignedURL(signedURL.String())
	require.NoError(t, err)
	assert.Equal(t, relPath, gotPath)
}

func TestValidate_SubdirectoryAndSpecialCharsRoundTrip(t *testing.T) {
	signer, rootDir := newTestSigner(t)
	// 子目录 + 空格 + 中文 + 字面量百分号 + 加号（易被当作空格的字符）
	relPath := "sub dir/中文 名称+tag%20.png"
	writeSource(t, filepath.Join(rootDir, "sub dir", "中文 名称+tag%20.png"))

	signedURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, relPath))
	require.NoError(t, err)

	gotPath, err := signer.ValidateSignedURL(signedURL.String())
	require.NoError(t, err)
	assert.Equal(t, relPath, gotPath, "特殊字符应原样往返")
}

func TestValidate_TamperedRelPathFails(t *testing.T) {
	signer, rootDir := newTestSigner(t)
	writeSource(t, filepath.Join(rootDir, "photo.jpg"))
	writeSource(t, filepath.Join(rootDir, "secret.jpg"))

	signedURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, "photo.jpg"))
	require.NoError(t, err)

	sig, _, rawQuery := splitSignedURL(t, signedURL.String())
	// 换成另一张图，签名不变
	forged := "/image/" + sig + "/secret.jpg?" + rawQuery
	_, err = signer.ValidateSignedURL(forged)
	assert.Error(t, err, "替换相对路径必须导致签名失效")
}

func TestValidate_AbsoluteForm(t *testing.T) {
	// 部分代理以 absolute-form 转发；剥离 scheme 与 authority 必须原样切片，
	// 不得借助 url.URL 重建（EscapedPath 会重新编码，破坏被签名的字形）
	signer, rootDir := newTestSigner(t)
	relPath := "sub dir/中文.jpg"
	writeSource(t, filepath.Join(rootDir, "sub dir", "中文.jpg"))

	signedURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, relPath))
	require.NoError(t, err)

	for _, prefix := range []string{"http://localhost", "https://example.com:8080"} {
		got, err := signer.ValidateSignedURL(prefix + signedURL.String())
		require.NoError(t, err, "absolute-form 应能验签: %s", prefix)
		assert.Equal(t, filepath.ToSlash(relPath), got)
	}

	// 无法识别的目标串必须报错而不是当作路径
	for _, bad := range []string{"", "example.com/image/x", "//"} {
		_, err := signer.ValidateSignedURL(bad)
		assert.Error(t, err, "非法目标串应报错: %q", bad)
	}
}

func TestValidate_TamperedParamValueFails(t *testing.T) {
	signer, rootDir := newTestSigner(t)
	writeSource(t, filepath.Join(rootDir, "photo.jpg"))

	signedURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, "photo.jpg"), image.WithWidth(256))
	require.NoError(t, err)

	// 篡改已签名参数的值
	u, err := url.Parse(signedURL.String())
	require.NoError(t, err)
	q := u.Query()
	q.Set("w", "4096")
	u.RawQuery = q.Encode()

	_, err = signer.ValidateSignedURL(u.String())
	assert.Error(t, err, "篡改已签名参数的值必须导致签名失效")
}

func TestValidate_UnknownParamFails(t *testing.T) {
	// 核心安全属性：签名覆盖「相对路径?查询串」整串，
	// 因此任何未参与生成的参数（含未知参数）都会使签名失效
	signer, rootDir := newTestSigner(t)
	writeSource(t, filepath.Join(rootDir, "photo.jpg"))

	signedURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, "photo.jpg"))
	require.NoError(t, err)

	u, err := url.Parse(signedURL.String())
	require.NoError(t, err)
	q := u.Query()
	q.Set("unknown_param", "anything")
	u.RawQuery = q.Encode()

	_, err = signer.ValidateSignedURL(u.String())
	assert.Error(t, err, "追加未知参数必须导致签名失效")
}

func TestValidate_RawPresenceIsSigned(t *testing.T) {
	// raw 的「存在性」与「取值」都被签名覆盖：
	// 对不含 raw 的 URL 追加裸 raw（无等号）或 raw= 都必须失败，
	// 从而修复「按值签名、按存在性消费」造成的绕过
	signer, rootDir := newTestSigner(t)
	writeSource(t, filepath.Join(rootDir, "photo.jpg"))

	signedURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, "photo.jpg"), image.WithWidth(256))
	require.NoError(t, err)

	base, err := url.Parse(signedURL.String())
	require.NoError(t, err)

	t.Run("追加裸 raw", func(t *testing.T) {
		forged := *base
		forged.RawQuery = base.RawQuery + "&raw"
		_, err := signer.ValidateSignedURL(forged.String())
		assert.Error(t, err, "追加裸 raw 必须导致签名失效")
	})

	t.Run("追加 raw 空值", func(t *testing.T) {
		forged := *base
		forged.RawQuery = base.RawQuery + "&raw="
		_, err := signer.ValidateSignedURL(forged.String())
		assert.Error(t, err, "追加 raw= 必须导致签名失效")
	})

	t.Run("删除 raw 参数", func(t *testing.T) {
		// 由服务端签发的 raw URL 删掉 raw 后也必须失败
		rawURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, "photo.jpg"), image.WithRaw())
		require.NoError(t, err)
		u, err := url.Parse(rawURL.String())
		require.NoError(t, err)
		q := u.Query()
		q.Del("raw")
		u.RawQuery = q.Encode()
		_, err = signer.ValidateSignedURL(u.String())
		assert.Error(t, err, "删除 raw 必须导致签名失效")
	})
}

func TestValidate_MissingOrMalformedSignatureFails(t *testing.T) {
	signer, rootDir := newTestSigner(t)
	writeSource(t, filepath.Join(rootDir, "photo.jpg"))

	signedURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, "photo.jpg"))
	require.NoError(t, err)
	_, relPath, rawQuery := splitSignedURL(t, signedURL.String())

	cases := map[string]string{
		"空签名":    "/image//" + relPath + "?" + rawQuery,
		"签名非法编码": "/image/!!!not-base64!!!/" + relPath + "?" + rawQuery,
		"缺少路径":   "/image/abc123",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := signer.ValidateSignedURL(raw)
			assert.Error(t, err)
		})
	}
}

func TestValidate_TamperedTimestampFails(t *testing.T) {
	signer, rootDir := newTestSigner(t)
	writeSource(t, filepath.Join(rootDir, "photo.jpg"))

	signedURL, err := signer.GenerateSignedURL(filepath.Join(rootDir, "photo.jpg"))
	require.NoError(t, err)

	u, err := url.Parse(signedURL.String())
	require.NoError(t, err)
	q := u.Query()
	q.Set("t", "9999999999")
	u.RawQuery = q.Encode()

	_, err = signer.ValidateSignedURL(u.String())
	assert.Error(t, err, "篡改时间戳必须导致签名失效")
}

func TestToRelativePath(t *testing.T) {
	signer, _ := newTestSigner(t)

	tests := []struct {
		input    string
		expected string
	}{
		{"test.jpg", "test.jpg"},
		{"subdir/test.jpg", "subdir/test.jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result, err := signer.toRelativePath(tt.input)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToRelativePath_Absolute(t *testing.T) {
	signer, _ := newTestSigner(t)

	absPath := filepath.Join(t.TempDir(), "test.jpg")
	result, err := signer.toRelativePath(absPath)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}
