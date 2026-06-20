package hook

import (
	"context"
	domain "main/internal/domain/hook"
	"main/internal/scalar"
	"main/internal/shared"
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

// Hooks 获取所有外部钩子概要列表
func (h *Handler) Hooks(ctx context.Context) ([]*shared.HookDTO, error) {
	hooks, err := h.repo.List(ctx)
	if err != nil {
		return nil, err
	}
	var res []*shared.HookDTO
	for _, hk := range hooks {
		res = append(res, h.dtoFactory.New(hk))
	}
	return res, nil
}

// Dispatch 手动派发特定的外部钩子任务
func (h *Handler) Dispatch(ctx context.Context, ids []string, hookID scalar.ID, triggerName string) error {
	paths, err := h.imageService.GetPaths(ctx, ids)
	if err != nil {
		return err
	}
	return h.runner.Trigger(ctx, ids, paths, hookID, triggerName)
}

// DispatchMemo 手动派发笔记触发的外部钩子任务
func (h *Handler) DispatchMemo(ctx context.Context, memoRelPath string, hookID scalar.ID) error {
	return h.runner.TriggerForMemo(ctx, memoRelPath, hookID)
}
