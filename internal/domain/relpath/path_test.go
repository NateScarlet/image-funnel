package relpath

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// normalizedOnPlatform 返回构造成功时期望得到的规范形式。
//
// New 的规整规则就是「按平台自身的分隔符语义规范化」：
// 反斜杠在 Windows 上是分隔符、在其它平台是合法文件名字符，
// 因此期望值必须由 filepath 推导，不能写死某一种平台的字面量。
func normalizedOnPlatform(relPath string) string {
	return filepath.ToSlash(filepath.Clean(filepath.FromSlash(relPath)))
}

func TestNew(t *testing.T) {
	rootDir := t.TempDir()

	tests := []struct {
		name string
		path string
		// wantErr 为 true 时构造必须失败
		wantErr bool
		// wantRoot 为 true 时期望归一为根目录（空串）；
		// 其余用例的期望值由 normalizedOnPlatform 推导
		wantRoot bool
	}{
		{name: "empty string means root", path: "", wantRoot: true},
		{name: "current directory means root", path: ".", wantRoot: true},
		{name: "single subdirectory", path: "subdir"},
		{name: "nested subdirectory", path: "subdir/subdir"},
		{name: "directory with dot prefix", path: "subdir/..subdir"},
		{name: "directory with dot suffix", path: "subdir/subdir.."},
		{name: "directory with middle dots", path: "subdir/sub..dir"},
		{name: "redundant separators are cleaned", path: "subdir//nested/./file.jpg"},
		{name: "path traversal inside root is cleaned", path: "subdir/../other/file.jpg"},
		{name: "path traversal with single parent", path: "../escape", wantErr: true},
		{name: "path traversal with double parent", path: "../../escape", wantErr: true},
		{name: "path traversal with current parent", path: "./../escape", wantErr: true},
		{name: "path traversal with nested parent", path: "subdir/../../escape", wantErr: true},
		// 反斜杠只在 Windows 上构成目录穿越，其它平台它只是一个普通文件名
		{name: "path traversal with backslash (windows only)", path: `..\escape`, wantErr: runtime.GOOS == "windows"},
		{name: "backslash as filename", path: `back\slash`},
		{name: "normal path looks like escape", path: "..not_escape"},
		{name: "path traversal with double backslash (windows only)", path: `..\..\escape`, wantErr: runtime.GOOS == "windows"},
		{name: "absolute path", path: "/absolute/path", wantErr: true},
		// 盘符只在 Windows 上是卷的一部分，其它平台它只是根目录内的普通文件名
		{name: "absolute path with drive letter (windows only)", path: `C:\Windows\System32`, wantErr: runtime.GOOS == "windows"},
		{name: "drive letter as filename (non-windows)", path: "C:filename", wantErr: runtime.GOOS == "windows"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := New(rootDir, tt.path)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)

			want := normalizedOnPlatform(tt.path)
			if tt.wantRoot {
				want = ""
			}
			assert.Equal(t, want, got.String())
		})
	}
}

// 派生期望值有可能把错误的实现一起掩盖，因此把当前平台上的实际行为显式钉住
func TestNew_BackslashIsPlatformDependent(t *testing.T) {
	rootDir := t.TempDir()

	got, err := New(rootDir, `back\slash`)
	require.NoError(t, err)

	if runtime.GOOS == "windows" {
		assert.Equal(t, "back/slash", got.String(), "Windows 上反斜杠是分隔符，必须规整为正斜杠")
	} else {
		assert.Equal(t, `back\slash`, got.String(), "其它平台上反斜杠是合法文件名字符，按 filepath 语义原样保留")
	}
}

func TestZeroValueIsRoot(t *testing.T) {
	rootDir := t.TempDir()

	var zero Path

	assert.True(t, zero.IsRoot())
	assert.Empty(t, zero.String())
	assert.Empty(t, zero.Base())
	assert.Equal(t, filepath.Clean(rootDir), zero.Abs(rootDir))
}

func TestAbs(t *testing.T) {
	rootDir := t.TempDir()

	sub, err := New(rootDir, "subdir/file.jpg")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(rootDir, "subdir", "file.jpg"), sub.Abs(rootDir))

	var zero Path
	assert.Equal(t, filepath.Clean(rootDir), zero.Abs(rootDir))
}

