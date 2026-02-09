package session

import (
	"context"
	"errors"
	"iter"
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/pubsub"
	"main/internal/scalar"
	"main/internal/shared"
	"os"
	"path/filepath"
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

func (s *Service) subscribeFileChanges(ctx context.Context) {
	for e, err := range s.eventBus.SubscribeFileChanged(ctx) {
		if err != nil {
			s.logger.Error("failed to receive file changed event", zap.Error(err))
			continue
		}
		if err := s.handleFileChange(ctx, e); err != nil {
			s.logger.Error("failed to handle file changed event",
				zap.Stringer("action", e.Action),
				zap.String("relPath", e.RelPath),
				zap.Stringer("directoryID", e.DirectoryID),
				zap.Error(err))
		}
	}
}

func (s *Service) handleFileChange(ctx context.Context, e *shared.FileChangedEvent) error {
	var img *image.Image
	if e.Action == shared.FileActionCreate || e.Action == shared.FileActionWrite {
		var err error
		img, err = s.dirScanner.LookupImage(ctx, e.RelPath)
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
	}

	for sess, err := range s.sessionRepo.FindByDirectory(e.DirectoryID) {
		if err != nil {
			return err
		}

		changed := false
		if img != nil {
			// 创建或更新
			filterFunc := image.BuildImageFilter(sess.Filter())
			changed = sess.UpdateImage(img, filterFunc(img))
		} else {
			// 删除，或未获取到图片的创建/更新（按删除处理）
			changed = sess.RemoveImageByPath(filepath.Join(s.rootDir, e.RelPath))
		}

		if changed {
			if err := s.sessionRepo.Save(sess); err != nil {
				s.logger.Error("failed to save session",
					zap.Stringer("sessionID", sess.ID()),
					zap.Error(err))
				continue
			}
			s.sessionSaved.Publish(ctx, sess)
		}
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

func (s *Service) Commit(ctx context.Context, session *Session, writeActions *shared.WriteActions) (int, error) {
	var errs []error
	var successCount int
	var updatedImages []*image.Image

	// 1. 遍历并执行写入操作
	// 注意：这里只收集需要更新的图片，不直接修改 Session，避免死锁（Actions() 持有 RLock）
	for img, action := range session.Actions() {

		var rating int
		switch action {
		case shared.ImageActionKeep:
			rating = writeActions.KeepRating
		case shared.ImageActionShelve:
			rating = writeActions.ShelveRating
		case shared.ImageActionReject:
			rating = writeActions.RejectRating
		}

		// 显式重新加载图片最新状态
		// Session 中存储的是绝对路径，而 Scanner.LookupImage 期望相对路径
		relPath, err := filepath.Rel(s.rootDir, img.Path())
		if err != nil {
			errs = append(errs, err)
			continue
		}

		currentImg, err := s.dirScanner.LookupImage(ctx, relPath)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		// 如果 ID 不匹配（说明文件已被外部修改），记录错误并跳过
		if currentImg.ID() != img.ID() {
			errs = append(errs, errors.New("image ID mismatch (file modified externally): "+img.Path()))
			continue
		}

		// 如果当前磁盘状态（即刚刚加载的状态）已经符合目标 Rating，跳过写入
		if rating == currentImg.Rating() {
			continue
		}

		xmpData := metadata.NewXMPData(rating, action.String(), time.Now())

		if err := s.metadataRepo.Write(img.Path(), xmpData); err != nil {
			errs = append(errs, err)
			continue
		}
		successCount++

		// 写入成功后，构建新的 Image 对象用于后续更新内存
		// 强制使用新 Rating，但保留原图其他信息（如 ModTime，等待 FileWatcher 慢慢更新）
		newImg := image.NewImage(
			currentImg.ID(),
			currentImg.Filename(),
			currentImg.Path(),
			currentImg.Size(),
			currentImg.ModTime(),
			xmpData,
			currentImg.Width(),
			currentImg.Height(),
		)
		updatedImages = append(updatedImages, newImg)
	}

	// 2. 批量更新内存状态
	// 此时 Actions() 的锁已释放，可以安全请求写锁
	if len(updatedImages) > 0 {
		session.BatchUpdateImages(updatedImages)
	}

	if err := s.sessionRepo.Save(session); err != nil {
		errs = append(errs, err)
	}

	s.sessionSaved.Publish(ctx, session)

	return successCount, errors.Join(errs...)
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
func (s *Service) MarkImage(ctx context.Context, sessionID scalar.ID, imageID scalar.ID, action shared.ImageAction, options ...shared.MarkImageOption) error {
	sess, err := s.sessionRepo.Get(sessionID)
	if err != nil {
		return err
	}

	if err := sess.MarkImage(imageID, action, options...); err != nil {
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
