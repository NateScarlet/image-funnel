package session

import (
	"main/internal/domain/metadata"
)

func WriteXMP(imagePath string, data *metadata.XMPData) error {
	repo := metadata.NewInMemoryRepository()
	return repo.Write(imagePath, data)
}
