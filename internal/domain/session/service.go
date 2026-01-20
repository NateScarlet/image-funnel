package session

import (
	"main/internal/domain/directory"
	"main/internal/domain/image"
	"main/internal/domain/metadata"
	"main/internal/scalar"
	"main/internal/shared"
	"time"
)

type Service struct {
	sessionRepo  Repository
	metadataRepo metadata.Repository
	dirScanner   directory.Scanner
}

func NewService(sessionRepo Repository, metadataRepo metadata.Repository, dirScanner directory.Scanner) *Service {
	return &Service{
		sessionRepo:  sessionRepo,
		metadataRepo: metadataRepo,
		dirScanner:   dirScanner,
	}
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

func (s *Service) Commit(session *Session, writeActions *WriteActions) (int, []error) {
	var errors []error
	success := 0

	for _, img := range session.Images() {
		action := session.Action(img.ID())

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

	return success, errors
}

// Update 更新会话配置
// 使用 Options 模式支持灵活的更新选项
func (s *Service) Update(id scalar.ID, options ...UpdateOption) (*Session, error) {
	sess, err := s.sessionRepo.Get(id)
	if err != nil {
		return nil, err
	}

	opts := &UpdateOptions{}
	for _, opt := range options {
		opt(opts)
	}

	if opts.targetKeep != nil {
		if err := sess.UpdateTargetKeep(*opts.targetKeep); err != nil {
			return nil, err
		}
	}

	if opts.filter != nil {
		filterFunc := image.BuildImageFilter(opts.filter)
		var filteredImages []*image.Image
		for img, err := range s.dirScanner.Scan(sess.Directory()) {
			if err != nil {
				return nil, err
			}
			if filterFunc(img) {
				filteredImages = append(filteredImages, img)
			}
		}

		if err := sess.NextRound(opts.filter, filteredImages); err != nil {
			return nil, err
		}
	}

	if err := s.sessionRepo.Save(sess); err != nil {
		return nil, err
	}

	return sess, nil
}

// Create 初始化一个新的会话
// 扫描目录、应用过滤器并创建会话
func (s *Service) Create(id scalar.ID, directory string, filter *shared.ImageFilters, targetKeep int) (*Session, error) {
	filterFunc := image.BuildImageFilter(filter)
	var filteredImages []*image.Image
	for img, err := range s.dirScanner.Scan(directory) {
		if err != nil {
			return nil, err
		}
		if filterFunc(img) {
			filteredImages = append(filteredImages, img)
		}
	}

	sess := NewSession(id, directory, filter, targetKeep, filteredImages)
	if err := s.sessionRepo.Save(sess); err != nil {
		return nil, err
	}
	return sess, nil
}

// Get 根据 ID 获取会话
func (s *Service) Get(id scalar.ID) (*Session, error) {
	return s.sessionRepo.Get(id)
}

// MarkImage 标记图片并保存
func (s *Service) MarkImage(sessionID scalar.ID, imageID scalar.ID, action shared.ImageAction) (*Session, error) {
	sess, err := s.sessionRepo.Get(sessionID)
	if err != nil {
		return nil, err
	}

	if err := sess.MarkImage(imageID, action); err != nil {
		return nil, err
	}

	if err := s.sessionRepo.Save(sess); err != nil {
		return nil, err
	}

	return sess, nil
}

// Undo 撤销操作并保存
func (s *Service) Undo(sessionID scalar.ID) (*Session, error) {
	sess, err := s.sessionRepo.Get(sessionID)
	if err != nil {
		return nil, err
	}

	if err := sess.Undo(); err != nil {
		return nil, err
	}

	if err := s.sessionRepo.Save(sess); err != nil {
		return nil, err
	}

	return sess, nil
}
