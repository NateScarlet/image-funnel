package session

import (
	"context"
	"main/internal/domain/directory"
	"main/internal/domain/hook"
	"main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"

	"go.uber.org/zap"
)

// DirectoryResolver 解析目录 ID 为目录实体
type DirectoryResolver interface {
	GetDirectory(ctx context.Context, id scalar.ID) (*directory.Directory, error)
}

type Service struct {
	sessionRepo        Repository
	metadataRepo       metadata.Repository
	imageRepo          image.Repository
	fileChangedSub     pubsub.Topic[*shared.FileChangedEvent]
	metadataUpdatedPub pubsub.Topic[*shared.MetadataUpdatedEvent]
	directoryResolver  DirectoryResolver
	logger             *zap.Logger
	// 只发布 ID，订阅者需要自己 Acquire 后读取，避免跨 goroutine 持有 *Session 指针导致并发 map 读写
	sessionSaved       pubsub.Topic[scalar.ID]
	rootDir            string
	imageFilterBuilder *image.FilterBuilder
	hookRunner         hook.Runner
}

func NewService(
	sessionRepo Repository,
	metadataRepo metadata.Repository,
	imageRepo image.Repository,
	fileChangedSub pubsub.Topic[*shared.FileChangedEvent],
	metadataUpdatedPub pubsub.Topic[*shared.MetadataUpdatedEvent],
	directoryResolver DirectoryResolver,
	logger *zap.Logger,
	sessionSaved pubsub.Topic[scalar.ID],
	rootDir string,
	imageFilterBuilder *image.FilterBuilder,
	hookRunner hook.Runner,
) (*Service, func()) {
	s := &Service{
		sessionRepo:        sessionRepo,
		metadataRepo:       metadataRepo,
		imageRepo:          imageRepo,
		fileChangedSub:     fileChangedSub,
		metadataUpdatedPub: metadataUpdatedPub,
		directoryResolver:  directoryResolver,
		logger:             logger,
		sessionSaved:       sessionSaved,
		rootDir:            rootDir,
		imageFilterBuilder: imageFilterBuilder,
		hookRunner:         hookRunner,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cleanup := func() {
		cancel()
	}

	go s.subscribeFileChanges(ctx)

	return s, cleanup
}
