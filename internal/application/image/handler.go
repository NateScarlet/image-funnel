package image

import (
	"context"
	"iter"

	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/shared"

	"go.uber.org/zap"
)

// ClipboardProvider 剪贴板操作接口，避免循环依赖
type ClipboardProvider interface {
	ReadCustomFormat(formatName string) (string, error)
	ReadHTMLFormat() (string, error)
	AddFiles(filePaths []string) error
	ListFormats() []string
}

// EventBus 文件变更事件总线接口，避免直接依赖 session 包造成循环导入
type EventBus interface {
	SubscribeFileChanged(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error]
}

type Handler struct {
	imageService       *image.Service
	eventBus           EventBus
	imgRepo            image.Repository
	imgMover           image.Mover
	imgTrasher         image.Trasher
	dirSvc             *directory.Service
	dtoFactory         *DTOFactory
	imageFilterBuilder *image.FilterBuilder
	logger             *zap.Logger
	rootDir            string
	imgFactory         *image.Factory
	clipboard          ClipboardProvider
}

func NewHandler(
	imageService *image.Service,
	eventBus EventBus,
	imgRepo image.Repository,
	imgMover image.Mover,
	imgTrasher image.Trasher,
	dirSvc *directory.Service,
	dtoFactory *DTOFactory,
	imageFilterBuilder *image.FilterBuilder,
	logger *zap.Logger,
	rootDir string,
	imgFactory *image.Factory,
	clipboard ClipboardProvider,
) *Handler {
	return &Handler{
		imageService:       imageService,
		eventBus:           eventBus,
		imgRepo:            imgRepo,
		imgMover:           imgMover,
		imgTrasher:         imgTrasher,
		dirSvc:             dirSvc,
		dtoFactory:         dtoFactory,
		imageFilterBuilder: imageFilterBuilder,
		logger:             logger,
		rootDir:            rootDir,
		imgFactory:         imgFactory,
		clipboard:          clipboard,
	}
}