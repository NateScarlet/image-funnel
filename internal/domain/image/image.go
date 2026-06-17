package image

import (
	"errors"
	"fmt"
	"main/internal/domain/metadata"
	"main/internal/scalar"
	"path/filepath"
	"strings"
	"time"
)

type Image struct {
	id          scalar.ID
	filename    string
	relPath     string
	directoryID scalar.ID
	size        int64
	modTime     time.Time
	xmpData     *metadata.Data
	width       int
	height      int
}

// TODO: 改为带校验 Factory 模式，不再允许外部直接构建
// New 创建一个具有指定ID的图片对象，常用于会话中的图片重建或测试
func New(id scalar.ID, filename, relPath string, directoryID scalar.ID, size int64, modTime time.Time, xmpData *metadata.Data, width, height int) *Image {
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

// FromRelPath 从相对路径等文件系统信息创建图片，并自动基于路径和修改时间编码其ID
func FromRelPath(filename, relPath string, directoryID scalar.ID, size int64, modTime time.Time, xmpData *metadata.Data, width, height int) *Image {
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

func (i *Image) RelPath() string {
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

// encodeID 将图片相对路径和修改时间转换为明文格式的 ID
func encodeID(relPath string, modTime time.Time) scalar.ID {
	str := fmt.Sprintf("img:%d:%s", modTime.UnixNano(), relPath)
	return scalar.ToID(str)
}

// decodeID 解析明文格式的 ID 并还原为图片的相对路径和期望修改时间
func decodeID(id scalar.ID) (string, time.Time, error) {
	str := id.String()
	if !strings.HasPrefix(str, "img:") {
		return "", time.Time{}, errors.New("invalid image id format")
	}
	parts := strings.SplitN(strings.TrimPrefix(str, "img:"), ":", 2)
	if len(parts) != 2 {
		return "", time.Time{}, errors.New("invalid image id components")
	}
	nanoStr, relPath := parts[0], parts[1]
	var nano int64
	_, err := fmt.Sscanf(nanoStr, "%d", &nano)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("invalid image id timestamp: %w", err)
	}

	// 校验以确保路径不是绝对路径，防御性拒绝绝对路径的入参
	if filepath.IsAbs(relPath) {
		return "", time.Time{}, fmt.Errorf("absolute path not allowed in image ID: %s", relPath)
	}

	return relPath, time.Unix(0, nano), nil
}

// #endregion
