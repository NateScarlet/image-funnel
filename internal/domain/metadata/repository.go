package metadata

type Repository interface {
	// Read 返回 (nil, nil) 表示没有数据
	Read(imagePath string) (*Data, error)
	Write(imagePath string, data *Data) error
}
