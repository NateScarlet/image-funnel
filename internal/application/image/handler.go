package image

import (
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/pubsub"
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

type Handler struct {
	imageService       *image.Service
	fileChangedSub     pubsub.Topic[*shared.FileChangedEvent]
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
	fileChangedSub pubsub.Topic[*shared.FileChangedEvent],
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
		fileChangedSub:     fileChangedSub,
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