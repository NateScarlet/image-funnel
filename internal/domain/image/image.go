package image

import (
	"errors"
	"fmt"
	"main/internal/domain/metadata"
	"main/internal/domain/relpath"
	"main/internal/scalar"
	"strings"
	"time"
)

type Image struct {
	id          scalar.ID
	filename    string
	relPath     relpath.Path
	directoryID scalar.ID
	size        int64
	modTime     time.Time
	xmpData     *metadata.Data
	width       int
	height      int
}

// TODO: 改为带校验 Factory 模式，不再允许外部直接构建
// New 创建一个具有指定ID的图片对象，常用于会话中的图片重建或测试
func New(id scalar.ID, filename string, relPath relpath.Path, directoryID scalar.ID, size int64, modTime time.Time, xmpData *metadata.Data, width, height int) *Image {
	return &Image{
		id:          id,
		filename:    filename,
		relPath:     relPath,
		directoryID: directoryID,
		size:        size,
		modTime:     modTime,
		xmpData:     xmpData,
		width:       width,
		height:      height,
	}
}

// FromRelPath 从根相对路径等文件系统信息创建图片，并自动基于路径和修改时间编码其ID
func FromRelPath(filename string, relPath relpath.Path, directoryID scalar.ID, size int64, modTime time.Time, xmpData *metadata.Data, width, height int) *Image {
	return &Image{
		id:          encodeID(relPath, modTime),
		filename:    filename,
		relPath:     relPath,
		directoryID: directoryID,
		size:        size,
		modTime:     modTime,
		xmpData:     xmpData,
		width:       width,
		height:      height,
	}
}

func (i *Image) ID() scalar.ID {
	return i.id
}

func (i *Image) DirectoryID() scalar.ID {
	return i.directoryID
}

func (i *Image) Filename() string {
	return i.filename
}

// RelPath 返回图片的根相对路径，构造时即保证其位于根目录内
func (i *Image) RelPath() relpath.Path {
	return i.relPath
}

func (i *Image) Size() int64 {
	return i.size
}

func (i *Image) ModTime() time.Time {
	return i.modTime
}

func (i *Image) Rating() int {
	if i.xmpData != nil {
		return i.xmpData.Rating()
	}
	return 0
}

// Label 返回图片的 XMP 标签
func (i *Image) Label() string {
	if i.xmpData != nil {
		return i.xmpData.Label()
	}
	return ""
}

func (i *Image) XMPData() *metadata.Data {
	return i.xmpData
}

func (i *Image) XMPExists() bool {
	return i.xmpData != nil
}

func (i *Image) Width() int {
	return i.width
}

func (i *Image) Height() int {
	return i.height
}

// #region ID编码与解码

// encodeID 将图片根相对路径和修改时间转换为明文格式的 ID
func encodeID(relPath relpath.Path, modTime time.Time) scalar.ID {
	return scalar.ToID(fmt.Sprintf("img:%d:%s", modTime.UnixNano(), relPath.String()))
}

// decodeID 解析明文格式的 ID 并还原为图片的根相对路径和期望修改时间。
// rootDir 仅用于还原时重新围栏：ID 由服务端签发，但仍是不可信输入
func decodeID(rootDir string, id scalar.ID) (relpath.Path, time.Time, error) {
	str := id.String()
	if !strings.HasPrefix(str, "img:") {
		return relpath.Path{}, time.Time{}, errors.New("invalid image id format")
	}
	parts := strings.SplitN(strings.TrimPrefix(str, "img:"), ":", 2)
	if len(parts) != 2 {
		return relpath.Path{}, time.Time{}, errors.New("invalid image id components")
	}
	nanoStr, relStr := parts[0], parts[1]
	var nano int64
	if _, err := fmt.Sscanf(nanoStr, "%d", &nano); err != nil {
		return relpath.Path{}, time.Time{}, fmt.Errorf("invalid image id timestamp: %w", err)
	}

	rel, err := relpath.New(rootDir, relStr)
	if err != nil {
		return relpath.Path{}, time.Time{}, fmt.Errorf("invalid image id path: %w", err)
	}

	return rel, time.Unix(0, nano), nil
}

// #endregion
