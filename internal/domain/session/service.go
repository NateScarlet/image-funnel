package session

import (
	"context"
	"iter"
	"main/internal/domain/directory"
	"main/internal/domain/metadata"
	"main/internal/pubsub"
	"main/internal/shared"

	"go.uber.org/zap"
)

// EventBus 事件总线接口
type EventBus interface {
	SubscribeFileChanged(ctx context.Context) iter.Seq2[*shared.FileChangedEvent, error]
}

type Service struct {
	sessionRepo  Repository
	metadataRepo metadata.Repository
	dirScanner   directory.Scanner
	eventBus     EventBus
	logger       *zap.Logger
	sessionSaved pubsub.Topic[*Session]
	rootDir      string
}

func NewService(
	sessionRepo Repository,
	metadataRepo metadata.Repository,
	dirScanner directory.Scanner,
	eventBus EventBus,
	logger *zap.Logger,
	sessionSaved pubsub.Topic[*Session],
	rootDir string,
) (*Service, func()) {
	s := &Service{
		sessionRepo:  sessionRepo,
		metadataRepo: metadataRepo,
		dirScanner:   dirScanner,
		eventBus:     eventBus,
		logger:       logger,
		sessionSaved: sessionSaved,
		rootDir:      rootDir,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cleanup := func() {
		cancel()
	}

	go s.subscribeFileChanges(ctx)

	return s, cleanup
}
