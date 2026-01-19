package localfs

import (
	"main/internal/domain/metadata"
)

type mockMetadataRepository struct{}

func newMockMetadataRepository() metadata.Repository {
	return &mockMetadataRepository{}
}

func (m *mockMetadataRepository) Read(imagePath string) (*metadata.XMPData, error) {
	return nil, nil
}

func (m *mockMetadataRepository) Write(imagePath string, data *metadata.XMPData) error {
	return nil
}
