package directory

import "time"

type DirectoryInfo struct {
	path               string
	imageCount         int
	subdirectoryCount  int
	latestImageModTime time.Time
	latestImagePath    string
	ratingCounts       map[int]int
}

func NewDirectoryInfo(path string, imageCount, subdirectoryCount int, latestImageModTime time.Time, latestImagePath string, ratingCounts map[int]int) *DirectoryInfo {
	return &DirectoryInfo{
		path:               path,
		imageCount:         imageCount,
		subdirectoryCount:  subdirectoryCount,
		latestImageModTime: latestImageModTime,
		latestImagePath:    latestImagePath,
		ratingCounts:       ratingCounts,
	}
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
	id            string
	filename      string
	path          string
	size          int64
	currentRating int
	xmpExists     bool
}

func NewImageInfo(id, filename, path string, size int64, currentRating int, xmpExists bool) *ImageInfo {
	return &ImageInfo{
		id:            id,
		filename:      filename,
		path:          path,
		size:          size,
		currentRating: currentRating,
		xmpExists:     xmpExists,
	}
}

func (i *ImageInfo) ID() string {
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

func (i *ImageInfo) CurrentRating() int {
	return i.currentRating
}

func (i *ImageInfo) XMPExists() bool {
	return i.xmpExists
}

func (i *ImageInfo) SetCurrentRating(rating int) {
	i.currentRating = rating
}
