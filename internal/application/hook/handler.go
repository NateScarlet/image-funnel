package hook

import (
	"context"

	domain "main/internal/domain/hook"
)

// ImageService 用于动态获取图片物理路径的依赖接口，解决领域间直接依赖问题
type ImageService interface {
	GetPaths(ctx context.Context, ids []string) ([]string, error)
}

// Handler 钩子业务处理 Handler
type Handler struct {
	repo         domain.Repository
	runner       domain.Runner
	imageService ImageService
	dtoFactory   *DTOFactory
}

func NewHandler(repo domain.Repository, runner domain.Runner, imageService ImageService, dtoFactory *DTOFactory) *Handler {
	return &Handler{
		repo:         repo,
		runner:       runner,
		imageService: imageService,
		dtoFactory:   dtoFactory,
	}
}