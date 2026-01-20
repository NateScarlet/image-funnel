package image

import (
	"crypto/sha256"
	"encoding/hex"
	"main/internal/domain/metadata"
	"main/internal/scalar"
	"time"
)

type Image struct {
	id       scalar.ID
	filename string
	path     string
	size     int64
	modTime  time.Time
	xmpData  *metadata.XMPData
	width    int
	height   int
}

func NewImage(id scalar.ID, filename, path string, size int64, modTime time.Time, xmpData *metadata.XMPData, width, height int) *Image {
	return &Image{
		id:       id,
		filename: filename,
		path:     path,
		size:     size,
		modTime:  modTime,
		xmpData:  xmpData,
		width:    width,
		height:   height,
	}
}

func NewImageFromPath(filename, path string, size int64, modTime time.Time, xmpData *metadata.XMPData, width, height int) *Image {
	return &Image{
		id:       newID(path, modTime),
		filename: filename,
		path:     path,
		size:     size,
		modTime:  modTime,
		xmpData:  xmpData,
		width:    width,
		height:   height,
	}
}

func (i *Image) ID() scalar.ID {
	return i.id
}

func (i *Image) Filename() string {
	return i.filename
}

func (i *Image) Path() string {
	return i.path
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

func newID(path string, modTime time.Time) scalar.ID {
	hash := sha256.New()
	hash.Write([]byte(path))
	hash.Write([]byte(modTime.String()))
	return scalar.ToID(hex.EncodeToString(hash.Sum(nil))[:16])
}
