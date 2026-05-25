package memo

import (
	"main/internal/domain/memo"
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
	if m == nil {
		return nil
	}
	return &shared.MemoDTO{
		ID:         m.ID(),
		RelPath:    filepath.ToSlash(m.RelPath()),
		AbsPath:    m.AbsPath(),
		Content:    m.Content(),
		RawContent: m.RawContent(),
		Hidden:     m.Hidden(),
	}
}
