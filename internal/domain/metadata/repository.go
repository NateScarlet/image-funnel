package metadata

type Repository interface {
	// Read 返回 (nil, nil) 表示没有数据
	Read(imagePath string) (*XMPData, error)
	Write(imagePath string, data *XMPData) error
}
