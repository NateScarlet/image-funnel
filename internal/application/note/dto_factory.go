package note

import (
	"main/internal/domain/note"
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

func (f *DTOFactory) New(n *note.Note) *shared.NoteDTO {
	if n == nil {
		return nil
	}
	return &shared.NoteDTO{
		ID:         n.ID(),
		RelPath:    filepath.ToSlash(n.RelPath()),
		AbsPath:    n.AbsPath(),
		Content:    n.Content(),
		RawContent: n.RawContent(),
		Hidden:     n.Hidden(),
	}
}
