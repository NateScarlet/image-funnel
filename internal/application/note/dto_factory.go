package note

import (
	"main/internal/domain/note"
	"main/internal/shared"
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
		RelPath:    n.RelPath().String(),
		AbsPath:    n.AbsPath(),
		Content:    n.Content(),
		RawContent: n.RawContent(),
		Hidden:     n.Hidden(),
		ModTime:    n.ModTime(),
	}
}
