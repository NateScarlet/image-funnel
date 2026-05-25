package session

import (
	"context"
	"iter"
	"main/internal/domain/directory"
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

// EventBus 事件总线接口
type EventBus interface {
	SubscribeFileChanged(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error]
}

type Service struct {
	sessionRepo        Repository
	metadataRepo       metadata.Repository
	imageScanner       image.Scanner
	eventBus           EventBus
	directoryResolver  DirectoryResolver
	logger             *zap.Logger
	// 只发布 ID，订阅者需要自己 Acquire 后读取，避免跨 goroutine 持有 *Session 指针导致并发 map 读写
	sessionSaved       pubsub.Topic[scalar.ID]
	rootDir            string
	imageFilterBuilder *image.FilterBuilder
}

func NewService(
	sessionRepo Repository,
	metadataRepo metadata.Repository,
	imageScanner image.Scanner,
	eventBus EventBus,
	directoryResolver DirectoryResolver,
	logger *zap.Logger,
	sessionSaved pubsub.Topic[scalar.ID],
	rootDir string,
	imageFilterBuilder *image.FilterBuilder,
) (*Service, func()) {
	s := &Service{
		sessionRepo:        sessionRepo,
		metadataRepo:       metadataRepo,
		imageScanner:       imageScanner,
		eventBus:           eventBus,
		directoryResolver:  directoryResolver,
		logger:             logger,
		sessionSaved:       sessionSaved,
		rootDir:            rootDir,
		imageFilterBuilder: imageFilterBuilder,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cleanup := func() {
		cancel()
	}

	go s.subscribeFileChanges(ctx)

	return s, cleanup
}
