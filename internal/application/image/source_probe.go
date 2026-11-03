package image

import (
	"context"
	"fmt"
	stdimage "image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"

	"main/internal/shared"
	"main/internal/util"

	_ "golang.org/x/image/webp"
)

// #region 源图探针

// Source 一次打开源图得到的决策输入：宽度、嗅探出的 MIME 与可 Seek 读取器。
// Reader 由调用方负责关闭，且始终位于文件开头。
//
// Width 由 Open/Probe 的语义决定：Open 不读取宽度（Width 为 0），
// Probe 才会填充宽度；调用方不得把 Open 的 0 当作「源图更窄」。
type Source struct {
	Width       int
	ContentType string
	Reader      io.ReadSeekCloser
}

// SourceProbe 打开源图并取得 /image 决策所需的信息，避免对同一文件重复 open 与重复嗅探 MIME
type SourceProbe interface {
	// Open 只打开源图并嗅探 MIME；raw 直接返回源文件字节，不需要宽度
	Open(absPath string) (Source, error)
	// Probe 在 Open 的基础上取得宽度：标准库可解码的格式从同一句柄读取，
	// 本地无解码器的格式（如 AVIF）回退到 MetaReader，仅这类格式会多一次元数据读取
	Probe(ctx context.Context, absPath string) (Source, error)
}

// MetaReader 读取源图尺寸的端口，供探针为本地无法解码的格式取宽度
type MetaReader interface {
	Meta(ctx context.Context, srcPath string) (*shared.ImageMeta, error)
}

type localSourceProbe struct {
	meta MetaReader
}

// NewSourceProbe 创建源图探针。meta 是本地无解码器格式的宽度后备来源，必须显式注入
func NewSourceProbe(meta MetaReader) SourceProbe {
	if meta == nil {
		panic("source probe requires a meta reader")
	}
	return localSourceProbe{meta: meta}
}

func (p localSourceProbe) Open(absPath string) (Source, error) {
	file, err := os.Open(absPath)
	if err != nil {
		return Source{}, err
	}

	contentType, _, err := util.DetectContentType(file, filepath.Base(absPath))
	if err != nil {
		file.Close()
		return Source{}, err
	}

	// 嗅探已推进句柄偏移，服务必须从文件头开始
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return Source{}, err
	}
	return Source{ContentType: contentType, Reader: file}, nil
}

func (p localSourceProbe) Probe(ctx context.Context, absPath string) (Source, error) {
	source, err := p.Open(absPath)
	if err != nil {
		return Source{}, err
	}

	width, err := p.width(ctx, absPath, source)
	if err != nil {
		source.Reader.Close()
		return Source{}, err
	}

	// 宽度读取同样推进了偏移，回绕以保持 Reader 始终位于文件头的约定
	if _, err := source.Reader.Seek(0, io.SeekStart); err != nil {
		source.Reader.Close()
		return Source{}, err
	}
	source.Width = width
	return source, nil
}

func (p localSourceProbe) width(ctx context.Context, absPath string, source Source) (int, error) {
	if decodableByStdlib(source.ContentType) {
		config, _, err := stdimage.DecodeConfig(source.Reader)
		if err != nil {
			return 0, fmt.Errorf("decode source image config: %w", err)
		}
		return config.Width, nil
	}

	// 本地没有该格式的解码器：复用已有元数据端口（magick / std 混合处理器）取宽度
	meta, err := p.meta.Meta(ctx, absPath)
	if err != nil {
		return 0, err
	}
	return meta.Width, nil
}

// decodableByStdlib 判断标准库是否注册了该 MIME 的解码器。
// 其余格式（AVIF/BMP/TIFF 等）走 MetaReader 后备
func decodableByStdlib(contentType string) bool {
	switch contentType {
	case "image/jpeg", "image/png", "image/gif", "image/webp":
		return true
	}
	return false
}

// #endregion
