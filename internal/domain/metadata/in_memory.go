package metadata

import "time"

type InMemoryRepository struct {
	data map[string]*XMPData
}

func NewInMemoryRepository() *InMemoryRepository {
	return &InMemoryRepository{
		data: make(map[string]*XMPData),
	}
}

func (r *InMemoryRepository) Read(imagePath string) (*XMPData, error) {
	if data, ok := r.data[imagePath]; ok {
		return data, nil
	}
	return NewXMPData(0, "", "", time.Time{}, ""), nil
}

func (r *InMemoryRepository) Write(imagePath string, data *XMPData) error {
	r.data[imagePath] = data
	return nil
}

func (r *InMemoryRepository) BatchWrite(imagePaths []string, dataMap map[string]*XMPData) (int, []error) {
	success := 0
	for _, path := range imagePaths {
		if data, exists := dataMap[path]; exists {
			if err := r.Write(path, data); err == nil {
				success++
			}
		}
	}
	return success, nil
}
