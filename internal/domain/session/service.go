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
	"time"

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
}

func NewService(
	sessionRepo Repository,
	metadataRepo metadata.Repository,
	dirScanner directory.Scanner,
	eventBus EventBus,
	logger *zap.Logger,
	sessionSaved pubsub.Topic[*Session],
) (*Service, func()) {
	s := &Service{
		sessionRepo:  sessionRepo,
		metadataRepo: metadataRepo,
		dirScanner:   dirScanner,
		eventBus:     eventBus,
		logger:       logger,
		sessionSaved: sessionSaved,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cleanup := func() {
		cancel()
	}

	go s.subscribeFileChanges(ctx)

	return s, cleanup
}

func (s *Service) subscribeFileChanges(ctx context.Context) {
	for e, err := range s.eventBus.SubscribeFileChanged(ctx) {
		if err != nil {
			s.logger.Error("failed to receive file changed event", zap.Error(err))
			continue
		}
		if e.Action == shared.FileActionCreate {
			if err := s.handleFileCreate(ctx, e); err != nil {
				s.logger.Error("failed to handle file create",
					zap.String("relPath", e.RelPath),
					zap.Stringer("directoryID", e.DirectoryID),
					zap.Error(err))
			}
		}
	}
}

func (s *Service) handleFileCreate(ctx context.Context, e *shared.FileChangedEvent) error {
	// 先获取图片，避免每个会话重复获取
	img, err := s.dirScanner.LookupImage(ctx, e.RelPath)
	if err != nil {
		return err
	}
	if img == nil {
		return nil
	}

	for sess, err := range s.sessionRepo.FindByDirectory(e.DirectoryID) {
		if err != nil {
			return err
		}

		filterFunc := image.BuildImageFilter(sess.Filter())
		if !filterFunc(img) {
			continue
		}

		if err := sess.addFilteredImage(img); err != nil {
			s.logger.Warn("failed to add image to session",
				zap.Stringer("sessionID", sess.ID()),
				zap.String("imagePath", e.RelPath),
				zap.Error(err))
			continue
		}
		if err := s.sessionRepo.Save(sess); err != nil {
			s.logger.Error("failed to save session",
				zap.Stringer("sessionID", sess.ID()),
				zap.Error(err))
			continue
		}
		s.sessionSaved.Publish(ctx, sess)
	}

	return nil
}

// UpdateOptions 定义会话更新选项

type UpdateOptions struct {
	targetKeep *int
	filter     *shared.ImageFilters
}

// UpdateOption 定义更新选项的函数类型
type UpdateOption func(*UpdateOptions)

// WithTargetKeep 设置目标保留数量
func WithTargetKeep(targetKeep int) UpdateOption {
	return func(opts *UpdateOptions) {
		opts.targetKeep = &targetKeep
	}
}

// WithFilter 设置过滤器
func WithFilter(filter *shared.ImageFilters) UpdateOption {
	return func(opts *UpdateOptions) {
		opts.filter = filter
	}
}

func (s *Service) Commit(ctx context.Context, session *Session, writeActions *WriteActions) (int, []error) {
	var errors []error
	success := 0

	for img, action := range session.Actions() {
		var rating int
		switch action {
		case shared.ImageActionKeep:
			rating = writeActions.keepRating
		case shared.ImageActionPending:
			rating = writeActions.pendingRating
		case shared.ImageActionReject:
			rating = writeActions.rejectRating
		}
		if rating == img.Rating() {
			continue
		}

		xmpData := metadata.NewXMPData(rating, action.String(), time.Now())

		if err := s.metadataRepo.Write(img.Path(), xmpData); err != nil {
			errors = append(errors, err)
			continue
		}
		success++
	}
	if err := s.sessionRepo.Save(session); err != nil {
		errors = append(errors, err)
	}

	s.sessionSaved.Publish(ctx, session)

	return success, errors
}

// Update 更新会话配置
// 使用 Options 模式支持灵活的更新选项
func (s *Service) Update(ctx context.Context, id scalar.ID, options ...UpdateOption) error {
	sess, err := s.sessionRepo.Get(id)
	if err != nil {
		return err
	}

	opts := &UpdateOptions{}
	for _, opt := range options {
		opt(opts)
	}

	if opts.targetKeep != nil {
		if err := sess.UpdateTargetKeep(*opts.targetKeep); err != nil {
			return err
		}
	}

	if opts.filter != nil {
		directory, err := directory.DecodeID(sess.DirectoryID())
		if err != nil {
			return err
		}

		filterFunc := image.BuildImageFilter(opts.filter)
		var filteredImages []*image.Image
		for img, err := range s.dirScanner.Scan(ctx, directory) {
			if err != nil {
				return err
			}
			if filterFunc(img) {
				filteredImages = append(filteredImages, img)
			}
		}

		if err := sess.NextRound(opts.filter, filteredImages); err != nil {
			return err
		}
	}

	if err := s.sessionRepo.Save(sess); err != nil {
		return err
	}

	s.sessionSaved.Publish(ctx, sess)
	return nil
}

// Create 初始化一个新的会话
// 扫描目录、应用过滤器并创建会话
func (s *Service) Create(ctx context.Context, id scalar.ID, directoryID scalar.ID, filter *shared.ImageFilters, targetKeep int) error {
	directory, err := directory.DecodeID(directoryID)
	if err != nil {
		return err
	}

	filterFunc := image.BuildImageFilter(filter)
	var filteredImages []*image.Image
	for img, err := range s.dirScanner.Scan(ctx, directory) {
		if err != nil {
			return err
		}
		if filterFunc(img) {
			filteredImages = append(filteredImages, img)
		}
	}

	sess := NewSession(id, directoryID, filter, targetKeep, filteredImages)
	if err := s.sessionRepo.Save(sess); err != nil {
		return err
	}
	s.sessionSaved.Publish(ctx, sess)
	return nil
}

// Get 根据 ID 获取会话
func (s *Service) Get(id scalar.ID) (*Session, error) {
	return s.sessionRepo.Get(id)
}

// MarkImage 标记图片并保存
func (s *Service) MarkImage(ctx context.Context, sessionID scalar.ID, imageID scalar.ID, action shared.ImageAction) error {
	sess, err := s.sessionRepo.Get(sessionID)
	if err != nil {
		return err
	}

	if err := sess.MarkImage(imageID, action); err != nil {
		return err
	}

	if err := s.sessionRepo.Save(sess); err != nil {
		return err
	}

	s.sessionSaved.Publish(ctx, sess)
	return nil
}

// Undo 撤销操作并保存
func (s *Service) Undo(ctx context.Context, sessionID scalar.ID) error {
	sess, err := s.sessionRepo.Get(sessionID)
	if err != nil {
		return err
	}

	if err := sess.Undo(); err != nil {
		return err
	}

	if err := s.sessionRepo.Save(sess); err != nil {
		return err
	}

	s.sessionSaved.Publish(ctx, sess)
	return nil
}
