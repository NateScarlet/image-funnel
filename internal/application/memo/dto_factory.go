package memo

import (
	"main/internal/domain/memo"
	"main/internal/scalar"
	"main/internal/shared"
	"path/filepath"
)

type DTOFactory struct {
	rootDir string
}

func NewDTOFactory(rootDir string) *DTOFactory {
	return &DTOFactory{
		rootDir: rootDir,
	}
}

func (f *DTOFactory) New(m *memo.Memo) *shared.MemoDTO {
	relPath, _ := memo.DecodeID(m.ID())
	return &shared.MemoDTO{
		ID:         m.ID(),
		RelPath:    filepath.ToSlash(relPath),
		AbsPath:    m.AbsPath(),
		Content:    m.Content(),
		RawContent: m.RawContent(),
		Hidden:     m.Hidden(),
	}
}

func (f *DTOFactory) NewEmpty(id scalar.ID) (*shared.MemoDTO, error) {
	relPath, err := memo.DecodeID(id)
	if err != nil {
		return nil, err
	}
	return &shared.MemoDTO{
		ID:         id,
		RelPath:    filepath.ToSlash(relPath),
		AbsPath:    filepath.Join(f.rootDir, relPath),
		Content:    "",
		RawContent: "",
		Hidden:     false,
	}, nil
}
