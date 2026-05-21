package image

import (
	"errors"
	"fmt"
	"main/internal/domain/metadata"
	"main/internal/scalar"
	"strings"
	"time"
)

type Image struct {
	id          scalar.ID
	filename    string
	absPath     string
	directoryID scalar.ID
	size        int64
	modTime     time.Time
	xmpData     *metadata.XMPData
	width       int
	height      int
}

// NewImage 创建一个具有指定ID的图片对象，常用于会话中的图片重建或测试
func NewImage(id scalar.ID, filename, absPath string, directoryID scalar.ID, size int64, modTime time.Time, xmpData *metadata.XMPData, width, height int) *Image {
	return &Image{
		id:          id,
		filename:    filename,
		absPath:     absPath,
		directoryID: directoryID,
		size:        size,
		modTime:     modTime,
		xmpData:     xmpData,
		width:       width,
		height:      height,
	}
}

// NewImageFromAbsPath 从绝对路径等文件系统信息创建图片，并自动基于路径和修改时间编码其ID
func NewImageFromAbsPath(filename, absPath string, directoryID scalar.ID, size int64, modTime time.Time, xmpData *metadata.XMPData, width, height int) *Image {
	return &Image{
		id:          EncodeID(absPath, modTime),
		filename:    filename,
		absPath:     absPath,
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

func (i *Image) AbsPath() string {
	return i.absPath
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

func (i *Image) XMPData() *metadata.XMPData {
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

// EncodeID 将图片绝对路径和修改时间转换为明文格式的 ID
func EncodeID(absPath string, modTime time.Time) scalar.ID {
	str := fmt.Sprintf("img:%d:%s", modTime.UnixNano(), absPath)
	return scalar.ToID(str)
}

// DecodeID 解析明文格式的 ID 并还原为图片的绝对路径和期望修改时间
func DecodeID(id scalar.ID) (string, time.Time, error) {
	str := id.String()
	if !strings.HasPrefix(str, "img:") {
		return "", time.Time{}, errors.New("invalid image id format")
	}
	parts := strings.SplitN(strings.TrimPrefix(str, "img:"), ":", 2)
	if len(parts) != 2 {
		return "", time.Time{}, errors.New("invalid image id components")
	}
	nanoStr, absPath := parts[0], parts[1]
	var nano int64
	_, err := fmt.Sscanf(nanoStr, "%d", &nano)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("invalid image id timestamp: %w", err)
	}
	return absPath, time.Unix(0, nano), nil
}

// #endregion
