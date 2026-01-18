package directory

import (
	"crypto/sha256"
	"encoding/hex"
	"main/internal/scalar"
	"time"
)

type DirectoryInfo struct {
	id                 scalar.ID
	path               string
	imageCount         int
	subdirectoryCount  int
	latestImageModTime time.Time
	latestImagePath    string
	ratingCounts       map[int]int
}

func NewDirectoryInfo(path string, imageCount, subdirectoryCount int, latestImageModTime time.Time, latestImagePath string, ratingCounts map[int]int) *DirectoryInfo {
	return &DirectoryInfo{
		id:                 EncodeID(path),
		path:               path,
		imageCount:         imageCount,
		subdirectoryCount:  subdirectoryCount,
		latestImageModTime: latestImageModTime,
		latestImagePath:    latestImagePath,
		ratingCounts:       ratingCounts,
	}
}

func (d *DirectoryInfo) ID() scalar.ID {
	return d.id
}

func (d *DirectoryInfo) Path() string {
	return d.path
}

func (d *DirectoryInfo) ImageCount() int {
	return d.imageCount
}

func (d *DirectoryInfo) SubdirectoryCount() int {
	return d.subdirectoryCount
}

func (d *DirectoryInfo) LatestImageModTime() time.Time {
	return d.latestImageModTime
}

func (d *DirectoryInfo) LatestImagePath() string {
	return d.latestImagePath
}

func (d *DirectoryInfo) RatingCounts() map[int]int {
	return d.ratingCounts
}

type ImageInfo struct {
	id            scalar.ID
	filename      string
	path          string
	size          int64
	modTime       time.Time
	currentRating int
	xmpExists     bool
}

func NewImageInfo(filename, path string, size int64, modTime time.Time, currentRating int, xmpExists bool) *ImageInfo {
	return &ImageInfo{
		id:            newID(path, modTime),
		filename:      filename,
		path:          path,
		size:          size,
		modTime:       modTime,
		currentRating: currentRating,
		xmpExists:     xmpExists,
	}
}

func (i *ImageInfo) ID() scalar.ID {
	return i.id
}

func (i *ImageInfo) Filename() string {
	return i.filename
}

func (i *ImageInfo) Path() string {
	return i.path
}

func (i *ImageInfo) Size() int64 {
	return i.size
}

func (i *ImageInfo) ModTime() time.Time {
	return i.modTime
}

func (i *ImageInfo) CurrentRating() int {
	return i.currentRating
}

func (i *ImageInfo) XMPExists() bool {
	return i.xmpExists
}

func (i *ImageInfo) SetCurrentRating(rating int) {
	i.currentRating = rating
}

func newID(path string, modTime time.Time) scalar.ID {
	hash := sha256.New()
	hash.Write([]byte(path))
	hash.Write([]byte(modTime.String()))
	return scalar.ToID(hex.EncodeToString(hash.Sum(nil))[:16])
}
