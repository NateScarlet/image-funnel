package hook

import (
	domain "main/internal/domain/hook"
	"main/internal/shared"
)

// DTOFactory 外部钩子领域对象至 DTO 映射工厂
type DTOFactory struct{}

func NewDTOFactory() *DTOFactory {
	return &DTOFactory{}
}

func (f *DTOFactory) New(h *domain.Hook) *shared.HookDTO {
	if h == nil {
		return nil
	}
	var dir *shared.HookDirectiveDTO
	if h.Directive() != nil {
		dir = &shared.HookDirectiveDTO{
			Name:         h.Directive().Name,
			Usage:        h.Directive().Usage,
			Autocomplete: h.Directive().Autocomplete,
		}
	}
	return &shared.HookDTO{
		ID:                 h.ID(),
		Name:               h.Name(),
		Description:        h.Description(),
		CanDispatchByImage: h.CanDispatchByImage(),
		CanDispatchByNote:  h.CanDispatchByNote(),
		Directive:          dir,
	}
}

func (f *DTOFactory) NewAutocompleteSuggestion(s *domain.AutocompleteSuggestion) *shared.AutocompleteSuggestionDTO {
	if s == nil {
		return nil
	}
	return &shared.AutocompleteSuggestionDTO{
		Text:        s.Text,
		DisplayText: s.DisplayText,
		Description: s.Description,
		Type:        s.Type,
		Style:       s.Style,
	}
}
