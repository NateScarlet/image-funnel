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
	return &shared.HookDTO{
		ID:                 h.ID(),
		Name:               h.Name(),
		Description:        h.Description(),
		CanDispatchByImage: h.CanDispatchByImage(),
	}
}
