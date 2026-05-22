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
	title := filepath.Base(m.AbsPath())
	if relPath, err := memo.DecodeID(m.ID()); err == nil {
		title = filepath.Base(relPath)
	}
	return &shared.MemoDTO{
		ID:         m.ID(),
		Title:      title,
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
		Title:      filepath.Base(relPath),
		AbsPath:    filepath.Join(f.rootDir, relPath),
		Content:    "",
		RawContent: "",
		Hidden:     false,
	}, nil
}
