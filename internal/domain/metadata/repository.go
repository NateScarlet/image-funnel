package metadata

type Repository interface {
	Read(imagePath string) (*XMPData, error)
	Write(imagePath string, data *XMPData) error
}