func TestBase(t *testing.T) {
	rootDir := t.TempDir()

	tests := []struct {
		rel  string
		want string
	}{
		{rel: "", want: ""},
		{rel: "file.jpg", want: "file.jpg"},
		{rel: "subdir/file.jpg", want: "file.jpg"},
		{rel: "subdir/nested/file.jpg", want: "file.jpg"},
	}

	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			p, err := New(rootDir, tt.rel)
			require.NoError(t, err)
			assert.Equal(t, tt.want, p.Base())
		})
	}
}

func TestDir(t *testing.T) {
	rootDir := t.TempDir()

	tests := []struct {
		rel  string
		want string
	}{
		{rel: "file.jpg", want: ""},
		{rel: "subdir/file.jpg", want: "subdir"},
		{rel: "subdir/nested/file.jpg", want: "subdir/nested"},
	}

	for _, tt := range tests {
		t.Run(tt.rel, func(t *testing.T) {
			p, err := New(rootDir, tt.rel)
			require.NoError(t, err)
			assert.Equal(t, tt.want, p.Dir().String())
		})
	}
}

// 根目录的父目录仍是根目录：去掉一段只会更靠近根，不可能逃逸
func TestDirOfRootIsRoot(t *testing.T) {
	var zero Path

	assert.True(t, zero.Dir().IsRoot())
}

func TestJoin(t *testing.T) {
	rootDir := t.TempDir()

	sub, err := New(rootDir, "subdir")
	require.NoError(t, err)

	joined, err := sub.Join(rootDir, "file.jpg")
	require.NoError(t, err)
	assert.Equal(t, "subdir/file.jpg", joined.String())

	var zero Path
	joined, err = zero.Join(rootDir, "file.jpg")
	require.NoError(t, err)
	assert.Equal(t, "file.jpg", joined.String())

	// 拼接结果同样受围栏约束：逃逸的 name 无法构造成 Path
	_, err = sub.Join(rootDir, "../../escape")
	assert.Error(t, err)
}

// Path 是可比较的值，可直接作为 map key
func TestPathIsComparable(t *testing.T) {
	rootDir := t.TempDir()

	a, err := New(rootDir, "subdir/file.jpg")
	require.NoError(t, err)
	b, err := New(rootDir, "subdir/file.jpg")
	require.NoError(t, err)

	m := map[Path]int{a: 1}
	assert.Equal(t, 1, m[b])
}

func TestTrimExtSuffix(t *testing.T) {
	rootDir := t.TempDir()

	t.Run("大小写不敏感地判定与裁剪", func(t *testing.T) {
		for _, rel := range []string{"photo.jpg.xmp", "photo.jpg.XMP", "photo.jpg.Xmp"} {
			p, err := New(rootDir, rel)
			require.NoError(t, err)

			got, ok, err := p.TrimExtSuffix(rootDir, ".xmp")
			require.NoError(t, err)
			require.True(t, ok, "%q 应被识别为 XMP 配套文件", rel)
			assert.Equal(t, "photo.jpg", got.String(), "%q 裁剪后缀的结果必须与判定一致", rel)
		}
	})

	t.Run("后缀不匹配时原样返回", func(t *testing.T) {
		// 调用方几乎总是要用返回值；不匹配时若返回零值，非配套文件会被误当成根目录
		p, err := New(rootDir, "photo.jpg")
		require.NoError(t, err)

		got, ok, err := p.TrimExtSuffix(rootDir, ".xmp")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, p, got)
	})

	t.Run("比后缀更短的路径不会被误裁", func(t *testing.T) {
		p, err := New(rootDir, "x")
		require.NoError(t, err)

		got, ok, err := p.TrimExtSuffix(rootDir, ".xmp")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, p, got)
	})

	t.Run("根目录没有后缀", func(t *testing.T) {
		var zero Path
		got, ok, err := zero.TrimExtSuffix(rootDir, ".xmp")
		require.NoError(t, err)
		assert.False(t, ok)
		assert.Equal(t, zero, got)
	})

	t.Run("裁剪结果重新围栏", func(t *testing.T) {
		// `...xmp` 本身是根内合法文件名，但裁剪掉 `.xmp` 后即为 `..`
		p, err := New(rootDir, "...xmp")
		require.NoError(t, err)

		_, _, err = p.TrimExtSuffix(rootDir, ".xmp")
		assert.Error(t, err)
	})
}
